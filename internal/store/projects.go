package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gerrygoo/cladex-web/internal/money"
)

// A proyecto is what an issued quote opens: the deal itself, followed from the day the
// client gets the quote until it is delivered and closed. It is named after the base
// folio of the quote that opened it (QA0105) and keeps going through that quote's
// revisions (QA0105-R1...). Its current quote is the one quote of the proyecto that is
// not revisada. See migrations/0016_projects.sql.

// Stages a proyecto is in (projects.status, migrations/0041_project_states.sql). A
// proyecto is a prospecto while the client has the quote, en curso once the client's
// purchase order is received (ReceiveOC, the only way in), and perdido or cancelado when
// it doesn't happen. Once en curso, three states derived from its records say where it
// stands — facturación, entrega and cobro (ProjectStates) — and when all three are
// complete it is Cerrado · Ganado, which is never stored either (StageCerrado).
const (
	StageProspecto = "prospecto"
	StageEnCurso   = "en_curso"
	StagePerdido   = "perdido"
	StageCancelado = "cancelado"
	// StageCerrado is how an en curso proyecto whose three states are complete is shown
	// and filtered (Project.Stage); no row stores it.
	StageCerrado = "cerrado"
)

// StatusLabels are the words users see for each quote status.
var StatusLabels = map[string]string{
	QuoteBorrador: "Borrador",
	QuoteEmitida:  "Emitida",
	QuoteRevisada: "Revisada",
}

// StageLabels are the words users see for each proyecto stage.
var StageLabels = map[string]string{
	StageProspecto: "Prospecto",
	StageEnCurso:   "En curso",
	StageCerrado:   "Cerrado · Ganado",
	StagePerdido:   "Perdido",
	StageCancelado: "Cancelado",
}

// ProbabilityStep is one of the fixed values a prospecto's probabilidad de cierre takes.
// Users pick and read the word; the percentage is a reference and the weight in totals.
type ProbabilityStep struct {
	Percent int
	Label   string
}

// ProbabilitySteps are the probabilidad de cierre choices, lowest first. A new proyecto
// starts at the first. There is no 0 or 100 in the data: receiving the purchase order ends
// the guess. The follow-up form shows "Orden colocada 100%" and "Perdida" after these as
// labels for en_curso and perdido, not as values to store.
var ProbabilitySteps = []ProbabilityStep{
	{10, "Inicial"}, {25, "Baja"}, {50, "Media"}, {75, "Alta"}, {90, "Inminente"},
}

// ReceiveOCProbability is the probability at which the proyecto page offers the purchase
// order: Inminente, when the sale is about to close.
const ReceiveOCProbability = 90

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

// ErrBadTransition is returned when a proyecto isn't in the stage an action needs (a
// purchase order on a proyecto that is no longer a prospecto, a probability on one past
// prospecto, reopening one that isn't perdido...) or the action's data isn't valid.
var ErrBadTransition = errors.New("store: project can't move to that stage")

// PaymentMethods are the métodos de pago a proyecto's factura can have, and
// PaymentMethodLabels how users read them. The método is chosen when the factura is
// recorded (issue #61); proyectos that received their purchase order before that keep the
// one chosen then, and it is the starting choice when they are invoiced.
var PaymentMethods = []string{"PUE", "PPD"}

var PaymentMethodLabels = map[string]string{
	"PUE": "P.U.E.",
	"PPD": "P.P.D.",
}

// OC is the client's purchase order as recorded on a proyecto: its number and its date
// (YYYY-MM-DD). Both are required.
type OC struct {
	Number string
	Date   string
}

func (oc OC) valid() bool {
	if strings.TrimSpace(oc.Number) == "" {
		return false
	}
	_, err := time.Parse("2006-01-02", oc.Date)
	return err == nil
}

// text is the purchase order as the history shows it: "4411 · 07/10/2026".
func (oc OC) text() string {
	return oc.Number + " · " + dayText(oc.Date)
}

// NewFile is a file about to be attached to a proyecto.
type NewFile struct {
	Filename    string
	ContentType string
	Data        []byte
}

// ProjectFile is a project_files row without its bytes; UploadedByName is the uploader's
// current name (join).
type ProjectFile struct {
	ID             int64
	ProjectID      int64
	Kind           string
	Filename       string
	ContentType    string
	Size           int64
	UploadedByName string
	UploadedAt     string
}

