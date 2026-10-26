package devicesocket

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// Stable has no separate register/serve methods. Hold AddDevice inside the
// handshake Write, after sending HTTP 101 but before Upgrade returns, instead
// of copying production registration into the test. Peek observes the client's
// readiness probe without consuming it: the real read loop must answer it.
type registrationGateConn struct {
	net.Conn
	reader            *bufio.Reader
	handshakeWritten  bool
	heartbeatReceived chan struct{}
	release           <-chan struct{}
}

func (c *registrationGateConn) Read(p []byte) (int, error) { return c.reader.Read(p) }

func (c *registrationGateConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if c.handshakeWritten || err != nil {
		return n, err
	}
	c.handshakeWritten = true
	if err := c.Conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return n, err
	}
	if _, err := c.reader.Peek(1); err != nil {
		return n, err
	}
	close(c.heartbeatReceived)
	<-c.release
	return n, c.Conn.SetReadDeadline(time.Time{})
}

type registrationGateWriter struct {
	http.ResponseWriter
	heartbeatReceived chan struct{}
	release           <-chan struct{}
}

func (w *registrationGateWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := w.ResponseWriter.(http.Hijacker).Hijack()
	if err != nil {
		return nil, nil, err
	}
	return &registrationGateConn{
		Conn: conn, reader: bufio.NewReader(conn),
		heartbeatReceived: w.heartbeatReceived, release: w.release,
	}, rw, nil
}

// HTTP 101 is not a registration acknowledgement. Neither a dial nor the
// old socket's close proves stable's post-close counter adjustment completed.
// A real heartbeat round trip establishes both without polling or sleeping.
func TestDialDeviceWaitsForRegistration(t *testing.T) {
	dm := NewDeviceManager(3600, 3600)
	defer dm.Stop()

	heartbeatReceived := make(chan struct{})
	releaseRegistration := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseRegistration) }) }
	t.Cleanup(release)
	const userID = int64(31)
	handlerDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerDone)
		dm.AddDevice(&registrationGateWriter{w, heartbeatReceived, releaseRegistration}, r, "session", userID, "dev1", 5)
	}))
	t.Cleanup(srv.Close)

	dialed := make(chan *websocket.Conn, 1)
	dialDone := make(chan struct{})
	go func() {
		defer close(dialDone)
		dialed <- dialDevice(t, srv, "dev1")
	}()
	// Even a failed assertion must release the handler and join the helper
	// before test cleanup ends, so no goroutine reports errors afterwards.
	t.Cleanup(func() {
		release()
		for _, done := range []<-chan struct{}{dialDone, handlerDone} {
			select {
			case <-done:
			case <-time.After(4 * time.Second):
				t.Error("dial/registration handler did not finish")
			}
		}
	})

	select {
	case <-dialDone:
		t.Fatal("dial completed before the registration/readiness round trip")
	case <-heartbeatReceived:
	case <-time.After(3 * time.Second):
		t.Fatal("dial did not send its readiness heartbeat")
	}
	if got := atomic.LoadInt32(&dm.totalOnline); got != 0 {
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
		select {
		case <-dialed:
		default:
			t.Fatal("dial failed after registration")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("dial did not finish after registration")
	}
	if got := atomic.LoadInt32(&dm.totalOnline); got != 1 {
		t.Fatalf("ready socket has %d devices online, want 1", got)
	}
}

// Retry an old read loop's cleanup after the replacement has acknowledged
// readiness. Cleanup must match connection identity, not just DeviceID, and
// must preserve peers, other users, accounting and offline-event metadata.
func TestRetiredSocketCleanupKeepsReplacement(t *testing.T) {
	dm := NewDeviceManager(3600, 3600)
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
	var nextSession atomic.Int64
	serverForUser := func(uid int64) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session := fmt.Sprintf("session-%d", nextSession.Add(1))
			dm.AddDevice(w, r, session, uid, r.URL.Query().Get("device"), 5)
		}))
		t.Cleanup(srv.Close)
		return srv
	}
	srv := serverForUser(userID)
	otherSrv := serverForUser(userID + 1)
	register := func(srv *httptest.Server, uid int64, deviceID string) *Device {
		t.Helper()
		dialDevice(t, srv, deviceID)
		for _, d := range dm.snapshotDevices(uid) {
			if d.DeviceID == deviceID {
				t.Cleanup(func() { dm.removeDevice(uid, d) })
				return d
			}
		}
		t.Fatalf("ready socket %s was not registered", deviceID)
		return nil
	}
	old := register(srv, userID, "dev1")
	peer := register(srv, userID, "other-device")
	otherUser := register(otherSrv, userID+1, "dev1")
	replacement := register(srv, userID, "dev1")

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
	if got := atomic.LoadInt32(&dm.totalOnline); got != 3 {
		t.Fatalf("online after stale cleanup = %d, want 3", got)
	}
	select {
	case event := <-offline:
		t.Fatalf("retired socket cleanup emitted an offline event: %+v", event)
	default:
	}

	dm.removeDevice(userID, replacement)
	dm.removeDevice(userID, replacement)
	if got := atomic.LoadInt32(&dm.totalOnline); got != 2 {
		t.Fatalf("online after replacement disconnect = %d, want 2", got)
	}
	devices = dm.snapshotDevices(userID)
	if len(devices) != 1 || devices[0] != peer {
		t.Fatalf("replacement disconnect changed its peer: %+v", devices)
	}
	select {
	case event := <-offline:
		want := offlineEvent{userID, "dev1", replacement.Session, replacement.CreatedAt}
		if event != want {
			t.Fatalf("offline event = %+v, want %+v", event, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("replacement disconnect did not emit its offline event")
	}
}
