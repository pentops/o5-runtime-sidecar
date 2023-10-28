package ourprotojson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func Decode(jsonData []byte, msg protoreflect.Message) error {
	dec := json.NewDecoder(bytes.NewReader(jsonData))
	dec.UseNumber()
	d2 := &decoder{Decoder: dec}
	return d2.decodeMessage(msg)
}

type decoder struct {
	*json.Decoder
	next json.Token
}

func (d *decoder) Peek() (json.Token, error) {
	if d.next != nil {
		return nil, fmt.Errorf("unexpected call to Peek after Peek")
	}

	tok, err := d.Token()
	if err != nil {
		return nil, err
	}

	d.next = tok
	return tok, nil
}

func (d *decoder) Token() (json.Token, error) {
	if d.next != nil {
		tok := d.next
		d.next = nil
		return tok, nil
	}

	return d.Decoder.Token()
}

func (dec *decoder) decodeMessage(msg protoreflect.Message) error {
	wktDecoder := wellKnownTypeUnmarshaler(msg.Descriptor().FullName())
	if wktDecoder != nil {
		return wktDecoder(dec, msg)
	}

	tok, err := dec.Token()
	if err != nil {
		return err
	}

	if tok != json.Delim('{') {
		return fmt.Errorf("expected '{' but got %v", tok)
	}

	for {
		keyToken, err := dec.Token()
		if err != nil {
			return err
		}

		// Ends the object
		if keyToken == json.Delim('}') {
			return nil
		}

		// Otherwise should be a key
		keyTokenStr, ok := keyToken.(string)
		if !ok {
			return fmt.Errorf("expected string key but got %v", keyToken)
		}

		protoField := msg.Descriptor().Fields().ByJSONName(keyTokenStr)
		if protoField == nil {
			return fmt.Errorf("no such field %s", keyTokenStr)
		}

		switch protoField.Cardinality() {
		case protoreflect.Repeated:
			if err := dec.decodeRepeatedField(msg, protoField); err != nil {
				return err
			}
		case protoreflect.Optional, protoreflect.Required:
			if err := dec.decodeField(msg, protoField); err != nil {
				return err
			}

		default:
			return fmt.Errorf("unsupported cardinality %v", protoField.Cardinality())
		}

	}

}

func (dec *decoder) decodeField(msg protoreflect.Message, field protoreflect.FieldDescriptor) error {
	switch field.Kind() {
	case protoreflect.MessageKind:
		return dec.decodeMessageField(msg, field)

	default:
		scalarVal, err := dec.decodeScalarField(field)
		if err != nil {
			return err
		}
		msg.Set(field, scalarVal)
	}
	return nil
}

func (dec *decoder) decodeRepeatedField(msg protoreflect.Message, field protoreflect.FieldDescriptor) error {

	tok, err := dec.Token()
	if err != nil {
		return err
	}

	if tok != json.Delim('[') {
		return fmt.Errorf("expected '[' but got %v", tok)
	}

	kind := field.Kind()
	list := msg.Mutable(field).List()

	for {
		if !dec.More() {
			_, err := dec.Token()
			if err != nil {
				return err
			}
			break
		}

		switch kind {
		case protoreflect.MessageKind:
			subMsg := list.NewElement()
			if err := dec.decodeMessage(subMsg.Message()); err != nil {
				return err
			}
			list.Append(subMsg)

		default:

			value, err := dec.decodeScalarField(field)
			if err != nil {
				return err
			}
			list.Append(value)
		}

	}

	msg.Set(field, protoreflect.ValueOf(list))
	return nil
}

func (dec *decoder) decodeMessageField(msg protoreflect.Message, field protoreflect.FieldDescriptor) error {

	subMsg := msg.Mutable(field).Message()
	if err := dec.decodeMessage(subMsg); err != nil {
		return err
	}

	return nil
}

const (
	WKTProtoNamespace = "google.protobuf"
	WKTAny            = "Any"
	WKTTimestamp      = "Timestamp"
	WKTDuration       = "Duration"

	WKTBool   = "BoolValue"
	WKTInt32  = "Int32Value"
	WKTInt64  = "Int64Value"
	WKTUInt32 = "UInt32Value"
	WKTUInt64 = "UInt64Value"
	WKTFloat  = "FloatValue"
	WKTDouble = "DoubleValue"
	WKTString = "StringValue"
	WKTBytes  = "BytesValue"

	WKTEmpty = "Empty"
)

type unmarshalFunc func(*decoder, protoreflect.Message) error

// wellKnownTypeUnmarshaler returns a unmarshal function if the message type
// has specialized serialization behavior. It returns nil otherwise.
func wellKnownTypeUnmarshaler(name protoreflect.FullName) unmarshalFunc {
	if name.Parent() == WKTProtoNamespace {
		switch name.Name() {
		//case WKTAny:
		//	return unmarshalAny
		case WKTTimestamp:
			return unmarshalTimestamp
		//case WKTDuration:
		//	return unmarshalDuration
		case
			WKTBool,
			WKTInt32,
			WKTInt64,
			WKTUInt32,
			WKTUInt64,
			WKTFloat,
			WKTDouble,
			WKTString,
			WKTBytes:
			return unmarshalWrapperType
		case WKTEmpty:
			return unmarshalEmpty
		}
	}
	return nil
}

func unexpectedTokenError(got, expected interface{}) error {
	return fmt.Errorf("unexpected token %v, expected %v", got, expected)
}

func unmarshalTimestamp(dec *decoder, msg protoreflect.Message) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}

	stringVal, ok := tok.(string)
	if !ok {
		return fmt.Errorf("expected string but got %v", tok)
	}

	t, err := time.Parse(time.RFC3339Nano, stringVal)
	if err != nil {
		return err
	}

	msg.Set(msg.Descriptor().Fields().ByName("seconds"), protoreflect.ValueOf(t.Unix()))
	msg.Set(msg.Descriptor().Fields().ByName("nanos"), protoreflect.ValueOf(int32(t.Nanosecond())))
	return nil
}

func unmarshalWrapperType(dec *decoder, m protoreflect.Message) error {
	fd := m.Descriptor().Fields().ByName("value")
	val, err := dec.decodeScalarField(fd)
	if err != nil {
		return err
	}
	m.Set(fd, val)
	return nil
}

func unmarshalEmpty(d *decoder, msg protoreflect.Message) error {
	tok, err := d.Token()
	if err != nil {
		return err
	}
	if tok != json.Delim('{') {
		return unexpectedTokenError(tok, "{")
	}
	tok, err = d.Token()
	if err != nil {
		return err
	}
	if tok != json.Delim('}') {
		return unexpectedTokenError(tok, "}")
	}
	return nil
}
