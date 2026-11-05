package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/gomail.v2"
)

const (
	// dialTimeout bounds reaching the relay.
	dialTimeout = 10 * time.Second
	// sendTimeout bounds one whole delivery: dial, TLS, authentication and
	// the transfer. Without it a relay that stops answering mid-conversation
	// holds the sending worker forever.
	sendTimeout = 60 * time.Second
)

// errSTARTTLSRequired refuses a relay that offers no STARTTLS while the
// configuration requires encryption.
var errSTARTTLSRequired = errors.New("smtp: the relay does not offer STARTTLS but ssl requires encryption; " +
	"enable STARTTLS on the relay, use implicit TLS (port 465 or implicit_tls), or clear ssl to send in the clear")

// SMTPClient sends through an SMTP relay, one connection per message.
type SMTPClient struct {
	conf SMTPConfig
	// implicitTLS starts the connection with a TLS handshake; otherwise the
	// session upgrades through STARTTLS, which requireTLS makes mandatory.
	implicitTLS bool
	requireTLS  bool
	tlsConfig   *tls.Config
	// timeout bounds one delivery (sendTimeout).
	timeout time.Duration
}

// SMTPConfig is the stored configuration of the SMTP provider; SiteName,
// the sender's display name, comes from the site settings instead.
type SMTPConfig struct {
	Host    string `json:"host"`
	Port    int    `json:"port"`
	User    string `json:"user"`
	Pass    string `json:"pass"`
	From    string `json:"from"`
	ReplyTo string `json:"reply_to"`
	// SSL requires encryption: implicit TLS on port 465 or with ImplicitTLS,
	// STARTTLS otherwise, refusing a relay that does not offer it. Unset,
	// STARTTLS stays opportunistic and a relay without TLS is used in the
	// clear.
	SSL      bool   `json:"ssl"`
	SiteName string `json:"siteName"`
	// ImplicitTLS starts the connection with a TLS handshake (SMTPS) on a
	// port other than 465, which is implicit TLS by definition. The relay and
	// submission ports (25, 587, 2525) start in plaintext and upgrade through
	// STARTTLS, so they never need it.
	ImplicitTLS bool `json:"implicit_tls"`
	// InsecureSkipVerify accepts any server certificate. It exists only for
	// relays with self-signed certificates and must be set explicitly in the
	// stored platform config; certificates are verified by default.
	InsecureSkipVerify bool `json:"insecure_skip_verify"`
}

// NewSMTPClient returns a client for conf, or nil for a nil conf.
func NewSMTPClient(conf *SMTPConfig) *SMTPClient {
	if conf == nil {
		return nil
	}
	implicit := implicitTLS(conf)
	return &SMTPClient{
		conf:        *conf,
		implicitTLS: implicit,
		// SSL makes encryption mandatory, so without implicit TLS the session
		// has to upgrade through STARTTLS. Without the flag the upgrade stays
		// opportunistic: it happens whenever the relay offers it, and relays
		// without TLS keep working.
		requireTLS: conf.SSL && !implicit,
		tlsConfig: &tls.Config{
			InsecureSkipVerify: conf.InsecureSkipVerify, //nolint:gosec // G402: an explicit operator opt-in for self-signed relays; certificates are verified by default
			MinVersion:         tls.VersionTLS12,
			ServerName:         conf.Host,
		},
		timeout: sendTimeout,
	}
}

// implicitTLS reports whether the connection starts with a TLS handshake
// (SMTPS) instead of upgrading through STARTTLS: on port 465, which is
// implicit TLS by definition, and where the configuration asks for it. The
// SSL flag does not select it: the relay providers document that flag on
// their STARTTLS ports (587, 2525), where a TLS handshake meets a plaintext
// greeting and no mail goes out.
func implicitTLS(conf *SMTPConfig) bool {
	return conf.Port == 465 || conf.ImplicitTLS
}

// SendContext delivers the message over one SMTP conversation, bounded by
// ctx and by the client's own deadline. When ctx ends the conversation the
// error matches ctx's error; when the relay outlasts the client's deadline it
// is an ordinary delivery failure.
func (m *SMTPClient) SendContext(ctx context.Context, to []string, subject, body string) error {
	deliveryCtx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	err := m.deliver(deliveryCtx, m.message(to, subject, body))
	if err != nil && ctx.Err() != nil {
		return fmt.Errorf("%w: %w", ctx.Err(), err)
	}
	return err
}

