package config

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// Mirrors the pinned Xray SplitHTTPConfig JSON types/build checks. Unverified
// stream fields fail closed instead of claiming compatibility with the node.
type xhttpRange struct{ From, To int32 }

func (r *xhttpRange) UnmarshalJSON(raw []byte) error {
	var n int32
	if err := json.Unmarshal(raw, &n); err == nil {
		if n < 0 {
			return fmt.Errorf("invalid negative integer range")
		}
		r.From = n
		r.To = n
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return fmt.Errorf("invalid integer range")
	}
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "-")
	if len(parts) < 1 || len(parts) > 2 {
		return fmt.Errorf("invalid integer range")
	}
	var vals [2]int32
	for i, v := range parts {
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil || n < 0 {
			return fmt.Errorf("invalid integer range")
		}
		vals[i] = int32(n)
	}
	if len(parts) == 1 {
		vals[1] = vals[0]
	}
	if vals[0] > vals[1] {
		vals[0], vals[1] = vals[1], vals[0]
	}
	r.From, r.To = vals[0], vals[1]
	return nil
}

type xhttpXmux struct {
	MaxConcurrency   xhttpRange `json:"maxConcurrency"`
	MaxConnections   xhttpRange `json:"maxConnections"`
	CMaxReuseTimes   xhttpRange `json:"cMaxReuseTimes"`
	HMaxRequestTimes xhttpRange `json:"hMaxRequestTimes"`
	HMaxReusableSecs xhttpRange `json:"hMaxReusableSecs"`
	HKeepAlivePeriod int64      `json:"hKeepAlivePeriod"`
}
type xhttpSplit struct {
	Host                 string            `json:"host"`
	Path                 string            `json:"path"`
	Mode                 string            `json:"mode"`
	Headers              map[string]string `json:"headers"`
	XPaddingBytes        xhttpRange        `json:"xPaddingBytes"`
	XPaddingObfsMode     bool              `json:"xPaddingObfsMode"`
	XPaddingKey          string            `json:"xPaddingKey"`
	XPaddingHeader       string            `json:"xPaddingHeader"`
	XPaddingPlacement    string            `json:"xPaddingPlacement"`
	XPaddingMethod       string            `json:"xPaddingMethod"`
	UplinkHTTPMethod     string            `json:"uplinkHTTPMethod"`
	SessionIDPlacement   string            `json:"sessionIDPlacement"`
	SessionIDKey         string            `json:"sessionIDKey"`
	SessionIDTable       string            `json:"sessionIDTable"`
	SessionIDLength      xhttpRange        `json:"sessionIDLength"`
	SeqPlacement         string            `json:"seqPlacement"`
	SeqKey               string            `json:"seqKey"`
	UplinkDataPlacement  string            `json:"uplinkDataPlacement"`
	UplinkDataKey        string            `json:"uplinkDataKey"`
	UplinkChunkSize      xhttpRange        `json:"uplinkChunkSize"`
	NoGRPCHeader         bool              `json:"noGRPCHeader"`
	NoSSEHeader          bool              `json:"noSSEHeader"`
	ScMaxEachPostBytes   xhttpRange        `json:"scMaxEachPostBytes"`
	ScMinPostsIntervalMs xhttpRange        `json:"scMinPostsIntervalMs"`
	ScMaxBufferedPosts   int64             `json:"scMaxBufferedPosts"`
	ScStreamUpServerSecs xhttpRange        `json:"scStreamUpServerSecs"`
	ServerMaxHeaderBytes int32             `json:"serverMaxHeaderBytes"`
	Xmux                 xhttpXmux         `json:"xmux"`
	DownloadSettings     *xhttpStream      `json:"downloadSettings"`
	Extra                json.RawMessage   `json:"extra"`
}
type xhttpStream struct {
	Address             string          `json:"address"`
	Port                uint16          `json:"port"`
	Method              string          `json:"method"`
	Network             string          `json:"network"`
	Security            string          `json:"security"`
	XHTTPSettings       *xhttpSplit     `json:"xhttpSettings"`
	SplitHTTPSettings   *xhttpSplit     `json:"splithttpSettings"`
	WSSettings          *xhttpPath      `json:"wsSettings"`
	HTTPUpgradeSettings *xhttpPath      `json:"httpupgradeSettings"`
	TCPSettings         *xhttpTCP       `json:"tcpSettings"`
	RAWSettings         *xhttpTCP       `json:"rawSettings"`
	GRPCSettings        *xhttpGRPC      `json:"grpcSettings"`
	TLSSettings         json.RawMessage `json:"tlsSettings"`
	RealitySettings     json.RawMessage `json:"realitySettings"`
	Sockopt             json.RawMessage `json:"sockopt"`
	Finalmask           json.RawMessage `json:"finalmask"`
	KCPSettings         json.RawMessage `json:"kcpSettings"`
	HysteriaSettings    json.RawMessage `json:"hysteriaSettings"`
}
type xhttpStringList []string

