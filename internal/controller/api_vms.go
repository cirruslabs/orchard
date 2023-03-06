package controller

import (
	"errors"
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/internal/responder"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http"
	"time"
)

func (controller *Controller) createVM(ctx *gin.Context) responder.Responder {
	if !controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite) {
		return responder.Code(http.StatusUnauthorized)
	}

	var vm v1.VM

	if err := ctx.ShouldBindJSON(&vm); err != nil {
		return responder.Code(http.StatusBadRequest)
	}

	if vm.Name == "" || vm.Image == "" || vm.CPU == 0 || vm.Memory == 0 {
		return responder.Code(http.StatusPreconditionFailed)
	}

	vm.Status = v1.VMStatusPending
	vm.CreatedAt = time.Now()
	vm.UID = uuid.New().String()
	vm.Generation = 0

	return controller.storeUpdate(func(txn storepkg.Transaction) responder.Responder {
		// Does the VM resource with this name already exists?
		_, err := txn.GetVM(vm.Name)
		if !errors.Is(err, storepkg.ErrNotFound) {
			return responder.Code(http.StatusConflict)
		}

		if err := txn.SetVM(vm); err != nil {
			return responder.Code(http.StatusInternalServerError)
		}

		return responder.JSON(http.StatusOK, &vm)
	})
}

func (controller *Controller) updateVM(ctx *gin.Context) responder.Responder {
	if !controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite) {
		return responder.Code(http.StatusUnauthorized)
	}

	var userVM v1.VM

	if err := ctx.ShouldBindJSON(&userVM); err != nil {
		return responder.Code(http.StatusBadRequest)
	}

	if userVM.Name == "" {
		return responder.Code(http.StatusPreconditionFailed)
	}

	return controller.storeUpdate(func(txn storepkg.Transaction) responder.Responder {
		dbVM, err := txn.GetVM(userVM.Name)
		if err != nil {
			return responder.Error(err)
		}

		if dbVM.TerminalState() && dbVM.Status != userVM.Status {
			return responder.Code(http.StatusPreconditionFailed)
		}

		dbVM.Status = userVM.Status
		dbVM.Generation++

		if err := txn.SetVM(*dbVM); err != nil {
			return responder.Code(http.StatusInternalServerError)
		}

		return responder.JSON(http.StatusOK, dbVM)
	})
}

func (controller *Controller) getVM(ctx *gin.Context) responder.Responder {
	if !controller.authorize(ctx, v1.ServiceAccountRoleComputeRead) {
		return responder.Code(http.StatusUnauthorized)
	}

	name := ctx.Param("name")

	return controller.storeView(func(txn storepkg.Transaction) responder.Responder {
		vm, err := txn.GetVM(name)
		if err != nil {
			return responder.Error(err)
		}

		return responder.JSON(http.StatusOK, vm)
	})
}

func (controller *Controller) listVMs(ctx *gin.Context) responder.Responder {
	if !controller.authorize(ctx, v1.ServiceAccountRoleComputeRead) {
		return responder.Code(http.StatusUnauthorized)
	}

	return controller.storeView(func(txn storepkg.Transaction) responder.Responder {
		vms, err := txn.ListVMs()
		if err != nil {
			return responder.Error(err)
		}

		return responder.JSON(http.StatusOK, vms)
	})
}

func (controller *Controller) deleteVM(ctx *gin.Context) responder.Responder {
	if !controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite) {
		return responder.Code(http.StatusUnauthorized)
	}

	name := ctx.Param("name")

	return controller.storeUpdate(func(txn storepkg.Transaction) responder.Responder {
		if err := txn.DeleteVM(name); err != nil {
			return responder.Error(err)
		}

		return responder.Code(http.StatusOK)
	})
}
func (controller *Controller) appendVMEvents(ctx *gin.Context) responder.Responder {
	if !controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite) {
		return responder.Code(http.StatusUnauthorized)
	}

	var events []v1.Event

	if err := ctx.ShouldBindJSON(&events); err != nil {
		return responder.Code(http.StatusBadRequest)
	}

	name := ctx.Param("name")

	return controller.storeUpdate(func(txn storepkg.Transaction) responder.Responder {
		vm, err := txn.GetVM(name)
		if err != nil {
			return responder.Error(err)
		}
		for _, event := range events {
			if err := txn.AppendEvent(event, "vms", vm.UID); err != nil {
				return responder.Error(err)
			}
		}

		return responder.Code(http.StatusOK)
	})
}

func (controller *Controller) listVMEvents(ctx *gin.Context) responder.Responder {
	if !controller.authorize(ctx, v1.ServiceAccountRoleComputeRead) {
		return responder.Code(http.StatusUnauthorized)
	}

	name := ctx.Param("name")

	return controller.storeView(func(txn storepkg.Transaction) responder.Responder {
		vm, err := txn.GetVM(name)
		if err != nil {
			return responder.Error(err)
		}
		events, err := txn.ListEvents("vms", vm.UID)
		if err != nil {
			return responder.Error(err)
		}

		return responder.JSON(http.StatusOK, events)
	})
}
