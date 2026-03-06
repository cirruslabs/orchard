package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/avast/retry-go/v5"
	"github.com/cirruslabs/orchard/internal/responder"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/cirruslabs/orchard/rpc"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var errIPRequest = errors.New("failed to request VM's IP")

func (controller *Controller) ip(ctx *gin.Context) responder.Responder {
	if responder := controller.authorizeAny(ctx, v1.ServiceAccountRoleComputeWrite,
		v1.ServiceAccountRoleComputeConnect); responder != nil {
		return responder
	}

	// Retrieve and parse path and query parameters
	name := ctx.Param("name")

	waitRaw := ctx.DefaultQuery("wait", "0")
	wait, err := strconv.ParseUint(waitRaw, 10, 16)
	if err != nil {
		return responder.Code(http.StatusBadRequest)
	}
	waitContext, waitContextCancel := context.WithTimeout(ctx, time.Duration(wait)*time.Second)
	defer waitContextCancel()

	// Look-up the VM
	vm, responderImpl := controller.waitForVM(waitContext, name)
	if responderImpl != nil {
		return responderImpl
	}

	// Send an IP resolution request and wait for the result
	ip, err := retry.NewWithData[string](
		retry.Context(waitContext),
		retry.DelayType(retry.FixedDelay),
		retry.Delay(time.Second),
		retry.Attempts(0),
		retry.LastErrorOnly(true),
	).Do(func() (string, error) {
		return controller.vmIP(ctx, waitContext, vm.Worker, vm.UID)
	})
	if err != nil {
		if errors.Is(err, errIPRequest) {
			controller.logger.Warnf("failed to request VM's IP from the worker %s: %v",
				vm.Worker, err)

			return responder.Code(http.StatusServiceUnavailable)
		}

		return responder.Error(err)
	}

	result := struct {
		IP string `json:"ip"`
	}{
		IP: ip,
	}

	return responder.JSON(http.StatusOK, &result)
}

func (controller *Controller) vmIP(
	ctx context.Context,
	waitContext context.Context,
	workerName string,
	vmUID string,
) (string, error) {
	// Send an IP resolution request and wait for the result.
	session := uuid.New().String()
	boomerangConnCh, cancel := controller.ipRendezvous.Request(ctx, session)
	defer cancel()

	err := controller.workerNotifier.Notify(waitContext, workerName, &rpc.WatchInstruction{
		Action: &rpc.WatchInstruction_ResolveIpAction{
			ResolveIpAction: &rpc.WatchInstruction_ResolveIP{
				Session: session,
				VmUid:   vmUID,
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("%w: failed to request VM's IP from the worker %s: %v",
			errIPRequest, workerName, err)
	}

	select {
	case rendezvousResponse := <-boomerangConnCh:
		if rendezvousResponse.ErrorMessage != "" {
			return "", fmt.Errorf("VM's IP resolution on the worker %s failed: %s",
				workerName, rendezvousResponse.ErrorMessage)
		}

		return rendezvousResponse.Result, nil
	case <-waitContext.Done():
		return "", waitContext.Err()
	}
}
