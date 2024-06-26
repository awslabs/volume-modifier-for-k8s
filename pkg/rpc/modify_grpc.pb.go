// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v4.25.1
// source: modify.proto

package rpc

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// ModifyClient is the client API for Modify service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type ModifyClient interface {
	GetCSIDriverModificationCapability(ctx context.Context, in *GetCSIDriverModificationCapabilityRequest, opts ...grpc.CallOption) (*GetCSIDriverModificationCapabilityResponse, error)
	ModifyVolumeProperties(ctx context.Context, in *ModifyVolumePropertiesRequest, opts ...grpc.CallOption) (*ModifyVolumePropertiesResponse, error)
}

type modifyClient struct {
	cc grpc.ClientConnInterface
}

func NewModifyClient(cc grpc.ClientConnInterface) ModifyClient {
	return &modifyClient{cc}
}

func (c *modifyClient) GetCSIDriverModificationCapability(ctx context.Context, in *GetCSIDriverModificationCapabilityRequest, opts ...grpc.CallOption) (*GetCSIDriverModificationCapabilityResponse, error) {
	out := new(GetCSIDriverModificationCapabilityResponse)
	err := c.cc.Invoke(ctx, "/modify.v1.Modify/GetCSIDriverModificationCapability", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *modifyClient) ModifyVolumeProperties(ctx context.Context, in *ModifyVolumePropertiesRequest, opts ...grpc.CallOption) (*ModifyVolumePropertiesResponse, error) {
	out := new(ModifyVolumePropertiesResponse)
	err := c.cc.Invoke(ctx, "/modify.v1.Modify/ModifyVolumeProperties", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ModifyServer is the server API for Modify service.
// All implementations must embed UnimplementedModifyServer
// for forward compatibility
type ModifyServer interface {
	GetCSIDriverModificationCapability(context.Context, *GetCSIDriverModificationCapabilityRequest) (*GetCSIDriverModificationCapabilityResponse, error)
	ModifyVolumeProperties(context.Context, *ModifyVolumePropertiesRequest) (*ModifyVolumePropertiesResponse, error)
	mustEmbedUnimplementedModifyServer()
}

// UnimplementedModifyServer must be embedded to have forward compatible implementations.
type UnimplementedModifyServer struct {
}

func (UnimplementedModifyServer) GetCSIDriverModificationCapability(context.Context, *GetCSIDriverModificationCapabilityRequest) (*GetCSIDriverModificationCapabilityResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetCSIDriverModificationCapability not implemented")
}
func (UnimplementedModifyServer) ModifyVolumeProperties(context.Context, *ModifyVolumePropertiesRequest) (*ModifyVolumePropertiesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ModifyVolumeProperties not implemented")
}
func (UnimplementedModifyServer) mustEmbedUnimplementedModifyServer() {}

// UnsafeModifyServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to ModifyServer will
// result in compilation errors.
type UnsafeModifyServer interface {
	mustEmbedUnimplementedModifyServer()
}

func RegisterModifyServer(s grpc.ServiceRegistrar, srv ModifyServer) {
	s.RegisterService(&Modify_ServiceDesc, srv)
}

func _Modify_GetCSIDriverModificationCapability_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetCSIDriverModificationCapabilityRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ModifyServer).GetCSIDriverModificationCapability(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/modify.v1.Modify/GetCSIDriverModificationCapability",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ModifyServer).GetCSIDriverModificationCapability(ctx, req.(*GetCSIDriverModificationCapabilityRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Modify_ModifyVolumeProperties_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ModifyVolumePropertiesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ModifyServer).ModifyVolumeProperties(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/modify.v1.Modify/ModifyVolumeProperties",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ModifyServer).ModifyVolumeProperties(ctx, req.(*ModifyVolumePropertiesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Modify_ServiceDesc is the grpc.ServiceDesc for Modify service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Modify_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "modify.v1.Modify",
	HandlerType: (*ModifyServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetCSIDriverModificationCapability",
			Handler:    _Modify_GetCSIDriverModificationCapability_Handler,
		},
		{
			MethodName: "ModifyVolumeProperties",
			Handler:    _Modify_ModifyVolumeProperties_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "modify.proto",
}
