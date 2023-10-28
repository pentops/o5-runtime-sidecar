package ourprotojson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func Encode(msg protoreflect.Message) ([]byte, error) {
	enc := &encoder{
		b: &bytes.Buffer{},
	}
	if err := enc.encodeMessage(msg); err != nil {
		return nil, err

	}
	return enc.b.Bytes(), nil
}

type encoder struct {
	b *bytes.Buffer
}

func (enc *encoder) add(b []byte) {
	enc.b.Write(b)
}

// addJSON is a shortcut for actually writing the marshal code for scalars
func (enc *encoder) addJSON(v interface{}) error {
	jv, err := json.Marshal(v)
	if err != nil {
		return err
	}
	enc.add(jv)
	return nil
}

func (enc *encoder) encodeMessage(msg protoreflect.Message) error {

	wktEncoder := wellKnownTypeMarshaler(msg.Descriptor().FullName())
	if wktEncoder != nil {
		return wktEncoder(enc, msg)
	}

	enc.add([]byte("{"))

	first := true
	var outerError error

	msg.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if !first {
			enc.add([]byte(","))
		}
		first = false

		enc.add([]byte("\""))
		enc.add([]byte(field.JSONName()))
		enc.add([]byte("\":"))

		if err := enc.encodeField(field, value); err != nil {
			outerError = err
			return false
		}

		return true
	})

	if outerError != nil {
		return outerError
	}

	enc.add([]byte("}"))
	return nil
}

func (enc *encoder) encodeField(field protoreflect.FieldDescriptor, value protoreflect.Value) error {
	if field.IsList() {
		enc.add([]byte("["))
		first := true
		var outerError error
		list := value.List()
		for i := 0; i < list.Len(); i++ {
			if !first {
				enc.add([]byte(","))
			}
			first = false
			if err := enc.encodeValue(field, value.List().Get(i)); err != nil {
				return err
			}
		}

		if outerError != nil {
			return outerError
		}
		enc.add([]byte("]"))
		return nil
	}

	return enc.encodeValue(field, value)
}

func (enc *encoder) encodeValue(field protoreflect.FieldDescriptor, value protoreflect.Value) error {

	switch field.Kind() {
	case protoreflect.MessageKind:
		return enc.encodeMessage(value.Message())

	case protoreflect.StringKind:
		return enc.addJSON(value.String())

	case protoreflect.BoolKind:
		return enc.addJSON(value.Bool())

	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return enc.addJSON(int32(value.Int()))

	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return enc.addJSON(int64(value.Int()))

	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return enc.addJSON(uint32(value.Int()))

	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return enc.addJSON(uint64(value.Int()))

	case protoreflect.FloatKind:
		return enc.addJSON(float32(value.Float()))

	case protoreflect.DoubleKind:
		return enc.addJSON(float64(value.Float()))

	case protoreflect.BytesKind:
		return enc.addJSON(value.Bytes())

	case protoreflect.EnumKind:
		enumVals := field.Enum().Values()
		unspecifiedField := enumVals.ByNumber(0)
		specifiedField := enumVals.ByNumber(value.Enum())
		returnName := string(specifiedField.Name())

		if unspecifiedField != nil {
			unspecifiedName := string(unspecifiedField.Name())
			if strings.HasSuffix(unspecifiedName, "_UNSPECIFIED") {
				unspecifiedPrefix := strings.TrimSuffix(unspecifiedName, "_UNSPECIFIED")
				returnName = strings.TrimPrefix(returnName, unspecifiedPrefix+"_")
			}
		}

		return enc.addJSON(returnName)

	default:
		return fmt.Errorf("unsupported kind %v", field.Kind())

	}

}
