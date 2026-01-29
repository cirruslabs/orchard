//nolint:testpackage // we need to have access for Controller for this test
package controller

import (
	"net/http"
	"testing"

	"github.com/cirruslabs/orchard/internal/responder"
	v1pkg "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAuthorizeInsecureAuthDisabled(t *testing.T) {
	ctx := &gin.Context{}
	controller := Controller{insecureAuthDisabled: true}

	require.Nil(t, controller.authorize(ctx, v1pkg.ServiceAccountRoleAdminWrite))
}

func TestAuthorizeUnauthenticated(t *testing.T) {
	ctx := &gin.Context{}
	controller := Controller{}

	require.Equal(t, responder.Code(http.StatusUnauthorized), controller.authorize(ctx))
}

func TestAuthorizeAuthenticatedNoRoles(t *testing.T) {
	ctx := &gin.Context{}
	ctx.Set(ctxServiceAccountKey, &v1pkg.ServiceAccount{})
	controller := Controller{}

	const requiredRole = v1pkg.ServiceAccountRoleAdminWrite

	require.Equal(t, responder.JSON(http.StatusUnauthorized, NewErrorResponse("all of the following roles must be present: %s", requiredRole)),
		controller.authorize(ctx, requiredRole))
}

func TestAuthorizeAuthenticatedHasRoles(t *testing.T) {
	ctx := &gin.Context{}
	const requiredRole = v1pkg.ServiceAccountRoleAdminWrite
	ctx.Set(ctxServiceAccountKey, &v1pkg.ServiceAccount{Roles: []v1pkg.ServiceAccountRole{requiredRole}})
	controller := Controller{}

	require.Nil(t, controller.authorize(ctx, requiredRole))
}
