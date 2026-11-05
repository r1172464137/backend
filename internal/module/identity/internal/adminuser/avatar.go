package adminuser

import (
	"encoding/base64"
	"strings"
)

// IsValidImageSize reports whether base64Str, a Base64 image with or without
// a data URL prefix, decodes and is at most maxSizeKB kilobytes.
func IsValidImageSize(base64Str string, maxSizeKB int64) bool {
	if base64Str == "" || maxSizeKB < 0 {
		return false
	}

	// Strip a data URL prefix such as "data:image/png;base64,".
	data := base64Str
	if idx := strings.Index(base64Str, ","); idx != -1 {
		data = base64Str[idx+1:]
	}

	// Base64 encodes 3 bytes in 4 characters, so the decoded size is about
	// three quarters of the input; an image already too large by that
	// estimate is refused without decoding it.
	approxSizeBytes := int64(len(data)) * 3 / 4
	approxSizeKB := approxSizeBytes / 1024
	if approxSizeKB > maxSizeKB {
		return false
	}

	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return false
	}
	return int64(len(decoded))/1024 <= maxSizeKB
}
