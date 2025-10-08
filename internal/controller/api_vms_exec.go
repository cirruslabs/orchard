package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/internal/execstream"
	"github.com/cirruslabs/orchard/internal/netconncancel"
	"github.com/cirruslabs/orchard/internal/responder"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/cirruslabs/orchard/rpc"
	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
	"net/http"
	"strconv"
	"time"
)

func (controller *Controller) execVM(ctx *gin.Context) responder.Responder {
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite); responder != nil {
		return responder
	}

	name := ctx.Param("name")

	command := ctx.Query("command")
	if command == "" {
		return responder.Code(http.StatusBadRequest)
	}

	args := ctx.QueryArray("arg")

	interactive, err := parseBoolWithDefault(ctx.Query("interactive"), false)
	if err != nil {
		return responder.Code(http.StatusBadRequest)
	}

	tty, err := parseBoolWithDefault(ctx.Query("tty"), false)
	if err != nil {
		return responder.Code(http.StatusBadRequest)
	}

	if tty {
		interactive = true
	}

	rows, err := parseUint32(ctx.Query("rows"))
	if err != nil {
		return responder.Code(http.StatusBadRequest)
	}

	cols, err := parseUint32(ctx.Query("cols"))
	if err != nil {
		return responder.Code(http.StatusBadRequest)
	}

	waitRaw := ctx.DefaultQuery("wait", "10")
	wait, err := strconv.ParseUint(waitRaw, 10, 16)
	if err != nil {
		return responder.Code(http.StatusBadRequest)
	}

	waitContext, waitContextCancel := context.WithTimeout(ctx, time.Duration(wait)*time.Second)
	defer waitContextCancel()

	vm, responderImpl := controller.waitForVM(waitContext, name)
	if responderImpl != nil {
		return responderImpl
	}

	var workerResource *v1.Worker

	if responderImpl := controller.storeView(func(txn storepkg.Transaction) responder.Responder {
		var err error

		workerResource, err = txn.GetWorker(vm.Worker)
		if err != nil {
			return responder.Error(err)
		}

		return nil
	}); responderImpl != nil {
		return responderImpl
	}

	if workerResource == nil || !workerResource.Capabilities.Has(v1.WorkerCapabilityExec) {
		return responder.JSON(http.StatusNotImplemented,
			NewErrorResponse("worker %s does not support exec", vm.Worker))
	}

	rendezvousCtx, rendezvousCtxCancel := context.WithCancel(ctx)
	defer rendezvousCtxCancel()

	session := uuid.New().String()

	boomerangConnCh, cancel := controller.connRendezvous.Request(rendezvousCtx, session)
	defer cancel()

	var terminalSize *rpc.WatchInstruction_Exec_TerminalSize
	if rows > 0 && cols > 0 {
		terminalSize = &rpc.WatchInstruction_Exec_TerminalSize{
			Rows: rows,
			Cols: cols,
		}
	}

	err = controller.workerNotifier.Notify(waitContext, vm.Worker, &rpc.WatchInstruction{
		Action: &rpc.WatchInstruction_ExecAction{
			ExecAction: &rpc.WatchInstruction_Exec{
				Session:     session,
				VmUid:       vm.UID,
				Command:     command,
				Args:        args,
				Interactive: interactive,
				Tty:         tty,
				TerminalSize: func() *rpc.WatchInstruction_Exec_TerminalSize {
					if terminalSize == nil {
						return nil
					}

					return &rpc.WatchInstruction_Exec_TerminalSize{
						Rows: terminalSize.Rows,
						Cols: terminalSize.Cols,
					}
				}(),
			},
		},
	})
	if err != nil {
		controller.logger.Warnf("failed to request exec session from the worker %s: %v",
			vm.Worker, err)

		return responder.Code(http.StatusServiceUnavailable)
	}

	select {
	case rendezvousResponse := <-boomerangConnCh:
		if rendezvousResponse.ErrorMessage != "" {
			return responder.Error(fmt.Errorf("failed to establish exec session on the worker: %s",
				rendezvousResponse.ErrorMessage))
		}

		if rendezvousResponse.Result == nil {
			return responder.Error(errors.New("failed to establish exec session on the worker: no connection"))
		}

		wsConn, err := websocket.Accept(ctx.Writer, ctx.Request, &websocket.AcceptOptions{
			OriginPatterns: []string{"*"},
		})
		if err != nil {
			_ = rendezvousResponse.Result.Close()

			return responder.Error(err)
		}
		defer func() {
			_ = wsConn.CloseNow()
		}()

		workerConnWithCancel := netconncancel.New(rendezvousResponse.Result, rendezvousCtxCancel)
		defer func() {
			_ = workerConnWithCancel.Close()
		}()

		expectedMsgType := websocket.MessageText
		wsConnAsNetConn := websocket.NetConn(ctx, wsConn, expectedMsgType)
		defer func() {
			_ = wsConnAsNetConn.Close()
		}()

		commandFrame := &execstream.Frame{
			Type: execstream.FrameTypeCommand,
			Command: &execstream.Command{
				Name:        command,
				Args:        args,
				Interactive: interactive,
				TTY:         tty,
			},
		}
		if terminalSize != nil {
			commandFrame.Command.Terminal = &execstream.TerminalSize{
				Rows: terminalSize.Rows,
				Cols: terminalSize.Cols,
			}
		}

		workerEncoder := execstream.NewEncoder(workerConnWithCancel)
		workerDecoder := execstream.NewDecoder(workerConnWithCancel)
		clientEncoder := execstream.NewEncoder(wsConnAsNetConn)
		clientDecoder := execstream.NewDecoder(wsConnAsNetConn)

		if err := execstream.WriteFrame(workerEncoder, commandFrame); err != nil {
			return controller.wsError(wsConn, websocket.StatusInternalError, "exec session",
				"failed to deliver command to worker", err)
		}

		workerErrCh := make(chan error, 1)
		clientErrCh := make(chan error, 1)
		exitCh := make(chan int32, 1)

		go controller.forwardExecFromWorker(workerDecoder, clientEncoder, workerErrCh, exitCh)
		go controller.forwardExecFromClient(clientDecoder, workerEncoder, clientErrCh)

		pingTicker := time.NewTicker(controller.pingInterval)
		defer pingTicker.Stop()

		for {
			select {
			case err := <-workerErrCh:
				if err == nil {
					continue
				}

				if errors.Is(err, context.Canceled) {
					return responder.Empty()
				}

				if statusErr, ok := status.FromError(err); ok && statusErr.Code() == codes.Canceled {
					return responder.Empty()
				}

				if errors.Is(err, io.EOF) {
					return controller.wsError(wsConn, websocket.StatusInternalError, "exec session",
						"worker closed the exec stream unexpectedly", err)
				}

				return controller.wsError(wsConn, websocket.StatusInternalError, "exec session",
					"failed while proxying worker stream", err)
			case err := <-clientErrCh:
				if err == nil {
					continue
				}

				var websocketCloseError websocket.CloseError
				if errors.As(err, &websocketCloseError) {
					return responder.Empty()
				}

				if errors.Is(err, io.EOF) {
					return responder.Empty()
				}

				return controller.wsError(wsConn, websocket.StatusInternalError, "exec session",
					"failed while proxying client stream", err)
			case exitCode := <-exitCh:
				if err := wsConn.Close(websocket.StatusNormalClosure,
					fmt.Sprintf("command exited with code %d", exitCode)); err != nil {
					controller.logger.Warnf("exec session: failed to close WebSocket connection: %v", err)
				}

				return responder.Empty()
			case <-pingTicker.C:
				pingCtx, pingCtxCancel := context.WithTimeout(ctx, 5*time.Second)

				if err := wsConn.Ping(pingCtx); err != nil {
					controller.logger.Warnf("exec session: failed to ping the client, "+
						"connection might time out: %v", err)
				}

				pingCtxCancel()
			case <-ctx.Done():
				return responder.Error(ctx.Err())
			}
		}
	case <-ctx.Done():
		return responder.Error(ctx.Err())
	}
}

