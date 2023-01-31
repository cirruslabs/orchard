package controller

import (
	"errors"
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/internal/responder"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http"
	"time"
)

func (controller *Controller) createWorker(ctx *gin.Context) responder.Responder {
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
	worker.DeletedAt = time.Time{}
	worker.UID = uuid.New().String()
	worker.Generation = 0

	return controller.storeUpdate(func(txn *storepkg.Txn) responder.Responder {
		// Does the worker resource with this name already exists?
		_, err := txn.GetWorker(worker.Name)
		if !errors.Is(err, storepkg.ErrNotFound) {
			return responder.Code(http.StatusConflict)
		}

		if err := txn.SetWorker(&worker); err != nil {
			return responder.Code(http.StatusInternalServerError)
		}

		return responder.JSON(200, &worker)
	})
}

func (controller *Controller) updateWorker(ctx *gin.Context) responder.Responder {
	var userWorker v1.Worker

	if err := ctx.ShouldBindJSON(&userWorker); err != nil {
		return responder.Code(http.StatusBadRequest)
	}

	return controller.storeUpdate(func(txn *storepkg.Txn) responder.Responder {
		dbWorker, err := txn.GetWorker(userWorker.Name)
		if err != nil {
			if errors.Is(err, storepkg.ErrNotFound) {
				return responder.Code(http.StatusNotFound)
			}

			return responder.Code(http.StatusInternalServerError)
		}

		dbWorker.LastSeen = userWorker.LastSeen
		dbWorker.Generation++

		if err := txn.SetWorker(dbWorker); err != nil {
			return responder.Code(http.StatusInternalServerError)
		}

		return responder.JSON(200, &dbWorker)
	})
}

func (controller *Controller) getWorker(ctx *gin.Context) responder.Responder {
	name := ctx.Param("name")

	return controller.storeView(func(txn *storepkg.Txn) responder.Responder {
		worker, err := txn.GetWorker(name)
		if err != nil {
			if errors.Is(err, storepkg.ErrNotFound) {
				return responder.Code(http.StatusNotFound)
			}

			return responder.Code(http.StatusInternalServerError)
		}

		return responder.JSON(http.StatusOK, &worker)
	})
}

func (controller *Controller) listWorkers(_ *gin.Context) responder.Responder {
	return controller.storeView(func(txn *storepkg.Txn) responder.Responder {
		workers, err := txn.ListWorkers()
		if err != nil {
			if errors.Is(err, storepkg.ErrNotFound) {
				return responder.Code(http.StatusNotFound)
			}

			return responder.Code(http.StatusInternalServerError)
		}

		return responder.JSON(http.StatusOK, &workers)
	})
}

func (controller *Controller) deleteWorker(ctx *gin.Context) responder.Responder {
	name := ctx.Param("name")

	return controller.storeUpdate(func(txn *storepkg.Txn) responder.Responder {
		if err := txn.DeleteWorker(name); err != nil {
			if errors.Is(err, storepkg.ErrNotFound) {
				return responder.Code(http.StatusNotFound)
			}

			return responder.Code(http.StatusInternalServerError)
		}

		return responder.Code(http.StatusOK)
	})
}
