// Package validation owns request-DTO validation independently from the
// application composition root. Transport handlers can depend on this package
// without receiving the application composition root.
package validation

import (
	"reflect"

	"errors"

	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	enTranslations "github.com/go-playground/validator/v10/translations/en"
)

var engine, translator = newEngine()

func newEngine() (*validator.Validate, ut.Translator) {
	enUS := en.New()
	engine := validator.New()
	engine.RegisterTagNameFunc(func(field reflect.StructField) string {
		return field.Tag.Get("label")
	})

	uni := ut.New(enUS)
	translator, _ := uni.GetTranslator("en")
	_ = enTranslations.RegisterDefaultTranslations(engine, translator)
	return engine, translator
}

// Validate checks a request DTO using the process-wide immutable validator.
// validator.Validate is safe for concurrent use after registrations finish.
func Validate(value any) error {
	if err := engine.Struct(value); err != nil {
		var validationErrors validator.ValidationErrors
		if !errors.As(err, &validationErrors) || len(validationErrors) == 0 {
			return err
		}
		return errors.New(validationErrors[0].Translate(translator))
	}
	return nil
}
