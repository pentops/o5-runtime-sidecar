// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.31.0
// 	protoc        (unknown)
// source: test/v1/test.proto

package testpb

import (
	_ "google.golang.org/genproto/googleapis/api/annotations"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type GetFooRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Id      string    `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	Number  int64     `protobuf:"varint,2,opt,name=number,proto3" json:"number,omitempty"`
	Numbers []float32 `protobuf:"fixed32,3,rep,packed,name=numbers,proto3" json:"numbers,omitempty"`
}

func (x *GetFooRequest) Reset() {
	*x = GetFooRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_test_v1_test_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *GetFooRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetFooRequest) ProtoMessage() {}

func (x *GetFooRequest) ProtoReflect() protoreflect.Message {
	mi := &file_test_v1_test_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetFooRequest.ProtoReflect.Descriptor instead.
func (*GetFooRequest) Descriptor() ([]byte, []int) {
	return file_test_v1_test_proto_rawDescGZIP(), []int{0}
}

func (x *GetFooRequest) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *GetFooRequest) GetNumber() int64 {
	if x != nil {
		return x.Number
	}
	return 0
}

func (x *GetFooRequest) GetNumbers() []float32 {
	if x != nil {
		return x.Numbers
	}
	return nil
}

type GetFooResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Id    string `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	Name  string `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	Field string `protobuf:"bytes,3,opt,name=field,proto3" json:"field,omitempty"`
}

func (x *GetFooResponse) Reset() {
	*x = GetFooResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_test_v1_test_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *GetFooResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetFooResponse) ProtoMessage() {}

func (x *GetFooResponse) ProtoReflect() protoreflect.Message {
	mi := &file_test_v1_test_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetFooResponse.ProtoReflect.Descriptor instead.
func (*GetFooResponse) Descriptor() ([]byte, []int) {
	return file_test_v1_test_proto_rawDescGZIP(), []int{1}
}

func (x *GetFooResponse) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *GetFooResponse) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *GetFooResponse) GetField() string {
	if x != nil {
		return x.Field
	}
	return ""
}

type PostFooRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Name  string   `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Field string   `protobuf:"bytes,2,opt,name=field,proto3" json:"field,omitempty"`
	Bar   *Bar     `protobuf:"bytes,3,opt,name=bar,proto3" json:"bar,omitempty"`
	Baz   []string `protobuf:"bytes,4,rep,name=baz,proto3" json:"baz,omitempty"`
	Bars  []*Bar   `protobuf:"bytes,5,rep,name=bars,proto3" json:"bars,omitempty"`
	// Types that are assignable to Theoneof:
	//
	//	*PostFooRequest_Onestring
	//	*PostFooRequest_Onebar
	Theoneof isPostFooRequest_Theoneof `protobuf_oneof:"theoneof"`
}

func (x *PostFooRequest) Reset() {
	*x = PostFooRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_test_v1_test_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *PostFooRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PostFooRequest) ProtoMessage() {}

func (x *PostFooRequest) ProtoReflect() protoreflect.Message {
	mi := &file_test_v1_test_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PostFooRequest.ProtoReflect.Descriptor instead.
func (*PostFooRequest) Descriptor() ([]byte, []int) {
	return file_test_v1_test_proto_rawDescGZIP(), []int{2}
}

func (x *PostFooRequest) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *PostFooRequest) GetField() string {
	if x != nil {
		return x.Field
	}
	return ""
}

func (x *PostFooRequest) GetBar() *Bar {
	if x != nil {
		return x.Bar
	}
	return nil
}

func (x *PostFooRequest) GetBaz() []string {
	if x != nil {
		return x.Baz
	}
	return nil
}

func (x *PostFooRequest) GetBars() []*Bar {
	if x != nil {
		return x.Bars
	}
	return nil
}

func (m *PostFooRequest) GetTheoneof() isPostFooRequest_Theoneof {
	if m != nil {
		return m.Theoneof
	}
	return nil
}

