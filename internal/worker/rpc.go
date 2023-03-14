package worker

import (
	"context"
	"fmt"
	"github.com/cirruslabs/orchard/internal/proxy"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/cirruslabs/orchard/rpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/types/known/emptypb"
	"time"

	//nolint:staticcheck // https://github.com/mitchellh/go-grpc-net-conn/pull/1
	"github.com/golang/protobuf/proto"
	grpc_net_conn "github.com/mitchellh/go-grpc-net-conn"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"net"
)

func (worker *Worker) watchRPC(ctx context.Context) error {
	conn, err := grpc.Dial(worker.client.GRPCTarget(),
		grpc.WithTransportCredentials(worker.client.GRPCTransportCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time: 30 * time.Second,
		}),
	)
	if err != nil {
		return err
	}

	client := rpc.NewControllerClient(conn)

	ctxWithMetadata := metadata.NewOutgoingContext(ctx, worker.GPRCMetadata())

	stream, err := client.Watch(ctxWithMetadata, &emptypb.Empty{})
	if err != nil {
		return err
	}

	for {
		watchFromController, err := stream.Recv()
		if err != nil {
			return err
		}

		portForwardAction, ok := watchFromController.Action.(*rpc.WatchInstruction_PortForwardAction)
		if !ok {
			continue
		}

		go worker.handlePortForward(ctxWithMetadata, client, portForwardAction.PortForwardAction)
	}
}

func (worker *Worker) handlePortForward(
	ctx context.Context,
	client rpc.ControllerClient,
	portForwardAction *rpc.WatchInstruction_PortForward,
) {
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	grpcMetadata := metadata.Join(
		worker.GPRCMetadata(),
		metadata.Pairs(rpc.MetadataWorkerPortForwardingSessionKey, portForwardAction.Session),
	)
	ctxWithMetadata := metadata.NewOutgoingContext(subCtx, grpcMetadata)
	stream, err := client.PortForward(ctxWithMetadata)
	if err != nil {
		worker.logger.Warnf("port forwarding failed: failed to call PortForward() RPC method: %v", err)

		return
	}

	// Obtain VM
	vm, err := worker.vmm.Get(v1.VM{
		Meta: v1.Meta{
			UID: portForwardAction.VmUid,
		},
	})
	if err != nil {
		worker.logger.Warnf("port forwarding failed: failed to get the VM: %v", err)

		return
	}

	// Obtain VM's IP address
	ip, err := vm.IP(ctx)
	if err != nil {
		worker.logger.Warnf("port forwarding failed: failed to get VM's IP: %v", err)

		return
	}

	// Connect to the VM's port
	vmConn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", ip, portForwardAction.VmPort))
	if err != nil {
		worker.logger.Warnf("port forwarding failed: failed to connect to the VM: %v", err)

		return
	}

	// Proxy bytes
	grpcConn := &grpc_net_conn.Conn{
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

	_ = proxy.Connections(vmConn, grpcConn)
}
