package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/avast/retry-go/v5"
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
	sessionID := ctx.Query("session")
	if sessionID == "" {
		sessionID = ctx.Query("cmux_session_id")
	}

	command := ctx.Query("command")
	if sessionID == "" && command == "" {
		return responder.JSON(http.StatusBadRequest,
			NewErrorResponse("\"command\" parameter cannot be empty"))
	}

	stdin := ctx.Query("stdin") == "true"

	waitRaw := ctx.DefaultQuery("wait", "10")
	wait, err := strconv.ParseUint(waitRaw, 10, 16)
	if err != nil {
		return responder.Code(http.StatusBadRequest)
	}

	if sessionID != "" {
		return controller.execVMReconnectable(ctx, name, sessionID, command, stdin, wait)
	}

	return controller.execVMLegacy(ctx, name, command, stdin, wait)
}

func (controller *Controller) execVMLegacy(
	ctx *gin.Context,
	name string,
	command string,
	stdin bool,
	wait uint64,
) responder.Responder {
	// Look-up the VM
	waitContext, waitContextCancel := context.WithTimeout(ctx, time.Duration(wait)*time.Second)
	defer waitContextCancel()

	vm, responderImpl := controller.waitForVM(waitContext, name)
	if responderImpl != nil {
		return responderImpl
	}

	session, err := controller.newSSHExecSession(
		ctx,
		waitContext,
		vm,
		execSessionKey{vmName: name},
		command,
		stdin,
		nil,
		legacyExecSessionPolicy,
	)
	if err != nil {
		return responder.JSON(http.StatusServiceUnavailable, NewErrorResponse("%v", err))
	}

	// Upgrade HTTP request to a WebSocket connection
	wsConn, err := websocket.Accept(ctx.Writer, ctx.Request, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		session.closeIfUnused()

		return responder.Error(err)
	}
	defer func() {
		// Ensure that we always close the accepted WebSocket connection,
		// otherwise resource leak is possible[1]
		//
		// [1]: https://github.com/coder/websocket/issues/445#issuecomment-2053792044
		_ = wsConn.CloseNow()
	}()

	return controller.serveExecSession(ctx, wsConn, session)
}

func (controller *Controller) execVMReconnectable(
	ctx *gin.Context,
	name string,
	sessionID string,
	command string,
	stdin bool,
	wait uint64,
) responder.Responder {
	key := execSessionKey{
		vmName:    name,
		sessionID: sessionID,
	}

	session, ok := controller.execSessions.get(key)
	if ok {
		if !session.commandMatches(command) {
			return responder.JSON(http.StatusConflict,
				NewErrorResponse("exec session %q is already running a different command", sessionID))
		}
	} else {
		if command == "" {
			return responder.JSON(http.StatusNotFound,
				NewErrorResponse("exec session %q does not exist", sessionID))
		}

		waitContext, waitContextCancel := context.WithTimeout(ctx, time.Duration(wait)*time.Second)
		defer waitContextCancel()

		vm, responderImpl := controller.waitForVM(waitContext, name)
		if responderImpl != nil {
			return responderImpl
		}

		var err error
		session, _, err = controller.execSessions.getOrCreate(waitContext, key, func() (*execSession, error) {
			return controller.newSSHExecSession(
				ctx,
				waitContext,
				vm,
				key,
				command,
				stdin,
				controller.execSessions,
				reconnectableExecSessionPolicy,
			)
		})
		if err != nil {
			return responder.JSON(http.StatusServiceUnavailable, NewErrorResponse("%v", err))
		}

		if !session.commandMatches(command) {
			return responder.JSON(http.StatusConflict,
				NewErrorResponse("exec session %q is already running a different command", sessionID))
		}
	}

	wsConn, err := websocket.Accept(ctx.Writer, ctx.Request, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		session.closeIfUnused()

		return responder.Error(err)
	}
	defer func() {
		_ = wsConn.CloseNow()
	}()

	return controller.serveExecSession(ctx, wsConn, session)
}

