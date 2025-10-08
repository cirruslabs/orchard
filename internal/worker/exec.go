package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/internal/execstream"
	"github.com/cirruslabs/orchard/internal/worker/vmmanager"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/cirruslabs/orchard/rpc"
	guestagentrpc "github.com/cirruslabs/orchard/rpc/guestagent"
	"github.com/samber/lo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

type execOptions struct {
	Session     string
	VMUID       string
	Command     string
	Args        []string
	Interactive bool
	TTY         bool
	Terminal    *execstream.TerminalSize
}

func execOptionsFromProto(action *rpc.WatchInstruction_Exec) execOptions {
	opts := execOptions{
		Session:     action.Session,
		VMUID:       action.VmUid,
		Command:     action.Command,
		Args:        append([]string(nil), action.Args...),
		Interactive: action.Interactive,
		TTY:         action.Tty,
	}

	if action.TerminalSize != nil {
		opts.Terminal = &execstream.TerminalSize{
			Rows: action.TerminalSize.Rows,
			Cols: action.TerminalSize.Cols,
		}
	}

	return opts
}

func execOptionsFromV1(action *v1.ExecAction) execOptions {
	opts := execOptions{
		Session:     action.Session,
		VMUID:       action.VMUID,
		Command:     action.Command,
		Args:        append([]string(nil), action.Args...),
		Interactive: action.Interactive,
		TTY:         action.TTY,
	}

	if action.Terminal != nil {
		opts.Terminal = &execstream.TerminalSize{
			Rows: action.Terminal.Rows,
			Cols: action.Terminal.Cols,
		}
	}

	return opts
}

