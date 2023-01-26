package controller

import (
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/internal/responder"
	"github.com/gin-gonic/gin"
	"net/http"
)

type storeTxFunc func(cb func(txn *storepkg.Txn) error) error
type apiTxFunc func(txn *storepkg.Txn) responder.Responder

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

func (controller *Controller) storeView(cb apiTxFunc) responder.Responder {
	return mapTxFuncs(controller.store.View, cb)
}

func (controller *Controller) storeUpdate(cb apiTxFunc) responder.Responder {
	return mapTxFuncs(controller.store.Update, cb)
}

func mapTxFuncs(txFunc storeTxFunc, cb apiTxFunc) responder.Responder {
	var result responder.Responder

	if err := txFunc(func(txn *storepkg.Txn) error {
		result = cb(txn)

		return nil
	}); err != nil {
		return responder.Code(http.StatusInternalServerError)
	}

	return result
}
