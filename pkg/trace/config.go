package trace

// TraceName represents the tracing name.
const TraceName = "ppanel"

// A Config is an opentelemetry config.
type Config struct {
	Name string `yaml:"Name"`
	// Endpoint is where the batcher sends spans; empty disables exporting.
	// For the jaeger batcher it is Jaeger's OTLP/HTTP receiver, either a URL
	// such as http://jaeger:4318 (posting to /v1/traces unless the URL names
	// another path; the scheme selects TLS) or a bare host:port (like
	// otlphttp). Jaeger has received OTLP natively since 1.35. The endpoints
	// of the former Jaeger Thrift exporter — the agent's udp://host:6831 and
	// the collector's http://host:14268/api/traces — are rewritten to OTLP on
	// port 4318 of the same host, with a warning in the log.
	Endpoint string  `yaml:"Endpoint"`
	Sampler  float64 `yaml:"Sampler" default:"0.1"`
	// Batcher selects the exporter: jaeger (OTLP/HTTP to Jaeger, see
	// Endpoint), zipkin, otlpgrpc, otlphttp or file.
	Batcher string `yaml:"Batcher" default:"jaeger"`
	// OtlpHeaders represents the headers for OTLP gRPC or HTTP transport.
	// For example:
	//  uptrace-dsn: 'http://project2_secret_token@localhost:14317/2'
	OtlpHeaders map[string]string `yaml:"OtlpHeaders"`
	// OtlpHttpPath represents the path for OTLP HTTP transport with a
	// host:port Endpoint.
	// For example
	// /v1/traces
	OtlpHttpPath string `yaml:"OtlpHttpPath"`
	// OtlpHttpSecure represents the scheme to use for OTLP HTTP transport
	// with a host:port Endpoint.
	OtlpHttpSecure bool `yaml:"OtlpHttpSecure"`
	// OtlpGrpcSecure makes the otlpgrpc batcher connect with TLS, verifying
	// the collector against the system roots. False, the default and the
	// only behaviour before the key existed, sends the spans in plaintext,
	// which suits a collector on the same host or private network only.
	OtlpGrpcSecure bool `yaml:"OtlpGrpcSecure"`
	// Disabled indicates whether StartAgent starts the agent.
	Disabled bool `yaml:"Disabled"`
}
