package notify

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gerrygoo/cladex-web/internal/appenv"
	"github.com/gerrygoo/cladex-web/internal/jobs"
	"github.com/gerrygoo/cladex-web/internal/store"
)

func TestResendSend(t *testing.T) {
	var got struct {
		auth, key string
		body      map[string]any
	}
	status, reply := http.StatusOK, `{"id":"re_123"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/emails" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		got.auth, got.key = r.Header.Get("Authorization"), r.Header.Get("Idempotency-Key")
		json.NewDecoder(r.Body).Decode(&got.body)
		w.WriteHeader(status)
		io.WriteString(w, reply)
	}))
	defer srv.Close()
	r := NewResend("re_key", "Cladex <avisos@avisos.cladex.com.mx>")
	r.baseURL = srv.URL
	m := Message{To: "ana@example.com", Subject: "Hola", Text: "texto", HTML: "<p>html</p>", IdempotencyKey: "k-1"}

	id, err := r.Send(context.Background(), m)
	if err != nil || id != "re_123" {
		t.Fatalf("Send = %q, %v", id, err)
	}
	if got.auth != "Bearer re_key" || got.key != "k-1" || got.body["from"] != "Cladex <avisos@avisos.cladex.com.mx>" ||
		got.body["subject"] != "Hola" || got.body["html"] != "<p>html</p>" {
		t.Errorf("request: %+v", got)
	}
	if to, _ := got.body["to"].([]any); len(to) != 1 || to[0] != "ana@example.com" {
		t.Errorf("to = %v", got.body["to"])
	}

	for _, tc := range []struct {
		status    int
		reply     string
		permanent bool
		want      string
	}{
		{422, `{"statusCode":422,"name":"validation_error","message":"Invalid to field."}`, true, "Invalid to field."},
		{403, `{"name":"validation_error","message":"The avisos.cladex.com.mx domain is not verified."}`, true, "not verified"},
		{429, `{"name":"rate_limit_exceeded","message":"Too many requests."}`, false, "Too many"},
		{500, `oops`, false, "oops"},
		{200, `{}`, true, "no trae id"},
	} {
		status, reply = tc.status, tc.reply
		_, err := r.Send(context.Background(), m)
		if err == nil || jobs.IsPermanent(err) != tc.permanent || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%d: err = %v (permanent %v), want permanent %v containing %q", tc.status, err, jobs.IsPermanent(err), tc.permanent, tc.want)
		}
	}
}

// smtpServer is just enough of an SMTP server to take one message: implicit TLS or
// STARTTLS, AUTH PLAIN, and rcptReply to RCPT TO.
func smtpServer(t *testing.T, implicit bool, rcptReply string) (port int, pool *x509.CertPool, received <-chan string) {
	t.Helper()
	hs := httptest.NewUnstartedServer(nil)
	hs.StartTLS()
	t.Cleanup(hs.Close)
	cfg := &tls.Config{Certificates: hs.TLS.Certificates}
	pool = x509.NewCertPool()
	pool.AddCert(hs.Certificate())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	out := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		secure := implicit
		if implicit {
			conn = tls.Server(conn, cfg)
		}
		tp := textproto.NewConn(conn)
		tp.PrintfLine("220 prueba ESMTP")
		for {
			line, err := tp.ReadLine()
			if err != nil {
				return
			}
			verb := strings.ToUpper(strings.Fields(line)[0])
			switch verb {
			case "EHLO":
				if secure {
					tp.PrintfLine("250-prueba\r\n250 AUTH PLAIN")
				} else {
					tp.PrintfLine("250-prueba\r\n250 STARTTLS")
				}
			case "STARTTLS":
				tp.PrintfLine("220 adelante")
				conn = tls.Server(conn, cfg)
				tp = textproto.NewConn(conn)
				secure = true
			case "AUTH":
				creds, _ := base64.StdEncoding.DecodeString(strings.Fields(line)[2])
				if string(creds) != "\x00avisos@cladex.com.mx\x00secreta" {
					tp.PrintfLine("535 credenciales incorrectas")
					continue
				}
				tp.PrintfLine("235 ok")
			case "MAIL":
				tp.PrintfLine("250 ok")
			case "RCPT":
				tp.PrintfLine("%s", rcptReply)
			case "DATA":
				tp.PrintfLine("354 adelante")
				b, _ := tp.ReadDotBytes()
				tp.PrintfLine("250 en cola")
				out <- string(b)
			case "QUIT":
				tp.PrintfLine("221 adiós")
				return
			default:
				tp.PrintfLine("502 ?")
			}
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, pool, out
}

func TestSMTPSend(t *testing.T) {
	for _, implicit := range []bool{true, false} {
		port, pool, received := smtpServer(t, implicit, "250 ok")
		s := &SMTP{Host: "127.0.0.1", Port: port, Username: "avisos@cladex.com.mx", Password: "secreta",
			From: "Cladex <avisos@cladex.com.mx>", ImplicitTLS: implicit, tlsConfig: &tls.Config{RootCAs: pool, ServerName: "127.0.0.1"}}
		text, html := Email("Hola, Ana.", "Acción: revisa el proyecto")
		id, err := s.Send(context.Background(), Message{To: "ana@example.com", Subject: "Aviso:\r\nBcc: x@y.z seguimiento", Text: text, HTML: html})
		if err != nil {
			t.Fatalf("implicit=%v: %v", implicit, err)
		}
		var raw string
		select {
		case raw = <-received:
		case <-time.After(5 * time.Second):
			t.Fatal("nothing received")
		}
		msg, err := mail.ReadMessage(strings.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		subject, _ := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
		if msg.Header.Get("Message-ID") != id || !strings.HasSuffix(id, "@cladex.com.mx>") ||
			msg.Header.Get("To") != "<ana@example.com>" || subject != "Aviso: Bcc: x@y.z seguimiento" || msg.Header.Get("Bcc") != "" {
			t.Errorf("implicit=%v: headers %v, subject %q", implicit, msg.Header, subject)
		}
		_, params, _ := mime.ParseMediaType(msg.Header.Get("Content-Type"))
		mr := multipart.NewReader(msg.Body, params["boundary"])
		var parts []string
		for {
			p, err := mr.NextPart()
			if err != nil {
				break
			}
			b, _ := io.ReadAll(p) // NextPart decodes quoted-printable
			parts = append(parts, p.Header.Get("Content-Type")+": "+string(b))
		}
		if len(parts) != 2 || !strings.Contains(parts[0], "Acción: revisa el proyecto") || !strings.Contains(parts[1], "<p>Acción: revisa el proyecto</p>") {
			t.Errorf("implicit=%v: parts %q", implicit, parts)
		}
	}
}

func TestSMTPErrors(t *testing.T) {
	port, pool, _ := smtpServer(t, true, "550 no existe ese buzón")
	s := &SMTP{Host: "127.0.0.1", Port: port, Username: "avisos@cladex.com.mx", Password: "secreta",
		From: "Cladex <avisos@cladex.com.mx>", ImplicitTLS: true, tlsConfig: &tls.Config{RootCAs: pool, ServerName: "127.0.0.1"}}
	if _, err := s.Send(context.Background(), Message{To: "nadie@example.com", Subject: "x", Text: "x"}); !jobs.IsPermanent(err) || !strings.Contains(err.Error(), "550") {
		t.Errorf("refused recipient: %v", err)
	}

	port, pool, _ = smtpServer(t, true, "451 intenta más tarde")
	s.Port, s.tlsConfig = port, &tls.Config{RootCAs: pool, ServerName: "127.0.0.1"}
	if _, err := s.Send(context.Background(), Message{To: "ana@example.com", Subject: "x", Text: "x"}); err == nil || jobs.IsPermanent(err) {
		t.Errorf("4xx should retry: %v", err)
	}

	port, pool, _ = smtpServer(t, true, "250 ok")
	s.Port, s.Password, s.tlsConfig = port, "mala", &tls.Config{RootCAs: pool, ServerName: "127.0.0.1"}
	if _, err := s.Send(context.Background(), Message{To: "ana@example.com", Subject: "x", Text: "x"}); !jobs.IsPermanent(err) {
		t.Errorf("bad password: %v", err)
	}
	if _, err := s.Send(context.Background(), Message{To: "no es un correo", Subject: "x"}); !jobs.IsPermanent(err) {
		t.Errorf("bad address: %v", err)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "resend")
	os.WriteFile(key, []byte("re_abc\n"), 0o600)
	prod := appenv.Env{Name: appenv.Production, Explicit: true}
	staging := appenv.Env{Name: appenv.Staging, Explicit: true}

	for _, tc := range []struct {
		name    string
		env     appenv.Env
		vars    map[string]string
		problem string // "" means email is on
	}{
		{"unset", prod, nil, "no está configurado"},
		{"resend", prod, map[string]string{"EMAIL_PROVIDER": "resend", "EMAIL_FROM": "Cladex <avisos@cladex.com.mx>", "RESEND_API_KEY_FILE": key}, ""},
		{"resend without key", prod, map[string]string{"EMAIL_PROVIDER": "resend", "EMAIL_FROM": "avisos@cladex.com.mx", "RESEND_API_KEY_FILE": filepath.Join(dir, "no")}, "falta la llave de Resend"},
		{"bad from", prod, map[string]string{"EMAIL_PROVIDER": "resend", "EMAIL_FROM": "Cladex"}, "EMAIL_FROM"},
		{"smtp without host", prod, map[string]string{"EMAIL_PROVIDER": "smtp", "EMAIL_FROM": "avisos@cladex.com.mx"}, "falta SMTP_HOST"},
		{"smtp bad port", prod, map[string]string{"EMAIL_PROVIDER": "smtp", "EMAIL_FROM": "avisos@cladex.com.mx", "SMTP_HOST": "h", "SMTP_PORT": "25"}, "usa 465 o 587"},
		{"smtp", prod, map[string]string{"EMAIL_PROVIDER": "smtp", "EMAIL_FROM": "avisos@cladex.com.mx", "SMTP_HOST": "h", "SMTP_PASSWORD_FILE": key}, ""},
		{"fake", staging, map[string]string{"EMAIL_PROVIDER": "fake"}, ""},
		{"unknown", prod, map[string]string{"EMAIL_PROVIDER": "sendgrid"}, "usa resend, smtp o fake"},
	} {
		cfg := Load(tc.env, func(k string) string { return tc.vars[k] })
		email := cfg.Status.Channel(store.ChannelEmail)
		if email.Enabled != (tc.problem == "") || !strings.Contains(email.Problem, tc.problem) {
			t.Errorf("%s: %+v", tc.name, email)
		}
		if (cfg.senders[store.ChannelEmail] != nil) != email.Enabled {
			t.Errorf("%s: sender and status disagree", tc.name)
		}
	}

	cfg := Load(prod, func(k string) string {
		return map[string]string{"EMAIL_PROVIDER": "smtp", "EMAIL_FROM": "Cladex <avisos@cladex.com.mx>", "SMTP_HOST": "smtpout.secureserver.net", "SMTP_PASSWORD_FILE": key}[k]
	})
	if s := cfg.senders[store.ChannelEmail].(*SMTP); s.Port != 465 || !s.ImplicitTLS || s.Username != "avisos@cladex.com.mx" || s.Password != "re_abc" {
		t.Errorf("smtp defaults: %+v", s)
	}
	if !cfg.Status.Allowed("cualquiera@example.com") {
		t.Error("production should send to anyone")
	}

	// Outside explicit production, only NOTIFY_ALLOW. Unset APP_ENV is production for
	// everything else, but not for sending.
	for _, env := range []appenv.Env{staging, {Name: appenv.Production}} {
		cfg = Load(env, func(k string) string {
			return map[string]string{"NOTIFY_ALLOW": " gerry@example.com, +523312345678 ,"}[k]
		})
		if !cfg.Status.Allowed("Gerry@example.com") || !cfg.Status.Allowed("+523312345678") || cfg.Status.Allowed("cliente@example.com") {
			t.Errorf("%+v: allow = %q", env, cfg.Status.Allow)
		}
	}
}