func (m *SMTPClient) message(to []string, subject, body string) *gomail.Message {
	msg := gomail.NewMessage()
	msg.SetAddressHeader("From", m.conf.From, m.conf.SiteName)
	if m.conf.ReplyTo != "" {
		msg.SetHeader("Reply-To", m.conf.ReplyTo)
	}
	msg.SetHeader("To", to...)
	msg.SetHeader("Subject", subject)
	msg.SetBody("text/html", body)
	return msg
}

func (m *SMTPClient) deliver(ctx context.Context, msg *gomail.Message) error {
	dialer := net.Dialer{Timeout: dialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(m.conf.Host, strconv.Itoa(m.conf.Port)))
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	// One deadline for the whole conversation, brought forward when ctx is
	// cancelled so a blocked read or write returns at once.
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return err
		}
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()

	client, err := m.handshake(conn)
	if err != nil {
		return err
	}
	if err := gomail.Send(transfer{client: client}, msg); err != nil {
		return err
	}
	// The relay has accepted the message; a failed goodbye changes nothing.
	_ = client.Quit()
	return nil
}

// handshake opens the SMTP session on conn: implicit TLS or STARTTLS
// (mandatory when the configuration requires encryption, opportunistic
// otherwise), then authentication when credentials are configured and the
// relay offers it.
func (m *SMTPClient) handshake(conn net.Conn) (*smtp.Client, error) {
	if m.implicitTLS {
		conn = tls.Client(conn, m.tlsConfig)
	}
	client, err := smtp.NewClient(conn, m.conf.Host)
	if err != nil {
		return nil, err
	}
	if !m.implicitTLS {
		ok, _ := client.Extension("STARTTLS")
		switch {
		case ok:
			if err := client.StartTLS(m.tlsConfig); err != nil {
				return nil, err
			}
		case m.requireTLS:
			// Only EHLO has left the client: refusing here keeps the
			// credentials and the message off the wire, whether the relay
			// lacks TLS or someone on the path stripped the extension.
			return nil, errSTARTTLSRequired
		}
	}
	if auth := m.auth(client); auth != nil {
		if err := client.Auth(auth); err != nil {
			return nil, err
		}
	}
	return client, nil
}

// auth picks the mechanism the way gomail did: CRAM-MD5 when offered, LOGIN
// when offered without PLAIN, PLAIN otherwise.
func (m *SMTPClient) auth(client *smtp.Client) smtp.Auth {
	if m.conf.User == "" {
		return nil
	}
	ok, mechanisms := client.Extension("AUTH")
	if !ok {
		return nil
	}
	switch {
	case strings.Contains(mechanisms, "CRAM-MD5"):
		return smtp.CRAMMD5Auth(m.conf.User, m.conf.Pass)
	case strings.Contains(mechanisms, "LOGIN") && !strings.Contains(mechanisms, "PLAIN"):
		return &loginAuth{username: m.conf.User, password: m.conf.Pass, host: m.conf.Host}
	default:
		return smtp.PlainAuth("", m.conf.User, m.conf.Pass, m.conf.Host)
	}
}

// transfer runs the mail transaction of one message on an open session.
type transfer struct {
	client *smtp.Client
}

func (t transfer) Send(from string, to []string, msg io.WriterTo) error {
	if err := t.client.Mail(from); err != nil {
		return err
	}
	for _, addr := range to {
		if err := t.client.Rcpt(addr); err != nil {
			return err
		}
	}
	w, err := t.client.Data()
	if err != nil {
		return err
	}
	if _, err := msg.WriteTo(w); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

// loginAuth implements the LOGIN mechanism, which net/smtp lacks. Like
// PLAIN it sends the credentials only over TLS, unless the relay advertised
// LOGIN.
type loginAuth struct {
	username, password, host string
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS {
		advertised := false
		for _, mechanism := range server.Auth {
			if mechanism == "LOGIN" {
				advertised = true
				break
			}
		}
		if !advertised {
			return "", nil, errors.New("smtp: unencrypted connection")
		}
	}
	if server.Name != a.host {
		return "", nil, errors.New("smtp: wrong host name")
	}
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch {
	case bytes.Equal(fromServer, []byte("Username:")):
		return []byte(a.username), nil
	case bytes.Equal(fromServer, []byte("Password:")):
		return []byte(a.password), nil
	default:
		return nil, fmt.Errorf("smtp: unexpected server challenge: %s", fromServer)
	}
}
