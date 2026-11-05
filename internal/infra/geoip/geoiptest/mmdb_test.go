package geoiptest

import (
	"net"
	"testing"

	"github.com/oschwald/geoip2-golang"
)

func TestMMDBSplitsTheAddressSpace(t *testing.T) {
	db, err := geoip2.FromBytes(MMDB("GeoLite2-City", map[string]any{"country": map[string]any{"iso_code": "AU"}}, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for ip, want := range map[string]string{"1.1.1.1": "AU", "127.255.255.255": "AU", "128.0.0.1": "", "203.0.113.7": ""} {
		record, err := db.Country(net.ParseIP(ip))
		if err != nil || record.Country.IsoCode != want {
			t.Fatalf("%s: country = %+v (err %v), want %q", ip, record, err, want)
		}
	}
}