func (x *PostFooRequest) GetOnestring() string {
	if x, ok := x.GetTheoneof().(*PostFooRequest_Onestring); ok {
		return x.Onestring
	}
	return ""
}

func (x *PostFooRequest) GetOnebar() *Bar {
	if x, ok := x.GetTheoneof().(*PostFooRequest_Onebar); ok {
		return x.Onebar
	}
	return nil
}

type isPostFooRequest_Theoneof interface {
	isPostFooRequest_Theoneof()
}

type PostFooRequest_Onestring struct {
	Onestring string `protobuf:"bytes,10,opt,name=onestring,proto3,oneof"`
}

type PostFooRequest_Onebar struct {
	Onebar *Bar `protobuf:"bytes,11,opt,name=onebar,proto3,oneof"`
}

func (*PostFooRequest_Onestring) isPostFooRequest_Theoneof() {}

func (*PostFooRequest_Onebar) isPostFooRequest_Theoneof() {}

type Bar struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Id    string `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	Name  string `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	Field string `protobuf:"bytes,3,opt,name=field,proto3" json:"field,omitempty"`
}

func (x *Bar) Reset() {
	*x = Bar{}
	if protoimpl.UnsafeEnabled {
		mi := &file_test_v1_test_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Bar) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Bar) ProtoMessage() {}

func (x *Bar) ProtoReflect() protoreflect.Message {
	mi := &file_test_v1_test_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Bar.ProtoReflect.Descriptor instead.
func (*Bar) Descriptor() ([]byte, []int) {
	return file_test_v1_test_proto_rawDescGZIP(), []int{3}
}

func (x *Bar) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *Bar) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *Bar) GetField() string {
	if x != nil {
		return x.Field
	}
	return ""
}

type PostFooResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Id    string `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	Name  string `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	Field string `protobuf:"bytes,3,opt,name=field,proto3" json:"field,omitempty"`
}

func (x *PostFooResponse) Reset() {
	*x = PostFooResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_test_v1_test_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *PostFooResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PostFooResponse) ProtoMessage() {}

func (x *PostFooResponse) ProtoReflect() protoreflect.Message {
	mi := &file_test_v1_test_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PostFooResponse.ProtoReflect.Descriptor instead.
func (*PostFooResponse) Descriptor() ([]byte, []int) {
	return file_test_v1_test_proto_rawDescGZIP(), []int{4}
}

func (x *PostFooResponse) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *PostFooResponse) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *PostFooResponse) GetField() string {
	if x != nil {
		return x.Field
	}
	return ""
}

type FooMessage struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Id    string `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	Name  string `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	Field string `protobuf:"bytes,3,opt,name=field,proto3" json:"field,omitempty"`
}

func (x *FooMessage) Reset() {
	*x = FooMessage{}
	if protoimpl.UnsafeEnabled {
		mi := &file_test_v1_test_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *FooMessage) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FooMessage) ProtoMessage() {}

func (x *FooMessage) ProtoReflect() protoreflect.Message {
	mi := &file_test_v1_test_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use FooMessage.ProtoReflect.Descriptor instead.
func (*FooMessage) Descriptor() ([]byte, []int) {
	return file_test_v1_test_proto_rawDescGZIP(), []int{5}
}

func (x *FooMessage) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *FooMessage) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *FooMessage) GetField() string {
	if x != nil {
		return x.Field
	}
	return ""
}

var File_test_v1_test_proto protoreflect.FileDescriptor

