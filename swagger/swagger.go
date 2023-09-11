package swagger

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type Document struct {
	OpenAPI    string     `json:"openapi"`
	Info       Info       `json:"info"`
	Paths      PathSet    `json:"paths"`
	Components Components `json:"components"`
}

func (dd *Document) GetSchema(name string) (*SchemaItem, bool) {
	name = strings.TrimPrefix(name, "#/components/schemas/")
	schema, ok := dd.Components.Schemas[name]
	return schema, ok
}

type Info struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Version     string `json:"version"`
}

type PathSet []*PathItem

func (ps PathSet) MarshalJSON() ([]byte, error) {
	return OrderedMap[*PathItem](ps).MarshalJSON()
}

type PathItem []*Operation

func (pi PathItem) MarshalJSON() ([]byte, error) {
	return OrderedMap[*Operation](pi).MarshalJSON()
}

func (pi *PathItem) AddOperation(op *Operation) {
	*pi = append(*pi, op)
}

func (pi PathItem) MapKey() string {
	if len(pi) == 0 {
		return ""
	}
	return pi[0].Path
}

type Components struct {
	Schemas         map[string]*SchemaItem `json:"schemas"`
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
	RequestBody *RequestBody `json:"requestBody,omitempty"`
	Responses   *ResponseSet `json:"responses,omitempty"`
}

type ResponseSet []Response

func (rs ResponseSet) MarshalJSON() ([]byte, error) {
	return OrderedMap[Response](rs).MarshalJSON()
}

type RequestBody struct {
	Description string           `json:"description,omitempty"`
	Required    bool             `json:"required,omitempty"`
	Content     OperationContent `json:"content"`
}
type Response struct {
	Code        int              `json:"-"`
	Description string           `json:"description"`
	Content     OperationContent `json:"content"`
}

type OperationContent struct {
	JSON *OperationSchema `json:"application/json,omitempty"`
}

type OperationSchema struct {
	Schema SchemaItem `json:"schema"`
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
		if len(si.OneOf) > 0 || si.ItemType != nil {
			return nil, fmt.Errorf("schema item has both a anyOf and other properties")
		}
		return json.Marshal(map[string]interface{}{
			"anyOf": si.AnyOf,
		})
	}

	if si.ItemType == nil {
		return nil, fmt.Errorf("schema item has no type")
	}

	propOut := map[string]json.RawMessage{}
	if err := jsonFieldMap(si.ItemType, propOut); err != nil {
		return nil, err
	}
	propOut["type"], _ = json.Marshal(si.TypeName())
	if si.Description != "" {
		propOut["description"], _ = json.Marshal(si.Description)
	}

	return json.Marshal(propOut)
}

type StringItem struct {
	Format string `json:"format,omitempty"`
	StringRules
}

func (ri StringItem) TypeName() string {
	return "string"
}

type StringRules struct {
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
	ExclusiveMaximum Optional[bool]    `json:"exclusiveMaximum,omitempty"`
	ExclusiveMinimum Optional[bool]    `json:"exclusiveMinimum,omitempty"`
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
	ExclusiveMaximum Optional[bool]  `json:"exclusiveMaximum,omitempty"`
	ExclusiveMinimum Optional[bool]  `json:"exclusiveMinimum,omitempty"`
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
	Properties       []*ObjectProperty `json:"properties,omitempty"`
	Required         []string          `json:"required,omitempty"`
	ProtoMessageName string            `json:"x-message"`
	debug            string
}

func (ri ObjectItem) TypeName() string {
	return "object"
}

func (op ObjectItem) GetProperty(name string) (*ObjectProperty, bool) {
	for _, prop := range op.Properties {
		if prop.ProtoFieldName == name {
			return prop, true
		}
	}
	return nil, false
}

func (op ObjectItem) jsonFieldMap(out map[string]json.RawMessage) error {
	properties := map[string]json.RawMessage{}
	required := []string{}
	for _, prop := range op.Properties {
		if prop.Skip {
			continue
		}

		jsonVal, err := prop.MarshalJSON()
		if err != nil {
			return fmt.Errorf("property %s: %w", prop.Name, err)
		}
		properties[prop.Name] = jsonVal

		if prop.Required {
			required = append(required, prop.Name)
		}
	}
	out["properties"], _ = json.Marshal(properties)

	if len(required) > 0 {
		out["required"], _ = json.Marshal(required)
	}
	return nil
}

type ObjectProperty struct {
	SchemaItem
	Skip             bool   `json:"-"`
	Name             string `json:"-"`
	Required         bool   `json:"-"` // this bubbles up to the required array of the object
	ReadOnly         bool   `json:"readOnly,omitempty"`
	WriteOnly        bool   `json:"writeOnly,omitempty"`
	Description      string `json:"description,omitempty"`
	ProtoFieldName   string `json:"x-proto-name,omitempty"`
	ProtoFieldNumber int    `json:"x-proto-number,omitempty"`
}

type ObjectRules struct {
	MinProperties Optional[uint64] `json:"minProperties,omitempty"`
	MaxProperties Optional[uint64] `json:"maxProperties,omitempty"`
}
