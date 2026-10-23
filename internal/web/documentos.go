package web

import (
	"errors"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// Documentos holds the /documentos handlers (issue #32): every user sees and opens the
// official documents; only admins upload, edit or remove documents and sections (the
// routes wrap every POST in RequireAdmin; issue #44 added the editing).
type Documentos struct {
	store *store.Store
}

func NewDocumentos(s *store.Store) *Documentos {
	return &Documentos{store: s}
}

// maxDocumentBytes caps an official document. It is stored in the database (see
// migrations/0023_official_documents.sql), but a presentation as PDF can be large.
const maxDocumentBytes = 25 << 20

// documentFileTypes are the kinds of file a document may be: PDF and images, by what the
// bytes are. SVG is left out: it can carry scripts.
var documentFileTypes = map[string]bool{
	"application/pdf": true, "image/jpeg": true, "image/png": true, "image/webp": true, "image/gif": true,
}

// documentosSaved are the messages for /documentos?guardado=….
var documentosSaved = map[string]string{
	"subido":             "Documento subido.",
	"eliminado":          "Documento eliminado.",
	"editado":            "Documento guardado.",
	"seccion-renombrada": "Sección renombrada.",
	"seccion-eliminada":  "Sección eliminada.",
	"seccion-movida":     "Orden de las secciones guardado.",
}

// List handles GET /documentos. ?seccion=<id> unfolds that section: the redirects after
// an action send it, so the admin sees what they just did.
func (d *Documentos) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	open, _ := strconv.ParseInt(q.Get("seccion"), 10, 64)
	d.render(w, r, views.DocumentosPage{Success: documentosSaved[q.Get("guardado")], Open: open}, http.StatusOK)
}

func (d *Documentos) render(w http.ResponseWriter, r *http.Request, page views.DocumentosPage, status int) {
	ctx := r.Context()
	sections, err := d.store.ListDocumentSections(ctx)
	if err != nil {
		serverError(w, r, err)
		return
	}
	page.Sections = sections
	user, _ := UserFromContext(ctx)
	w.WriteHeader(status)
	views.Documentos(page, navUserView(user)).Render(ctx, w)
}

// documentosAt is where an action on a section sends the browser back to: the page with
// that section unfolded and scrolled to.
func documentosAt(saved string, sectionID int64) string {
	u := "/documentos?guardado=" + saved
	if sectionID != 0 {
		id := strconv.FormatInt(sectionID, 10)
		u += "&seccion=" + id + "#" + views.SectionField(sectionID, "")
	}
	return u
}

// documentSection returns the id of the section the document is in, or 0 when it isn't
// on the page.
func (d *Documentos) documentSection(r *http.Request, docID int64) (int64, error) {
	sections, err := d.store.ListDocumentSections(r.Context())
	if err != nil {
		return 0, err
	}
	for _, sec := range sections {
		for _, doc := range sec.Documents {
			if doc.ID == docID {
				return sec.ID, nil
			}
		}
	}
	return 0, nil
}

// Editar handles POST /documentos/{id}/editar (admins): "titulo" and "seccion", the id
// of the section to move it to.
func (d *Documentos) Editar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	current, err := d.documentSection(r, id)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	if current == 0 {
		http.NotFound(w, r)
		return
	}
	titleKey, sectionKey := views.DocumentField(id, "titulo"), views.DocumentField(id, "seccion")
	title, section := strings.TrimSpace(r.FormValue("titulo")), r.FormValue("seccion")
	sectionID, _ := strconv.ParseInt(section, 10, 64)
	errs := map[string]string{}
	switch {
	case title == "":
		errs[titleKey] = "Escribe el título del documento."
	case utf8.RuneCountInString(title) > 200:
		errs[titleKey] = "El título no puede pasar de 200 caracteres."
	}
	if sectionID == 0 {
		errs[sectionKey] = "Elige una sección."
	}
	if len(errs) == 0 {
		ok, err := d.store.UpdateOfficialDocument(ctx, id, sectionID, title)
		switch {
		case errors.Is(err, store.ErrBadDocument):
			errs[sectionKey] = "Esa sección ya no existe; elige otra."
		case err != nil:
			http.Error(w, "error interno", http.StatusInternalServerError)
			return
		case !ok:
			http.NotFound(w, r)
			return
		default:
			http.Redirect(w, r, documentosAt("editado", sectionID), http.StatusSeeOther)
			return
		}
	}
	d.render(w, r, views.DocumentosPage{
		Errors: errs,
		Edit:   map[string]string{titleKey: title, sectionKey: section},
		Open:   current,
	}, http.StatusBadRequest)
}

