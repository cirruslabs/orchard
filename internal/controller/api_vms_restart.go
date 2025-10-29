package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/cirruslabs/orchard/internal/controller/lifecycle"
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/internal/responder"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/cirruslabs/orchard/rpc"
	"github.com/gin-gonic/gin"
)

func (controller *Controller) restartVM(ctx *gin.Context) responder.Responder {
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite); responder != nil {
		return responder
	}

	name := ctx.Param("name")

	var updatedVM v1.VM
	var workerName string
	var shouldNotify bool

	response := controller.storeUpdate(func(txn storepkg.Transaction) responder.Responder {
		vm, err := txn.GetVM(name)
		if err != nil {
			return responder.Error(err)
		}

		if vm.Worker == "" {
			return responder.JSON(http.StatusPreconditionFailed,
				NewErrorResponse("VM is not assigned to a worker, cannot restart"))
		}

		if !vm.RestartRequested {
			vm.RestartRequested = true
			vm.RestartedAt = time.Now()
			vm.RestartCount++
			shouldNotify = true
			workerName = vm.Worker

			lifecycle.Report(vm, "VM restart requested", controller.logger)
		}

		if err := txn.SetVM(*vm); err != nil {
			controller.logger.Errorf("failed to store VM restart request in the DB: %v", err)

			return responder.Code(http.StatusInternalServerError)
		}

		updatedVM = *vm

		return responder.JSON(http.StatusAccepted, &updatedVM)
	})

	if shouldNotify && workerName != "" {
		notifyCtx, notifyCtxCancel := context.WithTimeout(ctx, time.Second)
		defer notifyCtxCancel()

		if err := controller.workerNotifier.Notify(notifyCtx, workerName, &rpc.WatchInstruction{
			Action: &rpc.WatchInstruction_SyncVmsAction{
				SyncVmsAction: &rpc.WatchInstruction_SyncVMs{},
			},
		}); err != nil {
			controller.logger.Warnf("failed to notify worker %s about VM restart: %v",
				workerName, err)
		}
	}

	return response
}
