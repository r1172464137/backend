package integration

import (
	"crypto/sha256"
	"sync"
)

// unsupportedName is how a provider value without a name prints.
const unsupportedName = "unsupported"

// Platforms is a two-way table between the provider names stored in a
// configuration and a provider enum.
type Platforms[P comparable] struct {
	byName      map[string]P
	byValue     map[P]string
	unsupported P
}

// NewPlatforms builds the table from names; unsupported is the value an
// unknown name parses to.
func NewPlatforms[P comparable](unsupported P, names map[string]P) Platforms[P] {
	p := Platforms[P]{
		byName:      make(map[string]P, len(names)),
		byValue:     make(map[P]string, len(names)),
		unsupported: unsupported,
	}
	for name, value := range names {
		p.byName[name] = value
		p.byValue[value] = name
	}
	return p
}

// Parse returns the provider named name, or the unsupported value.
func (p Platforms[P]) Parse(name string) P {
	if value, ok := p.byName[name]; ok {
		return value
	}
	return p.unsupported
}

// Name returns the name value is stored under, or "unsupported".
func (p Platforms[P]) Name(value P) string {
	if name, ok := p.byValue[value]; ok {
		return name
	}
	return unsupportedName
}

// ClientCache keeps the provider client built for the configuration in
// force, so a send reuses the client — and the connections it holds —
// instead of building one per message. It is rebuilt when the configuration
// changes. The configuration is kept only as a digest.
type ClientCache[C any] struct {
	mu     sync.Mutex
	key    [sha256.Size]byte
	client C
	built  bool
}

// Get returns the client for the configuration parts, building it with
// build when they differ from the cached client's. A failed build leaves the
// cache as it was.
func (c *ClientCache[C]) Get(build func() (C, error), parts ...string) (C, error) {
	hash := sha256.New()
	for _, part := range parts {
		// The length prefix keeps ("ab", "c") and ("a", "bc") apart.
		_, _ = hash.Write([]byte{byte(len(part) >> 24), byte(len(part) >> 16), byte(len(part) >> 8), byte(len(part))})
		_, _ = hash.Write([]byte(part))
	}
	var key [sha256.Size]byte
	copy(key[:], hash.Sum(nil))

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.built && c.key == key {
		return c.client, nil
	}
	client, err := build()
	if err != nil {
		var zero C
		return zero, err
	}
	c.key, c.client, c.built = key, client, true
	return client, nil
}
