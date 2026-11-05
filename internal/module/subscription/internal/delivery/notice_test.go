package delivery

import "testing"

// A notice's second placeholder node is named after the site, the first host
// of the site host setting, not after the server's listen address.
func TestNoticeServersNameTheSite(t *testing.T) {
	nodes := createNoticeServers("www.example.test\nwww.example.org", "Subscription expired")
	if len(nodes) != 2 || nodes[0].Name != "Subscription expired" || nodes[1].Name != "www.example.test" {
		t.Fatalf("notice nodes = %q / %q, want the message and the first site host", nodes[0].Name, nodes[1].Name)
	}
}
