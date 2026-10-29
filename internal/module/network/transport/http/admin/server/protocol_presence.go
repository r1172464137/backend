package server

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Each submitted listener must explicitly state whether it is enabled. This
// prevents missing/null booleans from becoming a silent disabled listener.
func requireProtocolEnablePresence(body []byte, required bool) error {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(body, &top); err != nil {
		return err
	}
	raw, ok := top["protocols"]
	if !ok {
		if required {
			return fmt.Errorf("protocols must be explicitly supplied")
		}
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("protocols must be an array")
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return fmt.Errorf("protocols must be an array")
	}
	for i, entry := range entries {
		raw, ok := entry["enable"]
		var enabled *bool
		if !ok || json.Unmarshal(raw, &enabled) != nil || enabled == nil {
			return fmt.Errorf("protocol[%d] requires explicit enable", i+1)
		}
	}
	return nil
}
