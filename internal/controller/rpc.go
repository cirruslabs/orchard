package controller

import (
	"context"
	"github.com/cirruslabs/orchard/internal/controller/rendezvous"
	v1pkg "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/cirruslabs/orchard/rpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
	"net"

	//nolint:staticcheck // https://github.com/mitchellh/go-grpc-net-conn/pull/1
	"github.com/golang/protobuf/proto"
	grpc_net_conn "github.com/mitchellh/go-grpc-net-conn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (controller *Controller) Watch(_ *emptypb.Empty, stream rpc.Controller_WatchServer) error {
	if !controller.authorizeGRPC(stream.Context(), v1pkg.ServiceAccountRoleComputeWrite) {
		return status.Errorf(codes.Unauthenticated, "auth failed")
	}

	workerMetadataValue := metadata.ValueFromIncomingContext(stream.Context(), rpc.MetadataWorkerNameKey)
	if len(workerMetadataValue) == 0 {
		return status.Errorf(codes.InvalidArgument, "no worker ident in metadata")
	}

	worker := workerMetadataValue[0]
	workerCh, cancel := controller.workerNotifier.Register(stream.Context(), worker)
	defer cancel()

	for {
		select {
		case msg := <-workerCh:
			if err := stream.Send(msg); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}

func (controller *Controller) PortForward(stream rpc.Controller_PortForwardServer) error {
	if !controller.authorizeGRPC(stream.Context(), v1pkg.ServiceAccountRoleComputeWrite) {
		return status.Errorf(codes.Unauthenticated, "auth failed")
	}

	sessionMetadataValue := metadata.ValueFromIncomingContext(stream.Context(), rpc.MetadataWorkerPortForwardingSessionKey)
	if len(sessionMetadataValue) == 0 {
		return status.Errorf(codes.InvalidArgument, "no session in metadata")
	}

	conn := &grpc_net_conn.Conn{
		Stream:   stream,
		Request:  &rpc.PortForwardData{},
		Response: &rpc.PortForwardData{},
		Encode: grpc_net_conn.SimpleEncoder(func(message proto.Message) *[]byte {
			return &message.(*rpc.PortForwardData).Data
		}),
		Decode: grpc_net_conn.SimpleDecoder(func(message proto.Message) *[]byte {
			return &message.(*rpc.PortForwardData).Data
		}),
	}

	// make connection rendezvous aware of the connection
	proxyCtx, err := controller.connRendezvous.Respond(sessionMetadataValue[0],
		rendezvous.ResultWithErrorMessage[net.Conn]{
			Result: conn,
		},
	)
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

func (controller *Controller) ResolveIP(ctx context.Context, request *rpc.ResolveIPResult) (*emptypb.Empty, error) {
	if !controller.authorizeGRPC(ctx, v1pkg.ServiceAccountRoleComputeWrite) {
		return nil, status.Errorf(codes.Unauthenticated, "auth failed")
	}

	sessionMetadataValue := metadata.ValueFromIncomingContext(ctx, rpc.MetadataWorkerPortForwardingSessionKey)
	if len(sessionMetadataValue) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "no session in metadata")
	}

	// Respond with the resolved IP address
	_, err := controller.ipRendezvous.Respond(sessionMetadataValue[0], rendezvous.ResultWithErrorMessage[string]{
		Result: request.Ip,
	})
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
