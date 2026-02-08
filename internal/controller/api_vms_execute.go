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

type executeVMRequest struct {
	name    string
	command string
	args    []string
	wait    time.Duration
}

type executeSessionChannels struct {
	outputFrameCh chan execstream.Frame
	outputDoneCh  chan struct{}
	outputErrCh   chan error
	stdinErrCh    chan error
	exitCodeCh    chan int32
	exitErrCh     chan error
}

func (controller *Controller) executeVM(ctx *gin.Context) responder.Responder {
	if responder := controller.authorizeAny(ctx, v1.ServiceAccountRoleComputeWrite,
		v1.ServiceAccountRoleComputeConnect); responder != nil {
		return responder
	}

	request, responderImpl := parseExecuteVMRequest(ctx)
	if responderImpl != nil {
		return responderImpl
	}

	waitCtx, waitCancel := context.WithTimeout(ctx, request.wait)
	defer waitCancel()

	vm, responderImpl := controller.waitForVM(waitCtx, request.name)
	if responderImpl != nil {
		return responderImpl
	}

	tunnel, responderImpl := controller.establishExecuteSSHTunnel(ctx, waitCtx, vm)
	if responderImpl != nil {
		return responderImpl
	}
	defer func() {
		_ = tunnel.Close()
	}()

	wsConn, err := acceptExecuteWebSocket(ctx)
	if err != nil {
		return responder.Error(err)
	}
	defer func() {
		_ = wsConn.CloseNow()
	}()

	return controller.executeVMViaSSHTunnel(ctx, tunnel, wsConn, vm, request.command, request.args)
}

func parseExecuteVMRequest(ctx *gin.Context) (*executeVMRequest, responder.Responder) {
	command := ctx.Query("command")
	if command == "" {
		return nil, responder.Code(http.StatusBadRequest)
	}

	waitRaw := ctx.DefaultQuery("wait", "10")
	wait, err := strconv.ParseUint(waitRaw, 10, 16)
	if err != nil {
		return nil, responder.Code(http.StatusBadRequest)
	}

	return &executeVMRequest{
		name:    ctx.Param("name"),
		command: command,
		args:    ctx.QueryArray("arg"),
		wait:    time.Duration(wait) * time.Second,
	}, nil
}

