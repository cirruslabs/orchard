package controller

import (
	"context"
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
	"time"
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

	waitRaw := ctx.Query("wait")
	wait, err := strconv.ParseUint(waitRaw, 10, 16)
	if err != nil {
		return responder.Code(http.StatusBadRequest)
	}
	waitContext, waitContextCancel := context.WithTimeout(ctx, time.Duration(wait)*time.Second)
	defer waitContextCancel()

	// Look-up the VM
	var vm *v1.VM

	for {
		if lookupResponder := controller.storeView(func(txn storepkg.Transaction) responder.Responder {
			vm, err = txn.GetVM(name)
			if err != nil {
				return responder.Error(err)
			}

			return nil
		}); lookupResponder != nil {
			return lookupResponder
		}

		if vm.TerminalState() {
			return responder.JSON(http.StatusExpectationFailed, NewErrorResponse("VM is in a terminal state '%s'", vm.Status))
		}
		if vm.Status == v1.VMStatusRunning {
			// VM is running, proceed
			break
		}
		select {
		case <-waitContext.Done():
			return responder.JSON(http.StatusRequestTimeout, NewErrorResponse("VM is not running on '%s' worker", vm.Worker))
		case <-time.After(1 * time.Second):
			// try again
			continue
		}
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
