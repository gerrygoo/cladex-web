package notify

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// Fake is the sender for local development and tests (EMAIL_PROVIDER=fake): it logs and
// records messages instead of sending them. Err, when set, is what every send returns.
type Fake struct {
	mu   sync.Mutex
	sent []Message
	Err  error
}

func NewFake() *Fake { return &Fake{} }

func (f *Fake) Provider() string { return "fake" }

func (f *Fake) Send(_ context.Context, m Message) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return "", f.Err
	}
	f.sent = append(f.sent, m)
	slog.Info("notify: fake send", slog.String("to", m.To), slog.String("subject", m.Subject))
	return fmt.Sprintf("fake-%d", len(f.sent)), nil
}

// Sent returns the messages sent so far.
func (f *Fake) Sent() []Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Message(nil), f.sent...)
}
