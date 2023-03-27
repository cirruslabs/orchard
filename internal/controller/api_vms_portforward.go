package controller

import (
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/internal/proxy"
	"github.com/cirruslabs/orchard/internal/responder"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/cirruslabs/orchard/rpc"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/net/websocket"
	"net/http"
	"strconv"
)

func (controller *Controller) portForwardVM(ctx *gin.Context) responder.Responder {
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite); responder != nil {
		return responder
	}

	// Retrieve and parse path and query parameters
	name := ctx.Param("name")

	portRaw := ctx.Query("port")
	port, err := strconv.ParseUint(portRaw, 10, 16)
	if err != nil {
		return responder.Code(http.StatusBadRequest)
	}
	if port < 1 || port > 65535 {
		return responder.Code(http.StatusBadRequest)
	}

	// Look-up the VM
	var vm *v1.VM

	if responder := controller.storeView(func(txn storepkg.Transaction) responder.Responder {
		vm, err = txn.GetVM(name)
		if err != nil {
			return responder.Error(err)
		}

		return nil
	}); responder != nil {
		return responder
	}

	// Sanity-check
	if vm.Worker == "" {
		return responder.Code(http.StatusServiceUnavailable)
	}

	// Request and wait for a connection with a worker
	session := uuid.New().String()
	boomerangConnCh, cancel := controller.proxy.Request(ctx, session)
	defer cancel()

	// send request to worker to initiate port-forwarding connection back to us
	err = controller.workerNotifier.Notify(ctx, vm.Worker, &rpc.WatchInstruction{
		Action: &rpc.WatchInstruction_PortForwardAction{
			PortForwardAction: &rpc.WatchInstruction_PortForward{
				Session: session,
				VmUid:   vm.UID,
				VmPort:  uint32(port),
			},
		},
	})
	if err != nil {
		controller.logger.Warnf("failed to request port-forwarding from the worker %s: %v", vm.Worker, err)

		return responder.Code(http.StatusServiceUnavailable)
	}

	// worker will asynchronously start port-forwarding so we wait
	select {
	case fromWorkerConnection := <-boomerangConnCh:
		websocket.Handler(func(wsConn *websocket.Conn) {
			if err := proxy.Connections(wsConn, fromWorkerConnection); err != nil {
				controller.logger.Warnf("failed to port-forward: %v", err)
			}
		}).ServeHTTP(ctx.Writer, ctx.Request)

		return responder.Empty()
	case <-ctx.Done():
		return responder.Error(ctx.Err())
	}
}
