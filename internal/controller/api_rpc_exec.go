package controller

import (
	"context"
	"github.com/cirruslabs/orchard/internal/controller/rendezvous"
	"github.com/cirruslabs/orchard/internal/responder"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"net"
	"time"
)

func (controller *Controller) rpcExec(ctx *gin.Context) responder.Responder {
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite); responder != nil {
		return responder
	}

	session := ctx.Query("session")
	errorMessage := ctx.Query("errorMessage")

	wsConn, err := websocket.Accept(ctx.Writer, ctx.Request, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return responder.Error(err)
	}
	defer func() {
		_ = wsConn.CloseNow()
	}()

	proxyCtx, err := controller.connRendezvous.Respond(session, rendezvous.ResultWithErrorMessage[net.Conn]{
		Result:       websocket.NetConn(ctx, wsConn, websocket.MessageBinary),
		ErrorMessage: errorMessage,
	})
	if err != nil {
		return controller.wsError(wsConn, websocket.StatusInternalError, "exec RPC",
			"failure to respond with the established WebSocket connection", err)
	}

	for {
		select {
		case <-proxyCtx.Done():
			return responder.Empty()
		case <-time.After(controller.pingInterval):
			pingCtx, pingCtxCancel := context.WithTimeout(ctx, 5*time.Second)

			if err := wsConn.Ping(pingCtx); err != nil {
				controller.logger.Warnf("exec RPC: failed to ping the worker, "+
					"connection might time out: %v", err)
			}

			pingCtxCancel()
		case <-ctx.Done():
			return controller.wsErrorNoClose("exec RPC",
				"worker unexpectedly disconnected", ctx.Err())
		}
	}
}
