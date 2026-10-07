package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// A proyecto is what an issued quote opens: the deal itself, followed from the day the
// client gets the quote until it is delivered and closed. It is named after the base
// folio of the quote that opened it (QA0105) and keeps going through that quote's
// revisions (QA0105-R1...). Its current quote is the one quote of the proyecto that is
// not revisada. See migrations/0016_projects.sql.

// ProjectFlow is the order a proyecto follows: the client has the quote and it is being
// followed up (prospecto), the client's purchase order arrived (oc_recibida), the goods
// are on their way or delivered (en_entrega), and it was invoiced and collected
// (cerrado). A proyecto moves one step at a time. The schema also knows facturado and
// perdido, which no code uses yet.
var ProjectFlow = []string{"prospecto", "oc_recibida", "en_entrega", "cerrado"}

// StatusLabels are the words users see for each quote status.
var StatusLabels = map[string]string{
	"borrador": "Borrador",
	"emitida":  "Emitida",
	"revisada": "Revisada",
}

// StageLabels are the words users see for each proyecto stage.
var StageLabels = map[string]string{
	"prospecto":   "Prospecto",
	"oc_recibida": "O.C. recibida",
	"facturado":   "Facturado",
	"en_entrega":  "En entrega",
	"cerrado":     "Cerrado",
	"perdido":     "Perdido",
}

func flowIndex(stage string) int {
	for i, st := range ProjectFlow {
		if st == stage {
			return i
		}
	}
	return -1
}

// NextStage is the stage after stage in the flow, or "" when there is none (a stage that
// isn't in the flow, or the last one).
func NextStage(stage string) string {
	if i := flowIndex(stage); i >= 0 && i+1 < len(ProjectFlow) {
		return ProjectFlow[i+1]
	}
	return ""
}

// PrevStage is the stage before stage in the flow, or "" when there is none.
func PrevStage(stage string) string {
	if i := flowIndex(stage); i > 0 {
		return ProjectFlow[i-1]
	}
	return ""
}

// ProbabilityStep is one of the fixed values a prospecto's probabilidad de cierre takes.
// Users pick and read the word; the percentage is a reference and the weight in totals.
type ProbabilityStep struct {
	Percent int
	Label   string
}

// ProbabilitySteps are the probabilidad de cierre choices, lowest first. A new proyecto
// starts at the first. There is no 0 or 100: receiving the purchase order ends the guess.
var ProbabilitySteps = []ProbabilityStep{
	{10, "Inicial"}, {25, "Baja"}, {50, "Media"}, {75, "Alta"}, {90, "Inminente"},
}

// ForecastThreshold is the probability from which a prospecto is relevante para
// pronóstico, what the team calls "en pipeline" when it reviews what is about to close.
const ForecastThreshold = 75

// ProbabilityLabel is the word for a probability step, or "" for a value that isn't one.
func ProbabilityLabel(percent int) string {
	for _, st := range ProbabilitySteps {
		if st.Percent == percent {
			return st.Label
		}
	}
	return ""
}

// ForecastRelevant reports whether a proyecto counts for the forecast: a prospecto at
// ForecastThreshold or more. It is derived, never stored, so it can't disagree with the
// probability.
func ForecastRelevant(stage string, probability int) bool {
	return stage == "prospecto" && probability >= ForecastThreshold
}

// ErrBadTransition is returned by MoveProject when the proyecto isn't in the stage the
// move starts from, the target isn't the stage right before or after it, or it would
// leave prospecto while its current quote is a revision still in borrador. It is also
// what SetProjectProbability returns for a proyecto that is no longer a prospecto or a
// value that isn't a step.
var ErrBadTransition = errors.New("store: project can't move to that stage")

