package controller

import (
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/internal/responder"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (controller *Controller) initAPI() *gin.Engine {
	gin.SetMode(gin.DebugMode)
	ginEngine := gin.Default()

	// v1 API
	v1 := ginEngine.Group("/v1")

	// Workers
	v1.POST("/workers", func(c *gin.Context) {
		controller.createWorker(c).Respond(c)
	})
	v1.PUT("/workers/:name", func(c *gin.Context) {
		controller.updateWorker(c).Respond(c)
	})
	v1.GET("/workers/:name", func(c *gin.Context) {
		controller.getWorker(c).Respond(c)
	})
	v1.GET("/workers", func(c *gin.Context) {
		controller.listWorkers(c).Respond(c)
	})
	v1.DELETE("/workers/:name", func(c *gin.Context) {
		controller.deleteWorker(c).Respond(c)
	})

	// VMs
	v1.POST("/vms", func(c *gin.Context) {
		controller.createVM(c).Respond(c)
	})
	v1.PUT("/vms/:name", func(c *gin.Context) {
		controller.updateVM(c).Respond(c)
	})
	v1.GET("/vms/:name", func(c *gin.Context) {
		controller.getVM(c).Respond(c)
	})
	v1.GET("/vms", func(c *gin.Context) {
		controller.listVMs(c).Respond(c)
	})
	v1.DELETE("/vms/:name", func(c *gin.Context) {
		controller.deleteVM(c).Respond(c)
	})

	return ginEngine
}

type storeTransactionFunc func(operation func(txn storepkg.Transaction) error) error

func (controller *Controller) storeView(view func(txn storepkg.Transaction) responder.Responder) responder.Responder {
	return adaptResponderToStoreOperation(controller.store.View, view)
}

func (controller *Controller) storeUpdate(
	update func(txn storepkg.Transaction) responder.Responder,
) responder.Responder {
	return adaptResponderToStoreOperation(controller.store.Update, update)
}

func adaptResponderToStoreOperation(
	storeOperation storeTransactionFunc,
	responderOperation func(txn storepkg.Transaction) responder.Responder,
) responder.Responder {
	var result responder.Responder

	if err := storeOperation(func(txn storepkg.Transaction) error {
		result = responderOperation(txn)

		return nil
	}); err != nil {
		return responder.Code(http.StatusInternalServerError)
	}

	return result
}