// RenombrarSeccion handles POST /documentos/secciones/{id}/editar (admins): "nombre".
func (d *Documentos) RenombrarSeccion(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	key := views.SectionField(id, "nombre")
	name := strings.TrimSpace(r.FormValue("nombre"))
	msg := ""
	switch {
	case name == "":
		msg = "Escribe el nombre de la sección."
	case utf8.RuneCountInString(name) > 100:
		msg = "El nombre de la sección no puede pasar de 100 caracteres."
	default:
		err := d.store.RenameDocumentSection(r.Context(), id, name)
		switch {
		case errors.Is(err, store.ErrSectionNameTaken):
			msg = "Ya hay una sección con ese nombre; escribe otro."
		case errors.Is(err, store.ErrNoSection):
			http.NotFound(w, r)
			return
		case err != nil:
			http.Error(w, "error interno", http.StatusInternalServerError)
			return
		default:
			http.Redirect(w, r, documentosAt("seccion-renombrada", id), http.StatusSeeOther)
			return
		}
	}
	d.render(w, r, views.DocumentosPage{
		Errors: map[string]string{key: msg},
		Edit:   map[string]string{key: name},
		Open:   id,
	}, http.StatusBadRequest)
}

// MoverSeccion handles POST /documentos/secciones/{id}/mover (admins): "hacia" is
// "arriba" or "abajo", one place.
func (d *Documentos) MoverSeccion(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	by := 0
	switch r.FormValue("hacia") {
	case "arriba":
		by = -1
	case "abajo":
		by = 1
	default:
		http.Error(w, "dirección inválida", http.StatusBadRequest)
		return
	}
	err = d.store.MoveDocumentSection(r.Context(), id, by)
	switch {
	case errors.Is(err, store.ErrNoSection):
		http.NotFound(w, r)
	case err != nil:
		serverError(w, r, err)
	default:
		http.Redirect(w, r, documentosAt("seccion-movida", id), http.StatusSeeOther)
	}
}

// EliminarSeccion handles POST /documentos/secciones/{id}/eliminar (admins). A section
// with documents is refused: they are moved or removed first.
func (d *Documentos) EliminarSeccion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	user, _ := UserFromContext(ctx)
	err = d.store.RemoveDocumentSection(ctx, id, user.ID)
	switch {
	case errors.Is(err, store.ErrSectionNotEmpty):
		d.render(w, r, views.DocumentosPage{
			Errors: map[string]string{views.SectionField(id, ""): "No se puede eliminar una sección que tiene documentos. Primero muévelos a otra sección o elimínalos."},
			Open:   id,
		}, http.StatusConflict)
	case errors.Is(err, store.ErrNoSection):
		http.NotFound(w, r)
	case err != nil:
		http.Error(w, "error interno", http.StatusInternalServerError)
	default:
		http.Redirect(w, r, documentosAt("seccion-eliminada", 0), http.StatusSeeOther)
	}
}