func (worker *Worker) runExecSession(
	ctx context.Context,
	opts execOptions,
	controllerConn net.Conn,
	vmHint *vmmanager.VM,
) error {
	defer controllerConn.Close()

	controllerDecoder := execstream.NewDecoder(controllerConn)
	controllerEncoder := execstream.NewEncoder(controllerConn)

	commandDetails := execstream.Command{
		Name:        opts.Command,
		Args:        append([]string(nil), opts.Args...),
		Interactive: opts.Interactive,
		TTY:         opts.TTY,
		Terminal:    cloneTerminalSize(opts.Terminal),
	}

	var firstFrame execstream.Frame

	if err := execstream.ReadFrame(controllerDecoder, &firstFrame); err != nil {
		errWrapped := fmt.Errorf("failed to read command frame: %w", err)
		worker.sendExecErrorFrame(controllerEncoder, errWrapped.Error())

		return errWrapped
	}

	if firstFrame.Type != execstream.FrameTypeCommand || firstFrame.Command == nil {
		errWrapped := fmt.Errorf("expected command frame, got %q", firstFrame.Type)
		worker.sendExecErrorFrame(controllerEncoder, errWrapped.Error())

		return errWrapped
	}

	frameCommand := firstFrame.Command

	if frameCommand.Name != "" {
		commandDetails.Name = frameCommand.Name
	}
	if len(frameCommand.Args) != 0 {
		commandDetails.Args = append([]string(nil), frameCommand.Args...)
	}
	if frameCommand.Interactive {
		commandDetails.Interactive = true
	}
	if frameCommand.TTY {
		commandDetails.TTY = true
	}
	if frameCommand.Terminal != nil {
		commandDetails.Terminal = &execstream.TerminalSize{
			Rows: frameCommand.Terminal.Rows,
			Cols: frameCommand.Terminal.Cols,
		}
	} else if commandDetails.Terminal == nil && opts.Terminal != nil {
		commandDetails.Terminal = cloneTerminalSize(opts.Terminal)
	}

	if commandDetails.Name == "" {
		errWrapped := errors.New("command name is empty")
		worker.sendExecErrorFrame(controllerEncoder, errWrapped.Error())

		return errWrapped
	}

	if commandDetails.Args == nil {
		commandDetails.Args = []string{}
	}

	vm := vmHint

	if vm == nil {
		var err error

		vm, err = worker.findVMByUID(opts.VMUID)
		if err != nil {
			worker.sendExecErrorFrame(controllerEncoder, err.Error())

			return err
		}
	}

	socketPath, err := vmControlSocketPath(vm)
	if err != nil {
		errWrapped := fmt.Errorf("failed to determine control socket path: %w", err)
		worker.sendExecErrorFrame(controllerEncoder, errWrapped.Error())

		return errWrapped
	}

	agentConn, err := worker.connectToGuestAgent(ctx, socketPath)
	if err != nil {
		errWrapped := fmt.Errorf("failed to connect to guest agent: %w", err)
		worker.sendExecErrorFrame(controllerEncoder, errWrapped.Error())

		return errWrapped
	}
	defer agentConn.Close()

	var terminalSize *guestagentrpc.TerminalSize

	if commandDetails.Terminal != nil {
		terminalSize = &guestagentrpc.TerminalSize{
			Rows: commandDetails.Terminal.Rows,
			Cols: commandDetails.Terminal.Cols,
		}
	}

	commandReq := &guestagentrpc.ExecRequest{
		Type: &guestagentrpc.ExecRequest_Command_{
			Command: &guestagentrpc.ExecRequest_Command{
				Name:        commandDetails.Name,
				Args:        append([]string(nil), commandDetails.Args...),
				Interactive: commandDetails.Interactive,
				Tty:         commandDetails.TTY,
				TerminalSize: func() *guestagentrpc.TerminalSize {
					if terminalSize == nil {
						return nil
					}

					return &guestagentrpc.TerminalSize{
						Rows: terminalSize.Rows,
						Cols: terminalSize.Cols,
					}
				}(),
			},
		},
	}

	methods := []string{guestagentrpc.Agent_Exec_FullMethodName, "/Agent/Exec"}
	var lastErr error

outer:
	for idx, method := range methods {
		streamCtx, streamCancel := context.WithCancel(ctx)

		agentStream, err := worker.establishAgentExecStream(streamCtx, agentConn, method)
		if err != nil {
			streamCancel()
			lastErr = err
			if status.Code(err) == codes.Unimplemented && idx+1 < len(methods) {
				worker.logger.Debugf("exec session: guest agent refused method %s, falling back", method)
				continue outer
			}

			errWrapped := fmt.Errorf("failed to start exec stream: %w", err)
			worker.sendExecErrorFrame(controllerEncoder, errWrapped.Error())

			return errWrapped
		}

		if err := agentStream.Send(commandReq); err != nil {
			_ = agentStream.CloseSend()
			streamCancel()
			lastErr = err
			if status.Code(err) == codes.Unimplemented && idx+1 < len(methods) {
				worker.logger.Debugf("exec session: guest agent rejected method %s on send, falling back", method)
				continue outer
			}

			errWrapped := fmt.Errorf("failed to send command to guest agent: %w", err)
			worker.sendExecErrorFrame(controllerEncoder, errWrapped.Error())

			return errWrapped
		}

		agentErrCh := make(chan error, 1)
		exitCh := make(chan int32, 1)

		go worker.forwardAgentToController(agentStream, controllerEncoder, agentErrCh, exitCh)

		controllerErrCh := make(chan error, 1)
		controllerStarted := false
		timer := time.NewTimer(200 * time.Millisecond)

		for {
			select {
			case <-timer.C:
				if !controllerStarted {
					controllerStarted = true
					go worker.forwardControllerToAgent(agentStream, controllerDecoder, controllerErrCh, commandDetails.TTY)

					worker.logger.Infow("exec session started",
						"session", opts.Session,
						"vm_uid", opts.VMUID,
						"command", commandDetails.Name,
						"args", commandDetails.Args,
						"interactive", commandDetails.Interactive,
						"tty", commandDetails.TTY,
						"method", method,
					)
				}

			case err := <-controllerErrCh:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				streamCancel()
				if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
					worker.sendExecErrorFrame(controllerEncoder, fmt.Sprintf("controller stream error: %v", err))

					return err
				}

				return err

			case err := <-agentErrCh:
				if err != nil {
					if !controllerStarted && status.Code(err) == codes.Unimplemented && idx+1 < len(methods) {
						lastErr = err
						streamCancel()
						_ = agentStream.CloseSend()
						if !timer.Stop() {
							select {
							case <-timer.C:
							default:
							}
						}

						continue outer
					}

					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					streamCancel()
					worker.sendExecErrorFrame(controllerEncoder, fmt.Sprintf("guest agent error: %v", err))

					return err
				}

			case exitCode := <-exitCh:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				streamCancel()
				worker.logger.Infow("exec session finished",
					"session", opts.Session,
					"vm_uid", opts.VMUID,
					"exit_code", exitCode,
				)

				return nil

			case <-ctx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				streamCancel()

				return ctx.Err()
			}
		}
	}

	if lastErr != nil {
		worker.sendExecErrorFrame(controllerEncoder, fmt.Sprintf("guest agent error: %v", lastErr))

		return lastErr
	}

	unsupportedErr := fmt.Errorf("guest agent exec unsupported")
	worker.sendExecErrorFrame(controllerEncoder, unsupportedErr.Error())

	return unsupportedErr
}

func (worker *Worker) forwardControllerToAgent(
	agentStream guestagentrpc.Agent_ExecClient,
	decoder *json.Decoder,
	errCh chan<- error,
	tty bool,
) {
	for {
		var frame execstream.Frame

		if err := execstream.ReadFrame(decoder, &frame); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				_ = agentStream.CloseSend()
			}

			errCh <- err

			return
		}

		switch frame.Type {
		case execstream.FrameTypeStdin:
			data := frame.Data

			if tty && len(data) == 0 {
				// send EOT
				data = []byte{0x04}
			}

			if len(data) == 0 && !tty {
				continue
			}

			if err := agentStream.Send(&guestagentrpc.ExecRequest{
				Type: &guestagentrpc.ExecRequest_StandardInput{
					StandardInput: &guestagentrpc.IOChunk{Data: data},
				},
			}); err != nil {
				errCh <- err

				return
			}
		case execstream.FrameTypeResize:
			if !tty || frame.Terminal == nil {
				continue
			}

			if err := agentStream.Send(&guestagentrpc.ExecRequest{
				Type: &guestagentrpc.ExecRequest_TerminalResize{
					TerminalResize: &guestagentrpc.TerminalSize{
						Rows: frame.Terminal.Rows,
						Cols: frame.Terminal.Cols,
					},
				},
			}); err != nil {
				errCh <- err

				return
			}
		case execstream.FrameTypeError:
			errCh <- fmt.Errorf("controller reported error: %s", frame.Error)

			return
		default:
			// Ignore unsupported frame types
		}
	}
}