func (controller *Controller) newSSHExecSession(
	_ *gin.Context,
	waitContext context.Context,
	vm *v1.VM,
	key execSessionKey,
	command string,
	stdin bool,
	registry *execSessionRegistry,
	policy execSessionPolicy,
) (*execSession, error) {
	sessionContext, sessionContextCancel := context.WithCancel(context.Background())

	portForwardConn, err := retry.NewWithData[net.Conn](
		retry.Context(waitContext),
		retry.DelayType(retry.FixedDelay),
		retry.Delay(time.Second),
		retry.Attempts(0),
		retry.LastErrorOnly(true),
	).Do(func() (net.Conn, error) {
		return controller.portForwardConnection(sessionContext, waitContext, vm.Worker, vm.UID, 22)
	})
	if err != nil {
		sessionContextCancel()

		return nil, err
	}

	exec, err := sshexec.New(portForwardConn, vm.SSHUsername(), vm.SSHPassword(), stdin)
	if err != nil {
		sessionContextCancel()
		_ = portForwardConn.Close()

		return nil, fmt.Errorf("failed to establish SSH connection to a VM: %w", err)
	}

	return newExecSessionWithContext(
		sessionContext,
		sessionContextCancel,
		key,
		command,
		exec,
		portForwardConn,
		registry,
		controller.execSessionExitTTL,
		policy,
	), nil
}

func (controller *Controller) serveExecSession(
	ctx *gin.Context,
	wsConn *websocket.Conn,
	session *execSession,
) responder.Responder {
	subscriber, err := session.attach()
	if err != nil {
		_ = wsConn.Close(websocket.StatusNormalClosure, err.Error())

		return responder.Empty()
	}
	defer session.detach(subscriber)
	session.start()

	readFramesErrCh := make(chan error, 1)
	go func() {
		readFramesErrCh <- controller.readExecSessionFrames(ctx, wsConn, session, subscriber)
	}()

	for {
		select {
		case readFramesErr := <-readFramesErrCh:
			if readFramesErr != nil &&
				!errors.Is(readFramesErr, errExecSessionDetached) &&
				!errors.Is(readFramesErr, errExecSessionClosed) {
				controller.logger.Warnf("failed to read and process exec frames from WebSocket: %v",
					readFramesErr)
			}

			return responder.Empty()
		case outgoingFrame, ok := <-subscriber.frames:
			if !ok {
				if err := wsConn.Close(websocket.StatusNormalClosure, "Command finished"); err != nil {
					controller.logger.Warnf("exec: failed to close WebSocket cleanly: %v", err)
				}

				return responder.Empty()
			}

			if err := execstream.WriteFrame(ctx, wsConn, outgoingFrame); err != nil {
				controller.logger.Warnf("failed to write exec frame to the client: %v", err)

				return responder.Empty()
			}
		case <-time.After(controller.pingInterval):
			pingCtx, pingCtxCancel := context.WithTimeout(ctx, 5*time.Second)

			if err := wsConn.Ping(pingCtx); err != nil {
				controller.logger.Warnf("exec: failed to ping the client, "+
					"connection might time out: %v", err)
			}

			pingCtxCancel()
		case <-ctx.Done():
			controller.logger.Warnf("client disconnected prematurely")

			return responder.Empty()
		}
	}
}

var (
	errExecSessionDetached = errors.New("exec session detached")
	errExecSessionClosed   = errors.New("exec session closed")
)

func (controller *Controller) readExecSessionFrames(
	ctx context.Context,
	wsConn *websocket.Conn,
	session *execSession,
	subscriber *execSessionSubscriber,
) error {
	for {
		var frame execstream.Frame

		messageType, payloadBytes, err := wsConn.Read(ctx)
		if err != nil {
			var closeErr websocket.CloseError
			if errors.As(err, &closeErr) && closeErr.Code == websocket.StatusNormalClosure {
				return errExecSessionDetached
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
			if err := session.writeStdin(frame.Data); err != nil {
				return fmt.Errorf("failed to handle %q frame: %w", frame.Type, err)
			}
		case execstream.FrameTypeHistory:
			if !session.policy.replayEnabled {
				return fmt.Errorf("unexpected frame type received: %q", frame.Type)
			}

			session.sendHistory(subscriber, frame.Watermark)
		case execstream.FrameTypeAck:
			if !session.policy.replayEnabled {
				return fmt.Errorf("unexpected frame type received: %q", frame.Type)
			}

			session.ack(frame.Watermark)
		case execstream.FrameTypeDetach:
			if !session.policy.replayEnabled {
				return fmt.Errorf("unexpected frame type received: %q", frame.Type)
			}

			return errExecSessionDetached
		case execstream.FrameTypeClose:
			if !session.policy.replayEnabled {
				return fmt.Errorf("unexpected frame type received: %q", frame.Type)
			}

			session.close()

			return errExecSessionClosed
		default:
			return fmt.Errorf("unexpected frame type received: %q", frame.Type)
		}
	}
}