// Subir handles POST /documentos (admins): "archivo" (PDF or image, up to 25 MB), "titulo"
// (the file's name without its extension when blank), and the section: "seccion_nueva"
// when typed, else the picked "seccion".
func (d *Documentos) Subir(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// A little over the cap, for the form's other fields and the multipart framing.
	r.Body = http.MaxBytesReader(w, r.Body, maxDocumentBytes+1<<20)
	if err := r.ParseMultipartForm(maxDocumentBytes + 1<<20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		http.Error(w, "el archivo no puede pesar más de 25 MB", http.StatusRequestEntityTooLarge)
		return
	}
	values := views.DocumentoFormValues{
		Title:      strings.TrimSpace(r.FormValue("titulo")),
		Section:    r.FormValue("seccion"),
		NewSection: strings.TrimSpace(r.FormValue("seccion_nueva")),
	}
	errs := map[string]string{}
	sectionID, _ := strconv.ParseInt(values.Section, 10, 64)
	if values.NewSection == "" && sectionID == 0 {
		errs["seccion"] = "Elige una sección o escribe el nombre de una nueva."
	}
	if utf8.RuneCountInString(values.NewSection) > 100 {
		errs["seccion"] = "El nombre de la sección no puede pasar de 100 caracteres."
	}
	if utf8.RuneCountInString(values.Title) > 200 {
		errs["titulo"] = "El título no puede pasar de 200 caracteres."
	}
	file, msg, _ := uploadedFile(r, "archivo", maxDocumentBytes, documentFileTypes,
		"El archivo no puede pesar más de 25 MB.", "El archivo debe ser PDF o una imagen (JPG, PNG, WebP o GIF).", "documento")
	switch {
	case msg != "":
		errs["archivo"] = strings.ToUpper(msg[:1]) + msg[1:]
		if !strings.HasSuffix(errs["archivo"], ".") {
			errs["archivo"] += "."
		}
	case file == nil:
		errs["archivo"] = "Elige el archivo: PDF o imagen de hasta 25 MB."
	}
	if len(errs) > 0 {
		d.render(w, r, views.DocumentosPage{Upload: values, Errors: errs}, http.StatusBadRequest)
		return
	}
	title := values.Title
	if title == "" {
		title = strings.TrimSuffix(file.Filename, path.Ext(file.Filename))
		if strings.TrimSpace(title) == "" {
			title = file.Filename
		}
	}
	user, _ := UserFromContext(ctx)
	_, inSection, err := d.store.AddOfficialDocument(ctx, sectionID, values.NewSection, title, *file, user.ID)
	if err != nil {
		if errors.Is(err, store.ErrBadDocument) {
			d.render(w, r, views.DocumentosPage{Upload: values, Errors: map[string]string{"seccion": "Esa sección ya no existe; elige otra."}}, http.StatusBadRequest)
			return
		}
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, documentosAt("subido", inSection), http.StatusSeeOther)
}

// Eliminar handles POST /documentos/{id}/eliminar (admins).
func (d *Documentos) Eliminar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	user, _ := UserFromContext(ctx)
	sectionID, err := d.store.RemoveOfficialDocument(ctx, id, user.ID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if sectionID == 0 {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, documentosAt("eliminado", sectionID), http.StatusSeeOther)
}

// Archivo handles GET /documentos/{id}: opens the document in the browser (so a PDF or
// a picture can be read on a phone), or downloads it with ?descargar=1. Opened inline it
// is served sandboxed, with the type detected at upload and no sniffing, so it can't run
// as part of the app.
func (d *Documentos) Archivo(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	doc, data, err := d.store.OfficialDocumentData(r.Context(), id)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if doc == nil {
		http.NotFound(w, r)
		return
	}
	serveUploadedFile(w, r, doc.Filename, doc.ContentType, data)
}

// uploadCSP sandboxes an uploaded file shown inline, and replaces securityHeaders' policy
// for it, so it repeats frame-ancestors.
const uploadCSP = "sandbox; frame-ancestors 'none'"

// serveUploadedFile sends an uploaded file of the given type (detected at upload) inline
// or, with ?descargar=1, as a download. Inline it is sandboxed and never sniffed, so it
// can't run as part of the app.
func serveUploadedFile(w http.ResponseWriter, r *http.Request, filename, contentType string, data []byte) {
	disposition := "inline"
	if r.URL.Query().Get("descargar") != "" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": filename}))
	w.Header().Set("Content-Security-Policy", uploadCSP)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
}
