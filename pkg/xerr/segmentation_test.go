package xerr

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// legacyCodes are the error codes that predate the domain-band convention.
// They are FROZEN: clients switch on the numeric values, so renumbering or
// removing one is a breaking change, and this set must never grow — new
// codes go into their owning module's band (see err_code.go).
var legacyCodes = map[uint32]bool{
	200: true, 500: true, 400: true, 401: true,
	10001: true, 10002: true, 10003: true, 10004: true,
	20001: true, 20002: true, 20003: true, 20004: true, 20005: true,
	20006: true, 20007: true, 20008: true, 20009: true, 20010: true,
	30001: true, 30002: true, 30003: true, 30004: true, 30005: true,
	40002: true, 40003: true, 40004: true, 40005: true, 40006: true, 40007: true,
	50001: true, 50002: true, 50003: true, 50004: true, 50005: true, 50006: true,
	60001: true, 60002: true, 60003: true, 60004: true, 60005: true, 60006: true, 60007: true,
	61001: true, 61002: true, 61003: true, 61004: true, 61005: true,
	70001: true, 80001: true,
	90001: true, 90002: true, 90003: true, 90004: true, 90005: true, 90006: true,
	90007: true, 90008: true, 90009: true, 90010: true, 90011: true, 90012: true,
	90013: true, 90014: true, 90015: true, 90017: true, 90018: true,
}

// legacyWithoutMessage are frozen codes that historically have no entry in
// the message map (MapErrMsg falls back to "Internal Server Error"). Kept
// as-is to avoid changing API responses; the set may only shrink.
var legacyWithoutMessage = map[uint32]bool{
	UserCommissionNotEnough: true, // 20010
	ExistAvailableTraffic:   true, // 61005
	SendSmsError:            true, // 90002
	AreaCodeIsEmpty:         true, // 90009
}

var bands = map[string]uint32{
	"BandShared":       BandShared,
	"BandIdentity":     BandIdentity,
	"BandBilling":      BandBilling,
	"BandSubscription": BandSubscription,
	"BandNetwork":      BandNetwork,
	"BandSupport":      BandSupport,
	"BandPlatform":     BandPlatform,
	"BandNotification": BandNotification,
}

// collectCodes parses err_code.go and returns every declared error-code
// constant (name -> value), excluding the band markers themselves.
func collectCodes(t *testing.T) map[string]uint32 {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "err_code.go", nil, 0)
	if err != nil {
		t.Fatalf("parse err_code.go: %v", err)
	}
	codes := make(map[string]uint32)
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, name := range vs.Names {
				if _, isBand := bands[name.Name]; isBand || name.Name == "bandWidth" {
					continue
				}
				if i >= len(vs.Values) {
					t.Fatalf("%s: error codes must be assigned explicit literal values", name.Name)
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.INT {
					t.Fatalf("%s: error codes must be integer literals", name.Name)
				}
				v, err := strconv.ParseUint(lit.Value, 10, 32)
				if err != nil {
					t.Fatalf("%s: %v", name.Name, err)
				}
				codes[name.Name] = uint32(v)
			}
		}
	}
	if len(codes) == 0 {
		t.Fatal("no error codes found — parser out of sync with err_code.go")
	}
	return codes
}

// TestErrorCodeSegmentation enforces the domain-band convention: values are
// unique, the frozen legacy set neither grows nor shrinks, and every new
// code lives inside exactly one module band and has a message.
func TestErrorCodeSegmentation(t *testing.T) {
	codes := collectCodes(t)

	seen := make(map[uint32]string)
	legacySeen := make(map[uint32]bool)
	for name, value := range codes {
		if prev, dup := seen[value]; dup {
			t.Errorf("duplicate error code %d: %s and %s", value, prev, name)
		}
		seen[value] = name

		if legacyCodes[value] {
			legacySeen[value] = true
			continue
		}
		inBand := false
		for bandName, base := range bands {
			if value >= base && value < base+bandWidth {
				inBand = true
				_ = bandName
				break
			}
		}
		if !inBand {
			t.Errorf("%s = %d: new error codes must be allocated inside a domain band (see err_code.go); the legacy set is frozen", name, value)
		}
		if _, ok := message[value]; !ok {
			t.Errorf("%s = %d: new error codes must have a message in err_msg.go", name, value)
		}
	}

	for value := range legacyCodes {
		if !legacySeen[value] {
			t.Errorf("frozen legacy code %d disappeared from err_code.go — removing a code breaks clients switching on it", value)
		}
	}

	// Messages for legacy codes may not silently vanish either.
	for value := range legacyCodes {
		if legacyWithoutMessage[value] {
			continue
		}
		if _, ok := message[value]; !ok {
			t.Errorf("legacy code %d lost its message in err_msg.go", value)
		}
	}
}
