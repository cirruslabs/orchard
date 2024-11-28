package controller

import (
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/internal/responder"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (controller *Controller) getClusterSettings(ctx *gin.Context) responder.Responder {
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleAdminRead); responder != nil {
		return responder
	}

	return controller.storeView(func(txn storepkg.Transaction) responder.Responder {
		clusterSettings, err := txn.GetClusterSettings()
		if err != nil {
			controller.logger.Errorf("failed to retrieve cluster settings from the DB: %v", err)

			return responder.Code(http.StatusInternalServerError)
		}

		return responder.JSON(http.StatusOK, clusterSettings)
	})
}

func (controller *Controller) updateClusterSettings(ctx *gin.Context) responder.Responder {
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleAdminWrite); responder != nil {
		return responder
	}

	var clusterSettings v1.ClusterSettings

	if err := ctx.ShouldBindJSON(&clusterSettings); err != nil {
		return responder.JSON(http.StatusBadRequest, NewErrorResponse("invalid JSON was provided"))
	}

	// Validate
	for _, allowedHostDir := range clusterSettings.HostDirPolicies {
		if allowedHostDir.PathPrefix == "" {
			return responder.JSON(http.StatusBadRequest,
				NewErrorResponse("pathPrefix field cannot be empty"))
		}
	}

	if clusterSettings.SchedulerProfile == "" {
		clusterSettings.SchedulerProfile = v1.SchedulerProfileOptimizeUtilization
	} else {
		if _, err := v1.NewSchedulerProfile(string(clusterSettings.SchedulerProfile)); err != nil {
			return responder.JSON(http.StatusBadRequest, NewErrorResponse("%v", err))
		}
	}

	return controller.storeUpdate(func(txn storepkg.Transaction) responder.Responder {
		if err := txn.SetClusterSettings(clusterSettings); err != nil {
			controller.logger.Errorf("failed to set cluster settings in the DB: %v", err)

			return responder.Code(http.StatusInternalServerError)
		}

		return responder.JSON(http.StatusOK, &clusterSettings)
	})
}
