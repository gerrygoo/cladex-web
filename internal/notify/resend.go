package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gerrygoo/cladex-web/internal/jobs"
)

// ResendURL is Resend's API.
const ResendURL = "https://api.resend.com"

// Resend sends email through Resend's HTTP API (POST /emails). It reports bounces and
// complaints by webhook, which slice N5 listens to.
type Resend struct {
	key     string
	from    string
	baseURL string
	client  *http.Client
}

func NewResend(key, from string) *Resend {
	return &Resend{key: key, from: from, baseURL: ResendURL, client: &http.Client{Timeout: 30 * time.Second}}
}

func (r *Resend) Provider() string { return "resend" }

func (r *Resend) Send(ctx context.Context, m Message) (string, error) {
	body, err := json.Marshal(map[string]any{
		"from": r.from, "to": []string{m.To}, "subject": m.Subject, "text": m.Text, "html": m.HTML,
	})
	if err != nil {
		return "", jobs.Permanent(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/emails", bytes.NewReader(body))
	if err != nil {
		return "", jobs.Permanent(err)
	}
	req.Header.Set("Authorization", "Bearer "+r.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "cladex")
	// Resend answers a repeated key (within 24 h) with the first send instead of sending
	// again.
	if m.IdempotencyKey != "" {
		req.Header.Set("Idempotency-Key", m.IdempotencyKey)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("resend: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))

	if resp.StatusCode == http.StatusOK {
		var ok struct{ ID string }
		if err := json.Unmarshal(raw, &ok); err != nil || ok.ID == "" {
			// It was accepted; without its id there's nothing to look up later, but
			// sending again would duplicate it.
			return "", jobs.Permanent(fmt.Errorf("resend aceptó el correo pero su respuesta no trae id: %s", raw))
		}
		return ok.ID, nil
	}
	var e struct{ Name, Message string }
	_ = json.Unmarshal(raw, &e)
	msg := e.Message
	if msg == "" {
		msg = string(raw)
	}
	err = fmt.Errorf("resend %d %s: %s", resp.StatusCode, e.Name, msg)
	// Rate limits, server errors and a concurrent request with the same key pass;
	// anything else (bad address, unverified domain, bad key) won't on retry.
	switch {
	case resp.StatusCode == http.StatusTooManyRequests, resp.StatusCode == http.StatusConflict, resp.StatusCode >= 500:
		return "", err
	}
	return "", jobs.Permanent(err)
}