func parseBoolWithDefault(raw string, defaultValue bool) (bool, error) {
	if raw == "" {
		return defaultValue, nil
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, err
	}

	return value, nil
}

func parseUint32(raw string) (uint32, error) {
	if raw == "" {
		return 0, nil
	}

	value, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return 0, err
	}

	return uint32(value), nil
}

func (controller *Controller) forwardExecFromWorker(
	decoder *json.Decoder,
	encoder *json.Encoder,
	errCh chan<- error,
	exitCh chan<- int32,
) {
	for {
		var frame execstream.Frame

		if err := execstream.ReadFrame(decoder, &frame); err != nil {
			errCh <- err

			return
		}

		if err := execstream.WriteFrame(encoder, &frame); err != nil {
			errCh <- err

			return
		}

		if frame.Type == execstream.FrameTypeExit && frame.Exit != nil {
			exitCh <- frame.Exit.Code

			return
		}
	}
}

func (controller *Controller) forwardExecFromClient(
	decoder *json.Decoder,
	encoder *json.Encoder,
	errCh chan<- error,
) {
	for {
		var frame execstream.Frame

		if err := execstream.ReadFrame(decoder, &frame); err != nil {
			errCh <- err

			return
		}

		switch frame.Type {
		case execstream.FrameTypeStdin, execstream.FrameTypeResize:
			if err := execstream.WriteFrame(encoder, &frame); err != nil {
				errCh <- err

				return
			}
		default:
			errCh <- fmt.Errorf("unsupported frame type %q received from client", frame.Type)

			return
		}
	}
}
