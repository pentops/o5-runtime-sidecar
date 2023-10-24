package ourprotojson

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func (dec *decoder) decodeScalarField(kind protoreflect.Kind) (protoreflect.Value, error) {

	switch kind {
	/*
		case protoreflect.BoolKind:
			return decodeBoolField(dec, msg, field)
	*/
	case protoreflect.StringKind:
		return dec.decodeStringField()
		/*
			case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
				return decodeInt32Field(dec, msg, field)
			case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
				return decodeInt64Field(dec, msg, field)
			case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
				return decodeUint32Field(dec, msg, field)
			case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
				return decodeUint64Field(dec, msg, field)
			case protoreflect.FloatKind:
				return decodeFloat32Field(dec, msg, field)
			case protoreflect.DoubleKind:
				return decodeFloat64Field(dec, msg, field)
			case protoreflect.BytesKind:
				return decodeBytesField(dec, msg, field)
			case protoreflect.EnumKind:
				return decodeEnumField(dec, msg, field)
		*/
	default:
		return protoreflect.Value{}, fmt.Errorf("unsupported field kind %v", kind)
	}
}

func (dec *decoder) decodeStringField() (protoreflect.Value, error) {
	tok, err := dec.Token()
	if err != nil {
		return protoreflect.Value{}, err
	}

	str, ok := tok.(string)
	if !ok {
		return protoreflect.Value{}, fmt.Errorf("expected string but got %v", tok)
	}

	return protoreflect.ValueOfString(str), nil
}