// ReceiveOC records the client's purchase order on a proyecto, with its file if one is
// given. On a prospecto it is what moves the proyecto to en curso, the only way in: that
// needs all of oc and an issued current quote (a revision still in borrador has to be
// issued first). On a proyecto already en curso it corrects the data, while its
// facturación is still pendiente (a factura answers to the order it bills). Either way
// the history gets one comment saying what happened, ending with the user's note.
// Anything else is ErrBadTransition.
func (s *Store) ReceiveOC(ctx context.Context, projectID, userID int64, oc OC, file *NewFile, note string) error {
	oc.Number = strings.TrimSpace(oc.Number)
	if !oc.valid() || file != nil && (strings.TrimSpace(file.Filename) == "" || len(file.Data) == 0) {
		return ErrBadTransition
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: receive OC of project %d: %w", projectID, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return fmt.Errorf("store: receive OC of project %d: %w", projectID, err)
	}
	var status, facturacion string
	var old OC
	err = tx.QueryRowContext(ctx, `
		SELECT p.status, COALESCE(p.oc_number, ''), COALESCE(p.oc_date, ''), ps.facturacion
		FROM projects p JOIN project_states ps ON ps.project_id = p.id
		WHERE p.id = ?`, projectID).Scan(&status, &old.Number, &old.Date, &facturacion)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("store: receive OC of project %d: %w", projectID, err)
	}
	if err == sql.ErrNoRows || status != StageProspecto && (status != StageEnCurso || facturacion != FacturacionPendiente) {
		return ErrBadTransition
	}
	quoteID, quoteStatus, err := currentQuote(ctx, tx, projectID)
	if err != nil {
		return fmt.Errorf("store: receive OC of project %d: %w", projectID, err)
	}
	if quoteID == 0 || quoteStatus != QuoteEmitida {
		return ErrBadTransition
	}

	var changes []string
	switch {
	case status == StageProspecto:
		changes = append(changes, "Pasó a "+StageLabels[StageEnCurso]+".", "O.C. "+oc.text())
	case old != oc:
		changes = append(changes, "O.C. actualizada: "+oc.text())
	}
	if status == StageProspecto || old != oc {
		if _, err := tx.ExecContext(ctx, `
			UPDATE projects SET status = 'en_curso', oc_number = ?, oc_date = ?
			WHERE id = ?`, oc.Number, oc.Date, projectID); err != nil {
			return fmt.Errorf("store: receive OC of project %d: %w", projectID, err)
		}
	}
	// Orden colocada 100%: the avisos about it (#69 C and F) start from a job queued with
	// it, so they never go out for an O.C. that wasn't saved.
	if status == StageProspecto {
		if _, err := enqueueJob(ctx, tx, NewJob{Kind: AvisoOCJobKind,
			Payload: fmt.Sprintf(`{"project_id":%d}`, projectID)}, time.Now()); err != nil {
			return fmt.Errorf("store: receive OC of project %d: aviso: %w", projectID, err)
		}
	}
	if file != nil {
		if _, err := insertProjectFile(ctx, tx, projectID, userID, "oc", file); err != nil {
			return fmt.Errorf("store: receive OC of project %d: file: %w", projectID, err)
		}
		changes = append(changes, "Archivo de la O.C.: "+strings.TrimSpace(file.Filename))
	}
	if note = strings.TrimSpace(note); note != "" {
		changes = append(changes, note)
	}
	if len(changes) > 0 {
		if err := addProjectComment(ctx, tx, quoteID, userID, strings.Join(changes, "\n")); err != nil {
			return fmt.Errorf("store: receive OC of project %d: comment: %w", projectID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: receive OC of project %d: %w", projectID, err)
	}
	return nil
}

// insertProjectFile attaches a file of kind ('oc', 'pago' or 'entrega') to a proyecto and
// returns its id.
func insertProjectFile(ctx context.Context, tx *sql.Tx, projectID, userID int64, kind string, file *NewFile) (int64, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT INTO project_files (project_id, kind, filename, content_type, size, data, uploaded_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		projectID, kind, strings.TrimSpace(file.Filename), file.ContentType, len(file.Data), file.Data, userID)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// addProjectComment writes one entry of a proyecto's history: a comment on its current
// quote (quoteID), so the history reads in one place.
func addProjectComment(ctx context.Context, tx *sql.Tx, quoteID, userID int64, body string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO quote_comments (quote_id, user_id, body) VALUES (?, ?, ?)`, quoteID, userID, body)
	return err
}

// ListProjectFiles returns a proyecto's files of one kind, newest first, without their
// bytes. The first is the one in force.
func (s *Store) ListProjectFiles(ctx context.Context, projectID int64, kind string) ([]ProjectFile, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT f.id, f.project_id, f.kind, f.filename, f.content_type, f.size, u.name, f.uploaded_at
		FROM project_files f JOIN users u ON u.id = f.uploaded_by
		WHERE f.project_id = ? AND f.kind = ? ORDER BY f.id DESC`, projectID, kind)
	if err != nil {
		return nil, fmt.Errorf("store: files of project %d: %w", projectID, err)
	}
	defer rows.Close()
	var files []ProjectFile
	for rows.Next() {
		var f ProjectFile
		if err := rows.Scan(&f.ID, &f.ProjectID, &f.Kind, &f.Filename, &f.ContentType, &f.Size, &f.UploadedByName, &f.UploadedAt); err != nil {
			return nil, fmt.Errorf("store: files of project %d: %w", projectID, err)
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// ProjectFileData returns one of a proyecto's files with its bytes, or nil when the
// proyecto has no file with that id.
func (s *Store) ProjectFileData(ctx context.Context, projectID, fileID int64) (*ProjectFile, []byte, error) {
	var f ProjectFile
	var data []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, kind, filename, content_type, size, uploaded_at, data
		FROM project_files WHERE id = ? AND project_id = ?`, fileID, projectID).
		Scan(&f.ID, &f.ProjectID, &f.Kind, &f.Filename, &f.ContentType, &f.Size, &f.UploadedAt, &data)
	if err == sql.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("store: file %d of project %d: %w", fileID, projectID, err)
	}
	return &f, data, nil
}

// OCReceipt is who put a proyecto's purchase order on record and when (a UTC
// timestamp): the user whose write moved it from prospecto to en curso (oc_recibida
// before migration 0041), as the audit log has it, the latest such move if it was
// reopened or its O.C. undone. A proyecto received before the
// log kept it, or by a write no user is attributed to, falls back to who uploaded its
// first purchase order file. Both are empty when neither is known.
type OCReceipt struct {
	UserName string
	At       string
}

// ProjectOCReceipt returns the proyecto's OCReceipt.
func (s *Store) ProjectOCReceipt(ctx context.Context, projectID int64) (OCReceipt, error) {
	var r OCReceipt
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(u.name, ''), a.at FROM audit_log a LEFT JOIN users u ON u.id = a.actor_id
		WHERE a.table_name = 'projects' AND a.row_key = CAST(? AS TEXT) AND a.op = 'update'
			AND json_extract(a.old_values, '$.status') = 'prospecto'
			AND json_extract(a.new_values, '$.status') IN ('en_curso', 'oc_recibida')
		ORDER BY a.id DESC LIMIT 1`, projectID).Scan(&r.UserName, &r.At)
	if err != nil && err != sql.ErrNoRows {
		return r, fmt.Errorf("store: OC receipt of project %d: %w", projectID, err)
	}
	if r.UserName != "" {
		return r, nil
	}
	err = s.db.QueryRowContext(ctx, `
		SELECT u.name, f.uploaded_at FROM project_files f JOIN users u ON u.id = f.uploaded_by
		WHERE f.project_id = ? AND f.kind = 'oc' ORDER BY f.id LIMIT 1`, projectID).Scan(&r.UserName, &r.At)
	if err != nil && err != sql.ErrNoRows {
		return r, fmt.Errorf("store: OC receipt of project %d: %w", projectID, err)
	}
	return r, nil
}

// Document is a fiscal document as it is typed in by hand: its folio and its day
// (YYYY-MM-DD). Both are required.
type Document struct {
	Ref  string
	Date string
}

func (d Document) valid() bool {
	if strings.TrimSpace(d.Ref) == "" {
		return false
	}
	_, err := time.Parse("2006-01-02", d.Date)
	return err == nil
}

// RecordInvoice puts the purchase order's factura on record by hand, typed in, with its
// método de pago (one of PaymentMethods, kept on the proyecto): the way to invoice while
// facturación can't stamp, until #63's manual mode replaces it. It counts as facturada
// for the proyecto's Estado de facturación; the stage doesn't change, and no payment is
// recorded (that is RecordPayment's, even for P.U.E.). The history gets one comment
// ("Factura registrada: F-1001 · 08/10/2026 · P.U.E."), ending with the user's note. A proyecto that isn't en curso with
// its purchase order on record, one with a factura already (typed in or stamped), or a
// método that isn't one, is ErrBadTransition.
func (s *Store) RecordInvoice(ctx context.Context, projectID, userID int64, invoice Document, method, note string) error {
	invoice.Ref = strings.TrimSpace(invoice.Ref)
	if !invoice.valid() || PaymentMethodLabels[method] == "" {
		return ErrBadTransition
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: record invoice of project %d: %w", projectID, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return fmt.Errorf("store: record invoice of project %d: %w", projectID, err)
	}
	var facturacion string
	err = tx.QueryRowContext(ctx, `
		SELECT ps.facturacion FROM projects p JOIN project_states ps ON ps.project_id = p.id
		WHERE p.id = ? AND p.status = 'en_curso' AND p.oc_number IS NOT NULL`,
		projectID).Scan(&facturacion)
	if err == sql.ErrNoRows || err == nil && facturacion != FacturacionPendiente {
		return ErrBadTransition
	}
	if err != nil {
		return fmt.Errorf("store: record invoice of project %d: %w", projectID, err)
	}
	quoteID, _, err := currentQuote(ctx, tx, projectID)
	if err != nil {
		return fmt.Errorf("store: record invoice of project %d: %w", projectID, err)
	}
	if quoteID == 0 {
		return ErrBadTransition
	}
	body := "Factura registrada: " + invoice.Ref + " · " + dayText(invoice.Date) + " · " + PaymentMethodLabels[method]
	if _, err := tx.ExecContext(ctx, `
		UPDATE projects SET invoice_ref = ?, invoice_date = ?, payment_method = ?
		WHERE id = ?`, invoice.Ref, invoice.Date, method, projectID); err != nil {
		return fmt.Errorf("store: record invoice of project %d: %w", projectID, err)
	}
	if note = strings.TrimSpace(note); note != "" {
		body += "\n" + note
	}
	if err := addProjectComment(ctx, tx, quoteID, userID, body); err != nil {
		return fmt.Errorf("store: record invoice of project %d: comment: %w", projectID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: record invoice of project %d: %w", projectID, err)
	}
	return nil
}

// FollowUp is what a salesperson reports about a prospecto in one go. A nil field is
// left as it is. Probability must be one of ProbabilitySteps; the dates are days
// (YYYY-MM-DD) and "" clears them.
type FollowUp struct {
	Probability  *int
	ExpectedOC   *string // when the client's purchase order is expected
	NextFollowUp *string // when to follow up next
	Note         string
}

// FollowUpProject saves a prospecto's follow-up — probabilidad de cierre, the expected
// purchase order date and the next follow-up date — and records what changed as one
// comment on its current quote ("Probabilidad: Baja → Alta." and so on), ending with
// the user's note, if any. Fields that already hold the given value change nothing and
// say nothing; a note alone is still saved as a comment. A proyecto that is no longer a
// prospecto, a probability that isn't a step and a malformed date are ErrBadTransition.
func (s *Store) FollowUpProject(ctx context.Context, projectID, userID int64, f FollowUp) error {
	if f.Probability != nil && ProbabilityLabel(*f.Probability) == "" {
		return ErrBadTransition
	}
	for _, d := range []*string{f.ExpectedOC, f.NextFollowUp} {
		if d != nil && *d != "" {
			if _, err := time.Parse("2006-01-02", *d); err != nil {
				return ErrBadTransition
			}
		}
	}
	note := strings.TrimSpace(f.Note)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: follow up project %d: %w", projectID, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return fmt.Errorf("store: follow up project %d: %w", projectID, err)
	}
	var probability int
	var expectedOC, nextFollowUp string
	err = tx.QueryRowContext(ctx, `
		SELECT probability, COALESCE(expected_oc_date, ''), COALESCE(next_followup_date, '')
		FROM projects WHERE id = ? AND status = 'prospecto'`, projectID).Scan(&probability, &expectedOC, &nextFollowUp)
	if err == sql.ErrNoRows {
		return ErrBadTransition
	}
	if err != nil {
		return fmt.Errorf("store: follow up project %d: %w", projectID, err)
	}
	quoteID, _, err := currentQuote(ctx, tx, projectID)
	if err != nil {
		return fmt.Errorf("store: follow up project %d: %w", projectID, err)
	}
	if quoteID == 0 {
		return ErrBadTransition
	}

	var changes []string
	if f.Probability != nil && *f.Probability != probability {
		if _, err := tx.ExecContext(ctx, `
			UPDATE projects SET probability = ?, probability_updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
			WHERE id = ?`, *f.Probability, projectID); err != nil {
			return fmt.Errorf("store: follow up project %d: %w", projectID, err)
		}
		changes = append(changes, fmt.Sprintf("Probabilidad: %s → %s.", ProbabilityLabel(probability), ProbabilityLabel(*f.Probability)))
	}
	for _, d := range []struct {
		value  *string
		old    string
		column string
		label  string
	}{
		{f.ExpectedOC, expectedOC, "expected_oc_date", "O.C. esperada"},
		{f.NextFollowUp, nextFollowUp, "next_followup_date", "Próximo seguimiento"},
	} {
		if d.value == nil || *d.value == d.old {
			continue
		}
		// d.column is one of the two literals above, never user input.
		if _, err := tx.ExecContext(ctx, `UPDATE projects SET `+d.column+` = NULLIF(?, '') WHERE id = ?`, *d.value, projectID); err != nil {
			return fmt.Errorf("store: follow up project %d: %w", projectID, err)
		}
		changes = append(changes, fmt.Sprintf("%s: %s.", d.label, dayText(*d.value)))
	}
	if note != "" {
		changes = append(changes, note)
	}
	if len(changes) > 0 {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO quote_comments (quote_id, user_id, body) VALUES (?, ?, ?)`,
			quoteID, userID, strings.Join(changes, "\n")); err != nil {
			return fmt.Errorf("store: follow up project %d: comment: %w", projectID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: follow up project %d: %w", projectID, err)
	}
	return nil
}

// dayText is a YYYY-MM-DD day as the history shows it (15/10/2026), or "sin fecha".
func dayText(day string) string {
	if t, err := time.Parse("2006-01-02", day); err == nil {
		return t.Format("02/01/2006")
	}
	return "sin fecha"
}

// SetProjectProbability sets only a prospecto's probabilidad de cierre; see
// FollowUpProject.
func (s *Store) SetProjectProbability(ctx context.Context, projectID, userID int64, percent int, note string) error {
	return s.FollowUpProject(ctx, projectID, userID, FollowUp{Probability: &percent, Note: note})
}

// CanLose reports whether a proyecto in stage can be marked lost: only a prospecto. Once
// en curso, a sale that falls through is cancelled by an admin instead (CancelProject).
func CanLose(stage string) bool {
	return stage == StageProspecto
}

// LoseProject marks a proyecto lost, from `from` (see CanLose), with the reason, which is
// required. It records it as a comment on the current quote — "Se perdió." plus the
// reason — and remembers the stage it was lost from. It is conditional on the proyecto
// still being in `from`. There is no lost status on a quote: losing a quote and losing
// its proyecto are the same thing.
func (s *Store) LoseProject(ctx context.Context, projectID, userID int64, from, reason string) error {
	reason = strings.TrimSpace(reason)
	if !CanLose(from) || reason == "" {
		return ErrBadTransition
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: lose project %d: %w", projectID, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return fmt.Errorf("store: lose project %d: %w", projectID, err)
	}
	quoteID, _, err := currentQuote(ctx, tx, projectID)
	if err != nil {
		return fmt.Errorf("store: lose project %d: %w", projectID, err)
	}
	if quoteID == 0 {
		return ErrBadTransition
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE projects SET status = 'perdido', lost_reason = ?, lost_from = ?,
			lost_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ? AND status = ?`, reason, from, projectID, from)
	if err != nil {
		return fmt.Errorf("store: lose project %d: %w", projectID, err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrBadTransition
	}
	if err := addProjectComment(ctx, tx, quoteID, userID, "Se perdió.\n"+reason); err != nil {
		return fmt.Errorf("store: lose project %d: comment: %w", projectID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: lose project %d: %w", projectID, err)
	}
	return nil
}

// ReopenProject brings a lost proyecto back to the stage it was lost from and clears the
// reason, which stays in the history as the comment LoseProject wrote. A proyecto lost
// from prospecto can come back with a new probabilidad (one of ProbabilitySteps); nil
// keeps the one it had, and it is ignored for one lost while en curso (before Cancelado
// existed), whose purchase order is still on record. The reopening is a comment too, with
// the user's note if any. Who may reopen is the caller's decision.
func (s *Store) ReopenProject(ctx context.Context, projectID, userID int64, probability *int, note string) error {
	if probability != nil && ProbabilityLabel(*probability) == "" {
		return ErrBadTransition
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: reopen project %d: %w", projectID, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return fmt.Errorf("store: reopen project %d: %w", projectID, err)
	}
	var back string
	var before int
	err = tx.QueryRowContext(ctx,
		`SELECT lost_from, probability FROM projects WHERE id = ? AND status = 'perdido'`, projectID).Scan(&back, &before)
	if err == sql.ErrNoRows {
		return ErrBadTransition
	}
	if err != nil {
		return fmt.Errorf("store: reopen project %d: %w", projectID, err)
	}
	quoteID, _, err := currentQuote(ctx, tx, projectID)
	if err != nil {
		return fmt.Errorf("store: reopen project %d: %w", projectID, err)
	}
	if quoteID == 0 {
		return ErrBadTransition
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE projects SET status = ?, lost_reason = NULL, lost_from = NULL, lost_at = NULL
		WHERE id = ?`, back, projectID); err != nil {
		return fmt.Errorf("store: reopen project %d: %w", projectID, err)
	}
	body := fmt.Sprintf("Se reabrió como %s.", StageLabels[back])
	if back == StageProspecto && probability != nil && *probability != before {
		if _, err := tx.ExecContext(ctx, `
			UPDATE projects SET probability = ?, probability_updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
			WHERE id = ?`, *probability, projectID); err != nil {
			return fmt.Errorf("store: reopen project %d: %w", projectID, err)
		}
		body += fmt.Sprintf("\nProbabilidad: %s → %s.", ProbabilityLabel(before), ProbabilityLabel(*probability))
	}
	if note = strings.TrimSpace(note); note != "" {
		body += "\n" + note
	}
	if err := addProjectComment(ctx, tx, quoteID, userID, body); err != nil {
		return fmt.Errorf("store: reopen project %d: comment: %w", projectID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: reopen project %d: %w", projectID, err)
	}
	return nil
}

// Why an admin's change of stage is refused (CancelProject, UndoReceiveOC), besides
// ErrBadTransition for a proyecto that isn't in a stage it applies to.
var (
	// ErrHasFactura: the proyecto has a factura that may be stamped (timbrada, being
	// stamped, or with its cancellation pending), or, for UndoReceiveOC, one typed in.
	ErrHasFactura = errors.New("store: project has a factura")
	// ErrHasConfirmedPO: the proyecto has a confirmed supplier O.C.
	ErrHasConfirmedPO = errors.New("store: project has a confirmed supplier purchase order")
)

// maybeStampedComprobante is the SQL condition for a comprobante that may be a real CFDI:
// not one of deletableComprobanteStatuses, and not cancelled.
const maybeStampedComprobante = `status NOT IN (` + deletableComprobanteStatuses + `, 'cancelada')`

// stageBlockers checks what keeps an admin from taking an en curso proyecto out of its
// stage: a factura that may be stamped (and, when typed counts, a typed-in invoice_ref)
// and a confirmed supplier O.C.
func stageBlockers(ctx context.Context, tx *sql.Tx, projectID int64, typed bool) error {
	var factura, po int
	if err := tx.QueryRowContext(ctx, `
		SELECT
			(SELECT count(*) FROM comprobantes WHERE project_id = ?1 AND tipo = 'factura' AND `+maybeStampedComprobante+`)
				+ (SELECT count(*) FROM projects WHERE id = ?1 AND ?2 AND COALESCE(invoice_ref, '') != ''),
			(SELECT count(*) FROM purchase_orders WHERE project_id = ?1 AND status = 'confirmada')`,
		projectID, typed).Scan(&factura, &po); err != nil {
		return err
	}
	switch {
	case factura > 0:
		return ErrHasFactura
	case po > 0:
		return ErrHasConfirmedPO
	}
	return nil
}

// CancelProject marks a proyecto cancelado (Emilio's "pedido que se cae"), from
// prospecto or en curso, with the reason, which is required. It remembers the stage it
// came from, which ReactivateProject restores, and records it as a comment — "Se canceló."
// plus the reason. A factura that may be stamped or a confirmed supplier O.C. blocks it
// (ErrHasFactura, ErrHasConfirmedPO): those are cancelled first. Who may cancel (an
// admin) is the caller's decision.
func (s *Store) CancelProject(ctx context.Context, projectID, userID int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ErrBadTransition
	}
	return s.inTx(ctx, "cancel project", func(tx *sql.Tx) error {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM projects WHERE id = ?`, projectID).Scan(&status); err != nil {
			if err == sql.ErrNoRows {
				return ErrBadTransition
			}
			return err
		}
		if status != StageProspecto && status != StageEnCurso {
			return ErrBadTransition
		}
		if err := stageBlockers(ctx, tx, projectID, false); err != nil {
			return err
		}
		quoteID, _, err := currentQuote(ctx, tx, projectID)
		if err != nil {
			return err
		}
		if quoteID == 0 {
			return ErrBadTransition
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE projects SET status = 'cancelado', cancel_reason = ?, cancelled_from = ?,
				cancelled_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
			WHERE id = ?`, reason, status, projectID); err != nil {
			return err
		}
		return addProjectComment(ctx, tx, quoteID, userID, "Se canceló.\n"+reason)
	})
}

// ReactivateProject brings a cancelled proyecto back to the stage it was cancelled from,
// with the reason, which is required, in the comment that records it ("Se reactivó como
// En curso."). The cancellation's reason stays in the history. Who may reactivate (an
// admin) is the caller's decision.
func (s *Store) ReactivateProject(ctx context.Context, projectID, userID int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ErrBadTransition
	}
	return s.inTx(ctx, "reactivate project", func(tx *sql.Tx) error {
		var back string
		err := tx.QueryRowContext(ctx, `SELECT cancelled_from FROM projects WHERE id = ? AND status = 'cancelado'`, projectID).Scan(&back)
		if err == sql.ErrNoRows {
			return ErrBadTransition
		}
		if err != nil {
			return err
		}
		quoteID, _, err := currentQuote(ctx, tx, projectID)
		if err != nil {
			return err
		}
		if quoteID == 0 {
			return ErrBadTransition
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE projects SET status = ?, cancel_reason = NULL, cancelled_from = NULL, cancelled_at = NULL
			WHERE id = ?`, back, projectID); err != nil {
			return err
		}
		return addProjectComment(ctx, tx, quoteID, userID, "Se reactivó como "+StageLabels[back]+".\n"+reason)
	})
}

// UndoReceiveOC takes an en curso proyecto back to prospecto at Inminente 90%, for a
// purchase order received by mistake (the wrong file, or the wrong proyecto), with the
// reason, which is required. The order's number and date are cleared and stay in the
// history comment ("Se deshizo la O.C. recibida (O.C. 4411 · 07/10/2026)."); its files
// stay as they are, the newest in force if the order is received again. A factura blocks
// it, stamped or typed in (ErrHasFactura), and so does a confirmed supplier O.C.
// (ErrHasConfirmedPO). A factura draft never stamped is deleted with it, since it bills
// the order being undone. Who may undo (an admin) is the caller's decision.
func (s *Store) UndoReceiveOC(ctx context.Context, projectID, userID int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ErrBadTransition
	}
	return s.inTx(ctx, "undo receive OC", func(tx *sql.Tx) error {
		var oc OC
		err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(oc_number, ''), COALESCE(oc_date, '') FROM projects WHERE id = ? AND status = 'en_curso'`,
			projectID).Scan(&oc.Number, &oc.Date)
		if err == sql.ErrNoRows {
			return ErrBadTransition
		}
		if err != nil {
			return err
		}
		if err := stageBlockers(ctx, tx, projectID, true); err != nil {
			return err
		}
		quoteID, _, err := currentQuote(ctx, tx, projectID)
		if err != nil {
			return err
		}
		if quoteID == 0 {
			return ErrBadTransition
		}
		for _, q := range []string{
			`DELETE FROM comprobante_conceptos WHERE comprobante_id IN (SELECT id FROM comprobantes WHERE project_id = ?1 AND status IN (` + deletableComprobanteStatuses + `))`,
			`DELETE FROM comprobante_events WHERE comprobante_id IN (SELECT id FROM comprobantes WHERE project_id = ?1 AND status IN (` + deletableComprobanteStatuses + `))`,
			`DELETE FROM comprobantes WHERE project_id = ?1 AND status IN (` + deletableComprobanteStatuses + `)`,
			`UPDATE projects SET status = 'prospecto', probability = ` + fmt.Sprint(ReceiveOCProbability) + `,
				probability_updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), oc_number = NULL, oc_date = NULL
			WHERE id = ?1`,
		} {
			if _, err := tx.ExecContext(ctx, q, projectID); err != nil {
				return err
			}
		}
		what := "Se deshizo la O.C. recibida."
		if oc.Number != "" {
			what = "Se deshizo la O.C. recibida (O.C. " + oc.text() + ")."
		}
		body := what + "\nRegresó a " + StageLabels[StageProspecto] + " · " +
			ProbabilityLabel(ReceiveOCProbability) + " " + fmt.Sprint(ReceiveOCProbability) + "%.\n" + reason
		return addProjectComment(ctx, tx, quoteID, userID, body)
	})
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

// Project is a projects row with what its screens show next to it: the customer's and
// the owner's current names (joins) and its current quote, whose total is the
// proyecto's amount. CurrentQuoteStatus is emitida, or borrador while a revision is
// being worked on.
type Project struct {
	ID                   int64
	Folio                string
	CustomerID           int64
	CustomerName         string
	UserID               int64
	UserName             string
	Status               string // one of StageLabels' keys
	Probability          int
	ProbabilityUpdatedAt string
	CreatedAt            string
	CurrentQuoteID       int64
	CurrentQuoteFolio    string
	CurrentQuoteStatus   string
	Total                money.Centavos
	// LostReason, LostFrom and LostAt say why, from which stage and when a perdido
	// proyecto was lost; they are empty on any other.
	LostReason string
	LostFrom   string
	LostAt     string
	// ExpectedOCDate and NextFollowUpDate are the days (YYYY-MM-DD) a salesperson keeps
	// on a prospecto: when the purchase order is expected and when to follow up next.
	// Either may be empty.
	ExpectedOCDate   string
	NextFollowUpDate string
	// QuoteIssuedAt and QuoteValidUntil are the current quote's issue time and vigencia,
	// empty while it is a draft. Contact* is the customer's contact. Last* is the newest
	// entry of the proyecto's history, empty when there is none.
	QuoteIssuedAt   string
	QuoteValidUntil string
	ContactName     string
	ContactPhone    string
	ContactEmail    string
	LastBody        string
	LastUserName    string
	LastAt          string
	// OC is the client's purchase order, zero until it is received (and on proyectos
	// that reached oc_recibida before it was asked for).
	OC OC
	// PaymentMethod is the factura's método de pago, one of PaymentMethods: set when the
	// factura is recorded, or already on proyectos whose purchase order was received
	// while it was asked for there. Empty until then.
	PaymentMethod string
	// Invoice is the purchase order's factura as typed in, empty until it is on record.
	// (The payment state once typed next to it, payment_status and its folio and day, is
	// kept in its columns but no longer read: payments are recorded one by one, #67.)
	Invoice Document
	// Subtotal is the current quote's price before IVA and Profit what it earns on it (see
	// quoteProfitSQL); the cost is the difference. LastMovementAt is when the proyecto last
	// had news: its newest history entry, or when it was opened if it has none.
	Subtotal       money.Centavos
	Profit         money.Centavos
	LastMovementAt string
	// CancelReason, CancelledFrom and CancelledAt say why, from which stage and when a
	// cancelado proyecto was cancelled; they are empty on any other.
	CancelReason  string
	CancelledFrom string
	CancelledAt   string
	// Name is the proyecto's own name (issue #73), e.g. "Capuchinas", empty when it has
	// none. Shown next to the folio; see Project.Label.
	Name string
	// States are its facturación, entrega and cobro (the project_states view). They are
	// derived for every proyecto but only mean something once it is en curso.
	States ProjectStates
}

// The values of the three states (ProjectStates).
const (
	FacturacionPendiente            = "pendiente"
	FacturacionPendienteComplemento = "pendiente_complemento"
	FacturacionFacturada            = "facturada"
	EntregaSinOC                    = "sin_oc"
	EntregaEnProceso                = "en_proceso"
	EntregaEntregado                = "entregado"
	CobroPendiente                  = "pendiente"
	CobroParcial                    = "parcial"
	CobroPagado                     = "pagado"
)

// FacturacionLabels, EntregaLabels and CobroLabels are the words users see for each
// state, as #66 and #67 name them.
var (
	FacturacionLabels = map[string]string{
		FacturacionPendiente:            "Pendiente",
		FacturacionPendienteComplemento: "Pendiente complemento de pago",
		FacturacionFacturada:            "Facturada",
	}
	EntregaLabels = map[string]string{
		EntregaSinOC:     "Sin emitir O.C.",
		EntregaEnProceso: "En proceso de entrega",
		EntregaEntregado: "Entregado",
	}
	CobroLabels = map[string]string{
		CobroPendiente: "Pendiente de pago",
		CobroParcial:   "Pago parcial",
		CobroPagado:    "Pagado",
	}
)

// ProjectStates is where an en curso proyecto stands, as the project_states view derives
// it from its records (migrations/0041_project_states.sql): Facturacion, Entrega and Cobro
// (one of the constants above each), Paid the sum of its payments (centavos with IVA),
// LastPaidOn the day of the newest one ("" with none), and Cerrado when it is en curso
// and all three are complete.
type ProjectStates struct {
	Facturacion string
	Entrega     string
	Cobro       string
	Paid        money.Centavos
	LastPaidOn  string
	Cerrado     bool
}

// Stage is the stage the proyecto shows and is filtered by: its stored stage, or
// StageCerrado for an en curso proyecto whose three states are complete.
func (p Project) Stage() string {
	if p.Status == StageEnCurso && p.States.Cerrado {
		return StageCerrado
	}
	return p.Status
}

// Balance is what the client still owes: the total with IVA less what was paid, never
// below zero.
func (p Project) Balance() money.Centavos {
	return max(p.Total-p.States.Paid, 0)
}

// Cost is the current quote's cost before IVA: what is left of its price after the profit.
func (p Project) Cost() money.Centavos { return p.Subtotal - p.Profit }

// MarginPercent is the profit as a percentage of the price before IVA, 0 for a free quote.
func (p Project) MarginPercent() float64 {
	if p.Subtotal == 0 {
		return 0
	}
	return float64(p.Profit) * 100 / float64(p.Subtotal)
}

// HasOC reports whether the client's purchase order is on record.
func (p Project) HasOC() bool { return p.OC.Number != "" }

// MaxProjectName is how long a proyecto's name may be, in characters (issue #73).
const MaxProjectName = 40

// Label is how a proyecto is named in lists, avisos and dashboards: its folio, and its
// name after it when it has one ("QCCA0053 · Capuchinas").
func (p Project) Label() string { return ProjectLabel(p.Folio, p.Name) }

// ProjectLabel is Project.Label for a folio and name read on their own.
func ProjectLabel(folio, name string) string {
	if name == "" {
		return folio
	}
	return folio + " · " + name
}

// SetProjectName sets or changes the proyecto's name; blank clears it. The caller checks
// its length (MaxProjectName).
func (s *Store) SetProjectName(ctx context.Context, projectID int64, name string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: set project %d name: %w", projectID, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return fmt.Errorf("store: set project %d name: %w", projectID, err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE projects SET name = NULLIF(?, '') WHERE id = ?`,
		strings.TrimSpace(name), projectID); err != nil {
		return fmt.Errorf("store: set project %d name: %w", projectID, err)
	}
	return tx.Commit()
}

// FollowUpDue reports whether the prospecto's next follow-up is today or overdue.
func (p Project) FollowUpDue(today string) bool {
	return p.Status == "prospecto" && p.NextFollowUpDate != "" && p.NextFollowUpDate <= today
}

// Weight is the proyecto's amount times its probability, in centavo-percent: add them up
// and divide by 100 (see WeightedTotal) so the rounding happens once.
func (p Project) Weight() int64 { return int64(p.Total) * int64(p.Probability) }

// WeightedTotal turns a sum of Project.Weight into the expected amount.
func WeightedTotal(weight int64) money.Centavos {
	return money.Centavos(money.RoundHalfUp(weight, 100))
}

// ForecastRelevant reports whether the proyecto counts for the forecast.
func (p Project) ForecastRelevant() bool { return ForecastRelevant(p.Status, p.Probability) }

var projectSelectCols = `
	p.id, p.folio, p.customer_id, c.name, p.user_id, u.name, p.status, p.probability,
	p.probability_updated_at, p.created_at, q.id, q.folio, q.status, q.total,
	COALESCE(p.lost_reason, ''), COALESCE(p.lost_from, ''), COALESCE(p.lost_at, ''),
	COALESCE(p.expected_oc_date, ''), COALESCE(p.next_followup_date, ''),
	COALESCE(q.issued_at, ''), COALESCE(q.valid_until, ''),
	COALESCE(c.contact_name, ''), COALESCE(c.phone, ''), COALESCE(c.email, ''),
	COALESCE((` + projectLastComment("lc.body") + `), ''),
	COALESCE((` + projectLastComment("lu.name") + `), ''),
	COALESCE((` + projectLastComment("lc.created_at") + `), ''),
	COALESCE(p.oc_number, ''), COALESCE(p.oc_date, ''), COALESCE(p.payment_method, ''),
	COALESCE(p.invoice_ref, ''), COALESCE(p.invoice_date, ''),
	q.subtotal, ` + quoteProfitSQL + `, COALESCE((` + projectLastComment("lc.created_at") + `), p.created_at),
	COALESCE(p.cancel_reason, ''), COALESCE(p.cancelled_from, ''), COALESCE(p.cancelled_at, ''),
	ps.facturacion, ps.entrega, ps.cobro, ps.paid, ps.last_paid_on, ps.cerrado,
	COALESCE(p.name, '')`

// quoteProfitSQL is the current quote's profit before IVA, in centavos: its catalog lines'
// total times the margin it was issued with (a fraction of the sale price), rounded half
// up, plus each free line's total less its cost (issue #27), the cost rounded the way
// money.LineTotalCentavos does. A free line without a cost adds nothing, and a draft
// (no margin frozen yet) earns nothing. It is the same figure as views.QuoteMetrics shows
// on the quote.
const quoteProfitSQL = `(CASE WHEN q.margin_snapshot_micros IS NULL THEN 0 ELSE
		(COALESCE((SELECT SUM(ql.line_total) FROM quote_lines ql
			WHERE ql.quote_id = q.id AND ql.product_id IS NOT NULL), 0) * q.margin_snapshot_micros + 500000) / 1000000
		+ COALESCE((SELECT SUM(ql.line_total - (ql.cost_micros * ql.qty_milli + 5000000) / 10000000) FROM quote_lines ql
			WHERE ql.quote_id = q.id AND ql.product_id IS NULL AND ql.cost_micros IS NOT NULL), 0) END)`

// projectLastComment selects one column of the newest comment on any of the proyecto's
// quotes, for projectSelectCols.
func projectLastComment(col string) string {
	return `SELECT ` + col + ` FROM quote_comments lc
		JOIN quotes lq ON lq.id = lc.quote_id JOIN users lu ON lu.id = lc.user_id
		WHERE lq.project_id = p.id ORDER BY lc.created_at DESC, lc.id DESC LIMIT 1`
}

const projectFrom = `
	FROM projects p
	JOIN customers c ON c.id = p.customer_id
	JOIN users u ON u.id = p.user_id
	JOIN quotes q ON q.project_id = p.id AND q.status != 'revisada'
	JOIN project_states ps ON ps.project_id = p.id`

func scanProject(row interface{ Scan(...any) error }) (*Project, error) {
	var p Project
	err := row.Scan(&p.ID, &p.Folio, &p.CustomerID, &p.CustomerName, &p.UserID, &p.UserName, &p.Status,
		&p.Probability, &p.ProbabilityUpdatedAt, &p.CreatedAt,
		&p.CurrentQuoteID, &p.CurrentQuoteFolio, &p.CurrentQuoteStatus, &p.Total,
		&p.LostReason, &p.LostFrom, &p.LostAt,
		&p.ExpectedOCDate, &p.NextFollowUpDate, &p.QuoteIssuedAt, &p.QuoteValidUntil,
		&p.ContactName, &p.ContactPhone, &p.ContactEmail,
		&p.LastBody, &p.LastUserName, &p.LastAt,
		&p.OC.Number, &p.OC.Date, &p.PaymentMethod,
		&p.Invoice.Ref, &p.Invoice.Date,
		&p.Subtotal, &p.Profit, &p.LastMovementAt,
		&p.CancelReason, &p.CancelledFrom, &p.CancelledAt,
		&p.States.Facturacion, &p.States.Entrega, &p.States.Cobro, &p.States.Paid, &p.States.LastPaidOn, &p.States.Cerrado,
		&p.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ProjectByFolio returns the proyecto with the given folio, or nil if none exists.
func (s *Store) ProjectByFolio(ctx context.Context, folio string) (*Project, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+projectSelectCols+projectFrom+` WHERE p.folio = ?`, folio)
	p, err := scanProject(row)
	if err != nil {
		return nil, fmt.Errorf("store: project by folio %q: %w", folio, err)
	}
	return p, nil
}

// projectStageSQL is the stage a proyecto shows (Project.Stage): its stored stage, or
// 'cerrado' for an en curso one whose three states are complete.
const projectStageSQL = `CASE WHEN p.status = 'en_curso' AND ps.cerrado THEN 'cerrado' ELSE p.status END`

// projectStageOrder sorts by where a stage sits in the lifecycle rather than by its name.
const projectStageOrder = `CASE ` + projectStageSQL + ` WHEN 'prospecto' THEN 1 WHEN 'en_curso' THEN 2
	WHEN 'cerrado' THEN 3 WHEN 'cancelado' THEN 4 ELSE 5 END`

// projectSortColumns is the sortable-column whitelist for ListProjects. When sort
// doesn't match a known column, ListProjects uses projectDefaultOrder instead.
var projectSortColumns = []sortColumn{
	{"folio", "p.folio"},
	{"cliente", "c.name"},
	{"vendedor", "u.name COLLATE NOCASE"},
	{"etapa", projectStageOrder},
	{"probabilidad", "p.probability"},
	// Only a prospecto shows its next follow-up: those without a date come after the
	// dated ones, and every other stage after them.
	{"seguimiento", "CASE WHEN p.status = 'prospecto' THEN COALESCE(p.next_followup_date, '9999-12-30') ELSE '9999-12-31' END"},
	{"total", "q.total"},
	{"fecha", "p.created_at"},
}

// projectDefaultOrder is the list's order when no sort column is chosen: newest first.
const projectDefaultOrder = `ORDER BY p.created_at DESC, p.id DESC`

// projectFilterColumns are the per-column filters ListProjects accepts. The probability
// filter only matches prospectos, the one stage where it means something, and the cobro
// filter only proyectos en curso (Cerrado · Ganado included).
var projectFilterColumns = []filterColumn{
	{name: "folio", expr: "p.folio", kind: FilterText},
	{name: "cliente", expr: "c.name", kind: FilterText},
	{name: "vendedor", expr: "u.name", kind: FilterText},
	{name: "etapa", expr: projectStageSQL, kind: FilterEnum},
	{name: "cobro", expr: "CASE WHEN p.status = 'en_curso' THEN ps.cobro END", kind: FilterEnum},
	{name: "probabilidad", expr: "CASE WHEN p.status = 'prospecto' THEN CAST(p.probability AS TEXT) END", kind: FilterEnum},
	{name: "total", expr: "q.total", kind: FilterNumber, scale: 100},
	{name: "fecha", expr: "p.created_at", kind: FilterDate},
}

// ListProjects returns proyectos, optionally filtered by a case-insensitive substring
// match on folio, customer name or the proyecto's name and by per-column filters (see projectFilterColumns),
// sorted per sort/dir (see projectSortColumns). The list is of the proyectos still being
// worked: a Cerrado · Ganado one is left out unless the etapa filter asks for it or a
// search is typed (looking a proyecto up by its folio finds it whatever its stage).
func (s *Store) ListProjects(ctx context.Context, query, sort, dir string, filters Filters) ([]Project, error) {
	like := "%" + escapeLike(query) + "%"
	order := projectDefaultOrder
	for _, c := range projectSortColumns {
		if c.name == sort {
			order = orderByClause(projectSortColumns, sort, dir)
			break
		}
	}
	extra, extraArgs := filterWhere(ctx, projectFilterColumns, filters)
	if query == "" && len(filters["etapa"]) == 0 {
		extra += ` AND NOT (p.status = 'en_curso' AND ps.cerrado)`
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+projectSelectCols+projectFrom+`
		WHERE (? = '' OR p.folio LIKE ? ESCAPE '\' COLLATE NOCASE OR c.name LIKE ? ESCAPE '\' COLLATE NOCASE
			OR p.name LIKE ? ESCAPE '\' COLLATE NOCASE)`+extra+`
		`+order, append([]any{query, like, like, like}, extraArgs...)...,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list projects: %w", err)
	}
	defer rows.Close()
	var projects []Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list projects: %w", err)
		}
		projects = append(projects, *p)
	}
	return projects, rows.Err()
}

// forecastSortColumns is the sortable-column whitelist for ListProspects. Days without
// an update sorts on the date of the last movement, newest first, so ascending shows the
// freshest. A missing date sorts after every real one.
var forecastSortColumns = []sortColumn{
	{"folio", "p.folio"},
	{"cliente", "c.name COLLATE NOCASE"},
	{"vendedor", "u.name COLLATE NOCASE"},
	{"probabilidad", "p.probability"},
	{"seguimiento", "COALESCE(p.next_followup_date, '9999-12-31')"},
	{"oc", "COALESCE(p.expected_oc_date, '9999-12-31')"},
	{"vigencia", "COALESCE(q.valid_until, '9999-12-31')"},
	{"dias", "-julianday(COALESCE((" + `SELECT lc.created_at FROM quote_comments lc JOIN quotes lq ON lq.id = lc.quote_id
		WHERE lq.project_id = p.id ORDER BY lc.created_at DESC, lc.id DESC LIMIT 1` + "), p.created_at))"},
	{"costo", "(q.subtotal - " + quoteProfitSQL + ")"},
	{"precio", "q.subtotal"},
	{"utilidad", quoteProfitSQL},
	{"margen", "CASE WHEN q.subtotal = 0 THEN 0 ELSE 1.0 * " + quoteProfitSQL + " / q.subtotal END"},
	{"comentario", "COALESCE((" + `SELECT lc.created_at FROM quote_comments lc JOIN quotes lq ON lq.id = lc.quote_id
		WHERE lq.project_id = p.id ORDER BY lc.created_at DESC, lc.id DESC LIMIT 1` + "), '')"},
}

// forecastDefaultOrder is the Pronóstico order when no column is chosen: soonest expected
// purchase order first, those without a date last, then the likeliest and the largest.
const forecastDefaultOrder = `ORDER BY p.expected_oc_date IS NULL, p.expected_oc_date, p.probability DESC, q.total DESC, p.id`

// forecastFilterColumns are the filters of the Pronóstico table: by vendedor.
var forecastFilterColumns = []filterColumn{
	{name: "vendedor", expr: "u.name", kind: FilterText},
}

// ListProspects returns the prospectos for the Pronóstico view, the ones the team goes
// over when it reviews what is about to close: only those relevante para pronóstico, or
// every prospecto when all is set. Sorted per sort/dir (see forecastSortColumns), or by
// forecastDefaultOrder when sort isn't a known column, and filtered by vendedor.
func (s *Store) ListProspects(ctx context.Context, all bool, sort, dir string, filters Filters) ([]Project, error) {
	minProbability := ForecastThreshold
	if all {
		minProbability = 0
	}
	order := forecastDefaultOrder
	for _, c := range forecastSortColumns {
		if c.name == sort {
			order = orderByClause(forecastSortColumns, sort, dir) + ", p.id"
			break
		}
	}
	extra, extraArgs := filterWhere(ctx, forecastFilterColumns, filters)
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+projectSelectCols+projectFrom+`
		WHERE p.status = 'prospecto' AND p.probability >= ?`+extra+`
		`+order, append([]any{minProbability}, extraArgs...)...)
	if err != nil {
		return nil, fmt.Errorf("store: list prospects: %w", err)
	}
	defer rows.Close()
	var projects []Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list prospects: %w", err)
		}
		projects = append(projects, *p)
	}
	return projects, rows.Err()
}

