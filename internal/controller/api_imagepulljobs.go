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

func (controller *Controller) createImagePullJob(ctx *gin.Context) responder.Responder {
	// Auth
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite); responder != nil {
		return responder
	}

	// Parse user input
	var userPullJob v1.ImagePullJob

	if err := ctx.ShouldBindJSON(&userPullJob); err != nil {
		return responder.JSON(http.StatusBadRequest, NewErrorResponse("invalid JSON was provided"))
	}

	// Validate user input
	if userPullJob.Name == "" {
		return responder.JSON(http.StatusPreconditionFailed, NewErrorResponse("name field cannot be empty"))
	} else if err := simplename.Validate(userPullJob.Name); err != nil {
		return responder.JSON(http.StatusPreconditionFailed,
			NewErrorResponse("name field %v", err))
	}
	if userPullJob.Image == "" {
		return responder.JSON(http.StatusPreconditionFailed, NewErrorResponse("image field cannot be empty"))
	}

	// Provide defaults
	userPullJob.CreatedAt = time.Now()
	userPullJob.UID = uuid.NewString()

	response := controller.storeUpdate(func(txn storepkg.Transaction) responder.Responder {
		// Does this resource already exists?
		_, err := txn.GetImagePullJob(userPullJob.Name)
		if err != nil && !errors.Is(err, storepkg.ErrNotFound) {
			controller.logger.Errorf("failed to check if the image pull job exists in the DB: %v", err)

			return responder.Code(http.StatusInternalServerError)
		}
		if err == nil {
			return responder.JSON(http.StatusConflict, NewErrorResponse("image pull job with this name "+
				"already exists"))
		}

		if err := txn.SetImagePullJob(userPullJob); err != nil {
			controller.logger.Errorf("failed to create image pull job in the DB: %v", err)

			return responder.Code(http.StatusInternalServerError)
		}

		return responder.JSON(http.StatusOK, &userPullJob)
	})

	return response
}

func (controller *Controller) getImagePullJob(ctx *gin.Context) responder.Responder {
	// Auth
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeRead); responder != nil {
		return responder
	}

	name := ctx.Param("name")

	return controller.storeView(func(txn storepkg.Transaction) responder.Responder {
		dbPullJob, err := txn.GetImagePullJob(name)
		if err != nil {
			return responder.Error(err)
		}

		return responder.JSON(http.StatusOK, dbPullJob)
	})
}

func (controller *Controller) listImagePullJobs(ctx *gin.Context) responder.Responder {
	// Auth
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeRead); responder != nil {
		return responder
	}

	return controller.storeView(func(txn storepkg.Transaction) responder.Responder {
		dbPullJobs, err := txn.ListImagePullJobs()
		if err != nil {
			return responder.Error(err)
		}

		return responder.JSON(http.StatusOK, dbPullJobs)
	})
}

func (controller *Controller) deleteImagePullJob(ctx *gin.Context) responder.Responder {
	// Auth
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite); responder != nil {
		return responder
	}

	name := ctx.Param("name")

	return controller.storeUpdate(func(txn storepkg.Transaction) responder.Responder {
		_, err := txn.GetImagePullJob(name)
		if err != nil {
			return responder.Error(err)
		}

		err = txn.DeleteImagePullJob(name)
		if err != nil {
			return responder.Error(err)
		}

		return responder.Code(http.StatusOK)
	})
}
