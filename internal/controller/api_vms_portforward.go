package controller

import (
	"context"
	"fmt"
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/internal/netconncancel"
	"github.com/cirruslabs/orchard/internal/proxy"
	"github.com/cirruslabs/orchard/internal/responder"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/cirruslabs/orchard/rpc"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net/http"
	"nhooyr.io/websocket"
	"strconv"
	"time"
)

func (controller *Controller) portForwardVM(ctx *gin.Context) responder.Responder {
	if responder := controller.authorize(ctx, v1.ServiceAccountRoleComputeWrite); responder != nil {
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

	// Commence port-forwarding
	return controller.portForward(ctx, vm.Worker, vm.UID, uint32(port))
}

func (controller *Controller) portForward(
	ctx *gin.Context,
	workerName string,
	vmUID string,
	port uint32,
) responder.Responder {
	// Request and wait for a connection with a worker
	rendezvousCtx, rendezvousCtxCancel := context.WithCancel(ctx)
	defer rendezvousCtxCancel()

	session := uuid.New().String()

	boomerangConnCh, cancel := controller.connRendezvous.Request(rendezvousCtx, session)
	defer cancel()

	// send request to worker to initiate port-forwarding connection back to us
	err := controller.workerNotifier.Notify(ctx, workerName, &rpc.WatchInstruction{
		Action: &rpc.WatchInstruction_PortForwardAction{
			PortForwardAction: &rpc.WatchInstruction_PortForward{
				Session: session,
				VmUid:   vmUID,
				Port:    port,
			},
		},
	})
	if err != nil {
		controller.logger.Warnf("port forwarding on the worker %s failed: %v",
			workerName, err)

		return responder.Code(http.StatusServiceUnavailable)
	}

	// worker will asynchronously start port-forwarding so we wait
	select {
	case rendezvousResponse := <-boomerangConnCh:
		if rendezvousResponse.ErrorMessage != "" {
			return responder.Error(fmt.Errorf("failed to establish port forwarding session on the worker: %s",
				rendezvousResponse.ErrorMessage))
		}

		wsConn, err := websocket.Accept(ctx.Writer, ctx.Request, &websocket.AcceptOptions{
			OriginPatterns: []string{"*"},
		})
		if err != nil {
			return responder.Error(err)
		}

		expectedMsgType := websocket.MessageBinary

		// Backwards compatibility with older Orchard clients
		// using "golang.org/x/net/websocket" package
		if ctx.Request.Header.Get("User-Agent") == "" {
			expectedMsgType = websocket.MessageText
		}

		wsConnAsNetConn := websocket.NetConn(ctx, wsConn, expectedMsgType)
		fromWorkerConnectionWithCancel := netconncancel.New(rendezvousResponse.Result, rendezvousCtxCancel)

		if err := proxy.Connections(wsConnAsNetConn, fromWorkerConnectionWithCancel); err != nil {
			var websocketCloseError websocket.CloseError

			// Normal closure from the user
			if errors.As(err, &websocketCloseError) && websocketCloseError.Code == websocket.StatusNormalClosure {
				return responder.Empty()
			}

			if errors.Is(err, context.Canceled) {
				return responder.Empty()
			}

			if status, ok := status.FromError(err); ok && status.Code() == codes.Canceled {
				return responder.Empty()
			}

			controller.logger.Warnf("failed to port-forward: %v", err)
		}

		return responder.Empty()
	case <-ctx.Done():
		return responder.Error(ctx.Err())
	}
}

func (controller *Controller) waitForVM(ctx context.Context, name string) (*v1.VM, responder.Responder) {
	var vm *v1.VM
	var err error

	for {
		if lookupResponder := controller.storeView(func(txn storepkg.Transaction) responder.Responder {
			vm, err = txn.GetVM(name)
			if err != nil {
				return responder.Error(err)
			}

			return nil
		}); lookupResponder != nil {
			return nil, lookupResponder
		}

		if vm.TerminalState() {
			return nil, responder.JSON(http.StatusExpectationFailed,
				NewErrorResponse("VM is in a terminal state '%s'", vm.Status))
		}
		if vm.Status == v1.VMStatusRunning {
			// VM is running, proceed
			return vm, nil
		}
		select {
		case <-ctx.Done():
			return nil, responder.JSON(http.StatusRequestTimeout,
				NewErrorResponse("VM is not running on '%s' worker", vm.Worker))
		case <-time.After(1 * time.Second):
			// try again
			continue
		}
	}
}
