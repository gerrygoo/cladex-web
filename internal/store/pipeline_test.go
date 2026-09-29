package store

import (
	"context"
	"errors"
	"testing"
)

func TestNextAndPrevStatus(t *testing.T) {
	for _, c := range []struct{ status, next, prev string }{
		{"borrador", "", ""},
		{"revisada", "", ""},
		{"emitida", "pipeline", ""},
		{"pipeline", "oc_emitida", "emitida"},
		{"oc_emitida", "entregada", "pipeline"},
		{"entregada", "cerrada", "oc_emitida"},
		{"cerrada", "", "entregada"},
	} {
		if got := NextStatus(c.status); got != c.next {
			t.Errorf("NextStatus(%q) = %q, want %q", c.status, got, c.next)
		}
		if got := PrevStatus(c.status); got != c.prev {
			t.Errorf("PrevStatus(%q) = %q, want %q", c.status, got, c.prev)
		}
	}
}

func TestMoveQuote(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	customerID, err := s.CreateCustomer(ctx, Customer{Name: "Grupo PEME"})
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	user, err := s.CreateUser(ctx, "rfm", "Rodolfo", "hash", "vendedor")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO quotes (folio, prefix, customer_id, user_id, status) VALUES ('QA0001', 'QA', ?, ?, 'emitida')`,
		customerID, user)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	id, _ := res.LastInsertId()
	status := func() string {
		q, err := s.QuoteByID(ctx, id)
		if err != nil || q == nil {
			t.Fatalf("QuoteByID: %v", err)
		}
		return q.Status
	}

	// Skipping a stage, or a stage the quote isn't in, is refused and changes nothing.
	for _, c := range []struct{ from, to string }{
		{"emitida", "oc_emitida"}, {"emitida", "emitida"}, {"emitida", ""}, {"emitida", "revisada"},
		{"pipeline", "oc_emitida"}, // right step, but the quote is still emitida
	} {
		if err := s.MoveQuote(ctx, id, user, c.from, c.to, ""); !errors.Is(err, ErrBadTransition) {
			t.Errorf("MoveQuote(%q → %q) = %v, want ErrBadTransition", c.from, c.to, err)
		}
	}
	if got := status(); got != "emitida" {
		t.Fatalf("status after refused moves = %q", got)
	}

	if err := s.MoveQuote(ctx, id, user, "emitida", "pipeline", "Le interesa, pidió entrega en 3 semanas"); err != nil {
		t.Fatalf("MoveQuote forward: %v", err)
	}
	if got := status(); got != "pipeline" {
		t.Fatalf("status = %q, want pipeline", got)
	}
	if err := s.MoveQuote(ctx, id, user, "pipeline", "emitida", ""); err != nil {
		t.Fatalf("MoveQuote back: %v", err)
	}
	// A pipeline quote is no longer revisable: only emitida is.
	if err := s.MoveQuote(ctx, id, user, "emitida", "pipeline", ""); err != nil {
		t.Fatalf("MoveQuote forward again: %v", err)
	}
	if _, err := s.CreateRevision(ctx, id, user); !errors.Is(err, ErrQuoteNotIssued) {
		t.Errorf("CreateRevision(pipeline) = %v, want ErrQuoteNotIssued", err)
	}
	for _, to := range []string{"oc_emitida", "entregada", "cerrada"} {
		from := PrevStatus(to)
		if err := s.MoveQuote(ctx, id, user, from, to, ""); err != nil {
			t.Fatalf("MoveQuote %s → %s: %v", from, to, err)
		}
	}
	if err := s.MoveQuote(ctx, id, user, "cerrada", "", ""); !errors.Is(err, ErrBadTransition) {
		t.Errorf("MoveQuote past the last stage = %v", err)
	}

	// Each move left a comment, the first with the user's note.
	comments, err := s.ListQuoteComments(ctx, id)
	if err != nil {
		t.Fatalf("ListQuoteComments: %v", err)
	}
	if len(comments) != 6 {
		t.Fatalf("comments = %d, want 6", len(comments))
	}
	last := comments[len(comments)-1]
	if last.Body != "Pasó a Pipeline.\nLe interesa, pidió entrega en 3 semanas" {
		t.Errorf("first comment = %q", last.Body)
	}
	if comments[len(comments)-2].Body != "Regresó a Emitida." && comments[len(comments)-2].Body != "Pasó a Pipeline." {
		t.Errorf("second comment = %q", comments[len(comments)-2].Body)
	}
	if comments[0].Body != "Pasó a Entregada y cerrada." {
		t.Errorf("newest comment = %q", comments[0].Body)
	}
}
