package worker

import (
	"context"
	"fmt"
	"github.com/cirruslabs/orchard/internal/proxy"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/cirruslabs/orchard/rpc"
	"google.golang.org/grpc/keepalive"
	"time"

	//nolint:staticcheck // https://github.com/mitchellh/go-grpc-net-conn/pull/1
	"github.com/golang/protobuf/proto"
	grpc_net_conn "github.com/mitchellh/go-grpc-net-conn"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"net"
)

func (worker *Worker) pollRPC(ctx context.Context) error {
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

	ctxWithMetadata := metadata.NewOutgoingContext(ctx, worker.client.GPRCMetadata())

	stream, err := client.Poll(ctxWithMetadata)
	if err != nil {
		return err
	}

	if err := stream.Send(&rpc.PollFromWorker{
		Action: &rpc.PollFromWorker_InitAction{
			InitAction: &rpc.PollFromWorker_Init{
				WorkerUid: worker.name,
			},
		},
	}); err != nil {
		return err
	}

	for {
		pollFromController, err := stream.Recv()
		if err != nil {
			return err
		}

		portForwardAction, ok := pollFromController.Action.(*rpc.PollFromController_PortForwardAction)
		if !ok {
			continue
		}

		go worker.handlePortForward(ctxWithMetadata, client, portForwardAction.PortForwardAction)
	}
}

func (worker *Worker) handlePortForward(
	ctx context.Context,
	client rpc.ControllerClient,
	portForwardAction *rpc.PollFromController_PortForward,
) {
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream, err := client.PortForward(subCtx)
	if err != nil {
		worker.logger.Warnf("port forwarding failed: failed to call PortForward() RPC method: %v", err)

		return
	}

	if err := stream.Send(&rpc.PortForwardFromWorker{
		Action: &rpc.PortForwardFromWorker_InitAction{
			InitAction: &rpc.PortForwardFromWorker_Init{
				Token: portForwardAction.Token,
			},
		},
	}); err != nil {
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
		Stream: stream,
		Request: &rpc.PortForwardFromWorker{
			Action: &rpc.PortForwardFromWorker_DataAction{
				DataAction: &rpc.PortForwardFromWorker_Data{},
			},
		},
		Response: &rpc.PortForwardFromController{
			Action: &rpc.PortForwardFromController_DataAction{
				DataAction: &rpc.PortForwardFromController_Data{},
			},
		},
		Encode: grpc_net_conn.SimpleEncoder(func(message proto.Message) *[]byte {
			return &message.(*rpc.PortForwardFromWorker).Action.(*rpc.PortForwardFromWorker_DataAction).DataAction.Data
		}),
		Decode: grpc_net_conn.SimpleDecoder(func(message proto.Message) *[]byte {
			return &message.(*rpc.PortForwardFromController).Action.(*rpc.PortForwardFromController_DataAction).DataAction.Data
		}),
	}

	_ = proxy.Connections(vmConn, grpcConn)
}
