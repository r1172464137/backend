package devicesocket

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// dialDeviceAs dials the test server as the named device, with extra
// request headers, and returns the connection and the handshake's HTTP
// status (zero when no response came back).
func dialDeviceAs(t *testing.T, srv *deviceServer, device string, header http.Header) (*websocket.Conn, int, error) {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "?device=" + device
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, header)
	status := 0
	if resp != nil {
		status = resp.StatusCode
		_ = resp.Body.Close()
	}
	if err == nil {
		srv.waitRegistered(t, device)
	}
	return conn, status, err
}

// waitFor polls until cond holds or the deadline passes.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A device that never reads its socket must not stall anyone else: pushes
// to it are refused once its queue is full, pushes to the user's other
// device still arrive, heartbeats and the sweep return at once, and the
// stalled device is eventually dropped on its write deadline.
func TestSlowReaderDoesNotBlockOtherDevices(t *testing.T) {
	dm := NewDeviceManager(3600, 3600, nil)
	defer dm.Stop()

	const userID = int64(21)
	srv := deviceTestServer(t, dm, userID, 5)
	slow, _, err := dialDeviceAs(t, srv, "slow", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = slow.Close() }()
	fast, _, err := dialDeviceAs(t, srv, "fast", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fast.Close() }()
	waitFor(t, "both devices to register", func() bool { return dm.Online() == 2 })

	// The slow device never reads. Its socket buffers some pushes, then the
	// writer blocks on it and the queue fills; every further push is refused
	// without waiting.
	large := strings.Repeat("x", 64<<10)
	var refused error
	start := time.Now()
	for i := 0; i < 4096 && refused == nil; i++ {
		refused = dm.SendToDevice(userID, "slow", large)
	}
	if !errors.Is(refused, ErrSendQueueFull) {
		t.Fatalf("pushes to the stalled device = %v, want %v", refused, ErrSendQueueFull)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("pushes to the stalled device took %s; the caller must not wait on its socket", elapsed)
	}

	// The other device, the heartbeat path and the sweep are unaffected.
	done := make(chan struct{})
	go func() {
		defer close(done)
		dm.UpdateHeartbeat(userID, "slow")
		dm.checkHeartbeats()
		dm.Broadcast("hello")
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("heartbeat, sweep or broadcast stalled on the slow device")
	}
	if err := dm.SendToDevice(userID, "fast", "direct"); err != nil {
		t.Fatalf("push to the healthy device: %v", err)
	}
	if err := fast.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for len(got) < 2 {
		_, msg, err := fast.ReadMessage()
		if err != nil {
			t.Fatalf("healthy device read: %v (got %v)", err, got)
		}
		got[string(msg)] = true
	}
	if !got["hello"] || !got["direct"] {
		t.Fatalf("healthy device received %v, want the broadcast and the direct push", got)
	}
}

// A frame over the read limit ends the connection instead of being
// buffered.
func TestReadLimitDisconnectsAnOversizedFrame(t *testing.T) {
	dm := NewDeviceManager(3600, 3600, nil)
	defer dm.Stop()

	const userID = int64(23)
	srv := deviceTestServer(t, dm, userID, 5)
	conn := dialDevice(t, srv)
	defer func() { _ = conn.Close() }()
	waitFor(t, "the device to register", func() bool { return dm.Online() == 1 })

	if err := conn.WriteMessage(websocket.TextMessage, []byte(strings.Repeat("a", maxMessageSize+1))); err != nil {
		t.Fatal(err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("the oversized frame did not end the connection")
	}
	waitFor(t, "the device to be removed", func() bool { return dm.Online() == 0 })

	// A frame within the limit is fine.
	conn = dialDevice(t, srv)
	defer func() { _ = conn.Close() }()
	if err := conn.WriteMessage(websocket.TextMessage, []byte(strings.Repeat("a", maxMessageSize))); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte("ping")); err != nil {
		t.Fatal(err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, msg, err := conn.ReadMessage(); err != nil || string(msg) != heartbeatReply {
		t.Fatalf("heartbeat after a full-size frame = %q, %v", msg, err)
	}
}

// With allowed origins configured, a browser from another origin is refused
// at the handshake; an allowed origin, a request without an Origin header
// and an empty list are admitted.
func TestHandshakeRejectsADisallowedOrigin(t *testing.T) {
	dm := NewDeviceManager(3600, 3600, []string{"https://app.example.com", "HTTPS://Admin.Example.com/"})
	defer dm.Stop()
	const userID = int64(25)
	srv := deviceTestServer(t, dm, userID, 5)

	if _, status, err := dialDeviceAs(t, srv, "browser", http.Header{"Origin": {"https://evil.example.com"}}); err == nil || status != http.StatusForbidden {
		t.Fatalf("handshake from a foreign origin = %v, status %d; want refused with 403", err, status)
	}
	for _, origin := range []string{"https://app.example.com", "https://admin.example.com"} {
		conn, _, err := dialDeviceAs(t, srv, "browser", http.Header{"Origin": {origin}})
		if err != nil {
			t.Fatalf("handshake from %s: %v", origin, err)
		}
		_ = conn.Close()
	}
	conn, _, err := dialDeviceAs(t, srv, "app", nil)
	if err != nil {
		t.Fatalf("handshake without an Origin: %v", err)
	}
	_ = conn.Close()

	open := NewDeviceManager(3600, 3600, nil)
	defer open.Stop()
	openSrv := deviceTestServer(t, open, userID, 5)
	conn, _, err = dialDeviceAs(t, openSrv, "browser", http.Header{"Origin": {"https://anything.example.com"}})
	if err != nil {
		t.Fatalf("handshake with no allowed-origin list configured: %v", err)
	}
	_ = conn.Close()
}

// A device that sends nothing for the heartbeat timeout is dropped by its
// read deadline, without waiting for the sweep.
func TestSilentDeviceIsDroppedByTheReadDeadline(t *testing.T) {
	dm := NewDeviceManager(1, 3600, nil)
	defer dm.Stop()

	const userID = int64(27)
	srv := deviceTestServer(t, dm, userID, 5)
	conn := dialDevice(t, srv)
	defer func() { _ = conn.Close() }()
	waitFor(t, "the device to register", func() bool { return dm.Online() == 1 })

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("the silent device was kept")
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Fatalf("the silent device lived %s past the 1 second heartbeat timeout", elapsed)
	}
	waitFor(t, "the device to be removed", func() bool { return dm.Online() == 0 })
}
