package controller

import (
	"github.com/cirruslabs/orchard/internal/controller/rendezvous"
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/internal/proxy"
	"github.com/cirruslabs/orchard/internal/responder"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/net/websocket"
	"net/http"
	"strconv"
)

func (controller *Controller) portForwardVM(ctx *gin.Context) responder.Responder {
	if !controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite) {
		return responder.Code(http.StatusUnauthorized)
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

	// Request and wait for a rendez-vous with a worker
	token := uuid.New().String()

	rendezvousConnCh, cancel := controller.proxy.Request(ctx, token)
	defer cancel()

	err = controller.watcher.Notify(ctx, vm.Worker, &rendezvous.TopicMessage{
		Token:  token,
		VMUID:  vm.UID,
		VMPort: uint16(port),
	})
	if err != nil {
		controller.logger.Warnf("failed to rendez-vous with the worker %s: %v", vm.Worker, err)

		return responder.Code(http.StatusServiceUnavailable)
	}

	rendezvousConn := <-rendezvousConnCh

	websocket.Handler(func(wsConn *websocket.Conn) {
		if err := proxy.Connections(wsConn, rendezvousConn); err != nil {
			controller.logger.Warnf("failed to port-forward: %v", err)
		}
	}).ServeHTTP(ctx.Writer, ctx.Request)

	controller.logger.Infof("port-forward done!")

	return responder.Empty()
}
