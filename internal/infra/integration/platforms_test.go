package integration

import (
	"errors"
	"testing"
)

type provider int

const (
	providerA provider = iota
	providerB
	providerUnknown
)

func TestPlatformsMapBothWays(t *testing.T) {
	platforms := NewPlatforms(providerUnknown, map[string]provider{"a": providerA, "B": providerB})

	for name, want := range map[string]provider{"a": providerA, "B": providerB, "b": providerUnknown, "": providerUnknown} {
		if got := platforms.Parse(name); got != want {
			t.Fatalf("Parse(%q) = %d, want %d", name, got, want)
		}
	}
	for value, want := range map[provider]string{providerA: "a", providerB: "B", providerUnknown: "unsupported"} {
		if got := platforms.Name(value); got != want {
			t.Fatalf("Name(%d) = %q, want %q", value, got, want)
		}
	}
}

func TestClientCacheRebuildsOnlyWhenTheConfigurationChanges(t *testing.T) {
	var cache ClientCache[*int]
	builds := 0
	build := func() (*int, error) {
		builds++
		client := builds
		return &client, nil
	}

	first, _ := cache.Get(build, "smtp", `{"host":"a"}`)
	again, _ := cache.Get(build, "smtp", `{"host":"a"}`)
	if first != again || builds != 1 {
		t.Fatalf("builds = %d, want the client reused", builds)
	}
	changed, _ := cache.Get(build, "smtp", `{"host":"b"}`)
	if changed == first || builds != 2 {
		t.Fatalf("builds = %d, want a rebuild for a new configuration", builds)
	}
	// Parts are delimited: moving a byte between parts is a different key.
	if _, err := cache.Get(build, "smtp{", `"host":"b"}`); err != nil || builds != 3 {
		t.Fatalf("builds = %d, want the shifted configuration rebuilt", builds)
	}

	failure := errors.New("bad config")
	if _, err := cache.Get(func() (*int, error) { return nil, failure }, "x"); !errors.Is(err, failure) {
		t.Fatalf("error = %v, want the build failure", err)
	}
	if kept, _ := cache.Get(build, "smtp{", `"host":"b"}`); kept == nil || builds != 3 {
		t.Fatalf("builds = %d, a failed build must leave the cached client in place", builds)
	}
}
