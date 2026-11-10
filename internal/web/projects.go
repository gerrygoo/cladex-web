package web

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gerrygoo/cladex-web/internal/facturacion"
	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// Projects holds the /proyectos handlers: the list, a proyecto's page, and the actions
// that follow it up (stage, probabilidad de cierre, comments).
type Projects struct {
	store *store.Store
	// fact is facturación: when it can stamp, a proyecto's factura and pago are CFDIs
	// made from its page instead of folios typed in.
	fact *facturacion.Service
}

func NewProjects(s *store.Store) *Projects {
	return &Projects{store: s}
}

// List handles GET /proyectos: every proyecto, searchable, sortable and filterable by
// column like the other lists.
func (p *Projects) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	sortCol, dir := sortParams(r)
	filters := filterParams(r)
	projects, err := p.store.ListProjects(ctx, query, sortCol, dir, filters)
	if err != nil {
		serverError(w, r, err)
		return
	}
	projects, pager := paginate(r, projects)
	lv := views.ListView{Base: "/proyectos", Query: query, Sort: sortCol, Dir: dir, Filters: filters, Pager: pager}
	if r.Header.Get("HX-Request") == "true" {
		views.ProjectsTableBody(projects, today(r)).Render(ctx, w)
		views.ListPagerOOB(lv).Render(ctx, w)
		return
	}
	customers, owners, err := p.store.ProjectFilterChoices(ctx)
	if err != nil {
		serverError(w, r, err)
		return
	}
	user, _ := UserFromContext(ctx)
	views.ProjectsList(projects, today(r), lv, customers, owners, navUserView(user)).Render(ctx, w)
}

func (p *Projects) loadProjectOrNotFound(w http.ResponseWriter, r *http.Request) *store.Project {
	project, err := p.store.ProjectByFolio(r.Context(), r.PathValue("folio"))
	if err != nil {
		serverError(w, r, err)
		return nil
	}
	if project == nil {
		http.NotFound(w, r)
		return nil
	}
	return project
}

