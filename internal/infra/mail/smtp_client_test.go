package mail

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSMTPClientVerifiesCertificatesByDefault(t *testing.T) {
	client := NewSMTPClient(&SMTPConfig{Host: "smtp.example.test", Port: 587})
	tlsConfig := client.tlsConfig
	if tlsConfig == nil || tlsConfig.InsecureSkipVerify {
		t.Fatalf("TLS config = %+v, want certificate verification", tlsConfig)
	}
	if tlsConfig.ServerName != "smtp.example.test" || tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("TLS config = %+v, want the relay host and TLS 1.2 or later", tlsConfig)
	}
}

func TestSMTPSenderHonorsExplicitVerificationOptOut(t *testing.T) {
	sender, err := NewSender("smtp", `{"host":"smtp.example.test","port":587,"insecure_skip_verify":true}`, "Example")
	if err != nil {
		t.Fatal(err)
	}
	if !sender.(*SMTPClient).tlsConfig.InsecureSkipVerify {
		t.Fatal("insecure_skip_verify in the stored config was ignored")
	}
}

func TestSMTPClientTLSModeSelection(t *testing.T) {
	for _, tt := range []struct {
		port                       int
		ssl, implicitTLS           bool
		wantImplicit, wantRequired bool
	}{
		// 465 is SMTPS by definition, whatever the flags say.
		{port: 465, wantImplicit: true},
		{port: 465, ssl: true, wantImplicit: true},
		// On the relay and submission ports ssl means a mandatory STARTTLS:
		// the providers document the flag on 587 and 2525.
		{port: 25, ssl: true, wantRequired: true},
		{port: 587, ssl: true, wantRequired: true},
		{port: 2525, ssl: true, wantRequired: true},
		// Without ssl the upgrade stays opportunistic.
		{port: 587},
		{port: 2525},
		// implicit_tls starts with TLS on any port; ssl adds nothing then.
		{port: 2465, implicitTLS: true, wantImplicit: true},
		{port: 2465, implicitTLS: true, ssl: true, wantImplicit: true},
	} {
		client := NewSMTPClient(&SMTPConfig{Host: "smtp.example.test", Port: tt.port, SSL: tt.ssl, ImplicitTLS: tt.implicitTLS})
		if client.implicitTLS != tt.wantImplicit || client.requireTLS != tt.wantRequired {
			t.Errorf("port %d ssl=%v implicit_tls=%v: implicit TLS = %v, STARTTLS required = %v; want %v and %v",
				tt.port, tt.ssl, tt.implicitTLS, client.implicitTLS, client.requireTLS, tt.wantImplicit, tt.wantRequired)
		}
	}
}

// implicit_tls starts the session with the TLS handshake, on any port.
func TestSMTPClientImplicitTLSStartsWithTheHandshake(t *testing.T) {
	cert := selfSignedCertificate(t)
	relay := startFakeSMTP(t, relayOptions{cert: &cert, implicit: true})
	client := NewSMTPClient(&SMTPConfig{Host: "127.0.0.1", Port: relay.port, ImplicitTLS: true, From: "panel@example.test", InsecureSkipVerify: true})

	if err := client.SendContext(context.Background(), []string{"user@example.test"}, "subject", "body"); err != nil {
		t.Fatalf("send through an implicit TLS relay: %v", err)
	}
	if session := relay.session(); session.data == "" || session.tls {
		t.Fatalf("session = %+v, want the message delivered without a STARTTLS upgrade", session)
	}
}

// A relay presenting a certificate nobody vouches for must be refused, both
// when the session upgrades through STARTTLS and when it starts with TLS.
func TestSMTPClientRejectsUntrustedCertificate(t *testing.T) {
	cert := selfSignedCertificate(t)
	for name, implicit := range map[string]bool{"starttls": false, "implicit tls": true} {
		t.Run(name, func(t *testing.T) {
			relay := startFakeSMTP(t, relayOptions{cert: &cert, implicit: implicit})
			client := NewSMTPClient(&SMTPConfig{Host: "127.0.0.1", Port: relay.port, ImplicitTLS: implicit, From: "panel@example.test"})

			err := client.SendContext(context.Background(), []string{"user@example.test"}, "subject", "body")

			var verifyErr *tls.CertificateVerificationError
			if !errors.As(err, &verifyErr) {
				t.Fatalf("send error = %v, want a certificate verification failure", err)
			}
		})
	}
}

