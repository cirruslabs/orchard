package controller

import (
	"encoding/json"
	"errors"
	"github.com/cirruslabs/orchard/internal/responder"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/cirruslabs/orchard/rpc"
	"github.com/gin-gonic/gin"
	"nhooyr.io/websocket"
	"time"
)

func (controller *Controller) rpcWatch(ctx *gin.Context) responder.Responder {
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeRead); responder != nil {
		return responder
	}

	// Retrieve and parse path and query parameters
	workerName := ctx.Query("workerName")

	if workerName == "" {
		return responder.Error(errors.New("worker name cannot be empty"))
	}

	// Register with the worker notifier to forward requests from other
	// parts of the Orchard Controller destined to this specific worker
	workerCh, cancel := controller.workerNotifier.Register(ctx, workerName)
	defer cancel()

	// Perform WebSocket protocol upgrade
	wsConn, err := websocket.Accept(ctx.Writer, ctx.Request, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return responder.Error(err)
	}

	for {
		select {
		case msg := <-workerCh:
			var watchInstruction v1.WatchInstruction

			// Perform Protocol Buffers to v1 API data structure conversion
			switch typedAction := msg.Action.(type) {
			case *rpc.WatchInstruction_PortForwardAction:
				watchInstruction.PortForwardAction = &v1.PortForwardAction{
					Session: typedAction.PortForwardAction.Session,
					VMUID:   typedAction.PortForwardAction.VmUid,
					Port:    uint16(typedAction.PortForwardAction.Port),
				}
			case *rpc.WatchInstruction_SyncVmsAction:
				watchInstruction.SyncVMsAction = &v1.SyncVMsAction{}
			case *rpc.WatchInstruction_ResolveIpAction:
				watchInstruction.ResolveIPAction = &v1.ResolveIPAction{
					Session: typedAction.ResolveIpAction.Session,
					VMUID:   typedAction.ResolveIpAction.VmUid,
				}
			default:
				continue
			}

			watchInstructionJSONBytes, err := json.Marshal(&watchInstruction)
			if err != nil {
				return controller.wsError(wsConn, websocket.StatusInternalError, "watch RPC",
					"failure to marshal the watch instruction", err)
			}

			if err := wsConn.Write(ctx, websocket.MessageBinary, watchInstructionJSONBytes); err != nil {
				return controller.wsError(wsConn, websocket.StatusInternalError, "watch RPC",
					"failure to write the watch instruction", err)
			}
		case <-time.After(30 * time.Second):
			if err := wsConn.Ping(ctx); err != nil {
				controller.logger.Warnf("watch RPC: failed to ping the worker %s, "+
					"connection might time out: %v", workerName, err)
			}
		case <-ctx.Done():
			return controller.wsError(wsConn, websocket.StatusAbnormalClosure, "watch RPC",
				"unexpectedly disconnected worker", err)
		}
	}
}
