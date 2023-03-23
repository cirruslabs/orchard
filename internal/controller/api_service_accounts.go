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

func (controller *Controller) createServiceAccount(ctx *gin.Context) responder.Responder {
	if !controller.authorize(ctx, v1.ServiceAccountRoleAdminWrite) {
		return responder.Code(http.StatusUnauthorized)
	}

	var serviceAccount v1.ServiceAccount

	if err := ctx.ShouldBindJSON(&serviceAccount); err != nil {
		return responder.Code(http.StatusBadRequest)
	}

	if serviceAccount.Name == "" {
		return responder.Code(http.StatusPreconditionFailed)
	}

	if serviceAccount.Token == "" {
		serviceAccount.Token = uuid.New().String()
	}

	serviceAccount.CreatedAt = time.Now()
	serviceAccount.Generation = 0

	return controller.storeUpdate(func(txn storepkg.Transaction) responder.Responder {
		// Does the Service Account resource with this name already exists?
		_, err := txn.GetServiceAccount(serviceAccount.Name)
		if !errors.Is(err, storepkg.ErrNotFound) {
			return responder.Code(http.StatusConflict)
		}

		if err := txn.SetServiceAccount(&serviceAccount); err != nil {
			return responder.Code(http.StatusInternalServerError)
		}

		return responder.JSON(http.StatusOK, &serviceAccount)
	})
}

func (controller *Controller) updateServiceAccount(ctx *gin.Context) responder.Responder {
	if !controller.authorize(ctx, v1.ServiceAccountRoleAdminWrite) {
		return responder.Code(http.StatusUnauthorized)
	}

	var userServiceAccount v1.ServiceAccount

	if err := ctx.ShouldBindJSON(&userServiceAccount); err != nil {
		return responder.Code(http.StatusBadRequest)
	}

	if userServiceAccount.Name == "" {
		return responder.Code(http.StatusPreconditionFailed)
	}

	if userServiceAccount.Token == "" {
		return responder.Code(http.StatusPreconditionFailed)
	}

	return controller.storeUpdate(func(txn storepkg.Transaction) responder.Responder {
		dbServiceAccount, err := txn.GetServiceAccount(userServiceAccount.Name)
		if err != nil {
			return responder.Error(err)
		}

		dbServiceAccount.Generation++

		if err := txn.SetServiceAccount(dbServiceAccount); err != nil {
			return responder.Code(http.StatusInternalServerError)
		}

		return responder.JSON(http.StatusOK, &dbServiceAccount)
	})
}

func (controller *Controller) getServiceAccount(ctx *gin.Context) responder.Responder {
	if !controller.authorize(ctx, v1.ServiceAccountRoleAdminRead) {
		return responder.Code(http.StatusUnauthorized)
	}

	name := ctx.Param("name")

	return controller.storeView(func(txn storepkg.Transaction) responder.Responder {
		serviceAccount, err := txn.GetServiceAccount(name)
		if err != nil {
			return responder.Error(err)
		}

		return responder.JSON(http.StatusOK, &serviceAccount)
	})
}

func (controller *Controller) listServiceAccounts(ctx *gin.Context) responder.Responder {
	if !controller.authorize(ctx, v1.ServiceAccountRoleAdminRead) {
		return responder.Code(http.StatusUnauthorized)
	}

	return controller.storeView(func(txn storepkg.Transaction) responder.Responder {
		serviceAccounts, err := txn.ListServiceAccounts()
		if err != nil {
			return responder.Error(err)
		}

		return responder.JSON(http.StatusOK, &serviceAccounts)
	})
}

func (controller *Controller) deleteServiceAccount(ctx *gin.Context) responder.Responder {
	if !controller.authorize(ctx, v1.ServiceAccountRoleAdminWrite) {
		return responder.Code(http.StatusUnauthorized)
	}

	name := ctx.Param("name")

	return controller.storeUpdate(func(txn storepkg.Transaction) responder.Responder {
		if err := txn.DeleteServiceAccount(name); err != nil {
			return responder.Error(err)
		}

		return responder.Code(http.StatusOK)
	})
}