func (worker *Worker) forwardAgentToController(
	agentStream guestagentrpc.Agent_ExecClient,
	encoder *json.Encoder,
	errCh chan<- error,
	exitCh chan<- int32,
) {
	for {
		resp, err := agentStream.Recv()
		if err != nil {
			errCh <- err

			return
		}

		switch typed := resp.Type.(type) {
		case *guestagentrpc.ExecResponse_StandardOutput:
			if err := execstream.WriteFrame(encoder, &execstream.Frame{
				Type: execstream.FrameTypeStdout,
				Data: typed.StandardOutput.Data,
			}); err != nil {
				errCh <- err

				return
			}
		case *guestagentrpc.ExecResponse_StandardError:
			if err := execstream.WriteFrame(encoder, &execstream.Frame{
				Type: execstream.FrameTypeStderr,
				Data: typed.StandardError.Data,
			}); err != nil {
				errCh <- err

				return
			}
		case *guestagentrpc.ExecResponse_Exit_:
			if typed.Exit == nil {
				continue
			}

			if err := execstream.WriteFrame(encoder, &execstream.Frame{
				Type: execstream.FrameTypeExit,
				Exit: &execstream.Exit{Code: typed.Exit.Code},
			}); err != nil {
				errCh <- err

				return
			}

			exitCh <- typed.Exit.Code

			return
		default:
			// ignore unknown payloads
		}
	}
}

func (worker *Worker) findVMByUID(uid string) (*vmmanager.VM, error) {
	vm, ok := lo.Find(worker.vmm.List(), func(item *vmmanager.VM) bool {
		return item.Resource.UID == uid
	})
	if !ok {
		return nil, fmt.Errorf("VM with UID %q not found", uid)
	}

	if !vm.Started() {
		return nil, fmt.Errorf("VM with UID %q is not running", uid)
	}

	return vm, nil
}

func vmControlSocketPath(vm *vmmanager.VM) (string, error) {
	if vm == nil {
		return "", errors.New("nil VM provided")
	}

	tartHome := os.Getenv("TART_HOME")
	if tartHome == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to determine user home directory: %w", err)
		}

		tartHome = filepath.Join(homeDir, ".tart")
	}

	return filepath.Join(tartHome, "vms", vm.OnDiskName().String(), "control.sock"), nil
}

func (worker *Worker) connectToGuestAgent(ctx context.Context, socketPath string) (*grpc.ClientConn, error) {
	waitCtx, waitCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer waitCancel()

	backoff := 500 * time.Millisecond

	for {
		if _, err := os.Stat(socketPath); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				worker.logger.Warnf("exec session: control socket check failed: %v", err)
			}
		}

		attemptCtx, attemptCancel := context.WithTimeout(waitCtx, 5*time.Second)

		conn, err := grpc.DialContext(
			attemptCtx,
			"unix://"+socketPath,
			grpc.WithContextDialer(func(ctx context.Context, address string) (net.Conn, error) {
				var dialer net.Dialer

				return dialer.DialContext(ctx, "unix", socketPath)
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
		)

		attemptCancel()

		if err == nil {
			return conn, nil
		}

		if waitCtx.Err() != nil {
			return nil, err
		}

		worker.logger.Debugf("exec session: guest agent not ready yet: %v", err)

		select {
		case <-time.After(backoff):
			if backoff < 5*time.Second {
				backoff *= 2
			}
		case <-waitCtx.Done():
			return nil, waitCtx.Err()
		}
	}
}

func (worker *Worker) sendExecErrorFrame(encoder *json.Encoder, message string) {
	if encoder == nil {
		return
	}

	if err := execstream.WriteFrame(encoder, &execstream.Frame{
		Type:  execstream.FrameTypeError,
		Error: message,
	}); err != nil && !errors.Is(err, io.EOF) {
		worker.logger.Warnf("exec session: failed to send error frame: %v", err)
	}
}

func cloneTerminalSize(terminal *execstream.TerminalSize) *execstream.TerminalSize {
	if terminal == nil {
		return nil
	}

	return &execstream.TerminalSize{
		Rows: terminal.Rows,
		Cols: terminal.Cols,
	}
}

func (worker *Worker) establishAgentExecStream(
	ctx context.Context,
	conn *grpc.ClientConn,
	method string,
) (guestagentrpc.Agent_ExecClient, error) {
	stream, err := conn.NewStream(ctx, &guestagentrpc.Agent_ServiceDesc.Streams[0],
		method, grpc.StaticMethod())
	if err != nil {
		return nil, err
	}

	return &grpc.GenericClientStream[guestagentrpc.ExecRequest, guestagentrpc.ExecResponse]{
		ClientStream: stream,
	}, nil
}
