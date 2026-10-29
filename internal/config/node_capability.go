package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ValidateNodeURI mirrors the node stream guard and additionally bounds the
// REALITY spider parameters consumed by Xray's random/sleep/allocation paths.
func ValidateNodeURI(field, value string, spider bool) error {
	if value == "" {
		return nil
	}
	if value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return fmt.Errorf("invalid %s URI", field)
	}
	u, err := url.Parse(value)
	if err != nil || u == nil {
		return fmt.Errorf("invalid %s URI", field)
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return fmt.Errorf("invalid %s query", field)
	}
	valid := func(s string) bool { return utf8.ValidString(s) && strings.IndexFunc(s, unicode.IsControl) < 0 }
	if !valid(u.Path) || !valid(u.Fragment) {
		return fmt.Errorf("invalid %s URI", field)
	}
	for k, values := range query {
		if !valid(k) {
			return fmt.Errorf("invalid %s query", field)
		}
		for _, v := range values {
			if !valid(v) {
				return fmt.Errorf("invalid %s query", field)
			}
		}
	}
	if spider {
		if !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || u.IsAbs() || u.Host != "" || u.Fragment != "" {
			return fmt.Errorf("invalid %s URI", field)
		}
		if len(value) > maxNodeSpiderURIBytes {
			return fmt.Errorf("invalid %s URI size", field)
		}
		if err := validateNodeSpiderQuery(query); err != nil {
			return fmt.Errorf("invalid %s: %w", field, err)
		}
	}
	return nil
}

// These limits match the same-batch node's core/streamguard/guard.go; the
// cross-repository spider fixture tests compare both implementations.
const (
	maxNodeSpiderURIBytes                 = 4096
	maxNodeSpiderQueryKeys                = 32
	maxNodeSpiderPadding           uint64 = 4096
	maxNodeSpiderConcurrency       uint64 = 8
	maxNodeSpiderRequestsPerWorker uint64 = 16
	maxNodeSpiderTotalRequests     uint64 = 64
	maxNodeSpiderIntervalMS        uint64 = 30000
	maxNodeSpiderReturnMS          uint64 = 5000
)

func validateNodeSpiderQuery(query url.Values) error {
	if len(query) > maxNodeSpiderQueryKeys {
		return fmt.Errorf("too many query parameters")
	}
	limits := map[string]uint64{"p": maxNodeSpiderPadding, "c": maxNodeSpiderConcurrency, "t": maxNodeSpiderRequestsPerWorker, "i": maxNodeSpiderIntervalMS, "r": maxNodeSpiderReturnMS}
	var concurrency, requests uint64
	for key, values := range query {
		if len(key) > 128 || len(values) != 1 || len(values[0]) > 1024 {
			return fmt.Errorf("invalid or duplicate query parameter")
		}
		for special, limit := range limits {
			if !strings.EqualFold(key, special) {
				continue
			}
			if key != special {
				return fmt.Errorf("ambiguous spider parameter name")
			}
			maximum, err := parseNodeSpiderRange(values[0], limit)
			if err != nil {
				return fmt.Errorf("invalid spider parameter %s: %w", special, err)
			}
			if special == "c" {
				concurrency = maximum
			}
			if special == "t" {
				requests = maximum
			}
		}
	}
	if concurrency*requests > maxNodeSpiderTotalRequests {
		return fmt.Errorf("spider request budget exceeded")
	}
	return nil
}

func parseNodeSpiderRange(raw string, limit uint64) (uint64, error) {
	parts := strings.Split(raw, "-")
	if len(parts) < 1 || len(parts) > 2 {
		return 0, fmt.Errorf("expected nonnegative integer or min-max")
	}
	var values [2]uint64
	for i, part := range parts {
		if len(part) == 0 || len(part) > 20 {
			return 0, fmt.Errorf("invalid numeric length")
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return 0, fmt.Errorf("invalid numeric value")
			}
		}
		v, err := strconv.ParseUint(part, 10, 64)
		if err != nil || v > limit {
			return 0, fmt.Errorf("value exceeds %d", limit)
		}
		values[i] = v
	}
	if len(parts) == 1 {
		return values[0], nil
	}
	if values[0] > values[1] {
		return 0, fmt.Errorf("reversed range")
	}
	return values[1], nil
}

// ValidateNodeStreamSpiderJSON validates REALITY spider values even when
// stream_settings or XHTTP download/extra settings override projected fields.
// The caller also validates the raw JSON object and duplicate keys.
func ValidateNodeStreamSpiderJSON(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("invalid stream JSON: %w", err)
	}
	var walk func(any, int) error
	walk = func(value any, depth int) error {
		if depth > 96 {
			return fmt.Errorf("stream JSON nesting limit exceeded")
		}
		switch v := value.(type) {
		case map[string]any:
			for key, child := range v {
				if strings.EqualFold(key, "security") {
					security, ok := child.(string)
					if !ok {
						return fmt.Errorf("stream security must be a string")
					}
					switch strings.ToLower(security) {
					case "", "none", "tls", "reality":
					default:
						return fmt.Errorf("unsupported stream security")
					}
				}
				if strings.EqualFold(key, "realitySettings") || strings.EqualFold(key, "reality_settings") {
					if child != nil {
						object, ok := child.(map[string]any)
						if !ok {
							return fmt.Errorf("realitySettings must be an object")
						}
						for name, spider := range object {
							if !strings.EqualFold(name, "spiderX") && !strings.EqualFold(name, "spider_x") {
								continue
							}
							path, ok := spider.(string)
							if !ok {
								return fmt.Errorf("realitySettings.spiderX must be a string")
							}
							if err := ValidateNodeURI("realitySettings.spiderX", path, true); err != nil {
								return err
							}
						}
					}
				}
				// Headers are a string map, not nested transport settings.
				if strings.EqualFold(key, "headers") {
					continue
				}
				if err := walk(child, depth+1); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range v {
				if err := walk(child, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(value, 0)
}

func HasNodePluginOptions(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(v) != ""
	case map[string]any:
		return len(v) != 0
	default:
		return true
	}
}
