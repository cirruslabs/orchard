package controller

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/cirruslabs/orchard/internal/responder"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/cirruslabs/orchard/rpc"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (controller *Controller) ip(ctx *gin.Context) responder.Responder {
	if responder := controller.authorizeAny(ctx, v1.ServiceAccountRoleComputeWrite,
		v1.ServiceAccountRoleComputeConnect); responder != nil {
		return responder
	}

	// Retrieve and parse path and query parameters
	name := ctx.Param("name")

	waitRaw := ctx.DefaultQuery("wait", "0")
	wait, err := strconv.ParseUint(waitRaw, 10, 16)
	if err != nil {
		return responder.Code(http.StatusBadRequest)
	}
	waitContext, waitContextCancel := context.WithTimeout(ctx, time.Duration(wait)*time.Second)
	defer waitContextCancel()

	// Look-up the VM
	vm, responderImpl := controller.waitForVM(waitContext, name)
	if responderImpl != nil {
		return responderImpl
	}

	// Send an IP resolution request and wait for the result
	session := uuid.New().String()
	boomerangConnCh, cancel := controller.ipRendezvous.Request(ctx, session)
	defer cancel()

	err = controller.workerNotifier.Notify(waitContext, vm.Worker, &rpc.WatchInstruction{
		Action: &rpc.WatchInstruction_ResolveIpAction{
			ResolveIpAction: &rpc.WatchInstruction_ResolveIP{
				Session: session,
				VmUid:   vm.UID,
			},
		},
	})
	if err != nil {
		controller.logger.Warnf("failed to request VM's IP from the worker %s: %v",
			vm.Worker, err)

		return responder.Code(http.StatusServiceUnavailable)
	}

	select {
	case rendezvousResponse := <-boomerangConnCh:
		if rendezvousResponse.ErrorMessage != "" {
			return responder.Error(fmt.Errorf("VM's IP resolution on the worker %s failed: %s",
				vm.Worker, rendezvousResponse.ErrorMessage))
		}

		result := struct {
			IP string `json:"ip"`
		}{
			IP: rendezvousResponse.Result,
		}

		return responder.JSON(http.StatusOK, &result)
	case <-ctx.Done():
		return responder.Error(ctx.Err())
	}
}
