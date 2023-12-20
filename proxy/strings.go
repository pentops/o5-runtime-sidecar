package proxy

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/pentops/jsonapi/jsonapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func int32Mapper(fd protoreflect.FieldDescriptor, s string) (protoreflect.Value, error) {
	val, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return protoreflect.Value{}, err
	}
	return protoreflect.ValueOfInt32(int32(val)), nil
}

func uint32Mapper(fd protoreflect.FieldDescriptor, s string) (protoreflect.Value, error) {
	val, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return protoreflect.Value{}, err
	}
	return protoreflect.ValueOfUint32(uint32(val)), nil
}

func int64Mapper(fd protoreflect.FieldDescriptor, s string) (protoreflect.Value, error) {
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return protoreflect.Value{}, err
	}
	return protoreflect.ValueOfInt64(val), nil
}

func uint64Mapper(fd protoreflect.FieldDescriptor, s string) (protoreflect.Value, error) {
	val, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return protoreflect.Value{}, err
	}
	return protoreflect.ValueOfUint64(val), nil
}

type mapperFromString func(protoreflect.FieldDescriptor, string) (protoreflect.Value, error)

var mappersFromString = map[protoreflect.Kind]mapperFromString{
	// Signed Int
	protoreflect.Int32Kind: int32Mapper,
	protoreflect.Int64Kind: int64Mapper,
	// Insigned Int
	protoreflect.Uint32Kind: uint32Mapper,
	protoreflect.Uint64Kind: uint64Mapper,
	// Signed Int - the other type but go doesn't care
	protoreflect.Sint32Kind: int32Mapper,
	protoreflect.Sint64Kind: int64Mapper,
	// Signed Fixed uses the whole byte space, again the same in Go
	protoreflect.Sfixed32Kind: int32Mapper,
	protoreflect.Sfixed64Kind: int64Mapper,
	// Unsigned Fixed uses the whole byte space, again the same in Go
	protoreflect.Fixed32Kind: uint32Mapper,
	protoreflect.Fixed64Kind: uint64Mapper,

	protoreflect.StringKind: func(fd protoreflect.FieldDescriptor, s string) (protoreflect.Value, error) {
		return protoreflect.ValueOfString(s), nil
	},
	protoreflect.BoolKind: func(fd protoreflect.FieldDescriptor, s string) (protoreflect.Value, error) {
		return protoreflect.ValueOfBool(s == "true"), nil
	},
	protoreflect.FloatKind: func(fd protoreflect.FieldDescriptor, s string) (protoreflect.Value, error) {
		val, err := strconv.ParseFloat(s, 32)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfFloat32(float32(val)), nil
	},
	protoreflect.DoubleKind: func(fd protoreflect.FieldDescriptor, s string) (protoreflect.Value, error) {
		val, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfFloat64(val), nil
	},
	protoreflect.BytesKind: func(fd protoreflect.FieldDescriptor, s string) (protoreflect.Value, error) {
		val, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfBytes(val), nil
	},
	protoreflect.EnumKind: func(fd protoreflect.FieldDescriptor, s string) (protoreflect.Value, error) {
		enumValue := fd.Enum().Values().ByName(protoreflect.Name(s))
		if enumValue == nil {
			return protoreflect.Value{}, fmt.Errorf("invalid enum value %q for field %q", s, fd.Name())
		}
		return protoreflect.ValueOfEnum(enumValue.Number()), nil
	},
}

func setFieldFromString(codec jsonapi.Options, inputMessage protoreflect.Message, fd protoreflect.FieldDescriptor, provided string) error {

	if fd.Kind() == protoreflect.MessageKind {
		msg := inputMessage.NewField(fd).Message()
		if err := jsonapi.Decode(codec, []byte(provided), msg); err != nil {
			return err
		}
		inputMessage.Set(fd, protoreflect.ValueOfMessage(msg))
		return nil
	}

	tc, ok := mappersFromString[fd.Kind()]
	if !ok {
		return fmt.Errorf("unsupported field type %s", fd.Kind())
	}

	val, err := tc(fd, provided)
	if err != nil {
		return err
	}

	inputMessage.Set(fd, val)
	return nil
}

func setFieldFromStrings(codec jsonapi.Options, inputMessage protoreflect.Message, key string, provided []string) error {

	parts := strings.SplitN(key, ".", 2)
	if len(parts) > 1 {
		fd := inputMessage.Descriptor().Fields().ByName(protoreflect.Name(parts[0]))
		if fd == nil {
			return status.Error(codes.InvalidArgument, fmt.Sprintf("unknown obj query parameter %q", parts[0]))
		}
		if fd.Kind() != protoreflect.MessageKind {
			return status.Error(codes.InvalidArgument, fmt.Sprintf("unknown query parameter %q (%q is not an object)", parts, parts[0]))
		}

		if !inputMessage.Has(fd) {
			msgVal := inputMessage.NewField(fd)
			inputMessage.Set(fd, msgVal)
		}
		msgVal := inputMessage.Get(fd).Message()
		return setFieldFromStrings(codec, msgVal, parts[1], provided)
	}

	fd := inputMessage.Descriptor().Fields().ByName(protoreflect.Name(key))
	if fd == nil {
		return status.Error(codes.InvalidArgument, fmt.Sprintf("unknown query parameter %q", key))
	}

	if !fd.IsList() {
		if len(provided) > 1 {
			return fmt.Errorf("multiple values provided for non-repeated field %q", fd.Name())
		}
		return setFieldFromString(codec, inputMessage, fd, provided[0])
	}

	tc, ok := mappersFromString[fd.Kind()]
	if !ok {
		return fmt.Errorf("unsupported field type %s", fd.Kind())
	}

	field := inputMessage.NewField(fd)
	list := field.List()

	for _, s := range provided {
		val, err := tc(fd, s)
		if err != nil {
			return err
		}
		list.Append(val)
	}

	inputMessage.Set(fd, field)

	return nil
}
