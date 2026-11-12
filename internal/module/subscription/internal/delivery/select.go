package delivery

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/perfect-panel/server/internal/module/platform/entity/client"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

var headerLine = regexp.MustCompile(`^[A-Z][A-Z_]*=`)

var contentTypes = map[string]string{
	"base64": "text/plain; charset=utf-8",
	"yaml":   "application/yaml; charset=utf-8",
	"json":   "application/json; charset=utf-8",
	"conf":   "text/plain; charset=utf-8",
}

// selectApplication applies CONTRACT 5.1: explicit app id, then the longest
// case-insensitive UA keyword (ties to the smallest id), then the one default.
func selectApplication(apps []*client.SubscribeApplication, appParam, userAgent string) (*client.SubscribeApplication, error) {
	if appParam != "" {
		id, err := strconv.ParseInt(appParam, 10, 64)
		if err == nil && id > 0 {
			for _, a := range apps {
				if a.Id == id {
					return a, nil
				}
			}
		}
		return nil, errors.Wrap(xerr.NewErrCodeMsg(xerr.InvalidParams, "unknown subscribe application"), "app="+appParam)
	}
	ua := strings.ToLower(userAgent)
	var best, def *client.SubscribeApplication
	for _, a := range apps {
		if a.IsDefault && def == nil {
			def = a
		}
		kw := strings.ToLower(strings.TrimSpace(a.UserAgent))
		if ua == "" || kw == "" || !strings.Contains(ua, kw) {
			continue
		}
		// Stash also announces itself with other client names.
		if strings.Contains(ua, "stash") && !strings.Contains(kw, "stash") {
			continue
		}
		if best == nil || len(kw) > len(strings.TrimSpace(best.UserAgent)) || (len(kw) == len(strings.TrimSpace(best.UserAgent)) && a.Id < best.Id) {
			best = a
		}
	}
	if best != nil {
		return best, nil
	}
	if def == nil {
		return nil, errors.Wrap(xerr.NewErrCodeMsg(xerr.InvalidParams, "no subscribe application matches and no default is configured"), "no application")
	}
	return def, nil
}

// validateOutput rejects a rendered body that a client would misread
// (CONTRACT 5.3); it checks structure, not every protocol field.
func validateOutput(format string, body []byte) error {
	text := body
	if format == "base64" {
		decoded, err := base64.StdEncoding.Strict().DecodeString(string(body))
		if err != nil {
			return fmt.Errorf("base64: %w", err)
		}
		text = decoded
	}
	if len(bytes.TrimSpace(text)) == 0 {
		return errors.New("empty body")
	}
	if !utf8.Valid(text) {
		return errors.New("not UTF-8")
	}
	if bytes.Contains(text, []byte("{{")) || bytes.Contains(text, []byte("<no value>")) {
		return errors.New("unrendered template marker")
	}
	switch format {
	case "base64":
		// Share URIs plus client header lines (REMARKS=, STATUS=) and #
		// comments, as the production Default/Shadowrocket templates emit.
		links := 0
		for _, line := range strings.Split(string(text), "\n") {
			switch line = strings.TrimSpace(line); {
			case line == "" || strings.HasPrefix(line, "#") || headerLine.MatchString(line):
			case strings.Contains(line, "://"):
				links++
			default:
				return fmt.Errorf("line is not a share URI: %.40q", line)
			}
		}
		if links == 0 {
			return errors.New("no share URI")
		}
	case "yaml":
		var doc map[string]any
		if err := yaml.Unmarshal(text, &doc); err != nil || doc == nil {
			return fmt.Errorf("yaml top level is not a mapping: %v", err)
		}
	case "json":
		var doc map[string]any
		if err := json.Unmarshal(text, &doc); err != nil {
			return fmt.Errorf("json top level is not an object: %w", err)
		}
	}
	return nil
}
