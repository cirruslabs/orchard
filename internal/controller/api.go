package controller

import (
	"crypto/subtle"
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/internal/responder"
	v1pkg "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/deckarep/golang-set/v2"
	"github.com/gin-gonic/gin"
	"net/http"
)

const ctxServiceAccountKey = "service-account"

func (controller *Controller) initAPI() *gin.Engine {
	gin.SetMode(gin.DebugMode)
	ginEngine := gin.Default()

	// v1 API
	v1 := ginEngine.Group("/v1")

	// Auth
	v1.Use(controller.authenticateMiddleware)

	// A way to for the clients to check that the API is working
	v1.GET("/", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Service accounts
	v1.POST("/service-accounts", func(c *gin.Context) {
		controller.createServiceAccount(c).Respond(c)
	})
	v1.PUT("/service-accounts/:name", func(c *gin.Context) {
		controller.updateServiceAccount(c).Respond(c)
	})
	v1.GET("/service-accounts/:name", func(c *gin.Context) {
		controller.getServiceAccount(c).Respond(c)
	})
	v1.GET("/service-accounts", func(c *gin.Context) {
		controller.listServiceAccounts(c).Respond(c)
	})
	v1.DELETE("/service-accounts/:name", func(c *gin.Context) {
		controller.deleteServiceAccount(c).Respond(c)
	})

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
	v1.GET("/vms/:name/events", func(c *gin.Context) {
		controller.listVMEvents(c).Respond(c)
	})
	v1.PUT("/vms/:name/events", func(c *gin.Context) {
		controller.appendVMEvents(c).Respond(c)
	})

	return ginEngine
}

func (controller *Controller) authenticateMiddleware(c *gin.Context) {
	// Retrieve presented credentials (if any)
	user, password, ok := c.Request.BasicAuth()
	if !ok {
		c.Next()

		return
	}

	// Authenticate
	var serviceAccount *v1pkg.ServiceAccount
	var err error

	err = controller.store.View(func(txn storepkg.Transaction) error {
		serviceAccount, err = txn.GetServiceAccount(user)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		responder.Error(err).Respond(c)

		return
	}

	// No such service account found
	if serviceAccount == nil {
		responder.Code(http.StatusUnauthorized).Respond(c)

		return
	}

	// Service account's token provided is not valid
	if subtle.ConstantTimeCompare([]byte(serviceAccount.Token), []byte(password)) == 0 {
		responder.Code(http.StatusUnauthorized).Respond(c)

		return
	}

	// Remember service account for further authorize() calls
	c.Set(ctxServiceAccountKey, serviceAccount)

	c.Next()
}

func (controller *Controller) authorize(ctx *gin.Context, scopes ...v1pkg.ServiceAccountRole) bool {
	if controller.insecureAuthDisabled {
		return true
	}

	serviceAccountUntyped, ok := ctx.Get(ctxServiceAccountKey)
	if !ok {
		return false
	}

	serviceAccount := serviceAccountUntyped.(*v1pkg.ServiceAccount)

	return mapset.NewSet[v1pkg.ServiceAccountRole](serviceAccount.Roles...).Contains(scopes...)
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
