package ourprotojson

import (
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/pentops/o5-runtime-sidecar/testproto/gen/testpb"
)

func TestUnmarshal(t *testing.T) {

	testTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	inputProto := &testpb.PostFooRequest{
		SString: "nameVal",
		OString: proto.String("otherNameVal"),
		RString: []string{"r1", "r2"},

		SFloat: 1.1,
		OFloat: proto.Float32(2.2),
		RFloat: []float32{3.3, 4.4},

		Ts: timestamppb.New(testTime),
		RTs: []*timestamppb.Timestamp{
			timestamppb.New(testTime),
			timestamppb.New(testTime),
		},

		Enum: testpb.Enum_ENUM_VALUE1,
		REnum: []testpb.Enum{
			testpb.Enum_ENUM_VALUE1,
			testpb.Enum_ENUM_VALUE2,
		},

		SBar: &testpb.Bar{
			Id:    "barId",
			Name:  "barName",
			Field: "barField",
		},
		RBars: []*testpb.Bar{{
			Id: "bar1",
		}, {
			Id: "bar2",
		}},
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