func (v *xhttpStringList) UnmarshalJSON(data []byte) error {
	var values []string
	if err := json.Unmarshal(data, &values); err == nil {
		*v = values
		return nil
	}
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("invalid string list")
	}
	*v = strings.Split(raw, ",")
	return nil
}

type xhttpTCP struct {
	AcceptProxyProtocol bool            `json:"acceptProxyProtocol"`
	Header              json.RawMessage `json:"header"`
}

type xhttpGRPC struct {
	Authority           string `json:"authority"`
	ServiceName         string `json:"serviceName"`
	MultiMode           bool   `json:"multiMode"`
	IdleTimeout         int32  `json:"idle_timeout"`
	HealthCheckTimeout  int32  `json:"health_check_timeout"`
	PermitWithoutStream bool   `json:"permit_without_stream"`
	InitialWindowsSize  int32  `json:"initial_windows_size"`
	UserAgent           string `json:"user_agent"`
}

type xhttpTLS struct {
	ServerName              string            `json:"serverName"`
	Fingerprint             string            `json:"fingerprint"`
	AllowInsecure           bool              `json:"allowInsecure"`
	ALPN                    xhttpStringList   `json:"alpn"`
	MinVersion              string            `json:"minVersion"`
	MaxVersion              string            `json:"maxVersion"`
	PinnedPeerCertSha256    string            `json:"pinnedPeerCertSha256"`
	VerifyPeerCertByName    string            `json:"verifyPeerCertByName"`
	EnableSessionResumption bool              `json:"enableSessionResumption"`
	DisableSystemRoot       bool              `json:"disableSystemRoot"`
	CipherSuites            string            `json:"cipherSuites"`
	RejectUnknownSNI        bool              `json:"rejectUnknownSni"`
	CurvePreferences        xhttpStringList   `json:"curvePreferences"`
	Certificates            []json.RawMessage `json:"certificates"`
	MasterKeyLog            string            `json:"masterKeyLog"`
	ECHServerKeys           string            `json:"echServerKeys"`
	ECHConfigList           string            `json:"echConfigList"`
	ECHSocketSettings       json.RawMessage   `json:"echSockopt"`
}
type xhttpReality struct {
	Fingerprint   string `json:"fingerprint"`
	ServerName    string `json:"serverName"`
	Password      string `json:"password"`
	PublicKey     string `json:"publicKey"`
	ShortID       string `json:"shortId"`
	SpiderX       string `json:"spiderX"`
	Mldsa65Verify string `json:"mldsa65Verify"`
	Show          bool   `json:"show"`
}

func validXHTTPFingerprint(value string, reality bool) bool {
	switch strings.ToLower(value) {
	case "chrome", "firefox", "safari", "ios", "android", "edge", "360", "qq", "random", "randomized", "randomizednoalpn", "hellorandomized", "hellorandomizedalpn", "hellorandomizednoalpn", "hellofirefox_120", "hellofirefox_148", "hellochrome_120", "hellochrome_131", "hellochrome_133", "helloios_13", "helloios_14", "helloedge_106", "hellosafari_26_3", "hello360_11_0", "helloqq_11_1":
		return true
	case "", "unsafe", "hellogolang":
		return !reality
	default:
		return false
	}
}

