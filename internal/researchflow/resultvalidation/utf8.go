package resultvalidation

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"
	"unicode/utf8"
)

var (
	timeValueType = reflect.TypeOf(time.Time{})
	rawJSONType   = reflect.TypeOf(json.RawMessage{})
)

// validateUTF8Value rejects invalid UTF-8 before encoding/json can silently
// replace malformed string bytes with the Unicode replacement character.
func validateUTF8Value(value any) error {
	return validateUTF8Reflect(reflect.ValueOf(value), "input")
}

func validateUTF8Reflect(value reflect.Value, path string) error {
	if !value.IsValid() {
		return nil
	}
	if value.Type() == timeValueType {
		return nil
	}
	if value.Type() == rawJSONType {
		if !utf8.Valid(value.Bytes()) {
			return fmt.Errorf("%s contains invalid UTF-8 JSON bytes", path)
		}
		return nil
	}

	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if value.IsNil() {
			return nil
		}
		return validateUTF8Reflect(value.Elem(), path)
	case reflect.String:
		if !utf8.ValidString(value.String()) {
			return fmt.Errorf("%s contains invalid UTF-8", path)
		}
	case reflect.Struct:
		for index := 0; index < value.NumField(); index++ {
			field := value.Field(index)
			if !field.CanInterface() {
				continue
			}
			if err := validateUTF8Reflect(field, path+"."+value.Type().Field(index).Name); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for index := 0; index < value.Len(); index++ {
			if err := validateUTF8Reflect(value.Index(index), fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	case reflect.Map:
		iterator := value.MapRange()
		for iterator.Next() {
			key := iterator.Key()
			if key.Kind() == reflect.String && !utf8.ValidString(key.String()) {
				return fmt.Errorf("%s contains an invalid UTF-8 map key", path)
			}
			if err := validateUTF8Reflect(iterator.Value(), path+"["+fmt.Sprint(key.Interface())+"]"); err != nil {
				return err
			}
		}
	}
	return nil
}
