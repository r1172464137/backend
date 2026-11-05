// Package mapping copies between the entity, contract and DTO structs of the
// same data by field name, with the conversions the API needs: a time.Time
// becomes int64 Unix milliseconds, the form the JSON responses carry. It
// exists so that every copy applies the same policy.
package mapping

import (
	"errors"
	"fmt"
	"time"

	"github.com/jinzhu/copier"
)

// Copy deep-copies the fields of src into dst, a pointer, by name; a
// time.Time source field fills an int64 destination with its Unix
// milliseconds, and zero source fields are copied too. A failure (mismatched
// types) leaves dst partly filled and is returned.
func Copy(dst, src any) error {
	if err := copier.CopyWithOption(dst, src, copyOption); err != nil {
		return fmt.Errorf("copy %T into %T: %w", src, dst, err)
	}
	return nil
}

// copyOption is the copy policy every mapping shares.
var copyOption = copier.Option{
	DeepCopy: true,
	Converters: []copier.TypeConverter{
		{
			SrcType: time.Time{},
			DstType: int64(0),
			Fn: func(src any) (any, error) {
				s, ok := src.(time.Time)
				if !ok {
					return nil, errors.New("src type not matching")
				}
				return s.UnixMilli(), nil
			},
		},
	},
}
