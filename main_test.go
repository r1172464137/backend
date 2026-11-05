package main

import (
	"go/parser"
	"go/token"
	"strconv"
	"testing"
	"time"
)

// The binary must carry the zone database: the Docker image has no zone
// file besides Asia/Shanghai, so without it every other AppLocation fails to
// load there and the process silently runs on Local. Only a container
// without a system database can show the embedded copy being used, so the
// import is checked here; the load below runs on whichever copy the host
// offers first.
func TestBinaryEmbedsTheZoneDatabase(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range file.Imports {
		if path, _ := strconv.Unquote(spec.Path.Value); path != "time/tzdata" {
			continue
		}
		if _, err := time.LoadLocation("Europe/Paris"); err != nil {
			t.Fatalf("Europe/Paris does not load with the zone database embedded: %v", err)
		}
		return
	}
	t.Fatal("main.go does not import time/tzdata: an AppLocation other than Asia/Shanghai cannot load in the Docker image")
}