func acceptExecuteWebSocket(ctx *gin.Context) (*websocket.Conn, error) {
	return websocket.Accept(ctx.Writer, ctx.Request, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
}

func (controller *Controller) establishExecuteSSHTunnel(
	ctx context.Context,
	waitCtx context.Context,
	vm *v1.VM,
) (net.Conn, responder.Responder) {
	rendezvousCtx, rendezvousCtxCancel := context.WithCancel(ctx)

	session := uuid.New().String()
	connCh, cancelRequest := controller.connRendezvous.Request(rendezvousCtx, session)
	defer cancelRequest()

	err := controller.workerNotifier.Notify(waitCtx, vm.Worker, &rpc.WatchInstruction{
		Action: &rpc.WatchInstruction_PortForwardAction{
			PortForwardAction: &rpc.WatchInstruction_PortForward{
				Session: session,
				VmUid:   vm.UID,
				Port:    22,
			},
		},
	})
	if err != nil {
		rendezvousCtxCancel()

		controller.logger.Warnf("failed to request VM SSH port-forwarding from the worker %s: %v",
			vm.Worker, err)

		return nil, responder.Code(http.StatusServiceUnavailable)
	}

	timeoutTimer := time.NewTimer(executeSessionRendezvousTimeout)
	defer timeoutTimer.Stop()

	select {
	case rendezvousResp := <-connCh:
		if rendezvousResp.ErrorMessage != "" {
			rendezvousCtxCancel()

			return nil, responder.JSON(http.StatusServiceUnavailable, NewErrorResponse(
				"failed to establish SSH connection to the VM on the worker: %s", rendezvousResp.ErrorMessage))
		}

		if rendezvousResp.Result == nil {
			rendezvousCtxCancel()

			return nil, responder.Code(http.StatusServiceUnavailable)
		}

		return netconncancel.New(rendezvousResp.Result, rendezvousCtxCancel), nil
	case <-timeoutTimer.C:
		rendezvousCtxCancel()

		return nil, responder.JSON(http.StatusServiceUnavailable, NewErrorResponse(
			"timed out waiting for worker %s to establish SSH tunnel", vm.Worker))
	case <-ctx.Done():
		rendezvousCtxCancel()

		return nil, responder.Error(ctx.Err())
	}
}

func (controller *Controller) executeVMViaSSHTunnel(ctx context.Context, tunnel net.Conn, ws *websocket.Conn, vm *v1.VM, cmd string, args []string) responder.Responder {
	sshClient, err := newSSHClient(tunnel, vm)
	if err != nil {
		controller.closeExecuteWithFrameError(ws, nil,
			fmt.Sprintf("SSH handshake with the VM failed: %v", err))

		return responder.Empty()
	}
	defer func() {
		_ = sshClient.Close()
	}()

	execution, err := startSSHExecution(sshClient, cmd, args)
	if err != nil {
		controller.closeExecuteWithFrameError(ws, nil, err.Error())

		return responder.Empty()
	}
	defer func() {
		_ = execution.session.Close()
	}()

	return controller.runExecuteSession(ctx, ws, execution)
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

func startExecuteSessionChannels(
	sessionCtx context.Context,
	decoder *json.Decoder,
	execution *sshExecution,
) executeSessionChannels {
	channels := executeSessionChannels{
		outputFrameCh: make(chan execstream.Frame, 16),
		outputDoneCh:  make(chan struct{}, 2),
		outputErrCh:   make(chan error, 2),
		stdinErrCh:    make(chan error, 1),
		exitCodeCh:    make(chan int32, 1),
		exitErrCh:     make(chan error, 1),
	}

	go forwardSSHOutputFrames(sessionCtx, execution.stdout, execstream.FrameTypeStdout,
		channels.outputFrameCh, channels.outputDoneCh, channels.outputErrCh)
	go forwardSSHOutputFrames(sessionCtx, execution.stderr, execstream.FrameTypeStderr,
		channels.outputFrameCh, channels.outputDoneCh, channels.outputErrCh)
	go consumeClientInputFrames(decoder, execution.stdin, channels.stdinErrCh)
	go waitForSSHExecutionExit(execution.session, channels.exitCodeCh, channels.exitErrCh)

	return channels
}

func flushExecuteOutputFrames(encoder *json.Encoder, outputFrameCh <-chan execstream.Frame) error {
	for len(outputFrameCh) > 0 {
		frame := <-outputFrameCh
		if err := execstream.WriteFrame(encoder, &frame); err != nil {
			return err
		}
	}

	return nil
}

func firstExecuteOutputError(outputErrCh <-chan error) error {
	for len(outputErrCh) > 0 {
		err := <-outputErrCh
		if err != nil {
			return err
		}
	}

	return nil
}

func (controller *Controller) runExecuteSession(
	ctx context.Context,
	ws *websocket.Conn,
	execution *sshExecution,
) responder.Responder {
	sessionCtx, sessionCancel := context.WithCancel(ctx)
	defer sessionCancel()

	wsNetConn := websocket.NetConn(sessionCtx, ws, websocket.MessageText)
	defer func() {
		_ = wsNetConn.Close()
	}()

	encoder := execstream.NewEncoder(wsNetConn)
	decoder := execstream.NewDecoder(wsNetConn)

	channels := startExecuteSessionChannels(sessionCtx, decoder, execution)

	pingTicker := time.NewTicker(controller.pingInterval)
	defer pingTicker.Stop()

	outputReadersDone := 0
	exitSeen := false
	var exitCode int32

	for {
		if exitSeen && outputReadersDone >= 2 {
			if outputErr := firstExecuteOutputError(channels.outputErrCh); outputErr != nil {
				controller.closeExecuteWithFrameError(ws, encoder,
					fmt.Sprintf("failed while streaming command output: %v", outputErr))

				return responder.Empty()
			}

			if err := flushExecuteOutputFrames(encoder, channels.outputFrameCh); err != nil {
				return controller.wsError(ws, websocket.StatusInternalError, "execute session",
					"failed to stream execute output to the client", err)
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
		case frame := <-channels.outputFrameCh:
			if err := execstream.WriteFrame(encoder, &frame); err != nil {
				return controller.wsError(ws, websocket.StatusInternalError, "execute session",
					"failed to stream execute output to the client", err)
			}
		case <-channels.outputDoneCh:
			outputReadersDone++
		case err := <-channels.outputErrCh:
			if err == nil {
				continue
			}

			controller.closeExecuteWithFrameError(ws, encoder,
				fmt.Sprintf("failed while streaming command output: %v", err))

			return responder.Empty()
		case err := <-channels.stdinErrCh:
			if err == nil || errors.Is(err, context.Canceled) {
				channels.stdinErrCh = nil
				continue
			}

			if errors.Is(err, io.EOF) {
				return responder.Empty()
			}

			controller.closeExecuteWithFrameError(ws, encoder,
				fmt.Sprintf("failed while reading command stdin stream: %v", err))

			return responder.Empty()
		case code := <-channels.exitCodeCh:
			exitSeen = true
			exitCode = code
		case err := <-channels.exitErrCh:
			controller.closeExecuteWithFrameError(ws, encoder,
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

func consumeClientInputFrames(
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

func forwardSSHOutputFrames(
	ctx context.Context,
	reader io.Reader,
	frameType execstream.FrameType,
	outputFrameCh chan<- execstream.Frame,
	outputDoneCh chan<- struct{},
	outputErrCh chan<- error,
) {
	defer func() {
		outputDoneCh <- struct{}{}
	}()

	buffer := make([]byte, 4096)

	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			frame := execstream.Frame{
				Type: frameType,
				Data: append([]byte(nil), buffer[:n]...),
			}

			select {
			case outputFrameCh <- frame:
			case <-ctx.Done():
				return
			}
		}

		if errors.Is(err, io.EOF) {
			return
		}

		if err != nil {
			select {
			case outputErrCh <- err:
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
