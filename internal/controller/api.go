package controller

import (
	"context"
	"crypto/subtle"
	"errors"
	"github.com/cirruslabs/orchard/api"
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/internal/responder"
	v1pkg "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/cirruslabs/orchard/rpc"
	"github.com/deckarep/golang-set/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-openapi/runtime/middleware"
	"github.com/penglongli/gin-metrics/ginmetrics"
	"google.golang.org/grpc/metadata"
	"net/http"
	"strings"
)

const ctxServiceAccountKey = "service-account"

var ErrUnauthorized = errors.New("unauthorized")

func (controller *Controller) initAPI() *gin.Engine {
	ginEngine := gin.Default()

	// expose metrics
	monitor := ginmetrics.GetMonitor()
	monitor.SetMetricPath("/metrics")
	monitor.Use(ginEngine)

	// v1 API
	v1 := ginEngine.Group("/v1")

	// Auth
	v1.Use(controller.authenticateMiddleware)

	// OpenAPI docs/spec (if enabled) and a way to for the clients
	// to check that the API is working
	v1.GET("/", func(c *gin.Context) {
		if controller.enableSwaggerDocs {
			middleware.SwaggerUI(middleware.SwaggerUIOpts{
				Path:    "/v1",
				SpecURL: "/v1/openapi.yaml",
			}, nil).ServeHTTP(c.Writer, c.Request)
		} else {
			c.Status(http.StatusOK)
		}
	})
	if controller.enableSwaggerDocs {
		v1.GET("/openapi.yaml", func(c *gin.Context) {
			c.Data(200, "text/yaml", api.Spec)
		})
	}

	// Controller information
	v1.GET("/controller/info", func(c *gin.Context) {
		controller.controllerInfo(c).Respond(c)
	})

	// Cluster settings
	v1.GET("/cluster-settings", func(c *gin.Context) {
		controller.getClusterSettings(c).Respond(c)
	})
	v1.PUT("/cluster-settings", func(c *gin.Context) {
		controller.updateClusterSettings(c).Respond(c)
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
	v1.GET("/vms/:name/port-forward", func(c *gin.Context) {
		controller.portForwardVM(c).Respond(c)
	})
	v1.DELETE("/vms/:name", func(c *gin.Context) {
		controller.deleteVM(c).Respond(c)
	})
	v1.GET("/vms/:name/events", func(c *gin.Context) {
		controller.listVMEvents(c).Respond(c)
	})
	v1.POST("/vms/:name/events", func(c *gin.Context) {
		controller.appendVMEvents(c).Respond(c)
	})

	return ginEngine
}

func (controller *Controller) fetchServiceAccount(name string, token string) (*v1pkg.ServiceAccount, error) {
	var serviceAccount *v1pkg.ServiceAccount
	var err error

	err = controller.store.View(func(txn storepkg.Transaction) error {
		serviceAccount, err = txn.GetServiceAccount(name)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, storepkg.ErrNotFound) {
			return nil, ErrUnauthorized
		}

		return nil, err
	}

	if subtle.ConstantTimeCompare([]byte(serviceAccount.Token), []byte(token)) == 0 {
		return nil, ErrUnauthorized
	}

	return serviceAccount, nil
}

func (controller *Controller) authenticateMiddleware(c *gin.Context) {
	// Retrieve presented credentials (if any)
	user, password, ok := c.Request.BasicAuth()
	if !ok {
		c.Next()

		return
	}

	serviceAccount, err := controller.fetchServiceAccount(user, password)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			responder.Code(http.StatusUnauthorized).Respond(c)
		} else {
			responder.Error(err).Respond(c)
		}

		return
	}

	// Remember service account for further authorize() calls
	c.Set(ctxServiceAccountKey, serviceAccount)

	c.Next()
}

func (controller *Controller) authorize(
	ctx *gin.Context,
	requiredRoles ...v1pkg.ServiceAccountRole,
) responder.Responder {
	if controller.insecureAuthDisabled {
		return nil
	}

	serviceAccountUntyped, ok := ctx.Get(ctxServiceAccountKey)
	if !ok {
		return responder.Code(http.StatusUnauthorized)
	}
	serviceAccount := serviceAccountUntyped.(*v1pkg.ServiceAccount)
	serviceAccountRolesSet := mapset.NewSet[v1pkg.ServiceAccountRole](serviceAccount.Roles...)

	requiredRolesSet := mapset.NewSet[v1pkg.ServiceAccountRole](requiredRoles...)

	missingRoles := requiredRolesSet.Difference(serviceAccountRolesSet).ToSlice()
	if len(missingRoles) == 0 {
		return nil
	}

	var missingRolesStrings []string

	for _, missingRole := range missingRoles {
		missingRolesStrings = append(missingRolesStrings, string(missingRole))
	}

	return responder.JSON(http.StatusUnauthorized,
		NewErrorResponse("missing roles: %s", strings.Join(missingRolesStrings, ", ")))
}

func (controller *Controller) authorizeGRPC(ctx context.Context, scopes ...v1pkg.ServiceAccountRole) bool {
	if controller.insecureAuthDisabled {
		return true
	}

	name := metadata.ValueFromIncomingContext(ctx, rpc.MetadataServiceAccountNameKey)
	if len(name) != 1 {
		return false
	}
	token := metadata.ValueFromIncomingContext(ctx, rpc.MetadataServiceAccountTokenKey)
	if len(token) != 1 {
		return false
	}

	serviceAccount, err := controller.fetchServiceAccount(name[0], token[0])
	if err != nil {
		return false
	}

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
