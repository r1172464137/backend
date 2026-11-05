package telegram

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// getAuthDataCheckString returns a string ready to calculate validation hash.
// Telegram signs every field it actually sent, so the check string is built
// from the raw payload rather than the known struct fields: an unrecognised
// field would otherwise be dropped here and the hash would never match.
// Ref: https://core.telegram.org/widgets/login#checking-authorization
func getAuthDataCheckString(raw map[string]any) string {
	fields := make([]string, 0, len(raw))
	for key, value := range raw {
		if key == "hash" {
			continue
		}
		// A JSON null carries no value, and the login widget is known to
		// serialise absent optional fields as the literal string "null";
		// Telegram excludes both from its own check string.
		if value == nil {
			continue
		}
		formatted := formatCheckValue(value)
		if formatted == "null" {
			continue
		}
		fields = append(fields, key+"="+formatted)
	}
	sort.Strings(fields)
	return strings.Join(fields, "\n")
}

// formatCheckValue renders a JSON value the way Telegram had it before
// signing. json.Unmarshal decodes every number into float64, so integral
// values must not be printed in scientific notation.
func formatCheckValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// computeHash returns a hash calculated for the raw auth payload.
// Ref: https://core.telegram.org/widgets/login#checking-authorization
func computeHash(raw map[string]any, botToken []byte) string {
	checkString := getAuthDataCheckString(raw)
	key := sha256.Sum256(botToken)
	h := hmac.New(sha256.New, key[:])
	h.Write([]byte(checkString))
	return hex.EncodeToString(h.Sum(nil))
}
