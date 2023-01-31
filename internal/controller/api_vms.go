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
	var vm v1.VM

	if err := ctx.ShouldBindJSON(&vm); err != nil {
		return responder.Code(http.StatusBadRequest)
	}

	if vm.Name == "" || vm.Image == "" || vm.CPU == 0 || vm.Memory == 0 {
		return responder.Code(http.StatusPreconditionFailed)
	}

	vm.Status = v1.VMStatusPending
	vm.CreatedAt = time.Now()
	vm.DeletedAt = time.Time{}
	vm.UID = uuid.New().String()
	vm.Generation = 0

	return controller.storeUpdate(func(txn *storepkg.Txn) responder.Responder {
		// Does the VM resource with this name already exists?
		_, err := txn.GetVM(vm.Name)
		if !errors.Is(err, storepkg.ErrNotFound) {
			return responder.Code(http.StatusConflict)
		}

		if err := txn.SetVM(&vm); err != nil {
			return responder.Code(http.StatusInternalServerError)
		}

		return responder.JSON(http.StatusOK, &vm)
	})
}

func (controller *Controller) updateVM(ctx *gin.Context) responder.Responder {
	var userVM v1.VM

	if err := ctx.ShouldBindJSON(&userVM); err != nil {
		return responder.Code(http.StatusBadRequest)
	}

	if userVM.Name == "" {
		return responder.Code(http.StatusPreconditionFailed)
	}

	return controller.storeUpdate(func(txn *storepkg.Txn) responder.Responder {
		dbVM, err := txn.GetVM(userVM.Name)
		if err != nil {
			if errors.Is(err, storepkg.ErrNotFound) {
				return responder.Code(http.StatusNotFound)
			}

			return responder.Code(http.StatusInternalServerError)
		}

		dbVM.Status = userVM.Status
		dbVM.Generation++

		if err := txn.SetVM(dbVM); err != nil {
			return responder.Code(http.StatusInternalServerError)
		}

		return responder.JSON(http.StatusOK, &dbVM)
	})
}

func (controller *Controller) getVM(ctx *gin.Context) responder.Responder {
	name := ctx.Param("name")

	return controller.storeView(func(txn *storepkg.Txn) responder.Responder {
		vm, err := txn.GetVM(name)
		if err != nil {
			if errors.Is(err, storepkg.ErrNotFound) {
				return responder.Code(http.StatusNotFound)
			}

			return responder.Code(http.StatusInternalServerError)
		}

		return responder.JSON(http.StatusOK, &vm)
	})
}

func (controller *Controller) listVMs(_ *gin.Context) responder.Responder {
	return controller.storeView(func(txn *storepkg.Txn) responder.Responder {
		vms, err := txn.ListVMs()
		if err != nil {
			if errors.Is(err, storepkg.ErrNotFound) {
				return responder.Code(http.StatusNotFound)
			}

			return responder.Code(http.StatusInternalServerError)
		}

		return responder.JSON(http.StatusOK, &vms)
	})
}

func (controller *Controller) deleteVM(ctx *gin.Context) responder.Responder {
	name := ctx.Param("name")

	if ctx.Query("force") != "" {
		return controller.storeUpdate(func(txn *storepkg.Txn) responder.Responder {
			if err := txn.DeleteVM(name); err != nil {
				if errors.Is(err, storepkg.ErrNotFound) {
					return responder.Code(http.StatusNotFound)
				}

				return responder.Code(http.StatusInternalServerError)
			}

			return responder.Code(http.StatusOK)
		})
	}

	return controller.storeUpdate(func(txn *storepkg.Txn) responder.Responder {
		vm, err := txn.GetVM(name)
		if err != nil {
			if errors.Is(err, storepkg.ErrNotFound) {
				return responder.Code(http.StatusNotFound)
			}

			return responder.Code(http.StatusInternalServerError)
		}

		vm.DeletedAt = time.Now()

		if err := txn.SetVM(vm); err != nil {
			return responder.Code(http.StatusInternalServerError)
		}

		return responder.Code(http.StatusOK)
	})
}
