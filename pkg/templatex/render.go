// Package templatex renders the operator-configured text templates, such as
// the SMS message templates.
package templatex

import (
	"bytes"
	"text/template"
)

// RenderToString executes the text/template tmpl with data. A template that
// fails to parse or execute is an error rather than a partial text.
func RenderToString(tmpl string, data map[string]any) (string, error) {
	t, err := template.New("tmpl").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}
