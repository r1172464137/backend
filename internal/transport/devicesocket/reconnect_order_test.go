package devicesocket

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// HTTP 101 is not a registration acknowledgement: gorilla's Upgrade writes
// it before returning the connection to AddDevice. Reproduce that gap under
// channel control, and require the fixture's dial to complete the real
// heartbeat round trip before reporting a ready device.
func TestDialDeviceWaitsForRegistration(t *testing.T) {
	dm := NewDeviceManager(3600, 3600, nil)
	defer dm.Stop()

	heartbeatReceived := make(chan struct{})
	releaseRegistration := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseRegistration) }) }
	// A failed readiness assertion must not leave the handler blocked.
	t.Cleanup(release)
	const userID = int64(31)
	handlerDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerDone)
		conn, err := dm.upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		// Observe the fixture's probe without registering. The former helper
		// returned as soon as Upgrade wrote 101, without sending this probe.
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		_, message, err := conn.ReadMessage()
		if err != nil || string(message) != "ping" {
			_ = conn.Close()
			t.Errorf("readiness probe = %q, %v", message, err)
			return
		}
		close(heartbeatReceived)
		<-releaseRegistration
		device := dm.register(conn, "session", userID, testDeviceID, 5)
		// The handler already consumed the first heartbeat while holding the
		// registration gap open; answer it through the actual manager writer.
		dm.UpdateHeartbeat(userID, testDeviceID)
		go dm.serve(userID, device)
	}))
	t.Cleanup(srv.Close)

	dialed := make(chan *websocket.Conn, 1)
	dialDone := make(chan struct{})
	go func() {
		defer close(dialDone)
		dialed <- dialDevice(t, srv)
	}()
	// Release and join before any cleanup can finish the test: the dial
	// helper may still be reporting an error from its goroutine.
	t.Cleanup(func() {
		release()
		select {
		case <-dialDone:
		case <-time.After(4 * time.Second):
			t.Error("dial helper did not finish")
		}
		select {
		case <-handlerDone:
		case <-time.After(4 * time.Second):
			t.Error("registration handler did not finish")
		}
	})

	select {
	case <-dialDone:
		t.Fatal("dial completed before the registration/readiness round trip")
	case <-heartbeatReceived:
	case <-time.After(3 * time.Second):
		t.Fatal("dial did not send its readiness heartbeat")
	}
	if got := dm.Online(); got != 0 {
		t.Fatalf("device registered before the test released it: %d online", got)
	}
	select {
	case <-dialDone:
		t.Fatal("dial completed before its heartbeat was acknowledged")
	default:
	}
	release()
	select {
	case <-dialed:
	case <-dialDone:
		// The result can already be queued when dialDone closes too.
		select {
		case <-dialed:
		default:
			t.Fatal("dial failed after registration")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("dial did not finish after registration")
	}
	if got := dm.Online(); got != 1 {
		t.Fatalf("ready socket has %d devices online, want 1", got)
	}
}

// Keep serve's old-connection cleanup under direct control, rather than
// hoping its deferred removeDevice races with register on a particular run.
// A retired read loop must not remove the replacement, emit its offline
// event, alter the online count, or affect another device/user.
func TestRetiredSocketCleanupKeepsReplacement(t *testing.T) {
	dm := NewDeviceManager(3600, 3600, nil)
	defer dm.Stop()
	const userID = int64(33)
	type offlineEvent struct {
		userID    int64
		deviceID  string
		session   string
		createdAt time.Time
	}
	offline := make(chan offlineEvent, 8)
	dm.OnDeviceOffline = func(uid int64, deviceID, session string, createdAt time.Time) {
		offline <- offlineEvent{uid, deviceID, session, createdAt}
	}
	register := func(uid int64, deviceID, session string) *Device {
		d := dm.register(serverSocket(t), session, uid, deviceID, 5)
		t.Cleanup(func() {
			dm.removeDevice(uid, d)
			d.close()
			d.writer.Wait()
		})
		return d
	}
	old := register(userID, testDeviceID, "old-session")
	peer := register(userID, "other-device", "peer-session")
	otherUser := register(userID+1, testDeviceID, "other-user-session")
	replacement := register(userID, testDeviceID, "new-session")

	// Old serve cleanup can run after the replacement is visible, and can
	// be retried. Its identity is not the device ID or the current session.
	var cleanup sync.WaitGroup
	for range 8 {
		cleanup.Go(func() {
			for range 50 {
				dm.removeDevice(userID, old)
			}
		})
	}
	cleanup.Wait()
	devices := dm.snapshotDevices(userID)
	if len(devices) != 2 || devices[0] != peer || devices[1] != replacement {
		t.Fatalf("stale cleanup changed the replacement/peer: %+v", devices)
	}
	others := dm.snapshotDevices(userID + 1)
	if len(others) != 1 || others[0] != otherUser {
		t.Fatalf("stale cleanup changed another user's socket: %+v", others)
	}
	if got := dm.Online(); got != 3 {
		t.Fatalf("online after stale cleanup = %d, want 3", got)
	}
	select {
	case event := <-offline:
		t.Fatalf("retired socket cleanup emitted an offline event: %+v", event)
	default:
	}

	// Only the currently registered socket may remove its own entry and
	// emit the matching offline session/time. Repeated removal is a no-op.
	dm.removeDevice(userID, replacement)
	dm.removeDevice(userID, replacement)
	if got := dm.Online(); got != 2 {
		t.Fatalf("online after replacement disconnect = %d, want 2", got)
	}
	devices = dm.snapshotDevices(userID)
	if len(devices) != 1 || devices[0] != peer {
		t.Fatalf("replacement disconnect changed its peer: %+v", devices)
	}
	select {
	case event := <-offline:
		want := offlineEvent{userID, testDeviceID, replacement.Session, replacement.CreatedAt}
		if event != want {
			t.Fatalf("offline event = %+v, want %+v", event, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("replacement disconnect did not emit its offline event")
	}
}
