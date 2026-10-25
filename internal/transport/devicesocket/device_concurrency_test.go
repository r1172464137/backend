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

// testDeviceID is the device every test connection dials as; dialing it
// twice replaces the first connection.
const testDeviceID = "dev1"

func dialDevice(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "?device=" + testDeviceID
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
	// reconnecting keeps that counter at one even before registration.
	if err := conn.SetWriteDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte("ping")); err != nil {
		t.Fatalf("device readiness heartbeat: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, reply, err := conn.ReadMessage(); err != nil || string(reply) != heartbeatReply {
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
	dm := NewDeviceManager(3600, 3600, nil)
	defer dm.Stop()

	const userID = int64(7)
	srv := deviceTestServer(t, dm, userID, 5)
	conn := dialDevice(t, srv)

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
				// The queue is bounded: a burst faster than the client
				// drains is refused, to be retried, never blocked on.
				for {
					err := dm.SendToDevice(userID, testDeviceID, "push")
					if err == nil {
						break
					}
					if !errors.Is(err, ErrSendQueueFull) {
						t.Errorf("SendToDevice: %v", err)
						return
					}
					time.Sleep(time.Millisecond)
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
					dm.UpdateHeartbeat(userID, testDeviceID)
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
	_ = conn.Close()
	<-closed
	if got := received.Load(); got < pushWorkers*pushesPerWorker {
		t.Errorf("received %d messages, want at least %d", got, pushWorkers*pushesPerWorker)
	}
}

// The kick notification must reach the client before the connection closes,
// which requires the device to stay registered until OnDeviceKicked returns.
func TestKickDeliversNotificationThenCloses(t *testing.T) {
	dm := NewDeviceManager(3600, 3600, nil)
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
	conn := dialDevice(t, srv)

	dm.KickDevice(userID, testDeviceID)

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
	dm := NewDeviceManager(3600, 3600, nil)
	defer dm.Stop()

	const userID = int64(11)
	srv := deviceTestServer(t, dm, userID, 5)
	oldConn := dialDevice(t, srv)
	newConn := dialDevice(t, srv)

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

	if err := dm.SendToDevice(userID, testDeviceID, "hello"); err != nil {
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

	if got := dm.Online(); got != 1 {
		t.Errorf("Online() = %d after reconnect, want 1", got)
	}
}

// serverSocket returns the server side of a websocket connection no manager
// knows about, for a device registered by hand.
func serverSocket(t *testing.T) *websocket.Conn {
	t.Helper()
	accepted := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("websocket upgrade: %v", err)
			return
		}
		accepted <- conn
	}))
	t.Cleanup(srv.Close)
	client, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return <-accepted
}

// The heartbeat sweep gets each user's device list from the map's Range,
// before it holds the user's lock. A device that connects in between is in
// the map but not in that list; a sweep storing a list rebuilt from it
// would drop the device, leaving a socket nothing can kick and a
// totalOnline that never comes down.
func TestHeartbeatSweepKeepsADeviceConnectedWhileItWaited(t *testing.T) {
	dm := NewDeviceManager(3600, 3600, nil)
	defer dm.Stop()

	const userID = int64(13)
	srv := deviceTestServer(t, dm, userID, 2)
	conn := dialDevice(t, srv)
	t.Cleanup(func() { _ = conn.Close() })

	// The sweep starts while the test holds the user's lock: it has taken
	// its snapshot from Range and waits for the lock.
	mu := dm.getUserMutex(userID)
	mu.Lock()
	swept := make(chan struct{})
	go func() {
		defer close(swept)
		dm.checkHeartbeats()
	}()
	time.Sleep(100 * time.Millisecond)

	// Meanwhile a second device connects: AddDevice's registration, as it
	// runs under the lock.
	late := newDevice(serverSocket(t), "session", "dev2", time.Now())
	late.startWriter(userID)
	current, _ := dm.userDevices.Load(userID)
	dm.userDevices.Store(userID, append(append([]*Device(nil), current.([]*Device)...), late))
	dm.totalOnline.Add(1)
	mu.Unlock()
	<-swept

	if devices := dm.snapshotDevices(userID); len(devices) != 2 || devices[1] != late {
		t.Fatalf("devices after the sweep = %d, want both, the late one included", len(devices))
	}
	if got := dm.Online(); got != 2 {
		t.Fatalf("Online() after the sweep = %d, want 2", got)
	}
	if err := dm.SendToDevice(userID, "dev2", "hello"); err != nil {
		t.Fatalf("the late device is unreachable: %v", err)
	}
	dm.KickDevice(userID, "dev2")
	if got := dm.Online(); got != 1 {
		t.Fatalf("Online() after kicking the late device = %d, want 1", got)
	}
}
