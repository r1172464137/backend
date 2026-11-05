// Package protocolkey preserves the key and short-identifier encodings used
// by external protocols. These compatibility transforms are not token issuers.
package protocolkey

import (
	"crypto/md5" //nolint:gosec // G501: the providers' signing protocols prescribe MD5; not used to protect anything here
	"encoding/hex"
	"strings"
)

// Md5Encode is retained for providers whose signing protocol requires MD5.
func Md5Encode(value string, upper bool) string {
	sum := md5.Sum([]byte(value)) //nolint:gosec // G401: see the import note
	encoded := hex.EncodeToString(sum[:])
	if upper {
		return strings.ToUpper(encoded)
	}
	return encoded
}
