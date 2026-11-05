package geolocation

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

// roundTripFunc answers the lookups in place of the services.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// serveLookups routes every lookup to answer for the time of the test.
func serveLookups(t *testing.T, answer roundTripFunc) {
	t.Helper()
	previous := geoHTTPClient
	geoHTTPClient = &http.Client{Transport: answer, Timeout: time.Second}
	t.Cleanup(func() { geoHTTPClient = previous })
}

func answer(status int, encoding string, body []byte) *http.Response {
	header := http.Header{"Content-Type": {"application/json"}}
	if encoding != "" {
		header.Set("Content-Encoding", encoding)
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(bytes.NewReader(body))}
}

// Each service answers in its own shape: ipinfo only has a "loc" pair and
// ipapi names the country in full.
func TestFetchGeolocationReadsEachServicesAnswer(t *testing.T) {
	for _, tt := range []struct {
		service, url, body string
		want               GeoLocationResponse
	}{
		{
			service: ipinfo, url: "https://ipinfo.io/203.0.113.7/json",
			body: `{"country":"US","city":"Mountain View","loc":"37.4056,-122.0775"}`,
			want: GeoLocationResponse{Country: "US", City: "Mountain View", Loc: "37.4056,-122.0775", Latitude: "37.4056", Longitude: "-122.0775"},
		},
		{
			service: ipapi, url: "https://ipapi.co/203.0.113.7/json",
			body: `{"country_name":"Germany","city":"Berlin","latitude":"52.52","longitude":"13.40"}`,
			want: GeoLocationResponse{Country: "Germany", CountryName: "Germany", City: "Berlin", Latitude: "52.52", Longitude: "13.40"},
		},
		{
			service: ipbase, url: "https://api.ipbase.com/v1/json/203.0.113.7",
			body: `{"country_name":"Japan","region":"Tokyo","city":"Tokyo"}`,
			want: GeoLocationResponse{Country: "Japan", CountryName: "Japan", Region: "Tokyo", City: "Tokyo"},
		},
		{
			service: ipwhois, url: "https://ipwhois.app/json/203.0.113.7",
			body: `{"country":"France","city":"Paris"}`,
			want: GeoLocationResponse{Country: "France", City: "Paris"},
		},
	} {
		t.Run(tt.service, func(t *testing.T) {
			serveLookups(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != tt.url || r.Method != http.MethodGet {
					t.Errorf("request = %s %s, want GET %s", r.Method, r.URL, tt.url)
				}
				return answer(http.StatusOK, "", []byte(tt.body)), nil
			})
			got, err := fetchGeolocation(context.Background(), tt.service, "203.0.113.7")
			if err != nil {
				t.Fatalf("fetchGeolocation: %v", err)
			}
			if *got != tt.want {
				t.Fatalf("location = %+v, want %+v", *got, tt.want)
			}
		})
	}
}

// The request asks for compressed answers, so each encoding it offers must
// decode.
func TestFetchGeolocationDecodesCompressedAnswers(t *testing.T) {
	body := []byte(`{"country":"NL","city":"Amsterdam"}`)
	encoded := map[string][]byte{"": body}

	var gz bytes.Buffer
	gzipWriter := gzip.NewWriter(&gz)
	if _, err := gzipWriter.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	encoded["gzip"] = gz.Bytes()

	var br bytes.Buffer
	brotliWriter := brotli.NewWriter(&br)
	if _, err := brotliWriter.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := brotliWriter.Close(); err != nil {
		t.Fatal(err)
	}
	encoded["br"] = br.Bytes()

	zstdEncoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded["zstd"] = zstdEncoder.EncodeAll(body, nil)
	if err := zstdEncoder.Close(); err != nil {
		t.Fatal(err)
	}

	for encoding, data := range encoded {
		t.Run("encoding "+encoding, func(t *testing.T) {
			serveLookups(t, func(r *http.Request) (*http.Response, error) {
				if got := r.Header.Get("Accept-Encoding"); got != "gzip, deflate, br, zstd" {
					t.Errorf("Accept-Encoding = %q", got)
				}
				return answer(http.StatusOK, encoding, data), nil
			})
			got, err := fetchGeolocation(context.Background(), ipwhois, "203.0.113.7")
			if err != nil {
				t.Fatalf("fetchGeolocation: %v", err)
			}
			if got.Country != "NL" || got.City != "Amsterdam" {
				t.Fatalf("location = %+v", *got)
			}
		})
	}
}

// A refused lookup, an answer that is not JSON and a transport failure are
// errors, not an empty location.
func TestFetchGeolocationReportsFailedLookups(t *testing.T) {
	for name, respond := range map[string]roundTripFunc{
		"rate limited": func(*http.Request) (*http.Response, error) {
			return answer(http.StatusTooManyRequests, "", []byte(`{"error":"rate limited"}`)), nil
		},
		"not JSON": func(*http.Request) (*http.Response, error) {
			return answer(http.StatusOK, "", []byte("<html>blocked</html>")), nil
		},
		"corrupt gzip": func(*http.Request) (*http.Response, error) {
			return answer(http.StatusOK, "gzip", []byte("not gzip")), nil
		},
		"unreachable": func(*http.Request) (*http.Response, error) {
			return nil, errors.New("connection refused")
		},
	} {
		t.Run(name, func(t *testing.T) {
			serveLookups(t, respond)
			if got, err := fetchGeolocation(context.Background(), ipinfo, "203.0.113.7"); err == nil {
				t.Fatalf("fetchGeolocation = %+v, want an error", *got)
			}
		})
	}
	if _, err := fetchGeolocation(context.Background(), "example.com", "203.0.113.7"); err == nil || !strings.Contains(err.Error(), "unsupported service") {
		t.Fatalf("unknown service error = %v", err)
	}
}

// One failing service must not leave the server without a location: the
// lookup moves on to the next, and fails only when none answers.
func TestGetRegionByIpTriesTheServicesInTurn(t *testing.T) {
	var mu sync.Mutex
	asked := map[string]int{}
	serveLookups(t, func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		asked[r.URL.Host]++
		mu.Unlock()
		if r.URL.Host != ipwhois {
			return answer(http.StatusServiceUnavailable, "", nil), nil
		}
		return answer(http.StatusOK, "", []byte(`{"country":"SG","city":"Singapore"}`)), nil
	})
	got, err := GetRegionByIp(context.Background(), "203.0.113.7")
	if err != nil {
		t.Fatalf("GetRegionByIp: %v", err)
	}
	if got.Country != "SG" || got.City != "Singapore" {
		t.Fatalf("location = %+v", *got)
	}
	if asked[ipwhois] != 1 {
		t.Fatalf("lookups = %v, want the answering service asked once", asked)
	}

	serveLookups(t, func(*http.Request) (*http.Response, error) {
		return answer(http.StatusServiceUnavailable, "", nil), nil
	})
	if got, err := GetRegionByIp(context.Background(), "203.0.113.7"); err == nil {
		t.Fatalf("GetRegionByIp = %+v, want an error when no service answers", *got)
	}
}
