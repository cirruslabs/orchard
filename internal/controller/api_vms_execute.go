package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cirruslabs/orchard/internal/execstream"
	"github.com/cirruslabs/orchard/internal/netconncancel"
	"github.com/cirruslabs/orchard/internal/responder"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/cirruslabs/orchard/rpc"
	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
)

const executeSessionRendezvousTimeout = 15 * time.Second

type sshExecution struct {
	session *ssh.Session
	stdout  io.Reader
	stderr  io.Reader
	stdin   io.WriteCloser
}

func (controller *Controller) executeVM(ctx *gin.Context) responder.Responder {
	if responder := controller.authorizeAny(ctx, v1.ServiceAccountRoleComputeWrite,
		v1.ServiceAccountRoleComputeConnect); responder != nil {
		return responder
	}

	name := ctx.Param("name")

	command := ctx.Query("command")
	if command == "" {
		return responder.Code(http.StatusBadRequest)
	}

	args := ctx.QueryArray("arg")

	waitRaw := ctx.DefaultQuery("wait", "10")
	wait, err := strconv.ParseUint(waitRaw, 10, 16)
	if err != nil {
		return responder.Code(http.StatusBadRequest)
	}

	waitCtx, waitCancel := context.WithTimeout(ctx, time.Duration(wait)*time.Second)
	defer waitCancel()

	vm, responderImpl := controller.waitForVM(waitCtx, name)
	if responderImpl != nil {
		return responderImpl
	}

	rvCtx, rvCancel := context.WithCancel(ctx)
	defer rvCancel()

	session := uuid.New().String()

	connCh, cancel := controller.connRendezvous.Request(rvCtx, session)
	defer cancel()

	err = controller.workerNotifier.Notify(waitCtx, vm.Worker, &rpc.WatchInstruction{
		Action: &rpc.WatchInstruction_PortForwardAction{
			PortForwardAction: &rpc.WatchInstruction_PortForward{
				Session: session,
				VmUid:   vm.UID,
				Port:    22,
			},
		},
	})
	if err != nil {
		controller.logger.Warnf("failed to request VM SSH port-forwarding from the worker %s: %v",
			vm.Worker, err)

		return responder.Code(http.StatusServiceUnavailable)
	}

	timeoutTimer := time.NewTimer(executeSessionRendezvousTimeout)
	defer timeoutTimer.Stop()

	select {
	case rvResp := <-connCh:
		if rvResp.ErrorMessage != "" {
			return responder.JSON(http.StatusServiceUnavailable, NewErrorResponse(
				"failed to establish SSH connection to the VM on the worker: %s", rvResp.ErrorMessage))
		}

		if rvResp.Result == nil {
			return responder.Code(http.StatusServiceUnavailable)
		}

		ws, err := websocket.Accept(ctx.Writer, ctx.Request, &websocket.AcceptOptions{
			OriginPatterns: []string{"*"},
		})
		if err != nil {
			_ = rvResp.Result.Close()

			return responder.Error(err)
		}
		defer func() {
			_ = ws.CloseNow()
		}()

		tunnel := netconncancel.New(rvResp.Result, rvCancel)
		defer func() {
			_ = tunnel.Close()
		}()

		return controller.executeVMViaSSHTunnel(ctx, tunnel, ws, vm, command, args)
	case <-timeoutTimer.C:
		return responder.JSON(http.StatusServiceUnavailable, NewErrorResponse(
			"timed out waiting for worker %s to establish SSH tunnel", vm.Worker))
	case <-ctx.Done():
		return responder.Error(ctx.Err())
	}
}

func (controller *Controller) executeVMViaSSHTunnel(ctx context.Context, tunnel net.Conn, ws *websocket.Conn, vm *v1.VM, cmd string, args []string) responder.Responder {
	sshClient, err := newSSHClient(tunnel, vm)
	if err != nil {
		controller.closeExecuteWithFrameError(ctx, ws, nil,
			fmt.Sprintf("SSH handshake with the VM failed: %v", err))

		return responder.Empty()
	}
	defer func() {
		_ = sshClient.Close()
	}()

	execution, err := startSSHExecution(sshClient, cmd, args)
	if err != nil {
		controller.closeExecuteWithFrameError(ctx, ws, nil, err.Error())

		return responder.Empty()
	}
	defer func() {
		_ = execution.session.Close()
	}()

	return controller.pumpExecuteFrames(ctx, ws, execution)
}

