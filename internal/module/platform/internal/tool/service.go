// Package tool implements the admin tool subdomain of the platform module:
// the system log tail, the version, IP geolocation and the process restart.
// Only the module facade may reach it.
package tool

import (
	"github.com/oschwald/geoip2-golang"
)

// Deps declares the subdomain's dependencies; the module facade forwards
// them from the composition root.
type Deps struct {
	// LogPath is the logger output file read by the system-log tail.
	LogPath string
	// GeoIP returns the GeoIP database reader; nil (or a nil result) means
	// no database is configured.
	GeoIP func() *geoip2.Reader
	// Restart restarts the transport server.
	Restart func() error
}

// Service is the tool entry point used by the platform facade.
type Service struct {
	deps Deps
}

// NewService builds the admin tool service.
func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}
