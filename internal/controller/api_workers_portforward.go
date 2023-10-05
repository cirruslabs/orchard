package controller

import (
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/internal/responder"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

func (controller *Controller) portForwardWorker(ctx *gin.Context) responder.Responder {
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleAdminWrite); responder != nil {
		return responder
	}

	// Retrieve and parse path and query parameters
	name := ctx.Param("name")

	portRaw := ctx.Query("port")
	port, err := strconv.ParseUint(portRaw, 10, 16)
	if err != nil {
		return responder.Code(http.StatusBadRequest)
	}
	if port < 1 || port > 65535 {
		return responder.Code(http.StatusBadRequest)
	}

	var worker *v1.Worker

	if responder := controller.storeView(func(txn storepkg.Transaction) responder.Responder {
		worker, err = txn.GetWorker(name)
		if err != nil {
			return responder.Error(err)
		}

		return nil
	}); responder != nil {
		return responder
	}

	// Commence port-forwarding
	return controller.portForward(ctx, worker.Name, "", uint32(port))
}
