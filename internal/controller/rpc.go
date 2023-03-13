package controller

import (
	v1pkg "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/cirruslabs/orchard/rpc"
	//nolint:staticcheck // https://github.com/mitchellh/go-grpc-net-conn/pull/1
	"github.com/golang/protobuf/proto"
	grpc_net_conn "github.com/mitchellh/go-grpc-net-conn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (controller *Controller) Watch(stream rpc.Controller_WatchServer) error {
	if !controller.authorizeGRPC(stream.Context(), v1pkg.ServiceAccountRoleWorker) {
		return status.Errorf(codes.Unauthenticated, "auth failed")
	}

	// The first message is always an initialization
	watchFromWorker, err := stream.Recv()
	if err != nil {
		return err
	}

	initAction, ok := watchFromWorker.Action.(*rpc.WatchFromWorker_InitAction)
	if !ok {
		return status.Errorf(codes.FailedPrecondition, "expected an initialization message")
	}

	// Subscribe to rendez-vous requests from the API for this worker
	requestsCh, cancel := controller.watcher.Subscribe(stream.Context(), initAction.InitAction.WorkerUid)
	defer cancel()

	for {
		select {
		case request := <-requestsCh:
			// New rendez-vous request, tell the worker to establish
			// a new connection to us via the PortForward() RPC
			if err := stream.Send(&rpc.WatchFromController{
				Action: &rpc.WatchFromController_PortForwardAction{
					PortForwardAction: &rpc.WatchFromController_PortForward{
						Token:  request.Token,
						VmUid:  request.VMUID,
						VmPort: uint32(request.VMPort),
					},
				},
			}); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}

func (controller *Controller) PortForward(stream rpc.Controller_PortForwardServer) error {
	if !controller.authorizeGRPC(stream.Context(), v1pkg.ServiceAccountRoleWorker) {
		return status.Errorf(codes.Unauthenticated, "auth failed")
	}

	// The first message is always an initialization
	portForwardFromWorker, err := stream.Recv()
	if err != nil {
		return err
	}

	initAction, ok := portForwardFromWorker.Action.(*rpc.PortForwardFromWorker_InitAction)
	if !ok {
		return status.Errorf(codes.FailedPrecondition, "expected an initialization message")
	}

	// Satisfy the rendez-vous request
	conn := &grpc_net_conn.Conn{
		Stream: stream,
		Request: &rpc.PortForwardFromController{
			Action: &rpc.PortForwardFromController_DataAction{
				DataAction: &rpc.PortForwardFromController_Data{},
			},
		},
		Response: &rpc.PortForwardFromWorker{
			Action: &rpc.PortForwardFromWorker_DataAction{
				DataAction: &rpc.PortForwardFromWorker_Data{},
			},
		},
		Encode: grpc_net_conn.SimpleEncoder(func(message proto.Message) *[]byte {
			return &message.(*rpc.PortForwardFromController).Action.(*rpc.PortForwardFromController_DataAction).DataAction.Data
		}),
		Decode: grpc_net_conn.SimpleDecoder(func(message proto.Message) *[]byte {
			return &message.(*rpc.PortForwardFromWorker).Action.(*rpc.PortForwardFromWorker_DataAction).DataAction.Data
		}),
	}

	proxyCtx, err := controller.proxy.Respond(initAction.InitAction.Token, conn)
	if err != nil {
		return err
	}

	select {
	case <-proxyCtx.Done():
		return proxyCtx.Err()
	case <-stream.Context().Done():
		return stream.Context().Err()
	}
}
