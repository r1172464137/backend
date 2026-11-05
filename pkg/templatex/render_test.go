package templatex

import "testing"

func TestRenderToString(t *testing.T) {
	got, err := RenderToString("hello {{.Name}}, code {{.code}}", map[string]any{
		"Name": "world",
		"code": 123456,
	})
	if err != nil {
		t.Fatalf("RenderToString() error = %v", err)
	}
	if want := "hello world, code 123456"; got != want {
		t.Fatalf("RenderToString() = %q, want %q", got, want)
	}
}

// A missing key renders as "<no value>", as text/template does for maps.
func TestRenderToStringMissingKey(t *testing.T) {
	got, err := RenderToString("code {{.code}}", map[string]any{})
	if err != nil || got != "code <no value>" {
		t.Fatalf("RenderToString() = %q, %v", got, err)
	}
}

func TestRenderToStringReportsTemplateErrors(t *testing.T) {
	for _, tmpl := range []string{"{{.code", "{{template \"missing\"}}", "{{.code.Nested}}"} {
		got, err := RenderToString(tmpl, map[string]any{"code": "1"})
		if err == nil || got != "" {
			t.Fatalf("RenderToString(%q) = %q, %v; want an error and no text", tmpl, got, err)
		}
	}
}
