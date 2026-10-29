package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ValidateUniqueJSON rejects duplicate and case-fold-colliding object keys at
// every depth and trailing values. It is used before decoding policies and offline legacy inputs so
// ambiguous JSON cannot be normalized into an apparently valid candidate.
func ValidateUniqueJSON(data []byte) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("JSON is not UTF-8")
	}
	if err := validateJSONSurrogates(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := scanJSONValue(decoder, 0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON data")
	}
	return nil
}

// encoding/json otherwise replaces unpaired UTF-16 escapes with U+FFFD.
func validateJSONSurrogates(data []byte) error {
	inside := false
	for i := 0; i < len(data); i++ {
		switch data[i] {
		case '"':
			inside = !inside
		case '\\':
			if !inside {
				continue
			}
			i++
			if i >= len(data) {
				return fmt.Errorf("invalid JSON escape")
			}
			if data[i] != 'u' {
				continue
			}
			if i+4 >= len(data) {
				return fmt.Errorf("invalid Unicode escape")
			}
			read := func(at int) (uint16, bool) {
				var v uint16
				for j := 0; j < 4; j++ {
					c := data[at+j]
					v <<= 4
					switch {
					case c >= '0' && c <= '9':
						v += uint16(c - '0')
					case c >= 'a' && c <= 'f':
						v += uint16(c - 'a' + 10)
					case c >= 'A' && c <= 'F':
						v += uint16(c - 'A' + 10)
					default:
						return 0, false
					}
				}
				return v, true
			}
			v, ok := read(i + 1)
			if !ok {
				return fmt.Errorf("invalid Unicode escape")
			}
			i += 4
			if v >= 0xd800 && v <= 0xdbff {
				if i+6 >= len(data) || data[i+1] != '\\' || data[i+2] != 'u' {
					return fmt.Errorf("unpaired Unicode surrogate")
				}
				low, ok := read(i + 3)
				if !ok || low < 0xdc00 || low > 0xdfff {
					return fmt.Errorf("unpaired Unicode surrogate")
				}
				i += 6
			} else if v >= 0xdc00 && v <= 0xdfff {
				return fmt.Errorf("unpaired Unicode surrogate")
			}
		}
	}
	return nil
}

func validateRoutingJSONFields(data []byte) error {
	fields, err := exactJSONFields(data, reflect.TypeOf(NodeRouting{}))
	if err != nil {
		return err
	}
	for _, part := range []struct {
		name string
		typ  reflect.Type
	}{{"outbounds", reflect.TypeOf(NodeOutbound{})}, {"rules", reflect.TypeOf(NodeRoute{})}} {
		raw, ok := fields[part.name]
		if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			continue
		}
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return fmt.Errorf("%s must be an array", part.name)
		}
		for i, item := range items {
			if _, err := exactJSONFields(item, part.typ); err != nil {
				return fmt.Errorf("%s[%d]: %w", part.name, i, err)
			}
		}
	}
	return nil
}
func exactJSONFields(data []byte, typ reflect.Type) (map[string]json.RawMessage, error) {
	if trimmed := bytes.TrimSpace(data); len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, fmt.Errorf("expected JSON object")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	allowed := map[string]struct{}{}
	for i := 0; i < typ.NumField(); i++ {
		tag := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if tag != "" && tag != "-" {
			allowed[tag] = struct{}{}
		}
	}
	for field := range fields {
		if _, ok := allowed[field]; !ok {
			return nil, fmt.Errorf("unknown JSON field %q", field)
		}
	}
	return fields, nil
}

// encoding/json matches struct fields using Unicode simple folding. Use the
// same canonical fold to reject ambiguous keys before any decoding occurs.
func foldJSONName(key string) string {
	var b strings.Builder
	for _, r := range key {
		for {
			next := unicode.SimpleFold(r)
			if next <= r {
				r = next
				break
			}
			r = next
		}
		b.WriteRune(r)
	}
	return b.String()
}

func scanJSONValue(decoder *json.Decoder, depth int) error {
	// Reserve envelope nesting for the node decoder's 100-level limit.
	if depth > 96 {
		return fmt.Errorf("JSON nesting limit exceeded")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("invalid object key")
			}
			folded := foldJSONName(key)
			if _, exists := seen[folded]; exists {
				return fmt.Errorf("duplicate or case-fold-colliding JSON field")
			}
			seen[folded] = struct{}{}
			if err := scanJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return fmt.Errorf("invalid JSON object")
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return fmt.Errorf("invalid JSON array")
		}
	default:
		return fmt.Errorf("invalid JSON delimiter")
	}
	return nil
}
