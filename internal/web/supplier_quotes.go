package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// The cotización del proveedor of a QL quote (issue #39): the supplier's data and files,
// kept per revision chain (see store.SupplierQuote) and shown on the quote and on its
// proyecto. Whoever can edit the quote can add to it, which is anyone signed in.

// maxSupplierFileBytes caps one supplier quote file.
const maxSupplierFileBytes = 10 << 20

// maxSupplierRequestBytes caps one upload of several files: what nginx lets through.
const maxSupplierRequestBytes = 30 << 20

// supplierFileTypes are the kinds of file a supplier quote may be, by what the bytes are.
var supplierFileTypes = map[string]bool{"application/pdf": true, "image/jpeg": true, "image/png": true}

// supplierMessages and supplierErrors are the outcomes of a post, by the code the
// redirect carries; "%s" is the file's name.
var (
	supplierMessages = map[string]string{
		"guardado":  "Cotización del proveedor guardada.",
		"eliminado": "Archivo eliminado.",
	}
	supplierErrors = map[string]string{
		"tipo":      "No se subió ningún archivo: «%s» no es PDF, JPG ni PNG.",
		"tamano":    "No se subió ningún archivo: «%s» pesa más de 10 MB, el máximo por archivo.",
		"vacio":     "No se subió ningún archivo: «%s» está vacío.",
		"total":     "No se subió ningún archivo: juntos pesan más de 30 MB. Súbelos en partes.",
		"bloqueado": "Ese archivo ya no se puede eliminar: es parte de una cotización emitida.",
	}
)

// supplierQuoteView loads the section for a quote of the chain, or an empty view (no
// section) when quote isn't QL. filesBase and action are as in views.SupplierQuoteView.
func supplierQuoteView(r *http.Request, s *store.Store, quote *store.Quote, filesBase, action string) (views.SupplierQuoteView, error) {
	if quote == nil || !quote.SeriesFreeLines {
		return views.SupplierQuoteView{}, nil
	}
	sq, err := s.SupplierQuoteOf(r.Context(), quote.ID)
	if err != nil {
		return views.SupplierQuoteView{}, err
	}
	v := views.SupplierQuoteView{Quote: sq, FilesBase: filesBase, Action: action}
	query := r.URL.Query()
	v.Message = supplierMessages[query.Get("proveedor")]
	if msg := supplierErrors[query.Get("proveedor_error")]; msg != "" {
		v.Error = strings.Replace(msg, "%s", query.Get("archivo"), 1)
	}
	return v, nil
}

// supplierRedirect sends the browser back to page's section with the outcome.
func supplierRedirect(w http.ResponseWriter, r *http.Request, page, param, code, filename string) {
	v := url.Values{param: {code}}
	if filename != "" {
		v.Set("archivo", filename)
	}
	http.Redirect(w, r, page+"?"+v.Encode()+"#cotizacion-proveedor", http.StatusSeeOther)
}

// addSupplierFiles reads the form's "archivos" and attaches them to quoteID's chain, all
// or none. It returns the error code (and the file it is about) when nothing was added,
// "" when they were or none was sent.
func addSupplierFiles(r *http.Request, s *store.Store, quoteID, userID int64) (code, filename string, sent bool, err error) {
	if r.MultipartForm == nil {
		return "", "", false, nil
	}
	headers := r.MultipartForm.File["archivos"]
	if len(headers) == 0 {
		return "", "", false, nil
	}
	files := make([]store.NewFile, 0, len(headers))
	for _, h := range headers {
		// readUploadedFile's messages for a file too large or of another kind are the
		// codes themselves.
		f, msg, _ := readUploadedFile(h, maxSupplierFileBytes, supplierFileTypes, "tamano", "tipo", "cotizacion-proveedor")
		switch msg {
		case "":
		case "tamano", "tipo":
			return msg, h.Filename, true, nil
		case "el archivo está vacío":
			return "vacio", h.Filename, true, nil
		default:
			return "", "", true, errors.New(msg)
		}
		files = append(files, *f)
	}
	return "", "", true, s.AddSupplierQuoteFiles(r.Context(), quoteID, files, userID)
}