var file_test_v1_test_proto_rawDesc = []byte{
	0x0a, 0x12, 0x74, 0x65, 0x73, 0x74, 0x2f, 0x76, 0x31, 0x2f, 0x74, 0x65, 0x73, 0x74, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x12, 0x07, 0x74, 0x65, 0x73, 0x74, 0x2e, 0x76, 0x31, 0x1a, 0x1c, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x61, 0x6e, 0x6e, 0x6f, 0x74, 0x61,
	0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1b, 0x67, 0x6f, 0x6f,
	0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x65, 0x6d, 0x70,
	0x74, 0x79, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x51, 0x0a, 0x0d, 0x47, 0x65, 0x74, 0x46,
	0x6f, 0x6f, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x02, 0x69, 0x64, 0x12, 0x16, 0x0a, 0x06, 0x6e, 0x75, 0x6d,
	0x62, 0x65, 0x72, 0x18, 0x02, 0x20, 0x01, 0x28, 0x03, 0x52, 0x06, 0x6e, 0x75, 0x6d, 0x62, 0x65,
	0x72, 0x12, 0x18, 0x0a, 0x07, 0x6e, 0x75, 0x6d, 0x62, 0x65, 0x72, 0x73, 0x18, 0x03, 0x20, 0x03,
	0x28, 0x02, 0x52, 0x07, 0x6e, 0x75, 0x6d, 0x62, 0x65, 0x72, 0x73, 0x22, 0x4a, 0x0a, 0x0e, 0x47,
	0x65, 0x74, 0x46, 0x6f, 0x6f, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x0e, 0x0a,
	0x02, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x02, 0x69, 0x64, 0x12, 0x12, 0x0a,
	0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d,
	0x65, 0x12, 0x14, 0x0a, 0x05, 0x66, 0x69, 0x65, 0x6c, 0x64, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x05, 0x66, 0x69, 0x65, 0x6c, 0x64, 0x22, 0xe2, 0x01, 0x0a, 0x0e, 0x50, 0x6f, 0x73, 0x74,
	0x46, 0x6f, 0x6f, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61,
	0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x14,
	0x0a, 0x05, 0x66, 0x69, 0x65, 0x6c, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x66,
	0x69, 0x65, 0x6c, 0x64, 0x12, 0x1e, 0x0a, 0x03, 0x62, 0x61, 0x72, 0x18, 0x03, 0x20, 0x01, 0x28,
	0x0b, 0x32, 0x0c, 0x2e, 0x74, 0x65, 0x73, 0x74, 0x2e, 0x76, 0x31, 0x2e, 0x42, 0x61, 0x72, 0x52,
	0x03, 0x62, 0x61, 0x72, 0x12, 0x10, 0x0a, 0x03, 0x62, 0x61, 0x7a, 0x18, 0x04, 0x20, 0x03, 0x28,
	0x09, 0x52, 0x03, 0x62, 0x61, 0x7a, 0x12, 0x20, 0x0a, 0x04, 0x62, 0x61, 0x72, 0x73, 0x18, 0x05,
	0x20, 0x03, 0x28, 0x0b, 0x32, 0x0c, 0x2e, 0x74, 0x65, 0x73, 0x74, 0x2e, 0x76, 0x31, 0x2e, 0x42,
	0x61, 0x72, 0x52, 0x04, 0x62, 0x61, 0x72, 0x73, 0x12, 0x1e, 0x0a, 0x09, 0x6f, 0x6e, 0x65, 0x73,
	0x74, 0x72, 0x69, 0x6e, 0x67, 0x18, 0x0a, 0x20, 0x01, 0x28, 0x09, 0x48, 0x00, 0x52, 0x09, 0x6f,
	0x6e, 0x65, 0x73, 0x74, 0x72, 0x69, 0x6e, 0x67, 0x12, 0x26, 0x0a, 0x06, 0x6f, 0x6e, 0x65, 0x62,
	0x61, 0x72, 0x18, 0x0b, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0c, 0x2e, 0x74, 0x65, 0x73, 0x74, 0x2e,
	0x76, 0x31, 0x2e, 0x42, 0x61, 0x72, 0x48, 0x00, 0x52, 0x06, 0x6f, 0x6e, 0x65, 0x62, 0x61, 0x72,
	0x42, 0x0a, 0x0a, 0x08, 0x74, 0x68, 0x65, 0x6f, 0x6e, 0x65, 0x6f, 0x66, 0x22, 0x3f, 0x0a, 0x03,
	0x42, 0x61, 0x72, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x02, 0x69, 0x64, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x14, 0x0a, 0x05, 0x66, 0x69, 0x65, 0x6c, 0x64,
	0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x66, 0x69, 0x65, 0x6c, 0x64, 0x22, 0x4b, 0x0a,
	0x0f, 0x50, 0x6f, 0x73, 0x74, 0x46, 0x6f, 0x6f, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x02, 0x69, 0x64,
	0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04,
	0x6e, 0x61, 0x6d, 0x65, 0x12, 0x14, 0x0a, 0x05, 0x66, 0x69, 0x65, 0x6c, 0x64, 0x18, 0x03, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x05, 0x66, 0x69, 0x65, 0x6c, 0x64, 0x22, 0x46, 0x0a, 0x0a, 0x46, 0x6f,
	0x6f, 0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x02, 0x69, 0x64, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x14, 0x0a, 0x05,
	0x66, 0x69, 0x65, 0x6c, 0x64, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x66, 0x69, 0x65,
	0x6c, 0x64, 0x32, 0xb9, 0x01, 0x0a, 0x0a, 0x46, 0x6f, 0x6f, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63,
	0x65, 0x12, 0x54, 0x0a, 0x06, 0x47, 0x65, 0x74, 0x46, 0x6f, 0x6f, 0x12, 0x16, 0x2e, 0x74, 0x65,
	0x73, 0x74, 0x2e, 0x76, 0x31, 0x2e, 0x47, 0x65, 0x74, 0x46, 0x6f, 0x6f, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x1a, 0x17, 0x2e, 0x74, 0x65, 0x73, 0x74, 0x2e, 0x76, 0x31, 0x2e, 0x47, 0x65,
	0x74, 0x46, 0x6f, 0x6f, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x19, 0x82, 0xd3,
	0xe4, 0x93, 0x02, 0x13, 0x12, 0x11, 0x2f, 0x74, 0x65, 0x73, 0x74, 0x2f, 0x76, 0x31, 0x2f, 0x66,
	0x6f, 0x6f, 0x2f, 0x7b, 0x69, 0x64, 0x7d, 0x12, 0x55, 0x0a, 0x07, 0x50, 0x6f, 0x73, 0x74, 0x46,
	0x6f, 0x6f, 0x12, 0x17, 0x2e, 0x74, 0x65, 0x73, 0x74, 0x2e, 0x76, 0x31, 0x2e, 0x50, 0x6f, 0x73,
	0x74, 0x46, 0x6f, 0x6f, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x18, 0x2e, 0x74, 0x65,
	0x73, 0x74, 0x2e, 0x76, 0x31, 0x2e, 0x50, 0x6f, 0x73, 0x74, 0x46, 0x6f, 0x6f, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x17, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x11, 0x3a, 0x01, 0x2a,
	0x22, 0x0c, 0x2f, 0x74, 0x65, 0x73, 0x74, 0x2f, 0x76, 0x31, 0x2f, 0x66, 0x6f, 0x6f, 0x32, 0x40,
	0x0a, 0x08, 0x46, 0x6f, 0x6f, 0x54, 0x6f, 0x70, 0x69, 0x63, 0x12, 0x34, 0x0a, 0x03, 0x46, 0x6f,
	0x6f, 0x12, 0x13, 0x2e, 0x74, 0x65, 0x73, 0x74, 0x2e, 0x76, 0x31, 0x2e, 0x46, 0x6f, 0x6f, 0x4d,
	0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x1a, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x22, 0x00,
	0x42, 0x3c, 0x5a, 0x3a, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x70,
	0x65, 0x6e, 0x74, 0x6f, 0x70, 0x73, 0x2f, 0x6f, 0x35, 0x2d, 0x72, 0x75, 0x6e, 0x74, 0x69, 0x6d,
	0x65, 0x2d, 0x73, 0x69, 0x64, 0x65, 0x63, 0x61, 0x72, 0x2f, 0x74, 0x65, 0x73, 0x74, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x2f, 0x67, 0x65, 0x6e, 0x2f, 0x74, 0x65, 0x73, 0x74, 0x70, 0x62, 0x62, 0x06,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_test_v1_test_proto_rawDescOnce sync.Once
	file_test_v1_test_proto_rawDescData = file_test_v1_test_proto_rawDesc
)

