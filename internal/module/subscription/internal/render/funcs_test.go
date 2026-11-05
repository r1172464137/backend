package render

import (
	"strings"
	"testing"
	"time"
)

// Client templates are admin-authored and rendered into every subscription
// response: they must not read the server environment or resolve hosts.
func TestClientBuildRejectsEnvironmentAndNetworkFunctions(t *testing.T) {
	t.Setenv("PPANEL_TEMPLATE_PROBE", "database-password")
	for name, tpl := range map[string]string{
		"env":           `{{ env "PPANEL_TEMPLATE_PROBE" }}`,
		"expandenv":     `{{ expandenv "$PPANEL_TEMPLATE_PROBE" }}`,
		"getHostByName": `{{ getHostByName "localhost" }}`,
	} {
		t.Run(name, func(t *testing.T) {
			out, err := (&Client{ClientTemplate: tpl}).Build()
			if err == nil || !strings.Contains(err.Error(), `function "`+name+`" not defined`) {
				t.Fatalf("template calling %s rendered %q, err=%v", name, out, err)
			}
		})
	}
}

// The seeded templates stamp the generation time with now and date, so the
// remaining sprig helpers must keep working.
func TestClientBuildKeepsSeededTemplateHelpers(t *testing.T) {
	before := time.Now().Format("2006")
	out, err := (&Client{
		ClientTemplate: `{{ now | date "2006" }} {{ .UserInfo.Traffic | default 0 }} {{ b64enc "x" }}`,
		UserInfo:       User{Traffic: 7},
	}).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	after := time.Now().Format("2006")
	if got := string(out); got != before+" 7 eA==" && got != after+" 7 eA==" {
		t.Fatalf("rendered %q, want %q", got, after+" 7 eA==")
	}
}
