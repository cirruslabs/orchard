package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/cirruslabs/orchard/internal/execstream"
	"github.com/cirruslabs/orchard/internal/responder"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
)

func (controller *Controller) execVM(ctx *gin.Context) responder.Responder {
	if responder := controller.authorizeAny(ctx, v1.ServiceAccountRoleComputeWrite,
		v1.ServiceAccountRoleComputeConnect); responder != nil {
		return responder
	}

	// Retrieve and parse path and query parameters
	name := ctx.Param("name")

	command := ctx.Query("command")
	if command == "" {
		return responder.JSON(http.StatusBadRequest,
			NewErrorResponse("\"command\" parameter cannot be empty"))
	}

	stdin := ctx.Query("stdin") == "true"

	waitRaw := ctx.DefaultQuery("wait", "10")
	wait, err := strconv.ParseUint(waitRaw, 10, 16)
	if err != nil {
		return responder.Code(http.StatusBadRequest)
	}

	// Look-up the VM
	waitContext, waitContextCancel := context.WithTimeout(ctx, time.Duration(wait)*time.Second)
	defer waitContextCancel()

	vm, responderImpl := controller.waitForVM(waitContext, name)
	if responderImpl != nil {
		return responderImpl
	}

	// Establish a port-forwarding connection to a VM's SSH port
	portForwardConn, portForwardCancel, err := controller.portForwardConnection(ctx, waitContext,
		vm.Worker, vm.UID, 22)
	if err != nil {
		return responder.JSON(http.StatusServiceUnavailable, NewErrorResponse("%v", err))
	}
	defer portForwardCancel()

	// Upgrade HTTP request to a WebSocket connection
	wsConn, err := websocket.Accept(ctx.Writer, ctx.Request, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return responder.Error(err)
	}
	defer func() {
		// Ensure that we always close the accepted WebSocket connection,
		// otherwise resource leak is possible[1]
		//
		// [1]: https://github.com/coder/websocket/issues/445#issuecomment-2053792044
		_ = wsConn.CloseNow()
	}()

	// Start a goroutine that establishes an SSH connection to a VM and runs a command
	sshErrCh := make(chan error, 1)
	stdinHandleCh := make(chan io.WriteCloser, 1)
	outgoingFrames := make(chan *execstream.Frame)
	go func() {
		sshErrCh <- controller.execSSH(ctx, portForwardConn, vm, stdin, stdinHandleCh, command, outgoingFrames)
	}()

	var readFramesErrCh chan error

	for {
		select {
		case stdinHandle := <-stdinHandleCh:
			// SSH session is almost up, we have the standard input handle,
			// so we can start a goroutine that reads WebSocket frames
			readFramesErrCh = make(chan error, 1)

			go func() {
				readFramesErrCh <- controller.readFrames(ctx, wsConn, stdinHandle)
			}()
		case readFramesErr := <-readFramesErrCh:
			controller.logger.Warnf("failed to read and process frames from WebSocket: %v", readFramesErr)

			return responder.Empty()
		case outgoingFrame := <-outgoingFrames:
			if err := execstream.WriteFrame(ctx, wsConn, outgoingFrame); err != nil {
				controller.logger.Warnf("failed to write WebSocket frame to the client: %v", err)

				return responder.Empty()
			}
		case sshErr := <-sshErrCh:
			if sshErr != nil {
				if err := execstream.WriteFrame(ctx, wsConn, &execstream.Frame{
					Type:  execstream.FrameTypeError,
					Error: sshErr.Error(),
				}); err != nil {
					controller.logger.Warnf("exec: failed to write error frame to WebSocket: %v", err)
				}
			}

			if err := wsConn.Close(websocket.StatusNormalClosure, "Command finished"); err != nil {
				controller.logger.Warnf("exec: failed to close WebSocket cleanly: %v", err)
			}

			if readFramesErrCh != nil {
				// Read() on a WebSocket should unblock shortly after calling Close()
				<-readFramesErrCh
			}

			return responder.Empty()
		case <-time.After(controller.pingInterval):
			pingCtx, pingCtxCancel := context.WithTimeout(ctx, 5*time.Second)

			if err := wsConn.Ping(pingCtx); err != nil {
				controller.logger.Warnf("port forwarding: failed to ping the client, "+
					"connection might time out: %v", err)
			}

			pingCtxCancel()
		case <-ctx.Done():
			controller.logger.Warnf("client disconnected prematurely")

			return responder.Empty()
		}
	}
}

