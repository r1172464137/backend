package bootstrap

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
)

type nodeProofRoute struct {
	Kind string `json:"kind"`
	Tag  string `json:"tag"`
}
type nodeProofResult struct {
	OK    bool             `json:"ok"`
	Error string           `json:"error"`
	Rules []nodeProofRoute `json:"rules"`
}

func runNodeProof(t *testing.T, request any) nodeProofResult {
	t.Helper()
	binary := os.Getenv("PPANEL_NODE_PROOF")
	if binary == "" {
		t.Fatal("PPANEL_NODE_PROOF must name the helper built against the final ppnode source")
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary)
	cmd.Stdin = bytes.NewReader(body)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("node proof failed: %v", err)
	}
	// Xray's builder can emit warnings on stdout. The helper writes its
	// machine-readable result last, after the builder returns.
	lines := bytes.Split(bytes.TrimSpace(out), []byte("\n"))
	var result nodeProofResult
	if err := json.Unmarshal(lines[len(lines)-1], &result); err != nil {
		t.Fatalf("node proof JSON: %v: %s", err, out)
	}
	return result
}

// Build against the owner's real checkout in a temporary module, never by
// vendoring a drifting copy into backend. CI may provide PPANEL_NODE_ROOT or
// an already built PPANEL_NODE_PROOF; no network module downloads are allowed.
func requireNodeProof(t *testing.T) {
	t.Helper()
	if os.Getenv("PPANEL_NODE_PROOF") != "" {
		return
	}
	root := os.Getenv("PPANEL_NODE_ROOT")
	if root == "" {
		root = filepath.Join("..", "..", "..", "..", "ppanel-node")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Skip("set PPANEL_NODE_ROOT or PPANEL_NODE_PROOF for cross-repository capability contract")
	}
	dir := t.TempDir()
	for _, f := range []string{"go.mod", "go.sum"} {
		b, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	src, err := os.ReadFile(filepath.Join("testdata", "nodeproof", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), src, 0600); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build final ppnode proof: %v: %s", err, out)
		}
	}
	run("mod", "edit", "-module=ppanel-nodeproof", "-require=github.com/perfect-panel/ppanel-node@v0.0.0", "-replace=github.com/perfect-panel/ppanel-node="+root)
	binary := filepath.Join(dir, "nodeproof")
	run("build", "-mod=mod", "-o", binary, ".")
	t.Setenv("PPANEL_NODE_PROOF", binary)
}

func TestCutoverIntegrationRequiresProofBeforeDatabase(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestOrderedCutoverMySQL$")
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "PPANEL_TEST_ORDERED_ROUTING_MYSQL_DSN=") && !strings.HasPrefix(value, "PPANEL_NODE_PROOF=") && !strings.HasPrefix(value, "PPANEL_TEST_XRAY=") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.Env = append(cmd.Env, "PPANEL_TEST_ORDERED_ROUTING_MYSQL_DSN=ignored:ignored@tcp(127.0.0.1:1)/proof_test", "PPANEL_NODE_PROOF="+filepath.Join(t.TempDir(), "missing-proof"), "PPANEL_TEST_XRAY=/missing-xray")
	output, err := cmd.CombinedOutput()
	if err == nil || !bytes.Contains(output, []byte("node proof failed")) {
		t.Fatalf("cutover did not require helper before database: %v %s", err, output)
	}
}

func TestFinalNodeBuilderRejectsPreflightBlockers(t *testing.T) {
	requireNodeProof(t)
	key := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	for _, tc := range []struct {
		name string
		out  config.NodeOutbound
	}{
		{"tuic", config.NodeOutbound{Name: "tuic", Protocol: "tuic", Address: "127.0.0.1", Port: 443, UUID: "00000000-0000-0000-0000-000000000001", Password: "secret"}},
		{"tuic-transport", config.NodeOutbound{Name: "tuic-transport", Protocol: "direct", Transport: "tuic"}},
		{"plugin-options", config.NodeOutbound{Name: "plugin-options", Protocol: "direct", PluginOptions: map[string]any{"mode": "ws"}}},
		{"bad-spider", config.NodeOutbound{Name: "bad-spider", Protocol: "vless", Address: "example.com", Port: 443, UUID: "00000000-0000-0000-0000-000000000001", Security: "reality", Transport: "tcp", RealityPublicKey: key, SpiderX: "/%"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			preview, err := config.ConvertLegacyRouting(config.LegacyRoutingInput{Outbound: []config.NodeOutbound{tc.out}}, "global")
			if err != nil || preview.Draft != nil {
				t.Fatalf("preflight accepted %#v %v", preview, err)
			}
			proof := runNodeProof(t, map[string]any{"kind": "outbound", "outbound": tc.out})
			if proof.OK {
				t.Fatalf("final node builder accepted %s", tc.name)
			}
		})
	}
	for _, tc := range []struct {
		name     string
		protocol node.Protocol
	}{
		{"ss-plugin", node.Protocol{Type: "shadowsocks", Enable: true, Port: 443, Cipher: "aes-128-gcm", Plugin: "obfs"}},
		{"unsupported", node.Protocol{Type: "nowhere", Enable: true, Port: 443}},
		{"bad-path", node.Protocol{Type: "vless", Enable: true, Port: 443, Transport: "ws", Path: "/%"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal([]node.Protocol{tc.protocol})
			if err := node.ValidateNodeProtocols(&node.Server{Protocols: string(raw)}); err == nil {
				t.Fatal("preflight accepted unsupported listener")
			}
			proof := runNodeProof(t, map[string]any{"kind": "inbound", "protocol": tc.protocol})
			if proof.OK {
				t.Fatalf("final node builder accepted %s", tc.name)
			}
		})
	}
	goodReality := node.Protocol{Type: "vless", Port: 443, Enable: true, Transport: "tcp", Security: "reality", SNI: "example.com", RealityServerAddr: "example.com", RealityServerPort: 443, RealityPrivateKey: key, RealityShortId: "0123456789abcdef", RealityXver: 1}
	raw, _ := json.Marshal([]node.Protocol{goodReality})
	if err := node.ValidateNodeProtocols(&node.Server{Protocols: string(raw)}); err != nil {
		t.Fatalf("valid REALITY blocked: %v", err)
	}
	if proof := runNodeProof(t, map[string]any{"kind": "inbound", "protocol": goodReality}); !proof.OK {
		t.Fatalf("final node builder rejected REALITY: %s", proof.Error)
	}
	valid := config.NodeOutbound{Name: "socks", Protocol: "socks", Address: "127.0.0.1", Port: 1080}
	preview, err := config.ConvertLegacyRouting(config.LegacyRoutingInput{Outbound: []config.NodeOutbound{valid}}, "global")
	if err != nil || preview.Draft == nil {
		t.Fatalf("valid SOCKS blocked: %#v %v", preview, err)
	}
	if proof := runNodeProof(t, map[string]any{"kind": "outbound", "outbound": valid}); !proof.OK {
		t.Fatalf("final node builder rejected SOCKS: %s", proof.Error)
	}
}