func TestSMTPClientOptOutAcceptsSelfSignedRelay(t *testing.T) {
	cert := selfSignedCertificate(t)
	relay := startFakeSMTP(t, relayOptions{cert: &cert})
	client := NewSMTPClient(&SMTPConfig{Host: "127.0.0.1", Port: relay.port, From: "panel@example.test", InsecureSkipVerify: true})

	if err := client.SendContext(context.Background(), []string{"user@example.test"}, "subject", "body"); err != nil {
		t.Fatalf("send through self-signed relay with explicit opt-out: %v", err)
	}
	if relay.session().data == "" {
		t.Fatal("the message was not transferred")
	}
}

// Relays without TLS keep working: STARTTLS stays opportunistic.
func TestSMTPClientSendsThroughPlainRelay(t *testing.T) {
	relay := startFakeSMTP(t, relayOptions{})
	client := NewSMTPClient(&SMTPConfig{Host: "127.0.0.1", Port: relay.port, From: "panel@example.test"})

	if err := client.SendContext(context.Background(), []string{"user@example.test"}, "subject", "body"); err != nil {
		t.Fatalf("send through plain relay: %v", err)
	}
}

func TestSMTPClientTransfersTheMessage(t *testing.T) {
	relay := startFakeSMTP(t, relayOptions{})
	client := NewSMTPClient(&SMTPConfig{Host: "127.0.0.1", Port: relay.port, From: "panel@example.test", ReplyTo: "help@example.test", SiteName: "PPanel"})

	if err := client.SendContext(context.Background(), []string{"a@example.test", "b@example.test"}, "Welcome", "<p>hello</p>"); err != nil {
		t.Fatalf("SendContext: %v", err)
	}
	session := relay.session()
	if session.from != "<panel@example.test>" || strings.Join(session.rcpt, ",") != "<a@example.test>,<b@example.test>" {
		t.Fatalf("envelope = %q -> %v", session.from, session.rcpt)
	}
	for _, want := range []string{`From: "PPanel" <panel@example.test>`, "Reply-To: help@example.test", "To: a@example.test, b@example.test", "Subject: Welcome", "Content-Type: text/html", "<p>hello</p>"} {
		if !strings.Contains(session.data, want) {
			t.Fatalf("message = %q, want %q", session.data, want)
		}
	}
	if !session.quit {
		t.Fatal("the session was not closed with QUIT")
	}
}

// Configured credentials are presented with the mechanism the relay offers.
func TestSMTPClientAuthenticates(t *testing.T) {
	for name, mechanisms := range map[string]string{"plain": "PLAIN LOGIN", "login": "LOGIN"} {
		t.Run(name, func(t *testing.T) {
			relay := startFakeSMTP(t, relayOptions{auth: mechanisms})
			client := NewSMTPClient(&SMTPConfig{Host: "127.0.0.1", Port: relay.port, User: "mailer", Pass: "s3cret", From: "panel@example.test"})

			if err := client.SendContext(context.Background(), []string{"user@example.test"}, "subject", "body"); err != nil {
				t.Fatalf("send: %v", err)
			}
			if got := relay.session().credentials; got != "mailer:s3cret" {
				t.Fatalf("credentials = %q, want mailer:s3cret over %s", got, name)
			}
		})
	}
}

// A relay that stops answering must not hold the sender: the delivery gives
// up at the client's deadline with an ordinary failure, which a batch
// campaign records against the recipient instead of treating it as its own
// cancellation.
func TestSMTPClientGivesUpOnAStalledRelay(t *testing.T) {
	relay := startFakeSMTP(t, relayOptions{stall: true})
	client := NewSMTPClient(&SMTPConfig{Host: "127.0.0.1", Port: relay.port, From: "panel@example.test"})
	client.timeout = 200 * time.Millisecond

	start := time.Now()
	err := client.SendContext(context.Background(), []string{"user@example.test"}, "subject", "body")
	if err == nil || time.Since(start) > 3*time.Second {
		t.Fatalf("error = %v after %s, want a prompt failure", err, time.Since(start))
	}
	if !errors.Is(err, os.ErrDeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want an i/o timeout that is not a context error", err)
	}
}

