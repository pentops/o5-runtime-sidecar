package swagger

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestPathMap(t *testing.T) {

	dd := Document{
		Paths: PathSet{
			PathItem{
				Operation{
					OperationHeader: OperationHeader{
						Method:      "get",
						Path:        "/foo",
						OperationID: "test",
					},
				},
			},
		},
	}

	out, err := MarshalDynamic(dd)
	if err != nil {
		t.Fatal(err)
	}

	out.Print(t)

	out.AssertEqual(t, "paths./foo.get.operationId", "test")

}

func TestSchema(t *testing.T) {

	object := ObjectItem{
		debug: "a",
		Properties: map[string]ObjectProperty{
			"id": {
				SchemaItem: SchemaItem{
					Description: "desc",
					ItemType: StringItem{
						StringRules: StringRules{
							Format: "uuid",
						},
					},
				},
				Required: true,
			},
			"number": {
				SchemaItem: SchemaItem{
					ItemType: NumberItem{
						Format: "double",
						NumberRules: NumberRules{
							Minimum: Value(0.0),
							Maximum: Value(100.0),
						},
					},
				},
			},
			"object": {
				SchemaItem: SchemaItem{
					ItemType: ObjectItem{
						debug: "b",
						Properties: map[string]ObjectProperty{
							"foo": {
								Required: true,
								SchemaItem: SchemaItem{
									ItemType: StringItem{},
								},
							},
						},
					},
				},
			},
			"ref": {
				SchemaItem: SchemaItem{
					Ref: "#/definitions/foo",
				},
			},
			"oneof": {
				SchemaItem: SchemaItem{
					OneOf: []SchemaItem{
						{
							ItemType: StringItem{},
						},
						{
							Ref: "#/foo/bar",
						},
					},
				},
			},
		},
	}

	out, err := MarshalDynamic(object)
	if err != nil {
		t.Error(err)
	}

	out.Print(t)
	out.AssertEqual(t, "type", "object")
	out.AssertEqual(t, "properties.id.type", "string")
	out.AssertEqual(t, "properties.id.format", "uuid")
	out.AssertEqual(t, "properties.id.description", "desc")
	out.AssertEqual(t, "required.0", "id")

	out.AssertEqual(t, "properties.number.type", "number")
	out.AssertEqual(t, "properties.number.format", "double")
	out.AssertEqual(t, "properties.number.minimum", 0.0)
	out.AssertEqual(t, "properties.number.maximum", 100.0)
	out.AssertNotSet(t, "properties.number.exclusiveMinimum")

	out.AssertEqual(t, "properties.object.properties.foo.type", "string")

	out.AssertEqual(t, "properties.ref.$ref", "#/definitions/foo")

}

type DynamicJSON struct {
	JSON map[string]interface{}
}

func MarshalDynamic(v interface{}) (*DynamicJSON, error) {
	val, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	out := map[string]interface{}{}
	if err := json.Unmarshal(val, &out); err != nil {
		return nil, err
	}
	return &DynamicJSON{JSON: out}, nil
}

func (d *DynamicJSON) Print(t testing.TB) {
	out, err := json.MarshalIndent(d.JSON, "", "  ")
	if err != nil {
		t.Error(err)
	}
	t.Log(string(out))
}

func (d *DynamicJSON) Get(path string) (interface{}, bool) {
	parts := strings.Split(path, ".")
	return get(parts, d.JSON)
}

func get(path []string, object interface{}) (interface{}, bool) {
	if len(path) == 0 {
		return object, true
	}

	head, tail := path[0], path[1:]

	switch ot := object.(type) {
	case map[string]interface{}:
		property, ok := ot[head]
		if !ok {
			return nil, false
		}
		return get(tail, property)
	case []interface{}:
		index, err := strconv.Atoi(head)
		if err != nil {
			return nil, false
		}
		if index < 0 || index >= len(ot) {
			return nil, false
		}
		property := ot[index]
		return get(tail, property)
	default:
		return nil, false
	}
}

func (d *DynamicJSON) AssertEqual(t testing.TB, path string, value interface{}) {
	actual, ok := d.Get(path)
	if !ok {
		t.Errorf("path %q not found", path)
		return
	}

	if !reflect.DeepEqual(actual, value) {
		t.Errorf("expected %q, got %q", value, actual)
	}
}

func (d *DynamicJSON) AssertNotSet(t testing.TB, path string) {
	_, ok := d.Get(path)
	if ok {
		t.Errorf("path %q was set", path)
	}
}
