package ourprotojson

import (
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func unexpectedTokenError(got, expected interface{}) error {
	return fmt.Errorf("unexpected token %v, expected %v", got, expected)
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

type marshalFunc func(*encoder, protoreflect.Message) error

type unmarshalFunc func(*decoder, protoreflect.Message) error

// wellKnownTypeMarshaler returns a marshal function if the message type
// has specialized serialization behavior. It returns nil otherwise.
func wellKnownTypeMarshaler(name protoreflect.FullName) marshalFunc {
	if name.Parent() == WKTProtoNamespace {
		switch name.Name() {
		//case WKTAny:
		//	return marshalAny
		case WKTTimestamp:
			return marshalTimestamp
		//case WKTDuration:
		//	return marshalDuration
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
			return marshalWrapperType
		case WKTEmpty:
			return marshalEmpty
		}
	}
	return nil
}

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

func marshalTimestamp(enc *encoder, msg protoreflect.Message) error {
	seconds := msg.Get(msg.Descriptor().Fields().ByName("seconds")).Int()
	nanos := msg.Get(msg.Descriptor().Fields().ByName("nanos")).Int()
	t := time.Unix(seconds, nanos).In(time.UTC)

	return enc.addJSON(t.Format(time.RFC3339Nano))
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

func marshalEmpty(e *encoder, msg protoreflect.Message) error {
	return e.addJSON("{}")
}

func marshalWrapperType(e *encoder, msg protoreflect.Message) error {
	fd := msg.Descriptor().Fields().ByName("value")
	val := msg.Get(fd)
	return e.encodeValue(fd, val)
}