// parseSupplierForm reads a multipart post of at most maxSupplierRequestBytes. It
// reports false, having redirected to page with the error, when the post is too large.
func parseSupplierForm(w http.ResponseWriter, r *http.Request, page string) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxSupplierRequestBytes)
	if err := r.ParseMultipartForm(maxSupplierRequestBytes); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			supplierRedirect(w, r, page, "proveedor_error", "total", "")
			return false
		}
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return false
	}
	return true
}

// Proveedor handles POST /cotizaciones/{folio}/proveedor: on any quote of the chain it
// saves "proveedor" and "numero" and adds the files in "archivos" (PDF, JPG or PNG, 10 MB
// each). A refused file leaves out all of them, but the supplier's data is still saved.
func (q *Quotes) Proveedor(w http.ResponseWriter, r *http.Request) {
	quote := q.loadQuoteOrNotFound(w, r)
	if quote == nil {
		return
	}
	if !quote.SeriesFreeLines {
		http.NotFound(w, r)
		return
	}
	page := "/cotizaciones/" + quote.Folio
	if !parseSupplierForm(w, r, page) {
		return
	}
	saveSupplierQuote(w, r, q.store, quote.ID, page)
}

// saveSupplierQuote saves the posted supplier data and files onto quoteID's chain and
// redirects back to page with the outcome.
func saveSupplierQuote(w http.ResponseWriter, r *http.Request, s *store.Store, quoteID int64, page string) {
	ctx := r.Context()
	user, _ := UserFromContext(ctx)
	supplier := truncateRunes(r.FormValue("proveedor"), 200)
	number := truncateRunes(r.FormValue("numero"), 100)
	if err := s.SetSupplierQuoteInfo(ctx, quoteID, supplier, number, user.ID); err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	code, filename, _, err := addSupplierFiles(r, s, quoteID, user.ID)
	switch {
	case err != nil:
		http.Error(w, "error interno", http.StatusInternalServerError)
	case code != "":
		supplierRedirect(w, r, page, "proveedor_error", code, filename)
	default:
		supplierRedirect(w, r, page, "proveedor", "guardado", "")
	}
}

// ProveedorArchivo handles GET /cotizaciones/{folio}/proveedor/{id}: one of the chain's
// supplier quote files, opened in the browser or, with ?descargar=1, downloaded.
func (q *Quotes) ProveedorArchivo(w http.ResponseWriter, r *http.Request) {
	quote := q.loadQuoteOrNotFound(w, r)
	if quote == nil {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	file, data, err := q.store.SupplierQuoteFileData(r.Context(), quote.ID, id)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	if file == nil {
		http.NotFound(w, r)
		return
	}
	serveUploadedFile(w, r, file.Filename, file.ContentType, data)
}

// ProveedorEliminar handles POST /cotizaciones/{folio}/proveedor/{id}/eliminar. It is
// refused for a file an issued quote was based on, or once the chain's current quote
// isn't a borrador.
func (q *Quotes) ProveedorEliminar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	quote := q.loadQuoteOrNotFound(w, r)
	if quote == nil {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	user, _ := UserFromContext(ctx)
	found, err := q.store.RemoveSupplierQuoteFile(ctx, quote.ID, id, user.ID)
	page := "/cotizaciones/" + quote.Folio
	switch {
	case errors.Is(err, store.ErrSupplierFileLocked):
		supplierRedirect(w, r, page, "proveedor_error", "bloqueado", "")
	case err != nil:
		http.Error(w, "error interno", http.StatusInternalServerError)
	case !found:
		http.NotFound(w, r)
	default:
		supplierRedirect(w, r, page, "proveedor", "eliminado", "")
	}
}

// Proveedor handles POST /proyectos/{folio}/proveedor: saves the cotización del
// proveedor of a QL proyecto's quotes, as on the quote.
func (p *Projects) Proveedor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := p.loadProjectOrNotFound(w, r)
	if project == nil {
		return
	}
	quote, err := p.store.QuoteByID(ctx, project.CurrentQuoteID)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	if quote == nil || !quote.SeriesFreeLines {
		http.NotFound(w, r)
		return
	}
	page := "/proyectos/" + project.Folio
	if !parseSupplierForm(w, r, page) {
		return
	}
	saveSupplierQuote(w, r, p.store, quote.ID, page)
}

// truncateRunes trims s and cuts it to at most n characters.
func truncateRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