// Cancelling the caller's context ends a blocked conversation at once, and
// the error says so.
func TestSMTPClientStopsWhenTheContextEnds(t *testing.T) {
	relay := startFakeSMTP(t, relayOptions{stall: true})
	client := NewSMTPClient(&SMTPConfig{Host: "127.0.0.1", Port: relay.port, From: "panel@example.test"})
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)

	start := time.Now()
	err := client.SendContext(ctx, []string{"user@example.test"}, "subject", "body")
	if !errors.Is(err, context.Canceled) || time.Since(start) > 3*time.Second {
		t.Fatalf("error = %v after %s, want context.Canceled promptly", err, time.Since(start))
	}
}

// The ssl flag means "encryption required", not "start with TLS": the relay
// providers document it on their STARTTLS ports (2525, 587), where a TLS
// handshake would meet a plaintext greeting. The session upgrades through
// STARTTLS and the mail goes out.
func TestSMTPClientSSLMeansSTARTTLSOnSubmissionPorts(t *testing.T) {
	cert := selfSignedCertificate(t)
	relay := startFakeSMTP(t, relayOptions{cert: &cert, auth: "PLAIN LOGIN"})
	client := NewSMTPClient(&SMTPConfig{Host: "127.0.0.1", Port: relay.port, SSL: true, User: "mailer", Pass: "s3cret", From: "panel@example.test", InsecureSkipVerify: true})

	if err := client.SendContext(context.Background(), []string{"user@example.test"}, "subject", "body"); err != nil {
		t.Fatalf("send with ssl through a STARTTLS relay: %v", err)
	}
	session := relay.session()
	if !session.tls || session.data == "" || session.credentials != "mailer:s3cret" {
		t.Fatalf("session = %+v, want STARTTLS, the credentials and the message", session)
	}
}

// With ssl set, a relay that offers no STARTTLS (or a downgrade attack that
// strips the extension) is refused before anything travels in the clear:
// no credentials, no envelope, no message.
func TestSMTPClientSSLRefusesARelayWithoutSTARTTLS(t *testing.T) {
	relay := startFakeSMTP(t, relayOptions{auth: "PLAIN LOGIN"})
	client := NewSMTPClient(&SMTPConfig{Host: "127.0.0.1", Port: relay.port, SSL: true, User: "mailer", Pass: "s3cret", From: "panel@example.test"})

	err := client.SendContext(context.Background(), []string{"user@example.test"}, "subject", "body")

	if !errors.Is(err, errSTARTTLSRequired) {
		t.Fatalf("send error = %v, want the STARTTLS refusal", err)
	}
	if session := relay.session(); session.credentials != "" || session.from != "" || session.data != "" {
		t.Fatalf("session = %+v, want nothing sent in the clear", session)
	}
}

func selfSignedCertificate(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "self-signed relay"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

type relayOptions struct {
	// cert offers STARTTLS, or with implicit starts the session with TLS.
	cert     *tls.Certificate
	implicit bool
	// auth lists the AUTH mechanisms to advertise.
	auth string
	// stall accepts the connection and never answers.
	stall bool
}

type smtpSession struct {
	from        string
	rcpt        []string
	data        string
	credentials string
	quit        bool
	// tls reports that the session was upgraded through STARTTLS.
	tls bool
}

type fakeRelay struct {
	port int
	mu   sync.Mutex
	got  smtpSession
}

func (r *fakeRelay) session() smtpSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.got
}