func file_test_v1_test_proto_rawDescGZIP() []byte {
	file_test_v1_test_proto_rawDescOnce.Do(func() {
		file_test_v1_test_proto_rawDescData = protoimpl.X.CompressGZIP(file_test_v1_test_proto_rawDescData)
	})
	return file_test_v1_test_proto_rawDescData
}

var file_test_v1_test_proto_msgTypes = make([]protoimpl.MessageInfo, 6)
var file_test_v1_test_proto_goTypes = []interface{}{
	(*GetFooRequest)(nil),   // 0: test.v1.GetFooRequest
	(*GetFooResponse)(nil),  // 1: test.v1.GetFooResponse
	(*PostFooRequest)(nil),  // 2: test.v1.PostFooRequest
	(*Bar)(nil),             // 3: test.v1.Bar
	(*PostFooResponse)(nil), // 4: test.v1.PostFooResponse
	(*FooMessage)(nil),      // 5: test.v1.FooMessage
	(*emptypb.Empty)(nil),   // 6: google.protobuf.Empty
}
var file_test_v1_test_proto_depIdxs = []int32{
	3, // 0: test.v1.PostFooRequest.bar:type_name -> test.v1.Bar
	3, // 1: test.v1.PostFooRequest.bars:type_name -> test.v1.Bar
	3, // 2: test.v1.PostFooRequest.onebar:type_name -> test.v1.Bar
	0, // 3: test.v1.FooService.GetFoo:input_type -> test.v1.GetFooRequest
	2, // 4: test.v1.FooService.PostFoo:input_type -> test.v1.PostFooRequest
	5, // 5: test.v1.FooTopic.Foo:input_type -> test.v1.FooMessage
	1, // 6: test.v1.FooService.GetFoo:output_type -> test.v1.GetFooResponse
	4, // 7: test.v1.FooService.PostFoo:output_type -> test.v1.PostFooResponse
	6, // 8: test.v1.FooTopic.Foo:output_type -> google.protobuf.Empty
	6, // [6:9] is the sub-list for method output_type
	3, // [3:6] is the sub-list for method input_type
	3, // [3:3] is the sub-list for extension type_name
	3, // [3:3] is the sub-list for extension extendee
	0, // [0:3] is the sub-list for field type_name
}

func init() { file_test_v1_test_proto_init() }
func file_test_v1_test_proto_init() {
	if File_test_v1_test_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_test_v1_test_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*GetFooRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_test_v1_test_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*GetFooResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_test_v1_test_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*PostFooRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_test_v1_test_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Bar); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_test_v1_test_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*PostFooResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_test_v1_test_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*FooMessage); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	file_test_v1_test_proto_msgTypes[2].OneofWrappers = []interface{}{
		(*PostFooRequest_Onestring)(nil),
		(*PostFooRequest_Onebar)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_test_v1_test_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   6,
			NumExtensions: 0,
			NumServices:   2,
		},
		GoTypes:           file_test_v1_test_proto_goTypes,
		DependencyIndexes: file_test_v1_test_proto_depIdxs,
		MessageInfos:      file_test_v1_test_proto_msgTypes,
	}.Build()
	File_test_v1_test_proto = out.File
	file_test_v1_test_proto_rawDesc = nil
	file_test_v1_test_proto_goTypes = nil
	file_test_v1_test_proto_depIdxs = nil
}
