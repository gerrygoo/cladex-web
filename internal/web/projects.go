package web

import (
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gerrygoo/cladex-web/internal/facturacion"
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
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	projects, pager := paginate(r, projects)
	lv := views.ListView{Base: "/proyectos", Query: query, Sort: sortCol, Dir: dir, Filters: filters, Pager: pager}
	if r.Header.Get("HX-Request") == "true" {
		views.ProjectsTableBody(projects).Render(ctx, w)
		views.ListPagerOOB(lv).Render(ctx, w)
		return
	}
	customers, owners, err := p.store.ProjectFilterChoices(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	user, _ := UserFromContext(ctx)
	views.ProjectsList(projects, lv, customers, owners, navUserView(user)).Render(ctx, w)
}

func (p *Projects) loadProjectOrNotFound(w http.ResponseWriter, r *http.Request) *store.Project {
	project, err := p.store.ProjectByFolio(r.Context(), r.PathValue("folio"))
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
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
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	files, err := p.store.ListProjectFiles(ctx, project.ID, "oc")
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	comments, err := p.store.ListProjectComments(ctx, project.ID)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	comments, pager := paginate(r, comments)
	commentsLV := views.ListView{Base: "/proyectos/" + project.Folio, Pager: pager, Anchor: "historial"}
	facturas, err := p.store.ListComprobantes(ctx, project.ID)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
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
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	user, _ := UserFromContext(ctx)
	views.ProjectPage(*project, today(r), quotes, files, comments, commentsLV, facturas, p.fact != nil && p.fact.Enabled(), navUserView(user), supplier).Render(ctx, w)
}

// Etapa handles POST /proyectos/{folio}/etapa: moves the proyecto one stage along the
// flow ("to" is the stage; the optional "note" is saved as a comment). Anyone signed in
// can move a proyecto forward; going back a stage is for admins, since it undoes what a
// salesperson reported. Anything but the stage right before or after the current one is
// refused, as is a stale click on a proyecto that has since moved.
func (p *Projects) Etapa(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := p.loadProjectOrNotFound(w, r)
	if project == nil {
		return
	}
	to := r.FormValue("to")
	note := strings.TrimSpace(r.FormValue("note"))
	if utf8.RuneCountInString(note) > maxCommentLen {
		http.Error(w, "el comentario no puede pasar de 2000 caracteres", http.StatusBadRequest)
		return
	}
	user, _ := UserFromContext(ctx)
	if to != "" && to == store.PrevStage(project.Status) && user.Role != "admin" {
		http.Error(w, "solo un administrador puede regresar un proyecto de etapa", http.StatusForbidden)
		return
	}
	if err := p.store.MoveProject(ctx, project.ID, user.ID, project.Status, to, note); err != nil {
		if errors.Is(err, store.ErrBadTransition) {
			// Say what is missing when the move is one of the gated steps.
			switch {
			case project.Status == "prospecto" && to == "oc_recibida":
				http.Error(w, "primero captura la orden de compra del cliente", http.StatusConflict)
				return
			case project.Status == "oc_recibida" && (to == "facturado" || to == "en_entrega"):
				http.Error(w, "primero registra la factura: sin factura el proyecto no se puede entregar", http.StatusConflict)
				return
			case project.Status == "en_entrega" && to == "cerrado" && !project.Paid():
				http.Error(w, "primero registra el pago completado: un proyecto sin pagar no se puede cerrar", http.StatusConflict)
				return
			}
			http.Error(w, "el proyecto no puede pasar a esa etapa", http.StatusConflict)
			return
		}
		http.Error(w, "error interno", http.StatusInternalServerError)
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
			http.Error(w, "error interno", http.StatusInternalServerError)
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
		http.Error(w, "error interno", http.StatusInternalServerError)
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
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	_, owners, err := p.store.ProjectFilterChoices(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
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
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/proyectos/"+project.Folio+"#historial", http.StatusSeeOther)
}

// Perder handles POST /proyectos/{folio}/perder: marks the proyecto lost with a required
// "reason". Any signed-in user can, while it is a prospecto or has its purchase order
// in; a proyecto already being delivered or closed can't be lost.
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
		http.Error(w, "error interno", http.StatusInternalServerError)
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
		http.Error(w, "error interno", http.StatusInternalServerError)
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
// "oc_numero", "oc_fecha" (a day) and "forma_pago" (PUE or PPD), all required, with the
// "archivo" (PDF, JPG or PNG up to 10 MB) and an optional "note". On a prospecto this is
// what moves the proyecto to O.C. recibida, and the file is required; on one already there
// it corrects the data, and a new file is optional.
// Any signed-in user can.
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
		Number:        strings.TrimSpace(r.FormValue("oc_numero")),
		Date:          strings.TrimSpace(r.FormValue("oc_fecha")),
		PaymentMethod: r.FormValue("forma_pago"),
	}
	if oc.Number == "" || utf8.RuneCountInString(oc.Number) > 100 {
		http.Error(w, "escribe el número de la orden de compra, de no más de 100 caracteres", http.StatusBadRequest)
		return
	}
	if _, err := time.Parse("2006-01-02", oc.Date); err != nil {
		http.Error(w, "escribe la fecha de la orden de compra", http.StatusBadRequest)
		return
	}
	if store.PaymentMethodLabels[oc.PaymentMethod] == "" {
		http.Error(w, "elige la forma de pago: P.U.E. o P.P.D.", http.StatusBadRequest)
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
			http.Error(w, "la orden de compra solo se captura en un prospecto con su cotización emitida, o se corrige mientras el proyecto está en O.C. recibida", http.StatusConflict)
			return
		}
		http.Error(w, "error interno", http.StatusInternalServerError)
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
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	if file == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", file.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Filename}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
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
// record ("folio", "fecha", optional "note") and moves the proyecto from O.C. recibida to
// Facturado. For a P.U.E. order that is the payment too; for P.P.D. it is the factura de
// anticipo. Any signed-in user can.
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
	user, _ := UserFromContext(ctx)
	if err := p.store.RecordInvoice(ctx, project.ID, user.ID, invoice, note); err != nil {
		if errors.Is(err, store.ErrBadTransition) {
			http.Error(w, "la factura solo se registra en un proyecto en O.C. recibida con su orden de compra capturada", http.StatusConflict)
			return
		}
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/proyectos/"+project.Folio+"#pago", http.StatusSeeOther)
}

// Pago handles POST /proyectos/{folio}/pago: marks a P.P.D. proyecto that is facturado de
// anticipo as pagado, with its comprobante de pago ("folio", "fecha", optional "note").
// Any signed-in user can, while the proyecto is Facturado or En entrega.
func (p *Projects) Pago(w http.ResponseWriter, r *http.Request) {
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
	receipt, msg := documentForm(r)
	if msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	user, _ := UserFromContext(ctx)
	if err := p.store.RecordPayment(ctx, project.ID, user.ID, receipt, note); err != nil {
		if errors.Is(err, store.ErrBadTransition) {
			http.Error(w, "el pago completado solo se registra en un proyecto facturado de anticipo que está en Facturado o En entrega", http.StatusConflict)
			return
		}
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/proyectos/"+project.Folio+"#pago", http.StatusSeeOther)
}