// startFakeSMTP serves one SMTP session.
func startFakeSMTP(t *testing.T, opts relayOptions) *fakeRelay {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	relay := &fakeRelay{port: listener.Addr().(*net.TCPAddr).Port}
	var tlsConfig *tls.Config
	if opts.cert != nil {
		tlsConfig = &tls.Config{Certificates: []tls.Certificate{*opts.cert}}
	}
	done := make(chan struct{})
	t.Cleanup(func() { <-done })
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		// A client that never speaks must not hang the test.
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		if opts.stall {
			_, _ = conn.Read(make([]byte, 1))
			return
		}
		if opts.implicit {
			tlsConn := tls.Server(conn, tlsConfig)
			if tlsConn.Handshake() != nil {
				return
			}
			conn = tlsConn
			tlsConfig = nil
		}
		relay.serve(conn, tlsConfig, opts.auth)
	}()
	return relay
}

// serve speaks just enough SMTP for one message; a non-nil starttls config
// is offered through the STARTTLS extension.
func (r *fakeRelay) serve(conn net.Conn, starttls *tls.Config, auth string) {
	reader := bufio.NewReader(conn)
	reply := func(format string, args ...any) { _, _ = fmt.Fprintf(conn, format+"\r\n", args...) }
	readLine := func() (string, bool) {
		line, err := reader.ReadString('\n')
		return strings.TrimRight(line, "\r\n"), err == nil
	}
	decode := func(value string) string {
		decoded, _ := base64.StdEncoding.DecodeString(value)
		return string(decoded)
	}
	reply("220 fake ESMTP")
	for {
		line, ok := readLine()
		if !ok {
			return
		}
		command := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(command, "EHLO"), strings.HasPrefix(command, "HELO"):
			extensions := []string{"fake"}
			if starttls != nil {
				extensions = append(extensions, "STARTTLS")
			}
			if auth != "" {
				extensions = append(extensions, "AUTH "+auth)
			}
			for i, extension := range extensions {
				separator := "-"
				if i == len(extensions)-1 {
					separator = " "
				}
				reply("250%s%s", separator, extension)
			}
		case command == "STARTTLS" && starttls != nil:
			reply("220 ready")
			tlsConn := tls.Server(conn, starttls)
			if tlsConn.Handshake() != nil {
				return
			}
			conn, reader, starttls = tlsConn, bufio.NewReader(tlsConn), nil
			r.mu.Lock()
			r.got.tls = true
			r.mu.Unlock()
		case strings.HasPrefix(command, "AUTH PLAIN"):
			parts := strings.Split(decode(strings.TrimSpace(line[len("AUTH PLAIN"):])), "\x00")
			if len(parts) == 3 {
				r.mu.Lock()
				r.got.credentials = parts[1] + ":" + parts[2]
				r.mu.Unlock()
			}
			reply("235 ok")
		case command == "AUTH LOGIN":
			reply("334 %s", base64.StdEncoding.EncodeToString([]byte("Username:")))
			username, _ := readLine()
			reply("334 %s", base64.StdEncoding.EncodeToString([]byte("Password:")))
			password, _ := readLine()
			r.mu.Lock()
			r.got.credentials = decode(username) + ":" + decode(password)
			r.mu.Unlock()
			reply("235 ok")
		case strings.HasPrefix(command, "MAIL FROM:"):
			r.mu.Lock()
			r.got.from = line[len("MAIL FROM:"):]
			r.mu.Unlock()
			reply("250 ok")
		case strings.HasPrefix(command, "RCPT TO:"):
			r.mu.Lock()
			r.got.rcpt = append(r.got.rcpt, line[len("RCPT TO:"):])
			r.mu.Unlock()
			reply("250 ok")
		case command == "RSET", command == "NOOP":
			reply("250 ok")
		case command == "DATA":
			reply("354 go ahead")
			var data strings.Builder
			for {
				body, ok := readLine()
				if !ok {
					return
				}
				if body == "." {
					break
				}
				data.WriteString(body + "\n")
			}
			r.mu.Lock()
			r.got.data = data.String()
			r.mu.Unlock()
			reply("250 queued")
		case command == "QUIT":
			r.mu.Lock()
			r.got.quit = true
			r.mu.Unlock()
			reply("221 bye")
			return
		default:
			reply("502 unsupported")
		}
	}
}
