package notify

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/gerrygoo/cladex-web/internal/jobs"
)

// SMTP sends email through a mailbox's SMTP server, such as GoDaddy's: port 465 with
// implicit TLS, or 587 with STARTTLS. It never authenticates over a connection that
// isn't encrypted. The server only says whether it took the message: bounces land in
// the mailbox, not here.
type SMTP struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string // "Cladex <avisos@cladex.com.mx>"
	// ImplicitTLS is TLS from the first byte (port 465); otherwise the connection
	// upgrades with STARTTLS (587).
	ImplicitTLS bool

	tlsConfig *tls.Config // tests trust their own certificate
}

func (s *SMTP) Provider() string { return "smtp" }

// Send returns the Message-ID it gave the message, the only id there is.
func (s *SMTP) Send(ctx context.Context, m Message) (string, error) {
	from, err := mail.ParseAddress(s.From)
	if err != nil {
		return "", jobs.Permanent(fmt.Errorf("remitente %q inválido", s.From))
	}
	to, err := mail.ParseAddress(m.To)
	if err != nil {
		return "", jobs.Permanent(fmt.Errorf("dirección %q inválida", m.To))
	}
	msgID, data, err := buildEmail(from, to, m, time.Now())
	if err != nil {
		return "", jobs.Permanent(err)
	}

	tlsConfig := s.tlsConfig
	if tlsConfig == nil {
		tlsConfig = &tls.Config{ServerName: s.Host}
	}
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	dialer := &net.Dialer{Timeout: 30 * time.Second}
	var conn net.Conn
	if s.ImplicitTLS {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsConfig}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return "", fmt.Errorf("smtp: %w", err)
	}
	conn.SetDeadline(time.Now().Add(time.Minute))
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		conn.Close()
		return "", smtpError(err)
	}
	defer c.Close()
	if _, isTLS := conn.(*tls.Conn); !isTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return "", jobs.Permanent(errors.New("smtp: el servidor no ofrece STARTTLS; no se manda la contraseña sin cifrar"))
		}
		if err := c.StartTLS(tlsConfig); err != nil {
			return "", smtpError(err)
		}
	}
	steps := []func() error{
		func() error { return c.Auth(smtp.PlainAuth("", s.Username, s.Password, s.Host)) },
		func() error { return c.Mail(from.Address) },
		func() error { return c.Rcpt(to.Address) },
		func() error {
			w, err := c.Data()
			if err != nil {
				return err
			}
			if _, err := w.Write(data); err != nil {
				return err
			}
			return w.Close()
		},
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return "", smtpError(err)
		}
	}
	_ = c.Quit() // the message was accepted at the end of DATA
	return msgID, nil
}

// smtpError wraps a failed SMTP step: a 5xx reply (bad password, address refused) won't
// change on retry; a 4xx or a dropped connection may.
func smtpError(err error) error {
	err = fmt.Errorf("smtp: %w", err)
	var te *textproto.Error
	if errors.As(err, &te) && te.Code >= 500 {
		return jobs.Permanent(err)
	}
	return err
}

// buildEmail writes m as a MIME message with a plain text and an HTML part, and returns
// its Message-ID.
func buildEmail(from, to *mail.Address, m Message, now time.Time) (string, []byte, error) {
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return "", nil, err
	}
	domain := from.Address[strings.LastIndex(from.Address, "@")+1:]
	msgID := "<" + hex.EncodeToString(random) + "@" + domain + ">"

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for _, part := range []struct{ typ, content string }{{"text/plain", m.Text}, {"text/html", m.HTML}} {
		if part.content == "" {
			continue
		}
		w, err := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {part.typ + "; charset=utf-8"},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return "", nil, err
		}
		qp := quotedprintable.NewWriter(w)
		if _, err := qp.Write([]byte(part.content)); err != nil {
			return "", nil, err
		}
		if err := qp.Close(); err != nil {
			return "", nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return "", nil, err
	}

	// A line break in the subject would start a header of its own.
	subject := strings.Join(strings.Fields(m.Subject), " ")
	var msg bytes.Buffer
	for _, h := range [][2]string{
		{"From", from.String()},
		{"To", to.String()},
		{"Subject", mime.QEncoding.Encode("utf-8", subject)},
		{"Date", now.Format(time.RFC1123Z)},
		{"Message-ID", msgID},
		{"MIME-Version", "1.0"},
		{"Content-Type", "multipart/alternative; boundary=" + mw.Boundary()},
	} {
		fmt.Fprintf(&msg, "%s: %s\r\n", h[0], h[1])
	}
	msg.WriteString("\r\n")
	msg.Write(body.Bytes())
	return msgID, msg.Bytes(), nil
}