type xhttpPath struct {
	Host                string            `json:"host"`
	Path                string            `json:"path"`
	Headers             map[string]string `json:"headers"`
	AcceptProxyProtocol bool              `json:"acceptProxyProtocol"`
	HeartbeatPeriod     uint32            `json:"heartbeatPeriod"`
}

func ValidXHTTPMode(mode string) bool {
	switch mode {
	case "", "auto", "packet-up", "stream-up", "stream-one":
		return true
	}
	return false
}
func decodeXHTTP(raw []byte, target any) error {
	if err := ValidateUniqueJSON(raw); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(target)
}

// ValidateNodeRawXHTTP checks XHTTP sections embedded inside a raw stream
// override, including nested download streams and extra objects.
func ValidateNodeRawXHTTP(raw string) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &root); err != nil || root == nil {
		return fmt.Errorf("invalid stream JSON")
	}
	for key, value := range root {
		if !strings.EqualFold(key, "xhttpSettings") && !strings.EqualFold(key, "splithttpSettings") {
			continue
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			continue
		}
		var split *xhttpSplit
		if err := decodeXHTTP(value, &split); err != nil || split == nil {
			return fmt.Errorf("invalid XHTTP stream settings")
		}
		if err := validateXHTTPSplit(split, split.Mode, 0); err != nil {
			return err
		}
	}
	return nil
}

