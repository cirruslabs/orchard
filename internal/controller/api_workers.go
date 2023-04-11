package controller

import (
	"errors"
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/internal/responder"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

func (controller *Controller) createWorker(ctx *gin.Context) responder.Responder {
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite); responder != nil {
		return responder
	}

	var worker v1.Worker

	if err := ctx.ShouldBindJSON(&worker); err != nil {
		return responder.JSON(http.StatusBadRequest, NewErrorResponse("invalid JSON was provided"))
	}

	if worker.Name == "" {
		return responder.JSON(http.StatusPreconditionFailed, NewErrorResponse("worker name is empty"))
	}

	currentTime := time.Now()
	if worker.LastSeen.IsZero() {
		worker.LastSeen = currentTime
	}
	worker.CreatedAt = currentTime

	return controller.storeUpdate(func(txn storepkg.Transaction) responder.Responder {
		// In case there already exist a worker with the same name,
		// allow overwriting it if the request comes from a worker
		// with the same machine ID
		dbWorker, err := txn.GetWorker(worker.Name)
		if err != nil && !errors.Is(err, storepkg.ErrNotFound) {
			return responder.Code(http.StatusInternalServerError)
		}
		if err == nil && worker.MachineID != dbWorker.MachineID {
			return responder.JSON(http.StatusConflict,
				NewErrorResponse("this worker is managed from a different machine ID, "+
					"delete this worker first to be able to re-create it"))
		}

		if err := txn.SetWorker(worker); err != nil {
			return responder.Error(err)
		}

		return responder.JSON(200, worker)
	})
}

func (controller *Controller) updateWorker(ctx *gin.Context) responder.Responder {
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite); responder != nil {
		return responder
	}

	var userWorker v1.Worker

	if err := ctx.ShouldBindJSON(&userWorker); err != nil {
		return responder.JSON(http.StatusBadRequest, NewErrorResponse("invalid JSON was provided"))
	}

	return controller.storeUpdate(func(txn storepkg.Transaction) responder.Responder {
		dbWorker, err := txn.GetWorker(userWorker.Name)
		if err != nil {
			return responder.Error(err)
		}

		dbWorker.LastSeen = userWorker.LastSeen
		dbWorker.SchedulingPaused = userWorker.SchedulingPaused

		if err := txn.SetWorker(*dbWorker); err != nil {
			return responder.Code(http.StatusInternalServerError)
		}

		return responder.JSON(200, &dbWorker)
	})
}

func (controller *Controller) getWorker(ctx *gin.Context) responder.Responder {
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeRead); responder != nil {
		return responder
	}

	name := ctx.Param("name")

	return controller.storeView(func(txn storepkg.Transaction) responder.Responder {
		worker, err := txn.GetWorker(name)
		if err != nil {
			return responder.Error(err)
		}

		return responder.JSON(http.StatusOK, &worker)
	})
}

func (controller *Controller) listWorkers(ctx *gin.Context) responder.Responder {
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeRead); responder != nil {
		return responder
	}

	return controller.storeView(func(txn storepkg.Transaction) responder.Responder {
		workers, err := txn.ListWorkers()
		if err != nil {
			return responder.Error(err)
		}

		return responder.JSON(http.StatusOK, &workers)
	})
}

func (controller *Controller) deleteWorker(ctx *gin.Context) responder.Responder {
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite); responder != nil {
		return responder
	}

	name := ctx.Param("name")

	return controller.storeUpdate(func(txn storepkg.Transaction) responder.Responder {
		if err := txn.DeleteWorker(name); err != nil {
			return responder.Error(err)
		}

		return responder.Code(http.StatusOK)
	})
}
