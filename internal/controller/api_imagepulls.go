package controller

import (
	"errors"
	"net/http"
	"time"

	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/internal/responder"
	"github.com/cirruslabs/orchard/internal/simplename"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (controller *Controller) createImagePull(ctx *gin.Context) responder.Responder {
	// Auth
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite); responder != nil {
		return responder
	}

	// Parse user input
	var userPull v1.ImagePull

	if err := ctx.ShouldBindJSON(&userPull); err != nil {
		return responder.JSON(http.StatusBadRequest, NewErrorResponse("invalid JSON was provided"))
	}

	// Validate user input
	if userPull.Name == "" {
		return responder.JSON(http.StatusPreconditionFailed, NewErrorResponse("name field cannot is empty"))
	} else if err := simplename.Validate(userPull.Name); err != nil {
		return responder.JSON(http.StatusPreconditionFailed,
			NewErrorResponse("name field %v", err))
	}
	if userPull.Image == "" {
		return responder.JSON(http.StatusPreconditionFailed, NewErrorResponse("image field cannot be empty"))
	}
	if userPull.Worker == "" {
		return responder.JSON(http.StatusPreconditionFailed, NewErrorResponse("worker field cannot be empty"))
	}

	// Provide defaults
	userPull.CreatedAt = time.Now()
	userPull.UID = uuid.NewString()

	response := controller.storeUpdate(func(txn storepkg.Transaction) responder.Responder {
		// Does this resource already exists?
		_, err := txn.GetImagePull(userPull.Name)
		if err != nil && !errors.Is(err, storepkg.ErrNotFound) {
			controller.logger.Errorf("failed to check if the image pull exists in the DB: %v", err)

			return responder.Code(http.StatusInternalServerError)
		}
		if err == nil {
			return responder.JSON(http.StatusConflict, NewErrorResponse("image pull with this name already exists"))
		}

		if err := txn.SetImagePull(userPull); err != nil {
			controller.logger.Errorf("failed to create image pull in the DB: %v", err)

			return responder.Code(http.StatusInternalServerError)
		}

		return responder.JSON(http.StatusOK, &userPull)
	})

	return response
}

func (controller *Controller) updateImagePullState(ctx *gin.Context) responder.Responder {
	// Auth
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite); responder != nil {
		return responder
	}

	// Parse user input
	var userPull v1.ImagePull

	if err := ctx.ShouldBindJSON(&userPull); err != nil {
		return responder.JSON(http.StatusBadRequest, NewErrorResponse("invalid JSON was provided"))
	}

	name := ctx.Param("name")

	return controller.storeUpdate(func(txn storepkg.Transaction) responder.Responder {
		dbPull, err := txn.GetImagePull(name)
		if err != nil {
			return responder.Error(err)
		}

		dbPull.PullState = userPull.PullState

		if err := txn.SetImagePull(*dbPull); err != nil {
			controller.logger.Errorf("failed to update image pull in the DB: %v", err)

			return responder.Code(http.StatusInternalServerError)
		}

		return responder.JSON(http.StatusOK, dbPull)
	})
}

func (controller *Controller) getImagePull(ctx *gin.Context) responder.Responder {
	// Auth
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeRead); responder != nil {
		return responder
	}

	name := ctx.Param("name")

	return controller.storeView(func(txn storepkg.Transaction) responder.Responder {
		dbPull, err := txn.GetImagePull(name)
		if err != nil {
			return responder.Error(err)
		}

		return responder.JSON(http.StatusOK, dbPull)
	})
}

func (controller *Controller) listImagePulls(ctx *gin.Context) responder.Responder {
	// Auth
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeRead); responder != nil {
		return responder
	}

	return controller.storeView(func(txn storepkg.Transaction) responder.Responder {
		dbPulls, err := txn.ListImagePulls()
		if err != nil {
			return responder.Error(err)
		}

		return responder.JSON(http.StatusOK, dbPulls)
	})
}

func (controller *Controller) deleteImagePull(ctx *gin.Context) responder.Responder {
	// Auth
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite); responder != nil {
		return responder
	}

	name := ctx.Param("name")

	return controller.storeUpdate(func(txn storepkg.Transaction) responder.Responder {
		_, err := txn.GetImagePull(name)
		if err != nil {
			return responder.Error(err)
		}

		err = txn.DeleteImagePull(name)
		if err != nil {
			return responder.Error(err)
		}

		return responder.Code(http.StatusOK)
	})
}
