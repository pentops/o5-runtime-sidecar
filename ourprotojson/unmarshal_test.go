package ourprotojson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/pentops/o5-runtime-sidecar/testproto/gen/testpb"
)

func TestUnmarshal(t *testing.T) {

	testTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		name      string
		wantProto proto.Message
		json      string
	}{
		{
			name: "scalars",
			json: `{
				"sString": "nameVal",
				"oString": "otherNameVal",
				"rString": ["r1", "r2"],
				"sFloat": 1.1,
				"oFloat": 2.2,
				"rFloat": [3.3, 4.4],
				"enum": "VALUE1",
				"rEnum": ["VALUE1", "VALUE2"]
			}`,
			wantProto: &testpb.PostFooRequest{
				SString: "nameVal",
				OString: proto.String("otherNameVal"),
				RString: []string{"r1", "r2"},

				SFloat: 1.1,
				OFloat: proto.Float32(2.2),
				RFloat: []float32{3.3, 4.4},

				Enum: testpb.Enum_ENUM_VALUE1,
				REnum: []testpb.Enum{
					testpb.Enum_ENUM_VALUE1,
					testpb.Enum_ENUM_VALUE2,
				},
			},
		}, {
			name: "well known types",
			json: `{
				"ts": "2020-01-01T00:00:00Z",
				"rTs": ["2020-01-01T00:00:00Z", "2020-01-01T00:00:00Z"]
			}`,
			wantProto: &testpb.PostFooRequest{
				Ts: timestamppb.New(testTime),
				RTs: []*timestamppb.Timestamp{
					timestamppb.New(testTime),
					timestamppb.New(testTime),
				},
			},
		}, {
			name: "nested messages",
			json: `{
				"sBar": {
					"id": "barId",
					"name": "barName",
					"field": "barField"
				},
				"rBars": [{
					"id": "bar1"
				}, {
					"id": "bar2"
				}]
			}`,
			wantProto: &testpb.PostFooRequest{
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
			},
		}} {
		t.Run(tc.name, func(t *testing.T) {

			msg := &testpb.PostFooRequest{}
			if err := Decode([]byte(tc.json), msg.ProtoReflect()); err != nil {
				t.Fatal(err)
			}

			fmt.Printf("msg: %v\n", protojson.Format(msg))

			if !proto.Equal(tc.wantProto, msg) {
				a := protojson.Format(tc.wantProto)
				b := protojson.Format(msg)
				t.Fatalf("expected \n%v but got\n%v", string(a), string(b))
			}

			encoded, err := Encode(msg.ProtoReflect())
			if err != nil {
				t.Fatal(err)
			}

			aa := &bytes.Buffer{}
			json.Indent(aa, encoded, "", "  ")

			bb := &bytes.Buffer{}
			json.Indent(bb, []byte(tc.json), "", "  ")

			as := aa.String()
			bs := bb.String()

			if as != bs {
				t.Log("JSON Mismatch:")
				linesA := strings.Split(as, "\n")
				linesB := strings.Split(bs, "\n")
				for i, lineA := range linesA {
					lineB := ""
					if i < len(linesB) {
						lineB = linesB[i]
					}

					if lineA != lineB {
						t.Logf("BAD \nA: %s\n!: %s\n", lineA, lineB)
					} else {

						t.Logf("OK  \nA: %s\nB: %s\n", lineA, lineB)
					}

				}

				t.Fatalf("JSON Mismatch")

			}

		})
	}
}