func newSSHClient(conn net.Conn, vm *v1.VM) (*ssh.Client, error) {
	sshUser := vm.Username
	sshPassword := vm.Password
	if sshUser == "" && sshPassword == "" {
		sshUser = "admin"
		sshPassword = "admin"
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, "", &ssh.ClientConfig{
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
		User: sshUser,
		Auth: []ssh.AuthMethod{
			ssh.Password(sshPassword),
		},
		Timeout: 10 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	return ssh.NewClient(sshConn, chans, reqs), nil
}

func startSSHExecution(client *ssh.Client, cmd string, args []string) (*sshExecution, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to open SSH session: %v", err)
	}

	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		_ = session.Close()

		return nil, fmt.Errorf("failed to get SSH stdout pipe: %v", err)
	}

	stderrPipe, err := session.StderrPipe()
	if err != nil {
		_ = session.Close()

		return nil, fmt.Errorf("failed to get SSH stderr pipe: %v", err)
	}

	stdinPipe, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()

		return nil, fmt.Errorf("failed to get SSH stdin pipe: %v", err)
	}

	sshCommand := buildSSHCommand(cmd, args)
	if err := session.Start(sshCommand); err != nil {
		_ = session.Close()

		return nil, fmt.Errorf("failed to start SSH command: %v", err)
	}

	return &sshExecution{
		session: session,
		stdout:  stdoutPipe,
		stderr:  stderrPipe,
		stdin:   stdinPipe,
	}, nil
}

func (controller *Controller) pumpExecuteFrames(
	ctx context.Context,
	ws *websocket.Conn,
	execution *sshExecution,
) responder.Responder {
	wsNetConn := websocket.NetConn(ctx, ws, websocket.MessageText)
	defer func() {
		_ = wsNetConn.Close()
	}()

	encoder := execstream.NewEncoder(wsNetConn)
	decoder := execstream.NewDecoder(wsNetConn)

	outCh := make(chan execstream.Frame, 16)
	outDoneCh := make(chan struct{}, 2)
	outErrCh := make(chan error, 1)
	stdinErrCh := make(chan error, 1)
	exitCh := make(chan int32, 1)
	exitErrCh := make(chan error, 1)

	go streamExecuteOutput(execution.stdout, execstream.FrameTypeStdout, outCh, outDoneCh, outErrCh)
	go streamExecuteOutput(execution.stderr, execstream.FrameTypeStderr, outCh, outDoneCh, outErrCh)
	go streamExecuteClientFrames(decoder, execution.stdin, stdinErrCh)
	go waitForSSHExecutionExit(execution.session, exitCh, exitErrCh)

	pingTicker := time.NewTicker(controller.pingInterval)
	defer pingTicker.Stop()

	readersDone := 0
	exitObserved := false
	var exitCode int32

	for {
		if exitObserved && readersDone >= 2 {
			for len(outCh) > 0 {
				frame := <-outCh
				if err := execstream.WriteFrame(encoder, &frame); err != nil {
					return controller.wsError(ws, websocket.StatusInternalError, "execute session",
						"failed to stream execute output to the client", err)
				}
			}

			if err := execstream.WriteFrame(encoder, &execstream.Frame{
				Type: execstream.FrameTypeExit,
				Exit: &execstream.Exit{Code: exitCode},
			}); err != nil {
				return controller.wsError(ws, websocket.StatusInternalError, "execute session",
					"failed to send execute exit status to the client", err)
			}

			if err := ws.Close(websocket.StatusNormalClosure,
				fmt.Sprintf("command exited with code %d", exitCode)); err != nil {
				controller.logger.Warnf("execute session: failed to close WebSocket connection: %v", err)
			}

			return responder.Empty()
		}

		select {
		case frame := <-outCh:
			if err := execstream.WriteFrame(encoder, &frame); err != nil {
				return controller.wsError(ws, websocket.StatusInternalError, "execute session",
					"failed to stream execute output to the client", err)
			}
		case <-outDoneCh:
			readersDone++
		case err := <-outErrCh:
			if err == nil {
				continue
			}

			controller.closeExecuteWithFrameError(ctx, ws, encoder,
				fmt.Sprintf("failed while streaming command output: %v", err))

			return responder.Empty()
		case err := <-stdinErrCh:
			if err == nil || errors.Is(err, context.Canceled) {
				stdinErrCh = nil
				continue
			}

			if errors.Is(err, io.EOF) {
				return responder.Empty()
			}

			controller.closeExecuteWithFrameError(ctx, ws, encoder,
				fmt.Sprintf("failed while reading command stdin stream: %v", err))

			return responder.Empty()
		case code := <-exitCh:
			exitObserved = true
			exitCode = code
		case err := <-exitErrCh:
			controller.closeExecuteWithFrameError(ctx, ws, encoder,
				fmt.Sprintf("failed while waiting for command completion: %v", err))

			return responder.Empty()
		case <-pingTicker.C:
			pingCtx, pingCtxCancel := context.WithTimeout(ctx, 5*time.Second)

			if err := ws.Ping(pingCtx); err != nil {
				controller.logger.Warnf("execute session: failed to ping the client, "+
					"connection might time out: %v", err)
			}

			pingCtxCancel()
		case <-ctx.Done():
			return responder.Error(ctx.Err())
		}
	}
}

