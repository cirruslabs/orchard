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
	if !controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite, v1.ServiceAccountRoleWorker) {
		return responder.Code(http.StatusUnauthorized)
	}

	var worker v1.Worker

	if err := ctx.ShouldBindJSON(&worker); err != nil {
		return responder.Code(http.StatusBadRequest)
	}

	if worker.Name == "" {
		return responder.Code(http.StatusPreconditionFailed)
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
			return responder.Code(http.StatusConflict)
		}

		if err := txn.SetWorker(worker); err != nil {
			return responder.Error(err)
		}

		return responder.JSON(200, worker)
	})
}

func (controller *Controller) updateWorker(ctx *gin.Context) responder.Responder {
	if !controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite, v1.ServiceAccountRoleWorker) {
		return responder.Code(http.StatusUnauthorized)
	}

	var userWorker v1.Worker

	if err := ctx.ShouldBindJSON(&userWorker); err != nil {
		return responder.Code(http.StatusBadRequest)
	}

	return controller.storeUpdate(func(txn storepkg.Transaction) responder.Responder {
		dbWorker, err := txn.GetWorker(userWorker.Name)
		if err != nil {
			return responder.Error(err)
		}

		dbWorker.LastSeen = userWorker.LastSeen

		if err := txn.SetWorker(*dbWorker); err != nil {
			return responder.Code(http.StatusInternalServerError)
		}

		return responder.JSON(200, &dbWorker)
	})
}

func (controller *Controller) getWorker(ctx *gin.Context) responder.Responder {
	if !controller.authorize(ctx, v1.ServiceAccountRoleComputeRead, v1.ServiceAccountRoleWorker) {
		return responder.Code(http.StatusUnauthorized)
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
	if !controller.authorize(ctx, v1.ServiceAccountRoleComputeRead, v1.ServiceAccountRoleWorker) {
		return responder.Code(http.StatusUnauthorized)
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
	if !controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite, v1.ServiceAccountRoleWorker) {
		return responder.Code(http.StatusUnauthorized)
	}

	name := ctx.Param("name")

	return controller.storeUpdate(func(txn storepkg.Transaction) responder.Responder {
		if err := txn.DeleteWorker(name); err != nil {
			return responder.Error(err)
		}

		return responder.Code(http.StatusOK)
	})
}