func ValidateXHTTPExtra(raw string, mode string) error {
	var extra *xhttpSplit
	if err := decodeXHTTP([]byte(raw), &extra); err != nil || extra == nil {
		return fmt.Errorf("invalid XHTTP extra")
	}
	return validateXHTTPSplit(extra, mode, 0)
}
func validateXHTTPSplit(v *xhttpSplit, mode string, depth int) error {
	if depth > 32 {
		return fmt.Errorf("XHTTP nesting exceeds limit")
	}
	if !ValidXHTTPMode(v.Mode) || !ValidXHTTPMode(mode) {
		return fmt.Errorf("unsupported XHTTP mode")
	}
	if err := ValidateNodeURI("xhttp path", v.Path, false); err != nil {
		return err
	}
	if mode == "" {
		mode = "auto"
	}
	for key := range v.Headers {
		if strings.EqualFold(key, "host") {
			return fmt.Errorf("XHTTP headers cannot contain host")
		}
	}
	if v.XPaddingBytes != (xhttpRange{}) && (v.XPaddingBytes.From <= 0 || v.XPaddingBytes.To <= 0) {
		return fmt.Errorf("invalid XHTTP padding")
	}
	switch v.XPaddingPlacement {
	case "", "cookie", "header", "query", "queryInHeader":
	default:
		return fmt.Errorf("invalid XHTTP padding placement")
	}
	switch v.XPaddingMethod {
	case "", "repeat-x", "tokenish":
	default:
		return fmt.Errorf("invalid XHTTP padding method")
	}
	switch v.UplinkDataPlacement {
	case "", "auto", "body":
	case "cookie", "header":
		if mode != "packet-up" {
			return fmt.Errorf("XHTTP uplink placement requires packet-up")
		}
	default:
		return fmt.Errorf("invalid XHTTP uplink placement")
	}
	if strings.EqualFold(v.UplinkHTTPMethod, "GET") && mode != "packet-up" {
		return fmt.Errorf("XHTTP GET requires packet-up")
	}
	for _, p := range []string{v.SessionIDPlacement, v.SeqPlacement} {
		switch p {
		case "", "path", "cookie", "header", "query":
		default:
			return fmt.Errorf("invalid XHTTP placement")
		}
	}
	if v.ServerMaxHeaderBytes < 0 || v.ScMaxBufferedPosts < 0 || v.Xmux.HKeepAlivePeriod < 0 {
		return fmt.Errorf("invalid negative XHTTP number")
	}
	if v.Xmux.MaxConnections.To > 0 && v.Xmux.MaxConcurrency.To > 0 {
		return fmt.Errorf("XHTTP xmux conflict")
	}
	if v.SessionIDTable != "" {
		table := v.SessionIDTable
		switch table {
		case "ALPHABET", "alphabet":
			table = strings.Repeat("a", 26)
		case "Alphabet":
			table = strings.Repeat("a", 52)
		case "BASE36", "base36":
			table = strings.Repeat("a", 36)
		case "Base62":
			table = strings.Repeat("a", 62)
		case "HEX", "hex":
			table = strings.Repeat("a", 16)
		case "number":
			table = strings.Repeat("a", 10)
		}
		if v.SessionIDLength.From <= 0 || v.SessionIDLength.To > 128 {
			return fmt.Errorf("invalid XHTTP session length")
		}
		for _, ch := range table {
			if ch > 127 {
				return fmt.Errorf("invalid XHTTP session table")
			}
		}
		room := new(big.Int)
		base := big.NewInt(int64(len(table)))
		for i := v.SessionIDLength.From; i <= v.SessionIDLength.To; i++ {
			room.Add(room, new(big.Int).Exp(base, big.NewInt(int64(i)), nil))
		}
		if room.Cmp(big.NewInt(2<<30)) < 0 {
			return fmt.Errorf("XHTTP session space too small")
		}
	}
	if v.DownloadSettings != nil {
		if mode == "stream-one" {
			return fmt.Errorf("XHTTP stream-one cannot have download settings")
		}
		if err := validateXHTTPStream(v.DownloadSettings, depth+1); err != nil {
			return err
		}
	}
	if len(v.Extra) > 0 && !bytes.Equal(bytes.TrimSpace(v.Extra), []byte("null")) {
		var nested *xhttpSplit
		if err := decodeXHTTP(v.Extra, &nested); err != nil || nested == nil {
			return fmt.Errorf("invalid nested XHTTP extra")
		}
		if err := validateXHTTPSplit(nested, mode, depth+1); err != nil {
			return err
		}
	}
	return nil
}
func validateXHTTPStream(v *xhttpStream, depth int) error {
	if depth > 32 {
		return fmt.Errorf("XHTTP nesting exceeds limit")
	}
	for _, n := range []string{v.Network, v.Method} {
		switch strings.ToLower(n) {
		case "", "tcp", "raw", "ws", "websocket", "grpc", "httpupgrade", "xhttp", "splithttp":
		default:
			return fmt.Errorf("unsupported XHTTP download transport")
		}
	}
	security := strings.ToLower(v.Security)
	switch security {
	case "", "none", "tls", "reality":
	default:
		return fmt.Errorf("unsupported XHTTP download security")
	}
	if security == "tls" && len(bytes.TrimSpace(v.TLSSettings)) > 0 && !bytes.Equal(bytes.TrimSpace(v.TLSSettings), []byte("null")) {
		var tls *xhttpTLS
		if err := decodeXHTTP(v.TLSSettings, &tls); err != nil || tls == nil {
			return fmt.Errorf("invalid XHTTP download TLS settings")
		}
		if tls.AllowInsecure || !validXHTTPFingerprint(tls.Fingerprint, false) || len(tls.Certificates) != 0 || tls.MasterKeyLog != "" || len(bytes.TrimSpace(tls.ECHSocketSettings)) > 0 && !bytes.Equal(bytes.TrimSpace(tls.ECHSocketSettings), []byte("null")) {
			return fmt.Errorf("unsupported XHTTP download TLS settings")
		}
		if tls.ECHServerKeys != "" {
			if _, err := base64.StdEncoding.DecodeString(tls.ECHServerKeys); err != nil {
				return fmt.Errorf("invalid XHTTP ECH keys")
			}
		}
		if len(tls.ALPN) > 1 {
			for _, value := range tls.ALPN {
				if strings.EqualFold(value, "frommitm") {
					return fmt.Errorf("invalid XHTTP download ALPN")
				}
			}
		}
		for _, hash := range strings.Split(tls.PinnedPeerCertSha256, ",") {
			hash = strings.TrimSpace(hash)
			if hash == "" {
				continue
			}
			decoded, err := hex.DecodeString(strings.ReplaceAll(hash, ":", ""))
			if err != nil || len(decoded) != 32 {
				return fmt.Errorf("invalid XHTTP certificate pin")
			}
		}
	}
	if security == "reality" {
		var reality *xhttpReality
		if err := decodeXHTTP(v.RealitySettings, &reality); err != nil || reality == nil {
			return fmt.Errorf("invalid XHTTP download REALITY settings")
		}
		if !validXHTTPFingerprint(reality.Fingerprint, true) {
			return fmt.Errorf("invalid XHTTP download REALITY fingerprint")
		}
		key := reality.PublicKey
		if reality.Password != "" {
			key = reality.Password
		}
		decoded, err := base64.RawURLEncoding.DecodeString(key)
		if err != nil || len(decoded) != 32 {
			return fmt.Errorf("invalid XHTTP download REALITY key")
		}
		if len(reality.ShortID) > 16 {
			return fmt.Errorf("invalid XHTTP download REALITY short id")
		}
		if _, err := hex.DecodeString(reality.ShortID); err != nil {
			return fmt.Errorf("invalid XHTTP download REALITY short id")
		}
		if err := ValidateNodeURI("realitySettings.spiderX", reality.SpiderX, true); err != nil {
			return err
		}
		if reality.Mldsa65Verify != "" {
			decoded, err := base64.RawURLEncoding.DecodeString(reality.Mldsa65Verify)
			if err != nil || len(decoded) != 1952 {
				return fmt.Errorf("invalid XHTTP REALITY verification key")
			}
		}
		network := strings.ToLower(v.Network)
		if v.Method != "" {
			network = strings.ToLower(v.Method)
		}
		switch network {
		case "", "tcp", "raw", "grpc", "xhttp", "splithttp":
		default:
			return fmt.Errorf("unsupported XHTTP REALITY transport")
		}
	}
	for _, p := range []*xhttpPath{v.WSSettings, v.HTTPUpgradeSettings} {
		if p != nil {
			if err := ValidateNodeURI("download path", p.Path, false); err != nil {
				return err
			}
		}
	}
	if v.HTTPUpgradeSettings != nil {
		for key := range v.HTTPUpgradeSettings.Headers {
			if strings.EqualFold(key, "host") {
				return fmt.Errorf("httpupgrade headers cannot contain host")
			}
		}
	}
	if len(bytes.TrimSpace(v.RealitySettings)) > 0 && !bytes.Equal(bytes.TrimSpace(v.RealitySettings), []byte("null")) {
		var settings *xhttpReality
		if err := decodeXHTTP(v.RealitySettings, &settings); err != nil || settings == nil {
			return fmt.Errorf("invalid REALITY settings")
		}
		if err := ValidateNodeURI("realitySettings.spiderX", settings.SpiderX, true); err != nil {
			return err
		}
	}
	if security != "tls" && len(bytes.TrimSpace(v.TLSSettings)) > 0 && !bytes.Equal(bytes.TrimSpace(v.TLSSettings), []byte("null")) && !bytes.Equal(bytes.TrimSpace(v.TLSSettings), []byte("{}")) {
		return fmt.Errorf("unverified XHTTP download TLS settings")
	}
	for _, tcp := range []*xhttpTCP{v.TCPSettings, v.RAWSettings} {
		if tcp == nil || len(bytes.TrimSpace(tcp.Header)) == 0 || bytes.Equal(bytes.TrimSpace(tcp.Header), []byte("null")) {
			continue
		}
		var header struct {
			Type string `json:"type"`
		}
		if err := decodeXHTTP(tcp.Header, &header); err != nil || header.Type != "none" {
			return fmt.Errorf("unverified XHTTP TCP header")
		}
	}
	for _, raw := range []json.RawMessage{v.Sockopt, v.Finalmask, v.KCPSettings, v.HysteriaSettings} {
		if len(bytes.TrimSpace(raw)) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) && !bytes.Equal(bytes.TrimSpace(raw), []byte("{}")) {
			return fmt.Errorf("unverified XHTTP download stream settings")
		}
	}
	for _, split := range []*xhttpSplit{v.XHTTPSettings, v.SplitHTTPSettings} {
		if split != nil {
			if err := validateXHTTPSplit(split, split.Mode, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}
