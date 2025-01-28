package controller

import (
	"github.com/cirruslabs/orchard/internal/controller/rendezvous"
	"github.com/cirruslabs/orchard/internal/responder"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/gin-gonic/gin"
	"net"
	"nhooyr.io/websocket"
)

func (controller *Controller) rpcPortForward(ctx *gin.Context) responder.Responder {
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite); responder != nil {
		return responder
	}

	// Retrieve and parse path and query parameters
	session := ctx.Query("session")
	errorMessage := ctx.Query("errorMessage")

	// Perform WebSocket protocol upgrade
	wsConn, err := websocket.Accept(ctx.Writer, ctx.Request, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return responder.Error(err)
	}

	// Respond with the established connection
	proxyCtx, err := controller.connRendezvous.Respond(session, rendezvous.ResultWithErrorMessage[net.Conn]{
		Result:       websocket.NetConn(ctx, wsConn, websocket.MessageBinary),
		ErrorMessage: errorMessage,
	})
	if err != nil {
		return controller.wsError(wsConn, websocket.StatusInternalError, "port forwarding RPC",
			"failure to respond with the established WebSocket connection", err)
	}

	select {
	case <-proxyCtx.Done():
		// Do not close the WebSocket connection as it should be already closed by our rendezvous party
		return responder.Empty()
	case <-ctx.Done():
		// Connection should normally be closed by our rendezvous party, not by the worker
		return controller.wsError(wsConn, websocket.StatusAbnormalClosure, "port forwarding RPC",
			"unexpectedly disconnected worker", err)
	}
}