// ProjectFilterChoices returns the customer and owner names that appear on proyectos,
// for the Cliente and Vendedor filter dropdowns. A column with more than
// MaxFilterChoices distinct names returns nil, and its filter stays a free-text box.
func (s *Store) ProjectFilterChoices(ctx context.Context) (customers, owners []string, err error) {
	for i, expr := range []string{"c.name", "u.name"} {
		rows, err := s.db.QueryContext(ctx, `
			SELECT DISTINCT `+expr+projectFrom+`
			ORDER BY `+expr+` COLLATE NOCASE LIMIT ?`, MaxFilterChoices+1)
		if err != nil {
			return nil, nil, fmt.Errorf("store: project filter choices: %w", err)
		}
		var names []string
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				rows.Close()
				return nil, nil, fmt.Errorf("store: project filter choices: %w", err)
			}
			names = append(names, n)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, nil, fmt.Errorf("store: project filter choices: %w", err)
		}
		if len(names) > MaxFilterChoices {
			names = nil
		}
		if i == 0 {
			customers = names
		} else {
			owners = names
		}
	}
	return customers, owners, nil
}

// ListProjectQuotes returns every quote of a proyecto, newest first: its current quote
// and the ones revisions replaced.
func (s *Store) ListProjectQuotes(ctx context.Context, projectID int64) ([]Quote, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+quoteSelectCols+quoteFrom+`
		WHERE q.project_id = ? ORDER BY q.id DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("store: quotes of project %d: %w", projectID, err)
	}
	defer rows.Close()
	var quotes []Quote
	for rows.Next() {
		q, err := scanQuote(rows)
		if err != nil {
			return nil, fmt.Errorf("store: quotes of project %d: %w", projectID, err)
		}
		quotes = append(quotes, *q)
	}
	return quotes, rows.Err()
}

// ProjectComment is a comment on any of a proyecto's quotes, with that quote's folio.
type ProjectComment struct {
	QuoteComment
	QuoteFolio string
}

// ListProjectComments returns the proyecto's history, newest first: the comments of all
// its quotes in one timeline, stage moves and probability changes included.
func (s *Store) ListProjectComments(ctx context.Context, projectID int64) ([]ProjectComment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.quote_id, c.user_id, u.name, c.body, c.created_at, q.folio
		FROM quote_comments c
		JOIN quotes q ON q.id = c.quote_id
		JOIN users u ON u.id = c.user_id
		WHERE q.project_id = ?
		ORDER BY c.created_at DESC, c.id DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("store: comments of project %d: %w", projectID, err)
	}
	defer rows.Close()
	var out []ProjectComment
	for rows.Next() {
		var c ProjectComment
		if err := rows.Scan(&c.ID, &c.QuoteID, &c.UserID, &c.UserName, &c.Body, &c.CreatedAt, &c.QuoteFolio); err != nil {
			return nil, fmt.Errorf("store: comments of project %d: %w", projectID, err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