// Page handles GET /proyectos/{folio}: the proyecto, its quotes and its history.
func (p *Projects) Page(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := p.loadProjectOrNotFound(w, r)
	if project == nil {
		return
	}
	quotes, err := p.store.ListProjectQuotes(ctx, project.ID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	files, err := p.store.ListProjectFiles(ctx, project.ID, "oc")
	if err != nil {
		serverError(w, r, err)
		return
	}
	var receipt store.OCReceipt
	if project.HasOC() {
		if receipt, err = p.store.ProjectOCReceipt(ctx, project.ID); err != nil {
			serverError(w, r, err)
			return
		}
	}
	comments, err := p.store.ListProjectComments(ctx, project.ID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	comments, pager := paginate(r, comments)
	commentsLV := views.ListView{Base: "/proyectos/" + project.Folio, Pager: pager, Anchor: "historial"}
	facturas, err := p.store.ListComprobantes(ctx, project.ID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	var current *store.Quote
	for i := range quotes {
		if quotes[i].ID == project.CurrentQuoteID {
			current = &quotes[i]
		}
	}
	supplier, err := supplierQuoteView(r, p.store, current, "/cotizaciones/"+project.CurrentQuoteFolio, "/proyectos/"+project.Folio+"/proveedor")
	if err != nil {
		serverError(w, r, err)
		return
	}
	user, _ := UserFromContext(ctx)
	panel, err := p.adminPanel(ctx, user, project, facturas)
	if err != nil {
		serverError(w, r, err)
		return
	}
	pos, err := projectPOs(ctx, p.store, user, project, current)
	if err != nil {
		serverError(w, r, err)
		return
	}
	payments, err := p.store.ListPayments(ctx, project.ID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	cobro := views.ProjectCobro{Payments: payments, CanRecord: canEditPO(user, project), CanDelete: user != nil && user.Role == store.RoleAdmin}
	views.ProjectPage(*project, today(r), quotes, files, receipt, comments, commentsLV, facturas, p.fact != nil && p.fact.Enabled(), navUserView(user), supplier, panel, pos, cobro).Render(ctx, w)
}

// Cancelar handles POST /proyectos/{folio}/cancelar (admin): marks the proyecto cancelado
// with a required "reason" ("Motivo"), from prospecto or en curso. A factura that may be
// stamped or a confirmed supplier O.C. blocks it: those are cancelled first.
func (p *Projects) Cancelar(w http.ResponseWriter, r *http.Request) {
	p.adminStageChange(w, r, p.store.CancelProject, "este proyecto ya no se puede cancelar")
}

// Reactivar handles POST /proyectos/{folio}/reactivar (admin): brings a cancelled
// proyecto back to the stage it was cancelled from, with a required "reason".
func (p *Projects) Reactivar(w http.ResponseWriter, r *http.Request) {
	p.adminStageChange(w, r, p.store.ReactivateProject, "este proyecto no está cancelado")
}

// DeshacerOC handles POST /proyectos/{folio}/deshacer-oc (admin): takes an en curso
// proyecto back to prospecto at Inminente 90%, with a required "reason", while it has no
// factura and no confirmed supplier O.C.
func (p *Projects) DeshacerOC(w http.ResponseWriter, r *http.Request) {
	p.adminStageChange(w, r, p.store.UndoReceiveOC, "la O.C. recibida solo se deshace en un proyecto En curso")
}

// adminStageChange runs one of the admin's changes of stage, all alike: a required
// "reason" of up to 2000 characters, refused with a message that says what blocks it.
// stale is the message for a proyecto that isn't in a stage the change applies to.
func (p *Projects) adminStageChange(w http.ResponseWriter, r *http.Request, change func(ctx context.Context, projectID, userID int64, reason string) error, stale string) {
	ctx := r.Context()
	project := p.loadProjectOrNotFound(w, r)
	if project == nil {
		return
	}
	reason := strings.TrimSpace(r.FormValue("reason"))
	if reason == "" || utf8.RuneCountInString(reason) > maxCommentLen {
		http.Error(w, "escribe el motivo, de no más de 2000 caracteres", http.StatusBadRequest)
		return
	}
	user, _ := UserFromContext(ctx)
	if err := change(ctx, project.ID, user.ID, reason); err != nil {
		switch {
		case errors.Is(err, store.ErrHasFactura):
			http.Error(w, "este proyecto tiene una factura: primero hay que cancelarla", http.StatusConflict)
		case errors.Is(err, store.ErrHasConfirmedPO):
			http.Error(w, "este proyecto tiene una orden de compra al proveedor confirmada: primero hay que cancelarla", http.StatusConflict)
		case errors.Is(err, store.ErrBadTransition):
			http.Error(w, stale, http.StatusConflict)
		default:
			serverError(w, r, err)
		}
		return
	}
	http.Redirect(w, r, "/proyectos/"+project.Folio+"#historial", http.StatusSeeOther)
}

// Seguimiento handles POST /proyectos/{folio}/seguimiento: saves a prospecto's
// follow-up — "probabilidad" (the percentage of one of the fixed steps), "oc_esperada"
// (a day, blank clears it), "proximo_seguimiento" (a day after today) and the "note" in
// the comment that records what changed. The note and the next follow-up day are both
// required, whether or not the stage changes. "probabilidad=perdida" marks the proyecto
// lost instead, with a required "reason" and nothing else. Any signed-in user can, while
// the proyecto is a prospecto.
func (p *Projects) Seguimiento(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := p.loadProjectOrNotFound(w, r)
	if project == nil {
		return
	}
	user, _ := UserFromContext(ctx)
	if r.FormValue("probabilidad") == "perdida" {
		reason := strings.TrimSpace(r.FormValue("reason"))
		if reason == "" || utf8.RuneCountInString(reason) > maxCommentLen {
			http.Error(w, "escribe el motivo por el que se perdió, de no más de 2000 caracteres", http.StatusBadRequest)
			return
		}
		if err := p.store.LoseProject(ctx, project.ID, user.ID, project.Status, reason); err != nil {
			if errors.Is(err, store.ErrBadTransition) {
				http.Error(w, "este proyecto ya no se puede marcar como perdido", http.StatusConflict)
				return
			}
			serverError(w, r, err)
			return
		}
		http.Redirect(w, r, "/proyectos/"+project.Folio, http.StatusSeeOther)
		return
	}
	note := strings.TrimSpace(r.FormValue("note"))
	if note == "" {
		http.Error(w, "escribe un comentario: qué pasó y cuáles son los siguientes pasos", http.StatusBadRequest)
		return
	}
	if utf8.RuneCountInString(note) > maxCommentLen {
		http.Error(w, "el comentario no puede pasar de 2000 caracteres", http.StatusBadRequest)
		return
	}
	percent, err := strconv.Atoi(r.FormValue("probabilidad"))
	if err != nil || store.ProbabilityLabel(percent) == "" {
		http.Error(w, "elige una probabilidad de la lista", http.StatusBadRequest)
		return
	}
	expectedOC := strings.TrimSpace(r.FormValue("oc_esperada"))
	nextFollowUp := strings.TrimSpace(r.FormValue("proximo_seguimiento"))
	if _, err := time.Parse("2006-01-02", expectedOC); expectedOC != "" && err != nil {
		http.Error(w, "las fechas deben tener el formato AAAA-MM-DD", http.StatusBadRequest)
		return
	}
	if _, err := time.Parse("2006-01-02", nextFollowUp); err != nil {
		http.Error(w, "elige la fecha del próximo seguimiento", http.StatusBadRequest)
		return
	}
	if nextFollowUp <= today(r) {
		http.Error(w, "la fecha del próximo seguimiento debe ser posterior a hoy", http.StatusBadRequest)
		return
	}
	err = p.store.FollowUpProject(ctx, project.ID, user.ID, store.FollowUp{
		Probability: &percent, ExpectedOC: &expectedOC, NextFollowUp: &nextFollowUp, Note: note,
	})
	if err != nil {
		if errors.Is(err, store.ErrBadTransition) {
			http.Error(w, "solo a un prospecto se le anota probabilidad y fechas de seguimiento", http.StatusConflict)
			return
		}
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/proyectos/"+project.Folio+"#historial", http.StatusSeeOther)
}

// today is the viewer's current day (YYYY-MM-DD), in the zone their browser reported.
func today(r *http.Request) string {
	return time.Now().In(store.LocationFromContext(r.Context())).Format("2006-01-02")
}

// Pronostico handles GET /pronostico: the prospectos relevante para pronóstico, or every
// prospecto with ?todos=1, as a table sortable by every column and filterable by vendedor.
// Nothing is edited here; follow-up is kept on the proyecto's page.
func (p *Projects) Pronostico(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	all := r.URL.Query().Get("todos") == "1"
	sortCol, dir := sortParams(r)
	filters := filterParams(r)
	projects, err := p.store.ListProspects(ctx, all, sortCol, dir, filters)
	if err != nil {
		serverError(w, r, err)
		return
	}
	_, owners, err := p.store.ProjectFilterChoices(ctx)
	if err != nil {
		serverError(w, r, err)
		return
	}
	lv := views.ListView{Base: "/pronostico", Sort: sortCol, Dir: dir, Filters: filters}
	if all {
		lv.Keep = url.Values{"todos": {"1"}}
	}
	user, _ := UserFromContext(ctx)
	views.ForecastPage(views.Forecast{Projects: projects, All: all, Today: today(r)}, lv, owners, navUserView(user)).Render(ctx, w)
}

// Comentar handles POST /proyectos/{folio}/comentarios: any signed-in user adds a note
// to the proyecto's history. It is saved on the proyecto's current quote, so it also
// shows on that quote's page.
func (p *Projects) Comentar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := p.loadProjectOrNotFound(w, r)
	if project == nil {
		return
	}
	body := strings.TrimSpace(r.FormValue("comment"))
	if body == "" || utf8.RuneCountInString(body) > maxCommentLen {
		http.Error(w, "el comentario no puede estar vacío ni pasar de 2000 caracteres", http.StatusBadRequest)
		return
	}
	user, _ := UserFromContext(ctx)
	if err := p.store.AddQuoteComment(ctx, project.CurrentQuoteID, user.ID, body); err != nil {
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/proyectos/"+project.Folio+"#historial", http.StatusSeeOther)
}

// Perder handles POST /proyectos/{folio}/perder: marks the proyecto lost with a required
// "reason". Any signed-in user can, while it is a prospecto; once en curso, a sale that
// falls through is cancelled by an admin instead (Cancelar).
func (p *Projects) Perder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := p.loadProjectOrNotFound(w, r)
	if project == nil {
		return
	}
	reason := strings.TrimSpace(r.FormValue("reason"))
	if reason == "" || utf8.RuneCountInString(reason) > maxCommentLen {
		http.Error(w, "escribe el motivo, de no más de 2000 caracteres", http.StatusBadRequest)
		return
	}
	user, _ := UserFromContext(ctx)
	if err := p.store.LoseProject(ctx, project.ID, user.ID, project.Status, reason); err != nil {
		if errors.Is(err, store.ErrBadTransition) {
			http.Error(w, "este proyecto ya no se puede marcar como perdido", http.StatusConflict)
			return
		}
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/proyectos/"+project.Folio, http.StatusSeeOther)
}

// Reabrir handles POST /proyectos/{folio}/reabrir: brings a lost proyecto back to the
// stage it was lost from. The "note" is required: it goes into the comment that records
// it, as with any change of stage. For one lost from prospecto, "probabilidad" may pick
// the step it comes back at. Any user may reopen one (issue #22).
func (p *Projects) Reabrir(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := p.loadProjectOrNotFound(w, r)
	if project == nil {
		return
	}
	note := strings.TrimSpace(r.FormValue("note"))
	if note == "" || utf8.RuneCountInString(note) > maxCommentLen {
		http.Error(w, "escribe un comentario de no más de 2000 caracteres: por qué se reabre", http.StatusBadRequest)
		return
	}
	var probability *int
	if v := r.FormValue("probabilidad"); v != "" {
		percent, err := strconv.Atoi(v)
		if err != nil || store.ProbabilityLabel(percent) == "" {
			http.Error(w, "elige una probabilidad de la lista", http.StatusBadRequest)
			return
		}
		probability = &percent
	}
	user, _ := UserFromContext(ctx)
	if err := p.store.ReopenProject(ctx, project.ID, user.ID, probability, note); err != nil {
		if errors.Is(err, store.ErrBadTransition) {
			http.Error(w, "este proyecto no está perdido", http.StatusConflict)
			return
		}
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/proyectos/"+project.Folio+"#historial", http.StatusSeeOther)
}

// maxOCFileBytes caps the purchase order file. It is stored in the database (see
// migrations/0019_project_oc.sql), so it is kept small.
const maxOCFileBytes = 10 << 20

// ocFileTypes are the kinds of file a purchase order may be, by what the bytes are, not
// by what the name or the browser says.
var ocFileTypes = map[string]bool{"application/pdf": true, "image/jpeg": true, "image/png": true}

// OC handles POST /proyectos/{folio}/oc: records the client's purchase order —
// "oc_numero" and "oc_fecha" (a day), both required, with the "archivo" (PDF, JPG or PNG
// up to 10 MB) and an optional "note". On a prospecto this is what moves the proyecto to
// en curso, and the file is required; on one already there it corrects the data (while
// its facturación is pendiente), and a new file is optional. The método de pago is not asked here: it belongs to the factura
// (issue #61). Any signed-in user can.
func (p *Projects) OC(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := p.loadProjectOrNotFound(w, r)
	if project == nil {
		return
	}
	// A little over the cap, for the form's other fields and the multipart framing.
	r.Body = http.MaxBytesReader(w, r.Body, maxOCFileBytes+1<<20)
	if err := r.ParseMultipartForm(maxOCFileBytes + 1<<20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		http.Error(w, "el archivo no puede pesar más de 10 MB", http.StatusRequestEntityTooLarge)
		return
	}
	note := strings.TrimSpace(r.FormValue("note"))
	if utf8.RuneCountInString(note) > maxCommentLen {
		http.Error(w, "el comentario no puede pasar de 2000 caracteres", http.StatusBadRequest)
		return
	}
	oc := store.OC{
		Number: strings.TrimSpace(r.FormValue("oc_numero")),
		Date:   strings.TrimSpace(r.FormValue("oc_fecha")),
	}
	if oc.Number == "" || utf8.RuneCountInString(oc.Number) > 100 {
		http.Error(w, "escribe el número de la orden de compra, de no más de 100 caracteres", http.StatusBadRequest)
		return
	}
	if _, err := time.Parse("2006-01-02", oc.Date); err != nil {
		http.Error(w, "escribe la fecha de la orden de compra", http.StatusBadRequest)
		return
	}
	file, msg, status := ocFile(r)
	if msg != "" {
		http.Error(w, msg, status)
		return
	}
	// The file is what the proyecto needs to reach Orden colocada 100%; correcting the
	// data of one already there doesn't ask for it again.
	if file == nil && project.Status == "prospecto" {
		http.Error(w, "adjunta el archivo de la orden de compra: PDF, JPG o PNG de hasta 10 MB", http.StatusBadRequest)
		return
	}
	user, _ := UserFromContext(ctx)
	if err := p.store.ReceiveOC(ctx, project.ID, user.ID, oc, file, note); err != nil {
		if errors.Is(err, store.ErrBadTransition) {
			http.Error(w, "la orden de compra solo se captura en un prospecto con su cotización emitida, o se corrige mientras el proyecto está En curso y sin factura", http.StatusConflict)
			return
		}
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/proyectos/"+project.Folio+"#oc", http.StatusSeeOther)
}

// ocFile reads the form's optional "archivo". It returns nil when none was sent, and a
// message with its HTTP status when the file is too large, empty or not an accepted kind.
func ocFile(r *http.Request) (*store.NewFile, string, int) {
	return uploadedFile(r, "archivo", maxOCFileBytes, ocFileTypes,
		"el archivo no puede pesar más de 10 MB", "el archivo debe ser PDF, JPG o PNG", "orden-de-compra")
}

// uploadedFile reads the optional file in field, at most maxBytes, of one of types (by
// what the bytes are, not by the name or what the browser says). It returns nil when none
// was sent, and a message with its HTTP status when the file is too large (tooBig),
// empty or not an accepted kind (badType). The name keeps only its last path element,
// without control characters and at most 200 characters; fallback replaces a blank one.
func uploadedFile(r *http.Request, field string, maxBytes int, types map[string]bool, tooBig, badType, fallback string) (*store.NewFile, string, int) {
	f, header, err := r.FormFile(field)
	if err != nil {
		return nil, "", 0 // no file, or not a multipart form
	}
	f.Close()
	return readUploadedFile(header, maxBytes, types, tooBig, badType, fallback)
}

// readUploadedFile is uploadedFile for one file of a multipart form, which may hold
// several under one field.
func readUploadedFile(header *multipart.FileHeader, maxBytes int, types map[string]bool, tooBig, badType, fallback string) (*store.NewFile, string, int) {
	f, err := header.Open()
	if err != nil {
		return nil, "no se pudo leer el archivo", http.StatusBadRequest
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, int64(maxBytes)+1))
	if err != nil {
		return nil, "no se pudo leer el archivo", http.StatusBadRequest
	}
	if len(data) > maxBytes {
		return nil, tooBig, http.StatusRequestEntityTooLarge
	}
	if len(data) == 0 {
		return nil, "el archivo está vacío", http.StatusBadRequest
	}
	contentType := http.DetectContentType(data)
	if !types[contentType] {
		return nil, badType, http.StatusBadRequest
	}
	// Browsers send only the name, but an old one or a script may send a path.
	name := path.Base(strings.ReplaceAll(header.Filename, "\\", "/"))
	name = strings.Map(func(c rune) rune {
		if c < 0x20 || c == 0x7f {
			return -1
		}
		return c
	}, name)
	if name = strings.TrimSpace(name); name == "" || name == "." || name == "/" {
		name = fallback
	}
	if r := []rune(name); len(r) > 200 {
		name = string(r[len(r)-200:])
	}
	return &store.NewFile{Filename: name, ContentType: contentType, Data: data}, "", 0
}

// Archivo handles GET /proyectos/{folio}/archivos/{id}: downloads one of the proyecto's
// files. It is always sent as an attachment with the type detected at upload, never
// rendered in the app's origin.
func (p *Projects) Archivo(w http.ResponseWriter, r *http.Request) {
	project := p.loadProjectOrNotFound(w, r)
	if project == nil {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	file, data, err := p.store.ProjectFileData(r.Context(), project.ID, id)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if file == nil {
		http.NotFound(w, r)
		return
	}
	serveUploadedFile(w, r, file.Filename, file.ContentType, data)
}

// documentForm reads a hand-typed fiscal document from a form: "folio" and "fecha" (a
// day), both required. It returns a message when either is missing or malformed.
func documentForm(r *http.Request) (store.Document, string) {
	d := store.Document{Ref: strings.TrimSpace(r.FormValue("folio")), Date: strings.TrimSpace(r.FormValue("fecha"))}
	if d.Ref == "" || utf8.RuneCountInString(d.Ref) > 100 {
		return d, "escribe el folio, de no más de 100 caracteres"
	}
	if _, err := time.Parse("2006-01-02", d.Date); err != nil {
		return d, "escribe la fecha"
	}
	return d, ""
}

// Factura handles POST /proyectos/{folio}/factura: puts the purchase order's factura on
// record, typed in ("folio", "fecha", "metodo_pago" PUE or PPD, optional "note"), which
// makes the proyecto's Estado de facturación Facturada. It records no payment: that is
// Registrar pago (Pagos). Any signed-in user can, while the proyecto is en curso without
// a factura.
func (p *Projects) Factura(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := p.loadProjectOrNotFound(w, r)
	if project == nil {
		return
	}
	note := strings.TrimSpace(r.FormValue("note"))
	if utf8.RuneCountInString(note) > maxCommentLen {
		http.Error(w, "el comentario no puede pasar de 2000 caracteres", http.StatusBadRequest)
		return
	}
	invoice, msg := documentForm(r)
	if msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	method := r.FormValue("metodo_pago")
	if store.PaymentMethodLabels[method] == "" {
		http.Error(w, "elige el método de pago: P.U.E. o P.P.D.", http.StatusBadRequest)
		return
	}
	user, _ := UserFromContext(ctx)
	if err := p.store.RecordInvoice(ctx, project.ID, user.ID, invoice, method, note); err != nil {
		if errors.Is(err, store.ErrBadTransition) {
			http.Error(w, "la factura solo se registra en un proyecto En curso con su orden de compra capturada y sin otra factura", http.StatusConflict)
			return
		}
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/proyectos/"+project.Folio+"#facturacion", http.StatusSeeOther)
}

// Pagos handles POST /proyectos/{folio}/pagos: "Registrar pago" (#67), a payment the
// client made toward an en curso proyecto. "fecha" is the day it came in (required, not
// after the viewer's today), "monto" the amount with IVA (required, more than $0 and no
// more than the saldo pendiente), and "comprobante" sí or no; with sí, "archivo" (PDF,
// JPG or PNG up to 10 MB, like the O.C.'s) is required. The proyecto's vendedor and the
// admins can.
func (p *Projects) Pagos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := p.loadProjectOrNotFound(w, r)
	if project == nil {
		return
	}
	user, _ := UserFromContext(ctx)
	if !canEditPO(user, project) {
		http.Error(w, "solo el vendedor del proyecto o un administrador registra sus pagos", http.StatusForbidden)
		return
	}
	if project.Status != store.StageEnCurso {
		http.Error(w, "los pagos solo se registran en un proyecto En curso", http.StatusConflict)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxOCFileBytes+1<<20)
	if err := r.ParseMultipartForm(maxOCFileBytes + 1<<20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		http.Error(w, "el comprobante no puede pesar más de 10 MB", http.StatusRequestEntityTooLarge)
		return
	}
	day := strings.TrimSpace(r.FormValue("fecha"))
	if _, err := time.Parse("2006-01-02", day); err != nil {
		http.Error(w, "escribe la fecha de pago", http.StatusBadRequest)
		return
	}
	if day > today(r) {
		http.Error(w, "la fecha de pago no puede ser futura", http.StatusBadRequest)
		return
	}
	amount, ok := centavosInput(r.FormValue("monto"))
	if !ok || amount <= 0 {
		http.Error(w, "escribe el monto pagado, mayor a $0.00", http.StatusBadRequest)
		return
	}
	if amount > project.Balance() {
		http.Error(w, "el monto no puede ser mayor al saldo pendiente, "+project.Balance().String(), http.StatusBadRequest)
		return
	}
	var file *store.NewFile
	switch r.FormValue("comprobante") {
	case "si":
		f, msg, status := uploadedFile(r, "archivo", maxOCFileBytes, ocFileTypes,
			"el comprobante no puede pesar más de 10 MB", "el comprobante debe ser PDF, JPG o PNG", "comprobante-de-pago")
		if msg != "" {
			http.Error(w, msg, status)
			return
		}
		if f == nil {
			http.Error(w, "adjunta el comprobante de pago: PDF, JPG o PNG de hasta 10 MB", http.StatusBadRequest)
			return
		}
		file = f
	case "no", "":
	default:
		http.Error(w, "elige si quieres subir un comprobante de pago", http.StatusBadRequest)
		return
	}
	if err := p.store.RecordPayment(ctx, project.ID, user.ID, amount, day, file); err != nil {
		switch {
		case errors.Is(err, store.ErrPaymentOverBalance):
			http.Error(w, "el monto no puede ser mayor al saldo pendiente", http.StatusConflict)
		case errors.Is(err, store.ErrBadTransition):
			http.Error(w, "los pagos solo se registran en un proyecto En curso", http.StatusConflict)
		default:
			serverError(w, r, err)
		}
		return
	}
	http.Redirect(w, r, "/proyectos/"+project.Folio+"#cobro", http.StatusSeeOther)
}

// centavosInput reads an amount typed in pesos ("7467.98", "$7,467.98") as centavos. It
// is false for anything that isn't a number with at most two decimals.
func centavosInput(raw string) (money.Centavos, bool) {
	m, err := money.ParseMicros(amountInput(raw))
	if err != nil || m%10_000 != 0 {
		return 0, false
	}
	return money.Centavos(m / 10_000), true
}

// EliminarPago handles POST /proyectos/{folio}/pagos/{id}/eliminar (admin): takes a
// payment recorded by mistake out of the cobro, with a required "reason" ("Motivo") that
// goes into the history.
func (p *Projects) EliminarPago(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := p.loadProjectOrNotFound(w, r)
	if project == nil {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	reason := strings.TrimSpace(r.FormValue("reason"))
	if reason == "" || utf8.RuneCountInString(reason) > maxCommentLen {
		http.Error(w, "escribe el motivo, de no más de 2000 caracteres", http.StatusBadRequest)
		return
	}
	user, _ := UserFromContext(ctx)
	if err := p.store.DeletePayment(ctx, project.ID, id, user.ID, reason); err != nil {
		if errors.Is(err, store.ErrBadTransition) {
			http.Error(w, "ese pago no existe o ya se eliminó", http.StatusConflict)
			return
		}
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/proyectos/"+project.Folio+"#cobro", http.StatusSeeOther)
}

// adminPanel is the Administración section of a proyecto's page: nil for anyone but an
// admin. Deleting is blocked while the proyecto has a comprobante that may have been
// stamped, since a CFDI is a legal document.
func (p *Projects) adminPanel(ctx context.Context, user *store.AuthenticatedUser, project *store.Project, facturas []store.Comprobante) (*views.AdminPanel, error) {
	if user == nil || user.Role != store.RoleAdmin {
		return nil, nil
	}
	users, err := p.store.ActiveUsers(ctx)
	if err != nil {
		return nil, err
	}
	panel := &views.AdminPanel{
		Base:    "/proyectos/" + project.Folio,
		OwnerID: project.UserID,
		Users:   users,
		What:    "proyecto",
		DeleteWarning: "Borra el proyecto con todas sus cotizaciones y revisiones, su historial, " +
			"la orden de compra, sus órdenes de compra al proveedor y las facturas sin timbrar. No se puede deshacer.",
	}
	stamped := false
	for _, f := range facturas {
		if !f.Editable() {
			panel.BlockedReason = "No se puede eliminar: tiene una factura timbrada (o en proceso de timbrarse), que es un documento fiscal."
		}
		if f.Tipo == store.TipoFactura && !f.Editable() && f.Status != store.CompCancelada {
			stamped = true
		}
	}
	confirmedPO := project.States.Entrega != store.EntregaSinOC
	base := "/proyectos/" + project.Folio
	if project.Status == store.StageProspecto || project.Status == store.StageEnCurso {
		act := views.StageAction{
			Title:   "Cancelar proyecto",
			Path:    base + "/cancelar",
			Hint:    "Para un pedido que se cae. El proyecto queda Cancelado con el motivo, y se puede reactivar después en la etapa en que estaba.",
			Button:  "Cancelar proyecto",
			Confirm: "¿Cancelar este proyecto?",
		}
		switch {
		case stamped:
			act.Blocked = "No se puede cancelar: tiene una factura vigente. Primero cancela la factura."
		case confirmedPO:
			act.Blocked = "No se puede cancelar: tiene una orden de compra al proveedor confirmada. Primero cancélala."
		}
		panel.StageActions = append(panel.StageActions, act)
	}
	if project.Status == store.StageEnCurso {
		act := views.StageAction{
			Title:   "Deshacer O.C. recibida",
			Path:    base + "/deshacer-oc",
			Hint:    "Si la orden de compra se capturó por error (otro archivo u otro proyecto). El proyecto regresa a Prospecto en Inminente 90% y se borran el número y la fecha de la O.C.; quedan en el historial.",
			Button:  "Deshacer O.C. recibida",
			Confirm: "¿Deshacer la O.C. recibida y regresar el proyecto a Prospecto?",
		}
		switch {
		case stamped || project.Invoice.Ref != "":
			act.Blocked = "No se puede deshacer: el proyecto ya tiene factura."
		case confirmedPO:
			act.Blocked = "No se puede deshacer: tiene una orden de compra al proveedor confirmada."
		}
		panel.StageActions = append(panel.StageActions, act)
	}
	return panel, nil
}

// Reasignar handles POST /proyectos/{folio}/reasignar (admin): hands the proyecto and
// all its quotes to another enabled user, who becomes the one following it up.
func (p *Projects) Reasignar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := p.loadProjectOrNotFound(w, r)
	if project == nil {
		return
	}
	newID, err := strconv.ParseInt(r.FormValue("user_id"), 10, 64)
	if err != nil {
		http.Error(w, "elige un vendedor de la lista", http.StatusBadRequest)
		return
	}
	user, _ := UserFromContext(ctx)
	if newID != project.UserID {
		if err := p.store.ReassignProject(ctx, project.ID, user.ID, newID); err != nil {
			if errors.Is(err, store.ErrBadOwner) {
				http.Error(w, "elige un vendedor de la lista", http.StatusBadRequest)
				return
			}
			serverError(w, r, err)
			return
		}
	}
	http.Redirect(w, r, "/proyectos/"+project.Folio, http.StatusSeeOther)
}

// Eliminar handles POST /proyectos/{folio}/eliminar (admin): deletes the proyecto and
// everything under it, unless it has a stamped comprobante.
func (p *Projects) Eliminar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := p.loadProjectOrNotFound(w, r)
	if project == nil {
		return
	}
	if err := p.store.DeleteProject(ctx, project.ID); err != nil {
		if errors.Is(err, store.ErrHasStampedInvoice) {
			http.Error(w, "este proyecto tiene una factura timbrada y no se puede eliminar", http.StatusConflict)
			return
		}
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/proyectos", http.StatusSeeOther)
}
