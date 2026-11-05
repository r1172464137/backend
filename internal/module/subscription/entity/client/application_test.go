package client

import (
	"reflect"
	"testing"
)

func TestDefaultParamValues(t *testing.T) {
	cases := []struct {
		name    string
		stored  string
		want    map[string]string
		wantErr bool
	}{
		{name: "unset", stored: "", want: nil},
		{name: "single", stored: "mode=rule", want: map[string]string{"mode": "rule"}},
		{name: "several", stored: "mode=rule&emoji=1", want: map[string]string{"mode": "rule", "emoji": "1"}},
		{name: "valueless key", stored: "emoji", want: map[string]string{"emoji": ""}},
		{name: "escaped value", stored: "name=a%20b", want: map[string]string{"name": "a b"}},
		// The subscription URL's own query string keeps the first value of a
		// repeated key, so the stored defaults have to agree.
		{name: "repeated key keeps first", stored: "mode=rule&mode=global", want: map[string]string{"mode": "rule"}},
		{name: "malformed escape", stored: "mode=%zz", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := &SubscribeApplication{DefaultParams: tc.stored}
			got, err := app.DefaultParamValues()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("DefaultParamValues(%q) error = nil, want an error", tc.stored)
				}
				if got != nil {
					t.Fatalf("DefaultParamValues(%q) = %v, want nil alongside the error", tc.stored, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("DefaultParamValues(%q) error = %v", tc.stored, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("DefaultParamValues(%q) = %v, want %v", tc.stored, got, tc.want)
			}
		})
	}
}

func TestDefaultParamValuesNilReceiver(t *testing.T) {
	var app *SubscribeApplication
	got, err := app.DefaultParamValues()
	if err != nil || got != nil {
		t.Fatalf("DefaultParamValues() = (%v, %v), want (nil, nil)", got, err)
	}
}
