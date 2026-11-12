package application

import (
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

// outputFormats is the closed set of delivery formats (CONTRACT 5.2).
var outputFormats = map[string]bool{"base64": true, "yaml": true, "json": true, "conf": true}

func validateOutputFormat(format string) error {
	if !outputFormats[format] {
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidParams), "output_format must be one of base64, yaml, json, conf; got %q", format)
	}
	return nil
}

// saveError keeps the single-default rejection visible to the administrator.
func saveError(err error, code uint32, msg string) error {
	if errors.Is(err, repository.ErrDefaultApplicationRequired) {
		return errors.Wrap(xerr.NewErrCodeMsg(xerr.InvalidParams, "exactly one subscribe application must be the default; set another default first"), err.Error())
	}
	return errors.Wrap(xerr.NewErrCode(code), msg)
}
