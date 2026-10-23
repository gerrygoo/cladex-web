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
// official documents; only admins upload or remove them (the routes wrap those two in
// RequireAdmin).
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

// List handles GET /documentos.
func (d *Documentos) List(w http.ResponseWriter, r *http.Request) {
	successMsg := ""
	switch r.URL.Query().Get("guardado") {
	case "subido":
		successMsg = "Documento subido."
	case "eliminado":
		successMsg = "Documento eliminado."
	}
	d.render(w, r, views.DocumentoFormValues{}, nil, successMsg, http.StatusOK)
}

func (d *Documentos) render(w http.ResponseWriter, r *http.Request, values views.DocumentoFormValues, fieldErrors map[string]string, successMsg string, status int) {
	ctx := r.Context()
	sections, err := d.store.ListDocumentSections(ctx)
	if err != nil {
		serverError(w, r, err)
		return
	}
	user, _ := UserFromContext(ctx)
	w.WriteHeader(status)
	views.Documentos(sections, values, fieldErrors, successMsg, navUserView(user)).Render(ctx, w)
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
		d.render(w, r, values, errs, "", http.StatusBadRequest)
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
	if _, err := d.store.AddOfficialDocument(ctx, sectionID, values.NewSection, title, *file, user.ID); err != nil {
		if errors.Is(err, store.ErrBadDocument) {
			d.render(w, r, values, map[string]string{"seccion": "Esa sección ya no existe; elige otra."}, "", http.StatusBadRequest)
			return
		}
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/documentos?guardado=subido", http.StatusSeeOther)
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
	ok, err := d.store.RemoveOfficialDocument(ctx, id, user.ID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/documentos?guardado=eliminado", http.StatusSeeOther)
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