func waitForSSHExecutionExit(sshSession *ssh.Session, exitCodeCh chan<- int32, exitErrCh chan<- error) {
	if err := sshSession.Wait(); err != nil {
		var exitError *ssh.ExitError
		if errors.As(err, &exitError) {
			exitCodeCh <- int32(exitError.ExitStatus())

			return
		}

		exitErrCh <- err

		return
	}

	exitCodeCh <- 0
}

func streamExecuteClientFrames(
	decoder *json.Decoder,
	stdin io.WriteCloser,
	errCh chan<- error,
) {
	stdinClosed := false

	for {
		var frame execstream.Frame

		if err := execstream.ReadFrame(decoder, &frame); err != nil {
			if !stdinClosed {
				if closeErr := stdin.Close(); closeErr != nil {
					errCh <- closeErr

					return
				}
			}

			errCh <- err

			return
		}

		switch frame.Type {
		case execstream.FrameTypeStdin:
			if len(frame.Data) == 0 {
				if !stdinClosed {
					if err := stdin.Close(); err != nil {
						errCh <- err

						return
					}

					stdinClosed = true
				}

				errCh <- nil

				return
			}

			if stdinClosed {
				errCh <- errors.New("stdin is already closed")

				return
			}

			if _, err := stdin.Write(frame.Data); err != nil {
				errCh <- err

				return
			}
		case execstream.FrameTypeResize:
			// No-op for SSH backend without TTY support.
		default:
			errCh <- fmt.Errorf("unsupported frame type %q received from client", frame.Type)

			return
		}
	}
}

func streamExecuteOutput(
	reader io.Reader,
	frameType execstream.FrameType,
	outputCh chan<- execstream.Frame,
	doneCh chan<- struct{},
	errCh chan<- error,
) {
	defer func() {
		doneCh <- struct{}{}
	}()

	for {
		buffer := make([]byte, 4096)
		n, err := reader.Read(buffer)
		if n > 0 {
			outputCh <- execstream.Frame{
				Type: frameType,
				Data: append([]byte(nil), buffer[:n]...),
			}
		}

		if errors.Is(err, io.EOF) {
			return
		}

		if err != nil {
			select {
			case errCh <- err:
			default:
			}

			return
		}
	}
}

func buildSSHCommand(command string, args []string) string {
	parts := make([]string, 0, 1+len(args))
	parts = append(parts, shellQuoteArg(command))
	for _, arg := range args {
		parts = append(parts, shellQuoteArg(arg))
	}

	return strings.Join(parts, " ")
}

func shellQuoteArg(arg string) string {
	if arg == "" {
		return "''"
	}

	return "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
}

func (controller *Controller) closeExecuteWithFrameError(
	ctx context.Context,
	wsConn *websocket.Conn,
	encoder *json.Encoder,
	message string,
) {
	if encoder != nil {
		if err := execstream.WriteFrame(encoder, &execstream.Frame{
			Type:  execstream.FrameTypeError,
			Error: message,
		}); err != nil {
			controller.logger.Warnf("execute session: failed to send error frame: %v", err)
		}
	}

	if err := wsConn.Close(websocket.StatusInternalError, message); err != nil {
		controller.logger.Warnf("execute session: failed to close WebSocket connection: %v", err)
	}
}
