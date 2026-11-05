package conf

import (
	"fmt"
	"reflect"
	"strconv"
	"time"
)

var durationType = reflect.TypeOf(time.Duration(0))

// setDefaults sets each zero field of *v, nested structs included, to the
// value of its `default` tag. v must be a pointer to a struct. A default
// that does not fit its field, or a field of a kind defaults cannot express,
// is an error naming the field; unexported fields are left alone.
func setDefaults(v any) error {
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Pointer || val.IsNil() || val.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("conf: defaults need a pointer to a struct, got %T", v)
	}
	return setDefaultsRecursive(val.Elem(), "")
}

func setDefaultsRecursive(v reflect.Value, path string) error {
	typ := v.Type()
	for i := range v.NumField() {
		field := v.Field(i)
		fieldType := typ.Field(i)
		if !field.CanSet() {
			continue
		}
		name := path + fieldType.Name
		// if the field is a struct, set recursively
		if field.Kind() == reflect.Struct {
			if err := setDefaultsRecursive(field, name+"."); err != nil {
				return err
			}
		}
		// if the field is zero value and has default tag, set the default value
		defaultValue := fieldType.Tag.Get("default")
		if defaultValue == "" || !field.IsZero() {
			continue
		}
		if err := setDefault(field, defaultValue); err != nil {
			return fmt.Errorf("conf: default %q of %s: %w", defaultValue, name, err)
		}
	}
	return nil
}

// setDefault stores raw in field, converted to the field's kind. A
// time.Duration field takes a duration string ("30s") as well as an integer
// count of nanoseconds. Named types (type Level int) are set through their
// kind, so no value of another type is ever assigned.
func setDefault(field reflect.Value, raw string) error {
	if field.Type() == durationType {
		if d, err := time.ParseDuration(raw); err == nil {
			field.SetInt(int64(d))
			return nil
		}
	}
	switch field.Kind() {
	case reflect.String:
		field.SetString(raw)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		field.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(raw, 10, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetInt(i)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u, err := strconv.ParseUint(raw, 10, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetUint(u)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(raw, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetFloat(f)
	default:
		return fmt.Errorf("a %s field cannot take a default", field.Kind())
	}
	return nil
}
