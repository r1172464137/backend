package systemsetting

import (
	"context"
	"reflect"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/repository"
)

type configFieldValue struct {
	key       string
	value     string
	valueType string
}

func convertedConfigFields(data any) []configFieldValue {
	return configFields(data, config.ConvertValueToString)
}

func stringConfigFields(data any) []configFieldValue {
	return configFields(data, func(value reflect.Value) string {
		return value.String()
	})
}

func configFields(data any, valueFn func(reflect.Value) string) []configFieldValue {
	v := reflect.ValueOf(data)
	t := v.Type()
	fields := make([]configFieldValue, 0, v.NumField())
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		structField := t.Field(i)
		if structField.PkgPath != "" || structField.Tag.Get("json") == "-" {
			continue
		}
		fields = append(fields, configFieldValue{
			key:       structField.Name,
			value:     valueFn(field),
			valueType: configFieldType(field),
		})
	}
	return fields
}

func configFieldType(value reflect.Value) string {
	if value.Kind() == reflect.Ptr {
		value = reflect.New(value.Type().Elem()).Elem()
	}
	switch value.Kind() {
	case reflect.Bool:
		return "bool"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "int64"
	case reflect.Interface, reflect.Map, reflect.Slice, reflect.Struct:
		return "interface"
	default:
		return "string"
	}
}

func updateConfigFields(ctx context.Context, deps Deps, category string, fields []configFieldValue) error {
	return deps.Store.InPlatformTx(ctx, func(store repository.PlatformStore) error {
		return updateConfigFieldsWithStore(ctx, store.System(), category, fields)
	})
}

func updateConfigFieldsWithStore(ctx context.Context, systemStore repository.SystemRepo, category string, fields []configFieldValue) error {
	for _, field := range fields {
		if err := systemStore.UpdateValueByCategoryKey(ctx, category, field.key, field.value, field.valueType); err != nil {
			return err
		}
	}
	return nil
}
