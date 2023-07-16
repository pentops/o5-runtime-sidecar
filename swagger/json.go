package swagger

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

type Optional[T any] struct {
	Value T
	Set   bool
}

func (o Optional[T]) ValOk() (interface{}, bool) {
	return o.Value, o.Set
}

func Value[T any](val T) Optional[T] {
	return Optional[T]{
		Value: val,
		Set:   true,
	}
}

func jsonFieldMap(object interface{}, m map[string]json.RawMessage) error {

	val := reflect.ValueOf(object)
	if val.Kind() != reflect.Struct {
		return fmt.Errorf("object must be a struct, got %s", val.Kind().String())
	}

	for i := 0; i < val.NumField(); i++ {
		field := val.Type().Field(i)
		if field.Anonymous {
			err := jsonFieldMap(val.Field(i).Interface(), m)
			if err != nil {
				return fmt.Errorf("anon field %s: %w", field.Name, err)
			}
			continue
		}

		tag := field.Tag.Get("json")
		if tag == "" {
			// maybe map lower case?
			continue
		}
		parts := strings.Split(tag, ",")
		name := parts[0]
		if name == "-" {
			continue
		}
		omitempty := false
		for _, part := range parts[1:] {
			if part == "omitempty" {
				omitempty = true
			}
		}

		if omitempty && val.Field(i).IsZero() {
			continue
		}
		iv := val.Field(i).Interface()
		if optional, ok := iv.(interface{ ValOk() (interface{}, bool) }); ok {
			val, isSet := optional.ValOk()
			if !isSet {
				continue
			}
			iv = val
		}

		asJSON, err := json.Marshal(iv)
		if err != nil {
			return err
		}

		fmt.Printf("FIELD %s: %T %s\n", name, iv, asJSON)

		m[name] = json.RawMessage(asJSON)
	}

	return nil
}

type MapItem interface {
	MapKey() string
}

type OrderedMap[T MapItem] []T

func (om OrderedMap[T]) MarshalJSON() ([]byte, error) {
	fmt.Printf("MM")
	fields := make([]string, len(om))
	for idx, field := range om {
		val, err := json.Marshal(field)
		if err != nil {
			return nil, err
		}
		keyString := field.MapKey()
		key, _ := json.Marshal(keyString)
		fields[idx] = string(key) + ":" + string(val)
	}
	outStr := "{" + strings.Join(fields, ",") + "}"
	return []byte(outStr), nil
}
