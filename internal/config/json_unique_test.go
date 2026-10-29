package config

import "testing"

func TestValidateUniqueJSONRejectsAmbiguity(t *testing.T) {
	for _, raw := range []string{`{"a":1,"a":2}`, `{"nested":{"x":1,"x":2}}`, `{"nested":{"security":"tls","Security":"none"}}`, `{"outer":[{"K":1,"k":2}]}`, `{"a":1,"\u0061":2}`, `{"a":1} {}`, `{"s":"\ud800"}`, `{"s":"\udc00"}`, `{"s":"\ud800\u0041"}`} {
		if err := ValidateUniqueJSON([]byte(raw)); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	for _, raw := range []string{`{"s":"\ud83d\ude00"}`, `{"s":"\\ud800"}`, `{"a":[1,{"b":true}]}`} {
		if err := ValidateUniqueJSON([]byte(raw)); err != nil {
			t.Errorf("rejected %s: %v", raw, err)
		}
	}
}
