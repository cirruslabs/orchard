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
	message := fmt.Sprintf("%s: %v", reason, err)

	controller.logger.Warn(message)

	if err := wsConn.Close(code, message); err != nil {
		controller.logger.Warnf("%s: failed to close the WebSocket connection that entered error state"+
			" due to %s: %v", component, reason, err)
	}

	return responder.Empty()
}
