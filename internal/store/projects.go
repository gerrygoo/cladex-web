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

// ProjectFlow is the order a proyecto follows: the client has the quote and it is being
// followed up (prospecto), the client's purchase order arrived (oc_recibida), its
// factura is on record (facturado), the goods are on their way or delivered
// (en_entrega), and it was delivered and collected (cerrado). A proyecto moves one step
// at a time, and three of the steps are gated: only ReceiveOC enters oc_recibida, only
// RecordInvoice enters facturado, so nothing is delivered without a factura, and cerrado
// needs the proyecto to be pagado. perdido is not a step: a proyecto is marked lost from
// prospecto or oc_recibida (LoseProject) and only an admin's reopen brings it back.
var ProjectFlow = []string{"prospecto", "oc_recibida", "facturado", "en_entrega", "cerrado"}

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
// move starts from, the target isn't the stage right before or after it, it is a step
// forward that belongs to ReceiveOC or RecordInvoice, or it is closing a proyecto that
// isn't pagado. It is also
// what SetProjectProbability returns for a proyecto that is no longer a prospecto or a
// value that isn't a step.
var ErrBadTransition = errors.New("store: project can't move to that stage")

// MoveProject moves a proyecto one stage along ProjectFlow, from `from` to `to` (either
// the next stage or the previous one), and records it as a comment on its current quote
// — "Pasó a O.C. recibida." plus the user's note, if any — so the history reads in one
// place. The change is conditional on the proyecto still being in `from`, so two people
// clicking at once can't skip a stage. Two steps forward aren't its to take, because they
// need data: prospecto to oc_recibida is ReceiveOC's and oc_recibida to facturado is
// RecordInvoice's. Closing (en_entrega to cerrado) needs the proyecto to be pagado.
// Going back from facturado to oc_recibida undoes the factura: its folio and the payment
// state are cleared and stay only in the history. Whether the user may go backwards is
// the caller's decision.
func (s *Store) MoveProject(ctx context.Context, projectID, userID int64, from, to, note string) error {
	if NextStage(from) != to && PrevStage(from) != to || to == "" ||
		from == "prospecto" && to == "oc_recibida" || from == "oc_recibida" && to == "facturado" {
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
	quoteID, _, err := currentQuote(ctx, tx, projectID)
	if err != nil {
		return fmt.Errorf("store: move project %d: %w", projectID, err)
	}
	if quoteID == 0 {
		return ErrBadTransition
	}
	set, guard := "", ""
	switch {
	case from == "en_entrega" && to == "cerrado":
		guard = ` AND payment_status = 'pagado'`
	case from == "facturado" && to == "oc_recibida":
		set = `, invoice_ref = NULL, invoice_date = NULL, payment_status = NULL, payment_ref = NULL, paid_at = NULL`
	}
	res, err := tx.ExecContext(ctx, `UPDATE projects SET status = ?`+set+` WHERE id = ? AND status = ?`+guard, to, projectID, from)
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

// PaymentMethods are the formas de pago a purchase order can be received with, and
// PaymentMethodLabels how users read them.
var PaymentMethods = []string{"PUE", "PPD"}

var PaymentMethodLabels = map[string]string{
	"PUE": "P.U.E.",
	"PPD": "P.P.D.",
}

// OC is the client's purchase order as recorded on a proyecto: its number, its date
// (YYYY-MM-DD) and the forma de pago, one of PaymentMethods. All three are required.
type OC struct {
	Number        string
	Date          string
	PaymentMethod string
}

func (oc OC) valid() bool {
	if strings.TrimSpace(oc.Number) == "" || PaymentMethodLabels[oc.PaymentMethod] == "" {
		return false
	}
	_, err := time.Parse("2006-01-02", oc.Date)
	return err == nil
}

// text is the purchase order as the history shows it: "4411 · 07/10/2026 · P.U.E.".
func (oc OC) text() string {
	return oc.Number + " · " + dayText(oc.Date) + " · " + PaymentMethodLabels[oc.PaymentMethod]
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
// given. On a prospecto it is what moves the proyecto to oc_recibida, the only way in:
// that needs all of oc and an issued current quote (a revision still in borrador has to
// be issued first). On a proyecto already in oc_recibida it corrects the data. Either
// way the history gets one comment saying what happened, ending with the user's note.
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
	var status string
	var old OC
	err = tx.QueryRowContext(ctx, `
		SELECT status, COALESCE(oc_number, ''), COALESCE(oc_date, ''), COALESCE(payment_method, '')
		FROM projects WHERE id = ?`, projectID).Scan(&status, &old.Number, &old.Date, &old.PaymentMethod)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("store: receive OC of project %d: %w", projectID, err)
	}
	if err == sql.ErrNoRows || status != "prospecto" && status != "oc_recibida" {
		return ErrBadTransition
	}
	quoteID, quoteStatus, err := currentQuote(ctx, tx, projectID)
	if err != nil {
		return fmt.Errorf("store: receive OC of project %d: %w", projectID, err)
	}
	if quoteID == 0 || quoteStatus != "emitida" {
		return ErrBadTransition
	}

	var changes []string
	switch {
	case status == "prospecto":
		changes = append(changes, "Pasó a "+StageLabels["oc_recibida"]+".", "O.C. "+oc.text())
	case old != oc:
		changes = append(changes, "O.C. actualizada: "+oc.text())
	}
	if status == "prospecto" || old != oc {
		if _, err := tx.ExecContext(ctx, `
			UPDATE projects SET status = 'oc_recibida', oc_number = ?, oc_date = ?, payment_method = ?
			WHERE id = ?`, oc.Number, oc.Date, oc.PaymentMethod, projectID); err != nil {
			return fmt.Errorf("store: receive OC of project %d: %w", projectID, err)
		}
	}
	if file != nil {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO project_files (project_id, kind, filename, content_type, size, data, uploaded_by)
			VALUES (?, 'oc', ?, ?, ?, ?, ?)`,
			projectID, strings.TrimSpace(file.Filename), file.ContentType, len(file.Data), file.Data, userID); err != nil {
			return fmt.Errorf("store: receive OC of project %d: file: %w", projectID, err)
		}
		changes = append(changes, "Archivo de la O.C.: "+strings.TrimSpace(file.Filename))
	}
	if note = strings.TrimSpace(note); note != "" {
		changes = append(changes, note)
	}
	if len(changes) > 0 {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO quote_comments (quote_id, user_id, body) VALUES (?, ?, ?)`,
			quoteID, userID, strings.Join(changes, "\n")); err != nil {
			return fmt.Errorf("store: receive OC of project %d: comment: %w", projectID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: receive OC of project %d: %w", projectID, err)
	}
	return nil
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

// Payment states. A proyecto's payment state is separate from its stage: empty until the
// purchase order's factura is on record, then facturado de anticipo for a P.P.D. order
// still to be collected, and pagado. Until facturas are issued from the app these are
// kept by hand, with the folios typed in.
const (
	PaymentAdvanceInvoiced = "facturado_anticipo"
	PaymentPaid            = "pagado"
)

// PaymentStatusLabels are the words users see for each payment state; "" is a proyecto
// with nothing invoiced yet.
var PaymentStatusLabels = map[string]string{
	"":                     "Sin tramitar",
	PaymentAdvanceInvoiced: "Facturado de anticipo",
	PaymentPaid:            "Pagado",
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

// RecordInvoice puts the purchase order's factura on record and moves the proyecto from
// oc_recibida to facturado, the only way in. What it means depends on the forma de pago
// the order was received with. P.U.E.: the client paid and the factura was issued, so
// the proyecto is pagado as of the factura's day. P.P.D.: it is the factura de anticipo,
// and the proyecto is facturado de anticipo until RecordPayment. The history gets one
// comment saying which, ending with the user's note. A proyecto that isn't in
// oc_recibida with its purchase order on record is ErrBadTransition.
func (s *Store) RecordInvoice(ctx context.Context, projectID, userID int64, invoice Document, note string) error {
	invoice.Ref = strings.TrimSpace(invoice.Ref)
	if !invoice.valid() {
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
	var method string
	err = tx.QueryRowContext(ctx, `
		SELECT payment_method FROM projects
		WHERE id = ? AND status = 'oc_recibida' AND oc_number IS NOT NULL AND payment_method IS NOT NULL`,
		projectID).Scan(&method)
	if err == sql.ErrNoRows {
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
	status, paidAt := PaymentAdvanceInvoiced, any(nil)
	what := "Factura de anticipo " + invoice.Ref + " · " + dayText(invoice.Date)
	if method == "PUE" {
		status, paidAt = PaymentPaid, invoice.Date
		what = "Pago recibido · factura " + invoice.Ref + " · " + dayText(invoice.Date)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE projects SET status = 'facturado', invoice_ref = ?, invoice_date = ?,
			payment_status = ?, paid_at = ?, payment_ref = NULL
		WHERE id = ?`, invoice.Ref, invoice.Date, status, paidAt, projectID); err != nil {
		return fmt.Errorf("store: record invoice of project %d: %w", projectID, err)
	}
	body := "Pasó a " + StageLabels["facturado"] + ".\n" + what
	if note = strings.TrimSpace(note); note != "" {
		body += "\n" + note
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO quote_comments (quote_id, user_id, body) VALUES (?, ?, ?)`, quoteID, userID, body); err != nil {
		return fmt.Errorf("store: record invoice of project %d: comment: %w", projectID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: record invoice of project %d: %w", projectID, err)
	}
	return nil
}

// RecordPayment marks a P.P.D. proyecto that is facturado de anticipo as pagado, with the
// comprobante de pago issued when the payment was completed. It can happen while the
// proyecto is facturado or already en entrega (a P.P.D. order is delivered before it is
// collected), and it is what lets the proyecto be closed. The stage doesn't change.
// Anything else is ErrBadTransition.
func (s *Store) RecordPayment(ctx context.Context, projectID, userID int64, receipt Document, note string) error {
	receipt.Ref = strings.TrimSpace(receipt.Ref)
	if !receipt.valid() {
		return ErrBadTransition
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: record payment of project %d: %w", projectID, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return fmt.Errorf("store: record payment of project %d: %w", projectID, err)
	}
	quoteID, _, err := currentQuote(ctx, tx, projectID)
	if err != nil {
		return fmt.Errorf("store: record payment of project %d: %w", projectID, err)
	}
	if quoteID == 0 {
		return ErrBadTransition
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE projects SET payment_status = ?, payment_ref = ?, paid_at = ?
		WHERE id = ? AND payment_status = ? AND status IN ('facturado', 'en_entrega')`,
		PaymentPaid, receipt.Ref, receipt.Date, projectID, PaymentAdvanceInvoiced)
	if err != nil {
		return fmt.Errorf("store: record payment of project %d: %w", projectID, err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrBadTransition
	}
	body := "Pago completado · comprobante de pago " + receipt.Ref + " · " + dayText(receipt.Date)
	if note = strings.TrimSpace(note); note != "" {
		body += "\n" + note
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO quote_comments (quote_id, user_id, body) VALUES (?, ?, ?)`, quoteID, userID, body); err != nil {
		return fmt.Errorf("store: record payment of project %d: comment: %w", projectID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: record payment of project %d: %w", projectID, err)
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

// LostFromStages are the stages a proyecto can be marked lost from.
var LostFromStages = []string{"prospecto", "oc_recibida"}

// CanLose reports whether a proyecto in stage can be marked lost.
func CanLose(stage string) bool {
	return stage == "prospecto" || stage == "oc_recibida"
}

// LoseProject marks a proyecto lost, from `from` (one of LostFromStages), with the
// reason, which is required. It records it as a comment on the current quote — "Se
// perdió." plus the reason — and remembers the stage it was lost from. Like MoveProject
// it is conditional on the proyecto still being in `from`. There is no lost status on a
// quote: losing a quote and losing its proyecto are the same thing.
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
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO quote_comments (quote_id, user_id, body) VALUES (?, ?, ?)`,
		quoteID, userID, "Se perdió.\n"+reason); err != nil {
		return fmt.Errorf("store: lose project %d: comment: %w", projectID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: lose project %d: %w", projectID, err)
	}
	return nil
}

// ReopenProject brings a lost proyecto back to the stage it was lost from and clears the
// reason, which stays in the history as the comment LoseProject wrote. It records the
// reopening as a comment too, with the user's note if any. Who may reopen is the
// caller's decision.
func (s *Store) ReopenProject(ctx context.Context, projectID, userID int64, note string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: reopen project %d: %w", projectID, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return fmt.Errorf("store: reopen project %d: %w", projectID, err)
	}
	var back string
	err = tx.QueryRowContext(ctx,
		`SELECT lost_from FROM projects WHERE id = ? AND status = 'perdido'`, projectID).Scan(&back)
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
	if note = strings.TrimSpace(note); note != "" {
		body += "\n" + note
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO quote_comments (quote_id, user_id, body) VALUES (?, ?, ?)`, quoteID, userID, body); err != nil {
		return fmt.Errorf("store: reopen project %d: comment: %w", projectID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: reopen project %d: %w", projectID, err)
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
	// Invoice is the purchase order's factura as typed in (the factura de anticipo for a
	// P.P.D. order), PaymentStatus one of PaymentStatusLabels' keys, PaymentRef the folio
	// of the comprobante de pago of a P.P.D. order and PaidAt the day it was paid. All
	// empty until the factura is on record.
	Invoice       Document
	PaymentStatus string
	PaymentRef    string
	PaidAt        string
}

// Paid reports whether the proyecto has been collected, which closing it requires.
func (p Project) Paid() bool { return p.PaymentStatus == PaymentPaid }

// HasOC reports whether the client's purchase order is on record.
func (p Project) HasOC() bool { return p.OC.Number != "" }

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
	COALESCE(p.invoice_ref, ''), COALESCE(p.invoice_date, ''), COALESCE(p.payment_status, ''),
	COALESCE(p.payment_ref, ''), COALESCE(p.paid_at, '')`

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
	JOIN quotes q ON q.project_id = p.id AND q.status != 'revisada'`

func scanProject(row interface{ Scan(...any) error }) (*Project, error) {
	var p Project
	err := row.Scan(&p.ID, &p.Folio, &p.CustomerID, &p.CustomerName, &p.UserID, &p.UserName, &p.Status,
		&p.Probability, &p.ProbabilityUpdatedAt, &p.CreatedAt,
		&p.CurrentQuoteID, &p.CurrentQuoteFolio, &p.CurrentQuoteStatus, &p.Total,
		&p.LostReason, &p.LostFrom, &p.LostAt,
		&p.ExpectedOCDate, &p.NextFollowUpDate, &p.QuoteIssuedAt, &p.QuoteValidUntil,
		&p.ContactName, &p.ContactPhone, &p.ContactEmail,
		&p.LastBody, &p.LastUserName, &p.LastAt,
		&p.OC.Number, &p.OC.Date, &p.OC.PaymentMethod,
		&p.Invoice.Ref, &p.Invoice.Date, &p.PaymentStatus, &p.PaymentRef, &p.PaidAt)
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

// projectStageOrder sorts by where a stage sits in the lifecycle rather than by its name.
const projectStageOrder = `CASE p.status WHEN 'prospecto' THEN 1 WHEN 'oc_recibida' THEN 2
	WHEN 'facturado' THEN 3 WHEN 'en_entrega' THEN 4 WHEN 'cerrado' THEN 5 ELSE 6 END`

// projectSortColumns is the sortable-column whitelist for ListProjects. When sort
// doesn't match a known column, ListProjects uses projectDefaultOrder instead.
var projectSortColumns = []sortColumn{
	{"folio", "p.folio"},
	{"cliente", "c.name"},
	{"vendedor", "u.name COLLATE NOCASE"},
	{"etapa", projectStageOrder},
	{"probabilidad", "p.probability"},
	{"total", "q.total"},
	{"fecha", "p.created_at"},
}

// projectDefaultOrder is the list's order when no sort column is chosen: newest first.
const projectDefaultOrder = `ORDER BY p.created_at DESC, p.id DESC`

// projectFilterColumns are the per-column filters ListProjects accepts. The probability
// filter only matches prospectos, the one stage where it means something.
var projectFilterColumns = []filterColumn{
	{name: "folio", expr: "p.folio", kind: FilterText},
	{name: "cliente", expr: "c.name", kind: FilterText},
	{name: "vendedor", expr: "u.name", kind: FilterText},
	{name: "etapa", expr: "p.status", kind: FilterEnum},
	{name: "pago", expr: "CASE WHEN p.status IN ('facturado', 'en_entrega', 'cerrado') THEN COALESCE(p.payment_status, 'sin_tramitar') END", kind: FilterEnum},
	{name: "probabilidad", expr: "CASE WHEN p.status = 'prospecto' THEN CAST(p.probability AS TEXT) END", kind: FilterEnum},
	{name: "total", expr: "q.total", kind: FilterNumber, scale: 100},
	{name: "fecha", expr: "p.created_at", kind: FilterDate},
}

// ListProjects returns proyectos, optionally filtered by a case-insensitive substring
// match on folio or customer name and by per-column filters (see projectFilterColumns),
// sorted per sort/dir (see projectSortColumns).
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
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+projectSelectCols+projectFrom+`
		WHERE (? = '' OR p.folio LIKE ? ESCAPE '\' COLLATE NOCASE OR c.name LIKE ? ESCAPE '\' COLLATE NOCASE)`+extra+`
		`+order, append([]any{query, like, like}, extraArgs...)...,
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

// ListProspects returns the prospectos for the Pronóstico view, the ones the team goes
// over when it reviews what is about to close: only those relevante para pronóstico, or
// every prospecto when all is set. Soonest expected purchase order first, those without
// a date last, then the likeliest and the largest.
func (s *Store) ListProspects(ctx context.Context, all bool) ([]Project, error) {
	minProbability := ForecastThreshold
	if all {
		minProbability = 0
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+projectSelectCols+projectFrom+`
		WHERE p.status = 'prospecto' AND p.probability >= ?
		ORDER BY p.expected_oc_date IS NULL, p.expected_oc_date, p.probability DESC, q.total DESC, p.id`, minProbability)
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