func (controller *Controller) readFrames(
	ctx context.Context,
	wsConn *websocket.Conn,
	stdinHandle io.WriteCloser,
) error {
	for {
		var frame execstream.Frame

		messageType, payloadBytes, err := wsConn.Read(ctx)
		if err != nil {
			var closeErr websocket.CloseError
			if errors.As(err, &closeErr) {
				return nil
			}

			return fmt.Errorf("failed to read next frame from WebSocket: %w", err)
		}

		if messageType != websocket.MessageText {
			continue
		}

		if err := json.Unmarshal(payloadBytes, &frame); err != nil {
			return err
		}

		switch frame.Type {
		case execstream.FrameTypeStdin:
			if stdinHandle == nil {
				return fmt.Errorf("failed to handle %q frame: this exec session "+
					"has no stdin is enabled or already closed", frame.Type)
			}

			if len(frame.Data) == 0 {
				if err := stdinHandle.Close(); err != nil {
					return fmt.Errorf("failed to handle %q frame: failed to close "+
						"stdin: %w", frame.Type, err)
				}

				stdinHandle = nil

				continue
			}

			if _, err := stdinHandle.Write(frame.Data); err != nil {
				return fmt.Errorf("failed to handle %q frame: failed to write "+
					"to stdin: %w", frame.Type, err)
			}
		default:
			return fmt.Errorf("unexpected frame type received: %q", frame.Type)
		}
	}
}

func (controller *Controller) execSSH(
	ctx context.Context,
	portForwardConn net.Conn,
	vm *v1.VM,
	stdin bool,
	stdinHandleCh chan<- io.WriteCloser,
	command string,
	outgoingFrames chan<- *execstream.Frame,
) error {
	// Establish an SSH connection
	sshConn, sshChans, sshReqs, err := ssh.NewClientConn(portForwardConn, "", &ssh.ClientConfig{
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
		User: vm.SSHUsername(),
		Auth: []ssh.AuthMethod{
			ssh.Password(vm.SSHPassword()),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to a new SSH connection: %w", err)
	}

	sshClient := ssh.NewClient(sshConn, sshChans, sshReqs)
	defer sshClient.Close()

	// Create a new SSH session
	sshSession, err := sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create a new SSH session: %w", err)
	}
	defer sshSession.Close()

	if stdin {
		stdin, err := sshSession.StdinPipe()
		if err != nil {
			return fmt.Errorf("failed to create standard input pipe: %w", err)
		}

		stdinHandleCh <- stdin
	} else {
		stdinHandleCh <- nil
	}

	stdout, err := sshSession.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create standard output pipe: %w", err)
	}

	stderr, err := sshSession.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create standard error pipe: %w", err)
	}

	if err := sshSession.Start(command); err != nil {
		return fmt.Errorf("failed to start command %q: %w", command, err)
	}

	// Read bytes from standard output and standard error and stream them as frames
	ioGroup, ioGroupCtx := errgroup.WithContext(ctx)

	ioGroup.Go(func() error {
		return ioStreamReader(ioGroupCtx, stdout, execstream.FrameTypeStdout, outgoingFrames)
	})
	ioGroup.Go(func() error {
		return ioStreamReader(ioGroupCtx, stderr, execstream.FrameTypeStderr, outgoingFrames)
	})

	sshWaitErrCh := make(chan error, 1)
	go func() {
		sshWaitErrCh <- sshSession.Wait()
	}()

	// Wait for SSH command terminate while respecting context
	var sshWaitErr error

	select {
	case sshWaitErr = <-sshWaitErrCh:
		// Proceed
	case <-ctx.Done():
		return ctx.Err()
	}

	// Wait for the I/O to complete, otherwise we may
	// miss some bits of the command's output/error
	if err := ioGroup.Wait(); err != nil {
		return err
	}

	// Post an exit event
	exitFrame := &execstream.Frame{
		Type: execstream.FrameTypeExit,
		Exit: execstream.Exit{
			Code: 0,
		},
	}

	if sshWaitErr != nil {
		var sshExitError *ssh.ExitError
		if errors.As(sshWaitErr, &sshExitError) {
			exitFrame.Exit.Code = int32(sshExitError.ExitStatus())
		} else {
			return fmt.Errorf("failed to execute command %q: %w", command, sshWaitErr)
		}
	}

	select {
	case outgoingFrames <- exitFrame:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func ioStreamReader(
	ctx context.Context,
	r io.Reader,
	frameType execstream.FrameType,
	ch chan<- *execstream.Frame,
) error {
	buf := make([]byte, 4096)

	for {
		n, err := r.Read(buf)

		if n > 0 {
			frame := &execstream.Frame{
				Type: frameType,
				Data: slices.Clone(buf[:n]),
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case ch <- frame:
				// Proceed
			}
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}

			return err
		}
	}
}
