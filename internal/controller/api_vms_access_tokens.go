package controller

import (
	"errors"
	"io"
	"net/http"
	"time"

	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/internal/responder"
	"github.com/cirruslabs/orchard/internal/vmtempauth"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/gin-gonic/gin"
)

func (controller *Controller) issueVMAccessToken(ctx *gin.Context) responder.Responder {
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite); responder != nil {
		return responder
	}

	var request v1.IssueVMAccessTokenRequest

	if err := ctx.ShouldBindJSON(&request); err != nil && !errors.Is(err, io.EOF) {
		return responder.JSON(http.StatusBadRequest, NewErrorResponse("invalid JSON was provided"))
	}

	ttl, err := vmtempauth.NormalizeTTL(request.TTLSeconds)
	if err != nil {
		return responder.JSON(http.StatusPreconditionFailed, NewErrorResponse("%v", err))
	}

	name := ctx.Param("name")
	var vm *v1.VM

	if responder := controller.storeView(func(txn storepkg.Transaction) responder.Responder {
		vm, err = txn.GetVM(name)
		if err != nil {
			return responder.Error(err)
		}

		return nil
	}); responder != nil {
		return responder
	}

	serviceAccount, ok := controller.serviceAccountFromContext(ctx)
	if !ok {
		return responder.Code(http.StatusUnauthorized)
	}

	token, err := vmtempauth.Issue(controller.vmAccessTokenKey, vmtempauth.IssueInput{
		Issuer:  vmtempauth.AccessTokenIssuer,
		Subject: serviceAccount.Name,
		VMUID:   vm.UID,
		VMName:  vm.Name,
		TTL:     ttl,
		Now:     time.Now().UTC(),
	})
	if err != nil {
		controller.logger.Errorf("failed to issue VM access token: %v", err)

		return responder.Code(http.StatusInternalServerError)
	}

	return responder.JSON(http.StatusOK, &v1.VMAccessToken{
		Token:       token.Token,
		TokenType:   "Bearer",
		ExpiresAt:   token.ExpiresAt,
		SSHUsername: vmtempauth.SSHUsername,
		VMName:      vm.Name,
		VMUID:       vm.UID,
	})
}
