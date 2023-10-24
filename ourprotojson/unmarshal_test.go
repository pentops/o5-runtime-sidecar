package ourprotojson

import (
	"fmt"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/pentops/o5-runtime-sidecar/testproto/gen/testpb"
)

func TestUnmarshal(t *testing.T) {

	inputProto := &testpb.PostFooRequest{
		Name:  "nameVal",
		Field: "otherName",
		Bar: &testpb.Bar{
			Name: "barName",
		},
		Baz: []string{"baz1", "baz2"},
		Bars: []*testpb.Bar{
			{
				Name: "bar1",
			},
			{
				Name: "bar2",
			},
		},
	}

	jsonInput, err := protojson.Marshal(inputProto)
	if err != nil {
		t.Fatal(err)
	}

	msg := &testpb.PostFooRequest{}
	if err := Decode(jsonInput, msg.ProtoReflect()); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("msg: %v\n", protojson.Format(msg))

	if !proto.Equal(inputProto, msg) {
		a := protojson.Format(inputProto)
		b := protojson.Format(msg)
		t.Fatalf("expected \n%v but got\n%v", string(a), string(b))
	}

}
