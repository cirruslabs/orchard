package controller

import (
	"errors"
	"github.com/cirruslabs/orchard/internal/controller/lifecycle"
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/internal/responder"
	"github.com/cirruslabs/orchard/internal/simplename"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/samber/lo"
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
	} else if err := simplename.Validate(vm.Name); err != nil {
		return responder.JSON(http.StatusPreconditionFailed,
			NewErrorResponse("VM name %v", err))
	}
	if vm.Image == "" {
		return responder.JSON(http.StatusPreconditionFailed, NewErrorResponse("VM image is empty"))
	}

	vm.Status = v1.VMStatusPending
	vm.CreatedAt = time.Now()
	vm.RestartedAt = time.Time{}
	vm.RestartCount = 0
	vm.UID = uuid.New().String()

	// Provide resource defaults
	if vm.Resources == nil {
		vm.Resources = make(v1.Resources)
	}
	if _, ok := vm.Resources[v1.ResourceTartVMs]; !ok {
		vm.Resources[v1.ResourceTartVMs] = 1
	}

	// Validate image pull policy and provide a default value if it's missing
	if vm.ImagePullPolicy != "" {
		if _, err := v1.NewImagePullPolicyFromString(string(vm.ImagePullPolicy)); err != nil {
			return responder.JSON(http.StatusPreconditionFailed,
				NewErrorResponse("unsupported image pull policy: %q", vm.ImagePullPolicy))
		}
	} else {
		vm.ImagePullPolicy = v1.ImagePullPolicyIfNotPresent
	}

	// Validate restart policy and provide a default value if it's missing
	if vm.RestartPolicy != "" {
		if _, err := v1.NewRestartPolicyFromString(string(vm.RestartPolicy)); err != nil {
			return responder.JSON(http.StatusPreconditionFailed,
				NewErrorResponse("unsupported restart policy: %q", vm.RestartPolicy))
		}
	} else {
		vm.RestartPolicy = v1.RestartPolicyNever
	}

	// Validate hostDirs
	if responder := controller.validateHostDirs(vm.HostDirs); responder != nil {
		return responder
	}

	response := controller.storeUpdate(func(txn storepkg.Transaction) responder.Responder {
		// Does the VM resource with this name already exists?
		_, err := txn.GetVM(vm.Name)
		if err != nil && !errors.Is(err, storepkg.ErrNotFound) {
			controller.logger.Errorf("failed to check if the VM exists in the DB: %v", err)

			return responder.Code(http.StatusInternalServerError)
		}
		if err == nil {
			return responder.JSON(http.StatusConflict, NewErrorResponse("VM with this name already exists"))
		}

		if err := txn.SetVM(vm); err != nil {
			controller.logger.Errorf("failed to create VM in the DB: %v", err)

			return responder.Code(http.StatusInternalServerError)
		}

		return responder.JSON(http.StatusOK, &vm)
	})
	// request immediate scheduling
	controller.scheduler.RequestScheduling()
	return response
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

		if userVM.Status == v1.VMStatusRunning && dbVM.StartedAt.IsZero() {
			dbVM.StartedAt = time.Now()
		}

		dbVM.Status = userVM.Status
		dbVM.StatusMessage = userVM.StatusMessage
		dbVM.ImageFQN = userVM.ImageFQN

		if err := txn.SetVM(*dbVM); err != nil {
			controller.logger.Errorf("failed to update VM in the DB: %v", err)

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

		lifecycle.Report(vm, "VM deleted", controller.logger)

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

func (controller *Controller) validateHostDirs(hostDirs []v1.HostDir) responder.Responder {
	if len(hostDirs) == 0 {
		return nil
	}

	// Retrieve cluster settings
	var clusterSettings *v1.ClusterSettings
	var err error

	err = controller.store.View(func(txn storepkg.Transaction) error {
		clusterSettings, err = txn.GetClusterSettings()

		return err
	})
	if err != nil {
		controller.logger.Errorf("failed to retrieve cluster settings from the DB: %v", err)

		return responder.Code(http.StatusInternalServerError)
	}

	for _, hostDir := range hostDirs {
		if hostDir.Name == "" {
			return responder.JSON(http.StatusBadRequest,
				NewErrorResponse("hostDir volume's \"name\" field cannot be empty"))
		}

		if hostDir.Path == "" {
			return responder.JSON(http.StatusBadRequest,
				NewErrorResponse("hostDir volume's \"path\" field cannot be empty"))
		}

		if !lo.SomeBy(clusterSettings.HostDirPolicies, func(hostDirPolicy v1.HostDirPolicy) bool {
			return hostDirPolicy.Validate(hostDir.Path, hostDir.ReadOnly)
		}) {
			return responder.JSON(http.StatusBadRequest, NewErrorResponse("host directory %q is disallowed "+
				"by policy, check your cluster settings", hostDir.String()))
		}
	}

	return nil
}
