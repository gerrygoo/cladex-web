package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// PipelineFlow is the order an issued quote follows once the client has it: shown to
// the client (emitida), the client is interested and the proposal is moving (pipeline),
// the purchase order arrived and went to the manufacturer (oc_emitida), the cable was
// delivered (entregada), and it was invoiced and collected (cerrada). A quote moves one
// step at a time. borrador comes before it and revisada is a side branch (a revision of
// an emitida quote) — neither is a step of the flow.
var PipelineFlow = []string{"emitida", "pipeline", "oc_emitida", "entregada", "cerrada"}

// StatusLabels are the words users see for each quote status.
var StatusLabels = map[string]string{
	"borrador":   "Borrador",
	"emitida":    "Emitida",
	"revisada":   "Revisada",
	"pipeline":   "Pipeline",
	"oc_emitida": "OC emitida",
	"entregada":  "Entregada",
	"cerrada":    "Entregada y cerrada",
}

func flowIndex(status string) int {
	for i, st := range PipelineFlow {
		if st == status {
			return i
		}
	}
	return -1
}

// NextStatus is the stage after status in the flow, or "" when there is none (a quote
// that isn't in the flow, or the last stage).
func NextStatus(status string) string {
	if i := flowIndex(status); i >= 0 && i+1 < len(PipelineFlow) {
		return PipelineFlow[i+1]
	}
	return ""
}

// PrevStatus is the stage before status in the flow, or "" when there is none. An
// emitida quote has none: it can only leave that stage by being revised.
func PrevStatus(status string) string {
	if i := flowIndex(status); i > 0 {
		return PipelineFlow[i-1]
	}
	return ""
}

// ErrBadTransition is returned by MoveQuote when the quote isn't in the stage the move
// starts from, or the target isn't the stage right before or after it.
var ErrBadTransition = errors.New("store: quote can't move to that stage")

// MoveQuote moves an issued quote one stage along PipelineFlow, from `from` to `to`
// (either the next stage or the previous one), and records it as a comment on the quote
// — "Pasó a Pipeline." plus the user's note, if any — so the history reads in one
// place. The status change is conditional on the quote still being in `from`, so two
// people clicking at once can't skip a stage. Whether the user may go backwards is the
// caller's decision.
func (s *Store) MoveQuote(ctx context.Context, quoteID, userID int64, from, to, note string) error {
	if NextStatus(from) != to && PrevStatus(from) != to || to == "" {
		return ErrBadTransition
	}
	verb := "Pasó a"
	if flowIndex(to) < flowIndex(from) {
		verb = "Regresó a"
	}
	body := fmt.Sprintf("%s %s.", verb, StatusLabels[to])
	if note = strings.TrimSpace(note); note != "" {
		body += "\n" + note
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: move quote %d: %w", quoteID, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return fmt.Errorf("store: move quote %d: %w", quoteID, err)
	}
	res, err := tx.ExecContext(ctx, `UPDATE quotes SET status = ? WHERE id = ? AND status = ?`, to, quoteID, from)
	if err != nil {
		return fmt.Errorf("store: move quote %d: %w", quoteID, err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrBadTransition
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO quote_comments (quote_id, user_id, body) VALUES (?, ?, ?)`, quoteID, userID, body); err != nil {
		return fmt.Errorf("store: move quote %d: comment: %w", quoteID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: move quote %d: %w", quoteID, err)
	}
	return nil
}