// MoveProject moves a proyecto one stage along ProjectFlow, from `from` to `to` (either
// the next stage or the previous one), and records it as a comment on its current quote
// — "Pasó a O.C. recibida." plus the user's note, if any — so the history reads in one
// place. The change is conditional on the proyecto still being in `from`, so two people
// clicking at once can't skip a stage. A prospecto whose current quote is an unissued
// revision can't move on: the purchase order has to answer an issued quote. Whether the
// user may go backwards is the caller's decision.
func (s *Store) MoveProject(ctx context.Context, projectID, userID int64, from, to, note string) error {
	if NextStage(from) != to && PrevStage(from) != to || to == "" {
		return ErrBadTransition
	}
	verb := "Pasó a"
	if flowIndex(to) < flowIndex(from) {
		verb = "Regresó a"
	}
	body := fmt.Sprintf("%s %s.", verb, StageLabels[to])
	if note = strings.TrimSpace(note); note != "" {
		body += "\n" + note
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: move project %d: %w", projectID, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return fmt.Errorf("store: move project %d: %w", projectID, err)
	}
	quoteID, quoteStatus, err := currentQuote(ctx, tx, projectID)
	if err != nil {
		return fmt.Errorf("store: move project %d: %w", projectID, err)
	}
	if quoteID == 0 || from == "prospecto" && quoteStatus != "emitida" {
		return ErrBadTransition
	}
	res, err := tx.ExecContext(ctx, `UPDATE projects SET status = ? WHERE id = ? AND status = ?`, to, projectID, from)
	if err != nil {
		return fmt.Errorf("store: move project %d: %w", projectID, err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrBadTransition
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO quote_comments (quote_id, user_id, body) VALUES (?, ?, ?)`, quoteID, userID, body); err != nil {
		return fmt.Errorf("store: move project %d: comment: %w", projectID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: move project %d: %w", projectID, err)
	}
	return nil
}

// SetProjectProbability sets a prospecto's probabilidad de cierre to one of
// ProbabilitySteps and records it as a comment on its current quote — "Probabilidad:
// Baja → Alta." plus the user's note, if any. Setting the value it already has changes
// nothing, though a note is still saved as a comment.
func (s *Store) SetProjectProbability(ctx context.Context, projectID, userID int64, percent int, note string) error {
	if ProbabilityLabel(percent) == "" {
		return ErrBadTransition
	}
	note = strings.TrimSpace(note)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: set probability of project %d: %w", projectID, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return fmt.Errorf("store: set probability of project %d: %w", projectID, err)
	}
	var old int
	err = tx.QueryRowContext(ctx,
		`SELECT probability FROM projects WHERE id = ? AND status = 'prospecto'`, projectID).Scan(&old)
	if err == sql.ErrNoRows {
		return ErrBadTransition
	}
	if err != nil {
		return fmt.Errorf("store: set probability of project %d: %w", projectID, err)
	}
	quoteID, _, err := currentQuote(ctx, tx, projectID)
	if err != nil {
		return fmt.Errorf("store: set probability of project %d: %w", projectID, err)
	}
	if quoteID == 0 {
		return ErrBadTransition
	}
	body := note
	if old != percent {
		if _, err := tx.ExecContext(ctx, `
			UPDATE projects SET probability = ?, probability_updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
			WHERE id = ?`, percent, projectID); err != nil {
			return fmt.Errorf("store: set probability of project %d: %w", projectID, err)
		}
		body = fmt.Sprintf("Probabilidad: %s → %s.", ProbabilityLabel(old), ProbabilityLabel(percent))
		if note != "" {
			body += "\n" + note
		}
	}
	if body != "" {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO quote_comments (quote_id, user_id, body) VALUES (?, ?, ?)`, quoteID, userID, body); err != nil {
			return fmt.Errorf("store: set probability of project %d: comment: %w", projectID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: set probability of project %d: %w", projectID, err)
	}
	return nil
}

// currentQuote finds a proyecto's current quote, the one that is not revisada, and its
// status (emitida, or borrador for a revision in progress). The id is 0 when the
// proyecto has none.
func currentQuote(ctx context.Context, tx *sql.Tx, projectID int64) (id int64, status string, err error) {
	err = tx.QueryRowContext(ctx,
		`SELECT id, status FROM quotes WHERE project_id = ? AND status != 'revisada'`, projectID).Scan(&id, &status)
	if err == sql.ErrNoRows {
		return 0, "", nil
	}
	return id, status, err
}
