package swagger

import (
	"encoding/json"
	"fmt"
	"strconv"
)

type Document struct {
	OpenAPI    string     `json:"openapi"`
	Info       Info       `json:"info"`
	Paths      PathSet    `json:"paths"`
	Components Components `json:"components"`
}

type Info struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Version     string `json:"version"`
}

type PathSet []PathItem

func (ps PathSet) MarshalJSON() ([]byte, error) {
	return OrderedMap[PathItem](ps).MarshalJSON()
}

type PathItem []Operation

func (pi PathItem) MarshalJSON() ([]byte, error) {
	return OrderedMap[Operation](pi).MarshalJSON()
}

func (pi PathItem) MapKey() string {
	if len(pi) == 0 {
		return ""
	}
	return pi[0].Path
}

type Components struct {
	Schemas         map[string]SchemaItem  `json:"schemas"`
	SecuritySchemes map[string]interface{} `json:"securitySchemes"`
}

type OperationHeader struct {
	Method string `json:"-"`
	Path   string `json:"-"`

	OperationID  string      `json:"operationId,omitempty"`
	Summary      string      `json:"summary,omitempty"`
	Description  string      `json:"description,omitempty"`
	DisplayOrder int         `json:"x-display-order"`
	Parameters   []Parameter `json:"parameters,omitempty"`
}

type Operation struct {
	OperationHeader
	Request   *SchemaItem `json:"request,omitempty"`
	Responses ResponseSet `json:"responses,omitempty"`
}

type ResponseSet []Response

func (rs ResponseSet) MarshalJSON() ([]byte, error) {
	return OrderedMap[Response](rs).MarshalJSON()
}

type Response struct {
	Code        int         `json:"-"`
	Description string      `json:"description"`
	Content     interface{} `json:"content"`
}

func (rs Response) MapKey() string {
	return strconv.Itoa(rs.Code)
}

func (oo Operation) MapKey() string {
	return oo.Method
}

type Parameter struct {
	Name        string     `json:"name"`
	In          string     `json:"in"`
	Description string     `json:"description,omitempty"`
	Required    bool       `json:"required,omitempty"`
	Schema      SchemaItem `json:"schema"`
}

type ItemType interface {
	TypeName() string
}

type SchemaItem struct {
	Ref   string
	OneOf []SchemaItem
	AnyOf []SchemaItem

	ItemType
	Description string `json:"description,omitempty"`
	Mutex       string `json:"x-mutex,omitempty"`
	EventSubkey string `json:"x-event-subkey,omitempty"`
}

func (si SchemaItem) MarshalJSON() ([]byte, error) {
	if si.Ref != "" {
		if len(si.OneOf) > 0 || len(si.AnyOf) > 0 || si.ItemType != nil {
			return nil, fmt.Errorf("schema item has both a ref and other properties")
		}
		return json.Marshal(map[string]interface{}{
			"$ref": si.Ref,
		})
	}

	if len(si.OneOf) > 0 {
		if len(si.AnyOf) > 0 || si.ItemType != nil {
			return nil, fmt.Errorf("schema item has both oneOf and other properties")
		}
		return json.Marshal(map[string]interface{}{
			"oneOf": si.OneOf,
		})
	}

	if len(si.AnyOf) > 0 {
		if len(si.AnyOf) > 0 || si.ItemType != nil {
			return nil, fmt.Errorf("schema item has both a anyOf and other properties")
		}
		return json.Marshal(map[string]interface{}{
			"oneOf": si.OneOf,
		})
	}

	if si.ItemType == nil {
		return nil, fmt.Errorf("schema item has no type")
	}

	propOut := map[string]json.RawMessage{}
	if err := jsonFieldMap(si, propOut); err != nil {
		return nil, err
	}
	propOut["type"], _ = json.Marshal(si.TypeName())
	return json.Marshal(propOut)
}

type StringItem struct {
	StringRules
}

func (ri StringItem) TypeName() string {
	return "string"
}

type StringRules struct {
	Format    string  `json:"format,omitempty"`
	Pattern   string  `json:"pattern,omitempty"`
	MinLength *uint64 `json:"minLength,omitempty"`
	MaxLength *uint64 `json:"maxLength,omitempty"`
}

// EnumItem represents a PROTO enum in Swagger, so can only be a string
type EnumItem struct {
	EnumRules
}

func (ri EnumItem) TypeName() string {
	return "string"
}

type EnumRules struct {
	Enum []string `json:"enum,omitempty"`
}

type NumberItem struct {
	Format string `json:"format,omitempty"`
	NumberRules
}

func (ri NumberItem) TypeName() string {
	return "number"
}

type NumberRules struct {
	ExclusiveMaximum Optional[float64] `json:"exclusiveMaximum,omitempty"`
	ExclusiveMinimum Optional[float64] `json:"exclusiveMinimum,omitempty"`
	Minimum          Optional[float64] `json:"minimum,omitempty"`
	Maximum          Optional[float64] `json:"maximum,omitempty"`
	MultipleOf       Optional[float64] `json:"multipleOf,omitempty"`
}

type IntegerItem struct {
	Format string `json:"format,omitempty"`
	IntegerRules
}

func (ri IntegerItem) TypeName() string {
	return "integer"
}

type IntegerRules struct {
	ExclusiveMaximum Optional[int64] `json:"exclusiveMaximum,omitempty"`
	ExclusiveMinimum Optional[int64] `json:"exclusiveMinimum,omitempty"`
	Minimum          Optional[int64] `json:"minimum,omitempty"`
	Maximum          Optional[int64] `json:"maximum,omitempty"`
	MultipleOf       Optional[int64] `json:"multipleOf,omitempty"`
}

type BooleanItem struct {
	BooleanRules
}

func (ri BooleanItem) TypeName() string {
	return "boolean"
}

type BooleanRules struct {
}

type ArrayItem struct {
	ArrayRules
	Items SchemaItem `json:"items,omitempty"`
}

func (ri ArrayItem) TypeName() string {
	return "array"
}

type ArrayRules struct {
	MinItems    Optional[uint64] `json:"minItems,omitempty"`
	MaxItems    Optional[uint64] `json:"maxItems,omitempty"`
	UniqueItems Optional[bool]   `json:"uniqueItems,omitempty"`
}

type ObjectItem struct {
	ObjectRules
	Properties       map[string]ObjectProperty `json:"properties,omitempty"`
	Required         []string                  `json:"required,omitempty"`
	ProtoMessageName string                    `json:"x-message"`
	debug            string
}

func (ri ObjectItem) TypeName() string {
	return "object"
}

func (op ObjectItem) MarshalJSON() ([]byte, error) {
	properties := map[string]json.RawMessage{}
	required := []string{}
	for name, prop := range op.Properties {
		jsonVal, err := json.Marshal(prop)
		if err != nil {
			return nil, fmt.Errorf("property %s: %w", name, err)
		}
		properties[name] = jsonVal

		if prop.Required {
			required = append(required, name)
		}
	}

	out := map[string]interface{}{
		"type":       "object",
		"properties": properties,
		"x-debug":    op.debug,
	}
	if len(required) > 0 {
		out["required"] = required
	}

	return json.Marshal(out)
}

type ObjectProperty struct {
	SchemaItem
	Required  bool `json:"-"` // this bubbles up to the required array of the object
	ReadOnly  bool `json:"readOnly,omitempty"`
	WriteOnly bool `json:"writeOnly,omitempty"`
}

type ObjectRules struct {
	MinProperties Optional[uint64] `json:"minProperties,omitempty"`
	MaxProperties Optional[uint64] `json:"maxProperties,omitempty"`
}
