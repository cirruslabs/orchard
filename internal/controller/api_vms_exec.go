package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/cirruslabs/orchard/internal/controller/sshexec"
	"github.com/cirruslabs/orchard/internal/execstream"
	"github.com/cirruslabs/orchard/internal/responder"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
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

	// Establish an SSH connection to a VM
	exec, err := sshexec.New(portForwardConn, vm.SSHUsername(), vm.SSHPassword(), stdin)
	if err != nil {
		return responder.JSON(http.StatusServiceUnavailable, NewErrorResponse("failed to establish SSH connection to a VM: %v", err))
	}
	defer exec.Close()

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

	// Read WebSocket frames
	readFramesErrCh := make(chan error, 1)
	go func() {
		readFramesErrCh <- controller.readFrames(ctx, wsConn, exec.Stdin())
	}()

	// Run the command
	sshErrCh := make(chan error, 1)
	outgoingFrames := make(chan *execstream.Frame)
	go func() {
		sshErrCh <- exec.Run(ctx, command, outgoingFrames)
	}()

	for {
		select {
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
			if errors.As(err, &closeErr) && closeErr.Code == websocket.StatusNormalClosure {
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
