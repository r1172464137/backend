package devicesocket

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// deviceTestServer serves a websocket endpoint that registers every dial as
// the device named by the "device" query parameter.
func deviceTestServer(t *testing.T, dm *DeviceManager, userID int64, maxDevices int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dm.AddDevice(w, r, "session", userID, r.URL.Query().Get("device"), maxDevices)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func dialDevice(t *testing.T, srv *httptest.Server, deviceID string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "?device=" + deviceID
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	// Upgrade writes HTTP 101 before AddDevice registers the socket. A
	// second dial immediately after the handshake can therefore register
	// first and then be replaced by the delayed first registration. Wait
	// for the real read loop's reply, not a sleep or the online counter:
	// stable also settles the replacement's counter after closing the old
	// socket, but before starting the new socket's read loop.
	if err := conn.SetWriteDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte("ping")); err != nil {
		t.Fatalf("device readiness heartbeat: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, reply, err := conn.ReadMessage(); err != nil || string(reply) != "ping" {
		t.Fatalf("device readiness reply = %q, %v", reply, err)
	}
	if err := conn.SetWriteDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}
	return conn
}

// heartbeat replies and subscription pushes write to the same connection
// from different goroutines; the manager must serialize them instead of
// panicking with gorilla/websocket's concurrent-write assertion. Run with
// -race.
func TestConcurrentHeartbeatAndPushWrites(t *testing.T) {
	dm := NewDeviceManager(3600, 3600)
	defer dm.Stop()

	const userID = int64(7)
	srv := deviceTestServer(t, dm, userID, 5)
	conn := dialDevice(t, srv, "dev1")

	var received atomic.Int64
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
			received.Add(1)
		}
	}()

	const pushWorkers = 4
	const pushesPerWorker = 200
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < pushWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < pushesPerWorker; j++ {
				if err := dm.SendToDevice(userID, "dev1", "push"); err != nil {
					t.Errorf("SendToDevice: %v", err)
					return
				}
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					dm.UpdateHeartbeat(userID, "dev1")
					time.Sleep(time.Millisecond)
				}
			}
		}()
	}

	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()

	deadline := time.Now().Add(2 * time.Second)
	for received.Load() < pushWorkers*pushesPerWorker && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	conn.Close()
	<-closed
	if got := received.Load(); got < pushWorkers*pushesPerWorker {
		t.Errorf("received %d messages, want at least %d", got, pushWorkers*pushesPerWorker)
	}
}

// The kick notification must reach the client before the connection closes,
// which requires the device to stay registered until OnDeviceKicked returns.
func TestKickDeliversNotificationThenCloses(t *testing.T) {
	dm := NewDeviceManager(3600, 3600)
	defer dm.Stop()

	const userID = int64(9)
	var offlineEvents atomic.Int64
	dm.OnDeviceKicked = func(userID int64, deviceID, session string, operator Operator) {
		if err := dm.SendToDevice(userID, deviceID, `{"method":"kicked"}`); err != nil {
			t.Errorf("SendToDevice during kick callback: %v", err)
		}
	}
	dm.OnDeviceOffline = func(userID int64, deviceID, session string, createAt time.Time) {
		offlineEvents.Add(1)
	}

	srv := deviceTestServer(t, dm, userID, 5)
	conn := dialDevice(t, srv, "dev1")

	dm.KickDevice(userID, "dev1")

	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("expected kick notification before close, got error: %v", err)
	}
	if string(msg) != `{"method":"kicked"}` {
		t.Errorf("kick notification = %q, want %q", msg, `{"method":"kicked"}`)
	}
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Error("expected connection to close after the kick notification")
	}

	deadline := time.Now().Add(2 * time.Second)
	for offlineEvents.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := offlineEvents.Load(); got != 1 {
		t.Errorf("OnDeviceOffline fired %d times after kick, want 1", got)
	}
}

// Reconnecting with the same device ID must retire the previous socket:
// pushes reach the new connection and the online counter stays at one.
func TestReconnectReplacesPreviousSocket(t *testing.T) {
	dm := NewDeviceManager(3600, 3600)
	defer dm.Stop()

	const userID = int64(11)
	srv := deviceTestServer(t, dm, userID, 5)
	oldConn := dialDevice(t, srv, "dev1")
	newConn := dialDevice(t, srv, "dev1")

	if err := oldConn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	if _, _, err := oldConn.ReadMessage(); err == nil {
		t.Fatal("previous socket should be closed after reconnect")
	} else {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			t.Fatalf("previous socket stayed open until the read deadline: %v", err)
		}
	}

	if err := dm.SendToDevice(userID, "dev1", "hello"); err != nil {
		t.Fatalf("SendToDevice after reconnect: %v", err)
	}
	if err := newConn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	_, msg, err := newConn.ReadMessage()
	if err != nil {
		t.Fatalf("push lost after reconnect: %v", err)
	}
	if string(msg) != "hello" {
		t.Errorf("push = %q, want %q", msg, "hello")
	}

	if got := atomic.LoadInt32(&dm.totalOnline); got != 1 {
		t.Errorf("totalOnline = %d after reconnect, want 1", got)
	}
}
