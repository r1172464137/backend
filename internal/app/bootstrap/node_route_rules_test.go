package bootstrap

import "testing"

func TestMustUnmarshalNodeConfigJSONFailsClosed(t *testing.T) {
	for _, field := range []string{"domain block", "IP block", "protocol block", "outbound"} {
		t.Run(field, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("invalid persisted JSON did not panic during bootstrap")
				}
			}()
			var target []string
			mustUnmarshalNodeConfigJSON(field, `{`, &target)
		})
	}
}
