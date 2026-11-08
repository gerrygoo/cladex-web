package notify

import (
	"errors"
	"fmt"
	"io/fs"
	"net/mail"
	"os"
	"strconv"
	"strings"

	"github.com/gerrygoo/cladex-web/internal/appenv"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// Config is the senders Load set up, and their Status.
type Config struct {
	senders map[string]Sender
	Status  Status
}

// Status says which channels are on and, for each that is off, why, for the logs at
// startup and for Configuración del sistema. It never carries a secret.
type Status struct {
	Channels []ChannelStatus
	// Production is whether this instance sends to anyone. Otherwise it only sends to
	// Allow, and records everything else as cancelada.
	Production bool
	Allow      []string
}

// ChannelStatus is one channel's part of Status.
type ChannelStatus struct {
	Channel  string
	Enabled  bool
	Provider string
	From     string
	Problem  string // why it is off; empty when Enabled
}

// Channel returns channel's status; an unknown channel is off.
func (st Status) Channel(channel string) ChannelStatus {
	for _, c := range st.Channels {
		if c.Channel == channel {
			return c
		}
	}
	return ChannelStatus{Channel: channel, Problem: "no está configurado"}
}

// Allowed reports whether this instance may send to address: anyone in production, only
// NOTIFY_ALLOW elsewhere (the way staging can't stamp real CFDIs).
func (st Status) Allowed(address string) bool {
	if st.Production {
		return true
	}
	for _, a := range st.Allow {
		if strings.EqualFold(a, strings.TrimSpace(address)) {
			return true
		}
	}
	return false
}

// Secret files, as facturación's: Compose secrets under /run/secrets, never environment
// variables, which `docker inspect` shows. *_FILE overrides a path for local runs.
const (
	resendKeyFile    = "/run/secrets/resend_api_key"
	smtpPasswordFile = "/run/secrets/smtp_password"
)

// Load reads the notification settings:
//
//   - EMAIL_PROVIDER: resend, smtp or fake (logs instead of sending); unset, email is off.
//   - EMAIL_FROM: the sender, e.g. "Cladex <noreply@cladex.com.mx>".
//   - Resend: its API key in the resend_api_key secret.
//   - SMTP: SMTP_HOST, SMTP_PORT (465, implicit TLS, or 587, STARTTLS; default 465),
//     SMTP_USERNAME (default: EMAIL_FROM's address), and the smtp_password secret.
//   - NOTIFY_ALLOW: comma-separated addresses and numbers a non-production instance may
//     send to.
//
// Only an explicitly production instance sends to everyone.
func Load(env appenv.Env, getenv func(string) string) *Config {
	cfg := &Config{senders: map[string]Sender{}}
	cfg.Status.Production = env.IsExplicitProduction()
	for _, a := range strings.Split(getenv("NOTIFY_ALLOW"), ",") {
		if a = strings.TrimSpace(a); a != "" {
			cfg.Status.Allow = append(cfg.Status.Allow, a)
		}
	}

	email := ChannelStatus{Channel: store.ChannelEmail, Provider: getenv("EMAIL_PROVIDER")}
	sender, err := loadEmail(email.Provider, getenv)
	if err != nil {
		email.Problem = err.Error()
	} else {
		email.Enabled = true
		email.From = getenv("EMAIL_FROM")
		cfg.senders[store.ChannelEmail] = sender
	}
	whatsapp := ChannelStatus{Channel: store.ChannelWhatsApp, Problem: "todavía no está disponible"}
	cfg.Status.Channels = []ChannelStatus{email, whatsapp}
	return cfg
}

func loadEmail(provider string, getenv func(string) string) (Sender, error) {
	if provider == "" {
		return nil, errors.New("no está configurado")
	}
	from := getenv("EMAIL_FROM")
	switch provider {
	case "fake":
		return NewFake(), nil
	case "resend", "smtp":
	default:
		return nil, fmt.Errorf("EMAIL_PROVIDER %q: usa resend, smtp o fake", provider)
	}
	fromAddr, err := mail.ParseAddress(from)
	if err != nil {
		return nil, fmt.Errorf("EMAIL_FROM %q no es una dirección válida", from)
	}
	if provider == "resend" {
		key, err := readSecret(getenv, "RESEND_API_KEY_FILE", resendKeyFile, "la llave de Resend")
		if err != nil {
			return nil, err
		}
		return NewResend(key, from), nil
	}
	host := getenv("SMTP_HOST")
	if host == "" {
		return nil, errors.New("falta SMTP_HOST")
	}
	port := 465
	if p := getenv("SMTP_PORT"); p != "" {
		if port, err = strconv.Atoi(p); err != nil || (port != 465 && port != 587) {
			return nil, fmt.Errorf("SMTP_PORT %q: usa 465 o 587", p)
		}
	}
	user := getenv("SMTP_USERNAME")
	if user == "" {
		user = fromAddr.Address
	}
	password, err := readSecret(getenv, "SMTP_PASSWORD_FILE", smtpPasswordFile, "la contraseña SMTP")
	if err != nil {
		return nil, err
	}
	return &SMTP{Host: host, Port: port, Username: user, Password: password, From: from, ImplicitTLS: port == 465}, nil
}

func readSecret(getenv func(string) string, env, path, label string) (string, error) {
	if p := getenv(env); p != "" {
		path = p
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("falta %s", label)
	}
	if err != nil {
		return "", fmt.Errorf("no se pudo leer %s: %v", label, err)
	}
	v := strings.TrimSpace(string(b))
	if v == "" {
		return "", fmt.Errorf("%s está vacía", label)
	}
	return v, nil
}
