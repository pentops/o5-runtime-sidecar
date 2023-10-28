package ourprotojson

import (
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func (dec *decoder) decodeScalarField(field protoreflect.FieldDescriptor) (protoreflect.Value, error) {
	tok, err := dec.Token()
	if err != nil {
		return protoreflect.Value{}, err
	}

	switch field.Kind() {
	case protoreflect.StringKind:
		str, ok := tok.(string)
		if !ok {
			return protoreflect.Value{}, fmt.Errorf("expected string but got %v", tok)
		}
		return protoreflect.ValueOfString(str), nil

	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		i, ok := tok.(json.Number)
		if !ok {
			return protoreflect.Value{}, fmt.Errorf("expected int32 but got %v", tok)
		}
		intVal, err := i.Int64()
		if err != nil {
			return protoreflect.Value{}, err
		}

		return protoreflect.ValueOfInt32(int32(intVal)), nil

	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		i, ok := tok.(json.Number)
		if !ok {
			return protoreflect.Value{}, fmt.Errorf("expected int64 but got %v", tok)
		}
		intVal, err := i.Int64()
		if err != nil {
			return protoreflect.Value{}, err
		}

		return protoreflect.ValueOfInt64(int64(intVal)), nil

	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		i, ok := tok.(json.Number)
		if !ok {
			return protoreflect.Value{}, fmt.Errorf("expected uint32 but got %v", tok)
		}
		intVal, err := i.Int64()
		if err != nil {
			return protoreflect.Value{}, err
		}

		return protoreflect.ValueOfUint32(uint32(intVal)), nil

	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		i, ok := tok.(json.Number)
		if !ok {
			return protoreflect.Value{}, fmt.Errorf("expected uint64 but got %v", tok)
		}
		intVal, err := i.Int64()
		if err != nil {
			return protoreflect.Value{}, err
		}

		return protoreflect.ValueOfUint64(uint64(intVal)), nil

	case protoreflect.FloatKind:
		f, ok := tok.(json.Number)
		if !ok {
			return protoreflect.Value{}, fmt.Errorf("expected float but got %v", tok)
		}
		floatVal, err := f.Float64()
		if err != nil {
			return protoreflect.Value{}, err
		}

		return protoreflect.ValueOfFloat32(float32(floatVal)), nil

	case protoreflect.DoubleKind:
		f, ok := tok.(json.Number)
		if !ok {
			return protoreflect.Value{}, fmt.Errorf("expected double but got %v", tok)
		}
		floatVal, err := f.Float64()
		if err != nil {
			return protoreflect.Value{}, err
		}

		return protoreflect.ValueOfFloat64(floatVal), nil

	case protoreflect.EnumKind:
		stringVal, ok := tok.(string)
		if !ok {
			return protoreflect.Value{}, unexpectedTokenError(tok, "string")
		}
		enumValue, err := decodeEnumField(stringVal, field)
		if err != nil {
			return protoreflect.Value{}, err
		}

		return protoreflect.ValueOfEnum(enumValue), nil

	default:
		return protoreflect.Value{}, fmt.Errorf("unsupported field kind %v", field.Kind())
	}
}

func decodeEnumField(stringVal string, field protoreflect.FieldDescriptor) (protoreflect.EnumNumber, error) {

	enum := field.Enum()
	vals := enum.Values()
	unspecified := vals.ByNumber(0)
	if unspecified != nil {
		unspecifiedName := string(unspecified.Name())
		if strings.HasSuffix(unspecifiedName, "_UNSPECIFIED") {
			prefix := strings.TrimSuffix(unspecifiedName, "_UNSPECIFIED")
			fmt.Printf("prefix: %s, string %s\n", prefix, stringVal)
			if !strings.HasPrefix(stringVal, prefix) {
				stringVal = prefix + "_" + stringVal
			}
		}
	}
	enumVal := vals.ByName(protoreflect.Name(stringVal))
	if enumVal == nil {
		return 0, fmt.Errorf("unknown enum value %s for enum %s", stringVal, enum.FullName())
	}

	return enumVal.Number(), nil
}
