// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.4.0
// - protoc             (unknown)
// source: orchard.proto

package rpc

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.62.0 or later.
const _ = grpc.SupportPackageIsVersion8

const (
	Controller_Watch_FullMethodName       = "/Controller/Watch"
	Controller_PortForward_FullMethodName = "/Controller/PortForward"
	Controller_ResolveIP_FullMethodName   = "/Controller/ResolveIP"
)

// ControllerClient is the client API for Controller service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type ControllerClient interface {
	// message bus between the controller and a worker
	Watch(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (Controller_WatchClient, error)
	// single purpose method when a port forward is requested and running
	// session information is passed in the requests metadata
	PortForward(ctx context.Context, opts ...grpc.CallOption) (Controller_PortForwardClient, error)
	// worker calls this method when it has successfully resolved the VM's IP
	ResolveIP(ctx context.Context, in *ResolveIPResult, opts ...grpc.CallOption) (*emptypb.Empty, error)
}

type controllerClient struct {
	cc grpc.ClientConnInterface
}

func NewControllerClient(cc grpc.ClientConnInterface) ControllerClient {
	return &controllerClient{cc}
}

func (c *controllerClient) Watch(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (Controller_WatchClient, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &Controller_ServiceDesc.Streams[0], Controller_Watch_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &controllerWatchClient{ClientStream: stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type Controller_WatchClient interface {
	Recv() (*WatchInstruction, error)
	grpc.ClientStream
}

type controllerWatchClient struct {
	grpc.ClientStream
}

func (x *controllerWatchClient) Recv() (*WatchInstruction, error) {
	m := new(WatchInstruction)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *controllerClient) PortForward(ctx context.Context, opts ...grpc.CallOption) (Controller_PortForwardClient, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &Controller_ServiceDesc.Streams[1], Controller_PortForward_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &controllerPortForwardClient{ClientStream: stream}
	return x, nil
}

type Controller_PortForwardClient interface {
	Send(*PortForwardData) error
	Recv() (*PortForwardData, error)
	grpc.ClientStream
}

type controllerPortForwardClient struct {
	grpc.ClientStream
}

func (x *controllerPortForwardClient) Send(m *PortForwardData) error {
	return x.ClientStream.SendMsg(m)
}

func (x *controllerPortForwardClient) Recv() (*PortForwardData, error) {
	m := new(PortForwardData)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *controllerClient) ResolveIP(ctx context.Context, in *ResolveIPResult, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, Controller_ResolveIP_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ControllerServer is the server API for Controller service.
// All implementations must embed UnimplementedControllerServer
// for forward compatibility
type ControllerServer interface {
	// message bus between the controller and a worker
	Watch(*emptypb.Empty, Controller_WatchServer) error
	// single purpose method when a port forward is requested and running
	// session information is passed in the requests metadata
	PortForward(Controller_PortForwardServer) error
	// worker calls this method when it has successfully resolved the VM's IP
	ResolveIP(context.Context, *ResolveIPResult) (*emptypb.Empty, error)
	mustEmbedUnimplementedControllerServer()
}

// UnimplementedControllerServer must be embedded to have forward compatible implementations.
type UnimplementedControllerServer struct {
}

func (UnimplementedControllerServer) Watch(*emptypb.Empty, Controller_WatchServer) error {
	return status.Errorf(codes.Unimplemented, "method Watch not implemented")
}
func (UnimplementedControllerServer) PortForward(Controller_PortForwardServer) error {
	return status.Errorf(codes.Unimplemented, "method PortForward not implemented")
}
func (UnimplementedControllerServer) ResolveIP(context.Context, *ResolveIPResult) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ResolveIP not implemented")
}
func (UnimplementedControllerServer) mustEmbedUnimplementedControllerServer() {}

// UnsafeControllerServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to ControllerServer will
// result in compilation errors.
type UnsafeControllerServer interface {
	mustEmbedUnimplementedControllerServer()
}

func RegisterControllerServer(s grpc.ServiceRegistrar, srv ControllerServer) {
	s.RegisterService(&Controller_ServiceDesc, srv)
}

func _Controller_Watch_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(emptypb.Empty)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(ControllerServer).Watch(m, &controllerWatchServer{ServerStream: stream})
}

type Controller_WatchServer interface {
	Send(*WatchInstruction) error
	grpc.ServerStream
}

type controllerWatchServer struct {
	grpc.ServerStream
}

func (x *controllerWatchServer) Send(m *WatchInstruction) error {
	return x.ServerStream.SendMsg(m)
}

func _Controller_PortForward_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(ControllerServer).PortForward(&controllerPortForwardServer{ServerStream: stream})
}

type Controller_PortForwardServer interface {
	Send(*PortForwardData) error
	Recv() (*PortForwardData, error)
	grpc.ServerStream
}

type controllerPortForwardServer struct {
	grpc.ServerStream
}

func (x *controllerPortForwardServer) Send(m *PortForwardData) error {
	return x.ServerStream.SendMsg(m)
}

func (x *controllerPortForwardServer) Recv() (*PortForwardData, error) {
	m := new(PortForwardData)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func _Controller_ResolveIP_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ResolveIPResult)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ControllerServer).ResolveIP(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Controller_ResolveIP_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ControllerServer).ResolveIP(ctx, req.(*ResolveIPResult))
	}
	return interceptor(ctx, in, info, handler)
}

// Controller_ServiceDesc is the grpc.ServiceDesc for Controller service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Controller_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "Controller",
	HandlerType: (*ControllerServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "ResolveIP",
			Handler:    _Controller_ResolveIP_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Watch",
			Handler:       _Controller_Watch_Handler,
			ServerStreams: true,
		},
		{
			StreamName:    "PortForward",
			Handler:       _Controller_PortForward_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "orchard.proto",
}
