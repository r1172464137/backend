package mail

import (
	"bytes"
	"strings"
	"testing"
	"text/template"
)

// render executes a default template the way the email task does: with
// text/template, over the data the producer queued, decoded from JSON. The
// whitespace is collapsed, since the templates wrap their sentences.
func render(t *testing.T, text string, data map[string]any) string {
	t.Helper()
	tpl, err := template.New("email").Parse(text)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var result bytes.Buffer
	if err := tpl.Execute(&result, data); err != nil {
		t.Fatalf("execute: %v", err)
	}
	return strings.Join(strings.Fields(result.String()), " ")
}

// The verification mail names the code's purpose: the email task hands the
// template the purpose as a uint8, and JSON decoded the expiry as a float64.
func TestDefaultVerifyTemplateNamesTheCodesPurpose(t *testing.T) {
	for _, tt := range []struct {
		purpose     uint8
		want, avoid string
	}{
		{purpose: 1, want: "Registration Verification Code", avoid: "Password Reset Verification Code"},
		{purpose: 2, want: "Password Reset Verification Code", avoid: "Registration Verification Code"},
	} {
		got := render(t, DefaultEmailVerifyTemplate, map[string]any{
			"Type": tt.purpose, "SiteLogo": "https://panel.example.com/logo.png", "SiteName": "Perfect Panel",
			"Expire": float64(5), "Code": "481516",
		})
		for _, want := range []string{tt.want, "481516", "Perfect Panel", "https://panel.example.com/logo.png", ">5</span"} {
			if !strings.Contains(got, want) {
				t.Errorf("purpose %d: the mail lacks %q", tt.purpose, want)
			}
		}
		if strings.Contains(got, tt.avoid) {
			t.Errorf("purpose %d: the mail says %q", tt.purpose, tt.avoid)
		}
	}
}

// The notices render the values their producers queue.
func TestDefaultNoticeTemplatesRenderTheirData(t *testing.T) {
	site := map[string]any{"SiteLogo": "https://panel.example.com/logo.png", "SiteName": "Perfect Panel"}
	with := func(extra map[string]any) map[string]any {
		data := map[string]any{}
		for key, value := range site {
			data[key] = value
		}
		for key, value := range extra {
			data[key] = value
		}
		return data
	}
	for name, tt := range map[string]struct {
		text string
		data map[string]any
		want []string
	}{
		"maintenance": {
			text: DefaultMaintenanceEmailTemplate,
			data: with(map[string]any{"MaintenanceDate": "2026-10-01", "MaintenanceTime": "02:00-04:00"}),
			want: []string{"2026-10-01", "02:00-04:00"},
		},
		"expiration": {
			text: DefaultExpirationEmailTemplate,
			data: with(map[string]any{"ExpireDate": "2026-10-01 12:00:00"}),
			want: []string{"2026-10-01 12:00:00"},
		},
		"traffic exceeded": {text: DefaultTrafficExceedEmailTemplate, data: with(nil)},
	} {
		got := render(t, tt.text, tt.data)
		for _, want := range append(tt.want, "Perfect Panel", "https://panel.example.com/logo.png") {
			if !strings.Contains(got, want) {
				t.Errorf("%s: the mail lacks %q", name, want)
			}
		}
	}
}
