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
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite); responder != nil {
		return responder
	}

	var vm v1.VM

	if err := ctx.ShouldBindJSON(&vm); err != nil {
		return responder.JSON(http.StatusBadRequest, NewErrorResponse("invalid JSON was provided"))
	}

	if vm.Name == "" {
		return responder.JSON(http.StatusPreconditionFailed, NewErrorResponse("VM name is empty"))
	}
	if vm.Image == "" {
		return responder.JSON(http.StatusPreconditionFailed, NewErrorResponse("VM image is empty"))
	}
	if vm.CPU == 0 {
		return responder.JSON(http.StatusPreconditionFailed, NewErrorResponse("VM CPU is zero"))
	}
	if vm.Memory == 0 {
		return responder.JSON(http.StatusPreconditionFailed, NewErrorResponse("VM memory is zero"))
	}

	vm.Status = v1.VMStatusPending
	vm.CreatedAt = time.Now()
	vm.UID = uuid.New().String()

	// Provide resource defaults
	if vm.Resources == nil {
		vm.Resources = make(v1.Resources)
	}
	if _, ok := vm.Resources[v1.ResourceTartVMs]; !ok {
		vm.Resources[v1.ResourceTartVMs] = 1
	}

	return controller.storeUpdate(func(txn storepkg.Transaction) responder.Responder {
		// Does the VM resource with this name already exists?
		_, err := txn.GetVM(vm.Name)
		if err != nil && !errors.Is(err, storepkg.ErrNotFound) {
			return responder.Code(http.StatusInternalServerError)
		}
		if err == nil {
			return responder.JSON(http.StatusConflict, NewErrorResponse("VM with this name already exists"))
		}

		if err := txn.SetVM(vm); err != nil {
			return responder.Code(http.StatusInternalServerError)
		}

		return responder.JSON(http.StatusOK, &vm)
	})
}

func (controller *Controller) updateVM(ctx *gin.Context) responder.Responder {
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite); responder != nil {
		return responder
	}

	var userVM v1.VM

	if err := ctx.ShouldBindJSON(&userVM); err != nil {
		return responder.JSON(http.StatusBadRequest, NewErrorResponse("invalid JSON was provided"))
	}

	if userVM.Name == "" {
		return responder.JSON(http.StatusPreconditionFailed, NewErrorResponse("VM name is empty"))
	}

	return controller.storeUpdate(func(txn storepkg.Transaction) responder.Responder {
		dbVM, err := txn.GetVM(userVM.Name)
		if err != nil {
			return responder.Error(err)
		}

		if dbVM.TerminalState() && dbVM.Status != userVM.Status {
			return responder.JSON(http.StatusPreconditionFailed,
				NewErrorResponse("cannot update status for a VM in a terminal state"))
		}

		dbVM.Status = userVM.Status
		dbVM.StatusMessage = userVM.StatusMessage

		if err := txn.SetVM(*dbVM); err != nil {
			return responder.Code(http.StatusInternalServerError)
		}

		return responder.JSON(http.StatusOK, dbVM)
	})
}

func (controller *Controller) getVM(ctx *gin.Context) responder.Responder {
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeRead); responder != nil {
		return responder
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
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeRead); responder != nil {
		return responder
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
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite); responder != nil {
		return responder
	}

	name := ctx.Param("name")

	return controller.storeUpdate(func(txn storepkg.Transaction) responder.Responder {
		vm, err := txn.GetVM(name)
		if err != nil {
			return responder.Error(err)
		}
		err = txn.DeleteVM(name)
		if err != nil {
			return responder.Error(err)
		}
		err = txn.DeleteEvents("vms", vm.UID)
		if err != nil {
			return responder.Error(err)
		}

		return responder.Code(http.StatusOK)
	})
}
func (controller *Controller) appendVMEvents(ctx *gin.Context) responder.Responder {
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite); responder != nil {
		return responder
	}

	var events []v1.Event

	if err := ctx.ShouldBindJSON(&events); err != nil {
		return responder.JSON(http.StatusBadRequest, NewErrorResponse("invalid JSON was provided"))
	}

	name := ctx.Param("name")

	return controller.storeUpdate(func(txn storepkg.Transaction) responder.Responder {
		vm, err := txn.GetVM(name)
		if err != nil {
			return responder.Error(err)
		}
		if err := txn.AppendEvents(events, "vms", vm.UID); err != nil {
			return responder.Error(err)
		}

		return responder.Code(http.StatusOK)
	})
}

func (controller *Controller) listVMEvents(ctx *gin.Context) responder.Responder {
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeRead); responder != nil {
		return responder
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
