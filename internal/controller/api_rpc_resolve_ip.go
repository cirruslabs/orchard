package controller

import (
	"github.com/cirruslabs/orchard/internal/controller/rendezvous"
	"github.com/cirruslabs/orchard/internal/responder"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (controller *Controller) rpcResolveIP(ctx *gin.Context) responder.Responder {
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite); responder != nil {
		return responder
	}

	// Retrieve and parse path and query parameters
	session := ctx.Query("session")
	ip := ctx.Query("ip")
	errorMessage := ctx.Query("errorMessage")

	// Respond with the resolved IP address
	_, err := controller.ipRendezvous.Respond(session, rendezvous.ResultWithErrorMessage[string]{
		Result:       ip,
		ErrorMessage: errorMessage,
	})
	if err != nil {
		return responder.Error(err)
	}

	return responder.Code(http.StatusOK)
}
