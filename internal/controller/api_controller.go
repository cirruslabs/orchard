package controller

import (
	"github.com/cirruslabs/orchard/internal/responder"
	"github.com/cirruslabs/orchard/internal/version"
	v1pkg "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (controller *Controller) controllerInfo(ctx *gin.Context) responder.Responder {
	// Only require the service account to be valid,
	// no roles are needed to query this endpoint
	if responder := controller.authorize(ctx); responder != nil {
		return responder
	}

	return responder.JSON(http.StatusOK, &v1pkg.ControllerInfo{
		Version: version.Version,
		Commit:  version.Commit,
	})
}
