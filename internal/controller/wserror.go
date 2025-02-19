package controller

import (
	"fmt"
	"github.com/cirruslabs/orchard/internal/responder"
	"github.com/coder/websocket"
)

func (controller *Controller) wsError(
	wsConn *websocket.Conn,
	code websocket.StatusCode,
	component string,
	reason string,
	err error,
) responder.Responder {
	responder := controller.wsErrorNoClose(component, reason, err)

	if err := wsConn.Close(code, fmt.Sprintf("%s: %v", reason, err)); err != nil {
		controller.logger.Warnf("%s: failed to close the WebSocket connection that entered error state"+
			" due to %s: %v", component, reason, err)
	}

	return responder
}

func (controller *Controller) wsErrorNoClose(
	component string,
	reason string,
	err error,
) responder.Responder {
	controller.logger.Warnf("%s: %s: %v", component, reason, err)

	return responder.Empty()
}
