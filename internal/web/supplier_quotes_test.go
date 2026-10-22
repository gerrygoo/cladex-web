package web

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

type testUpload struct {
	name string
	data []byte
}

var (
	testPDF = []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n")
	testPNG = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x02\x00\x00\x00")
)

// postSupplier sends the Cotización del proveedor form as the browser does, multipart,
// with every file under "archivos".
func postSupplier(t *testing.T, a *Auth, userID int64, handler http.HandlerFunc, path, folio string, fields map[string]string, files ...testUpload) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	for _, f := range files {
		fw, err := mw.CreateFormFile("archivos", f.name)
		if err != nil {
			t.Fatalf("CreateFormFile: %v", err)
		}
		fw.Write(f.data)
	}
	mw.Close()
	req := httptest.NewRequest("POST", path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.SetPathValue("folio", folio)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec := httptest.NewRecorder()
	a.RequireAuth(handler).ServeHTTP(rec, req)
	return rec
}

// qlIssueForm is a complete QL line set, with what QL asks for before issuing.
func qlIssueForm() url.Values {
	return url.Values{
		"line_keys":             {"0"},
		"lines[0][kind]":        {"free"},
		"lines[0][description]": {"Switch de 24 puertos"},
		"lines[0][cost]":        {"1500"},
		"lines[0][margin]":      {"10"},
		"lines[0][qty]":         {"1"},
		"delivery_time":         {"5 días"},
		"currency":              {"MXN"},
		"valid_until":           {time.Now().AddDate(0, 0, 10).Format("2006-01-02")},
	}
}

// TestSupplierQuote covers issue #39: a QL quote's cotización del proveedor is uploaded,
// shown and served, its files can be removed only while nothing issued was based on
// them, revisions and the proyecto see it, and it never reaches the client's PDF.
func TestSupplierQuote(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	p := NewProjects(a.store)
	ctx := context.Background()
	ana := createTestUser(t, a, "ana", "vendedor", "hunter2")
	customerID, _, _ := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, ana)

	ql, err := a.store.CreateDraftQuote(ctx, customerID, ana, "QL")
	if err != nil {
		t.Fatal(err)
	}
	folio := ql.Folio
	pv := map[string]string{"folio": folio}
	base := "/cotizaciones/" + folio
	page := func(f string, query string) string {
		return doForm(t, a, ana, q.Builder, "GET", "/cotizaciones/"+f+query, map[string]string{"folio": f}, nil, false).Body.String()
	}

	// The section shows, empty, on a QL draft only.
	body := page(folio, "")
	for _, want := range []string{"Cotización del proveedor", "Todavía no hay archivos.", `name="proveedor"`, `name="numero"`, `form="proveedor-form"`, `enctype="multipart/form-data"`} {
		if !strings.Contains(body, want) {
			t.Errorf("QL draft page missing %q", want)
		}
	}
	qa, _ := a.store.CreateDraftQuote(ctx, customerID, ana, "QA")
	if strings.Contains(page(qa.Folio, ""), "Cotización del proveedor") {
		t.Error("a QA quote shows the supplier quote section")
	}
	if rec := postSupplier(t, a, ana, q.Proveedor, "/cotizaciones/"+qa.Folio+"/proveedor", qa.Folio, nil, testUpload{"c.pdf", testPDF}); rec.Code != http.StatusNotFound {
		t.Errorf("upload to QA = %d, want 404", rec.Code)
	}

	// A PDF and an image together, with the supplier and the number.
	fields := map[string]string{"proveedor": " Ingram Micro ", "numero": "COT-7781"}
	rec := postSupplier(t, a, ana, q.Proveedor, base+"/proveedor", folio, fields, testUpload{"cotizacion.pdf", testPDF}, testUpload{"foto.png", testPNG})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != base+"?proveedor=guardado#cotizacion-proveedor" {
		t.Fatalf("upload = %d %q; body = %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	sq, err := a.store.SupplierQuoteOf(ctx, ql.ID)
	if err != nil || sq.Supplier != "Ingram Micro" || sq.Number != "COT-7781" || len(sq.Files) != 2 {
		t.Fatalf("supplier quote = %+v, %v", sq, err)
	}
	if sq.Files[0].ContentType != "application/pdf" || sq.Files[1].ContentType != "image/png" || sq.Files[0].UploadedByName == "" || !sq.Files[0].Removable {
		t.Fatalf("files = %+v", sq.Files)
	}
	body = page(folio, "?proveedor=guardado")
	for _, want := range []string{"Cotización del proveedor guardada.", "cotizacion.pdf", "foto.png", `value="Ingram Micro"`, `value="COT-7781"`, "Eliminar"} {
		if !strings.Contains(body, want) {
			t.Errorf("page after upload missing %q", want)
		}
	}

	// Open inline, sandboxed, or download.
	pdfID := strconv.FormatInt(sq.Files[0].ID, 10)
	pngID := strconv.FormatInt(sq.Files[1].ID, 10)
	serve := func(f, id, query string) *httptest.ResponseRecorder {
		return doForm(t, a, ana, q.ProveedorArchivo, "GET", "/cotizaciones/"+f+"/proveedor/"+id+query, map[string]string{"folio": f, "id": id}, nil, false)
	}
	rec = serve(folio, pdfID, "")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/pdf" || rec.Header().Get("Content-Security-Policy") != uploadCSP ||
		!strings.HasPrefix(rec.Header().Get("Content-Disposition"), "inline") || !bytes.Equal(rec.Body.Bytes(), testPDF) {
		t.Fatalf("open = %d %v", rec.Code, rec.Header())
	}
	if rec = serve(folio, pngID, "?descargar=1"); !strings.HasPrefix(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("download disposition = %q", rec.Header().Get("Content-Disposition"))
	}
	if rec = serve(qa.Folio, pdfID, ""); rec.Code != http.StatusNotFound {
		t.Errorf("another chain's file = %d, want 404", rec.Code)
	}

	// Wrong kind and too large are refused with a message, and nothing is added.
	refused := func(file testUpload, wantMsg string) {
		t.Helper()
		rec := postSupplier(t, a, ana, q.Proveedor, base+"/proveedor", folio, nil, testUpload{"buena.pdf", testPDF}, file)
		loc := rec.Header().Get("Location")
		if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "proveedor_error=") {
			t.Fatalf("upload of %s = %d %q", file.name, rec.Code, loc)
		}
		u, _ := url.Parse(loc)
		if body := page(folio, "?"+u.RawQuery); !strings.Contains(body, wantMsg) {
			t.Errorf("message for %s missing %q", file.name, wantMsg)
		}
		if sq, _ := a.store.SupplierQuoteOf(ctx, ql.ID); len(sq.Files) != 2 {
			t.Errorf("after refusing %s there are %d files", file.name, len(sq.Files))
		}
	}
	refused(testUpload{"cotizacion.docx", []byte("PK\x03\x04 word")}, "No se subió ningún archivo: «cotizacion.docx» no es PDF, JPG ni PNG.")
	big := append(append([]byte{}, testPDF...), make([]byte, maxSupplierFileBytes)...)
	refused(testUpload{"enorme.pdf", big}, "No se subió ningún archivo: «enorme.pdf» pesa más de 10 MB, el máximo por archivo.")

	// While the draft has never been issued, a file can go.
	del := func(f, id string) *httptest.ResponseRecorder {
		return doForm(t, a, ana, q.ProveedorEliminar, "POST", "/cotizaciones/"+f+"/proveedor/"+id+"/eliminar", map[string]string{"folio": f, "id": id}, url.Values{}, false)
	}
	if rec = del(folio, pngID); rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "proveedor=eliminado") {
		t.Fatalf("delete in draft = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec = serve(folio, pngID, ""); rec.Code != http.StatusNotFound {
		t.Errorf("removed file still served: %d", rec.Code)
	}

	// Issuing doesn't depend on the section, and the client's PDF has none of it.
	if rec = doForm(t, a, ana, q.Emitir, "POST", base+"/emitir", pv, qlIssueForm(), false); rec.Code != http.StatusSeeOther {
		t.Fatalf("Emitir = %d; body = %s", rec.Code, rec.Body.String())
	}
	issued, _ := a.store.QuoteByID(ctx, ql.ID)
	lines, _ := a.store.ListQuoteLines(ctx, ql.ID)
	doc, err := issuedQuoteDocument(*issued, lines)
	if err != nil {
		t.Fatal(err)
	}
	docText := fmt.Sprintf("%+v", doc)
	for _, secret := range []string{"Ingram", "COT-7781", "cotizacion.pdf", "proveedor"} {
		if strings.Contains(docText, secret) {
			t.Errorf("the client's PDF data includes %q", secret)
		}
	}
	if rec = doForm(t, a, ana, q.PDF, "GET", base+"/pdf", pv, nil, false); rec.Code != 200 || bytes.Contains(rec.Body.Bytes(), []byte("Ingram")) {
		t.Errorf("PDF = %d, or it names the supplier", rec.Code)
	}

	// Once issued, the file it was based on is locked, but more can be added and the
	// data can still be edited.
	if rec = del(folio, pdfID); rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "proveedor_error=bloqueado") {
		t.Fatalf("delete after issue = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	body = page(folio, "")
	if !strings.Contains(body, `name="proveedor"`) || !strings.Contains(body, "Ingram Micro") || strings.Contains(body, "/proveedor/"+pdfID+"/eliminar") {
		t.Error("issued quote page should keep the data editable and the issued file locked")
	}
	if rec = postSupplier(t, a, ana, q.Proveedor, base+"/proveedor", folio, map[string]string{"proveedor": "Ingram Micro México", "numero": "COT-7781"}, testUpload{"anexo.png", testPNG}); rec.Code != http.StatusSeeOther {
		t.Fatalf("upload after issue = %d", rec.Code)
	}
	if sq, _ := a.store.SupplierQuoteOf(ctx, ql.ID); len(sq.Files) != 2 || sq.Supplier != "Ingram Micro México" {
		t.Fatalf("after issue: %+v", sq)
	}

	// The proyecto shows the same section and takes more files.
	projPage := func(query string) string {
		return doForm(t, a, ana, p.Page, "GET", "/proyectos/"+folio+query, pv, nil, false).Body.String()
	}
	body = projPage("")
	for _, want := range []string{"Cotización del proveedor", "Ingram Micro México", "COT-7781", "cotizacion.pdf", "anexo.png", `name="proveedor"`, `action="/proyectos/` + folio + `/proveedor"`} {
		if !strings.Contains(body, want) {
			t.Errorf("proyecto page missing %q", want)
		}
	}
	if strings.Contains(body, "/proveedor/"+pdfID+"/eliminar") {
		t.Error("proyecto page lets a file be removed")
	}
	if rec = postSupplier(t, a, ana, p.Proveedor, "/proyectos/"+folio+"/proveedor", folio, map[string]string{"proveedor": "Ingram Micro México", "numero": "COT-7790"}); rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "proveedor=guardado") {
		t.Errorf("proyecto data-only save = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if sq, _ := a.store.SupplierQuoteOf(ctx, ql.ID); sq.Number != "COT-7790" {
		t.Errorf("number after saving from the proyecto = %q", sq.Number)
	}
	if rec = postSupplier(t, a, ana, p.Proveedor, "/proyectos/"+folio+"/proveedor", folio, map[string]string{"proveedor": "Ingram Micro México", "numero": "COT-7790"}, testUpload{"desde-proyecto.pdf", testPDF}); rec.Code != http.StatusSeeOther {
		t.Fatalf("proyecto upload = %d; body = %s", rec.Code, rec.Body.String())
	}

	// A revision sees everything without copying, and edits the shared data.
	rev, err := a.store.CreateRevision(ctx, ql.ID, ana)
	if err != nil {
		t.Fatal(err)
	}
	sq, _ = a.store.SupplierQuoteOf(ctx, rev.ID)
	if sq.RootQuoteID != ql.ID || sq.Supplier != "Ingram Micro México" || len(sq.Files) != 3 {
		t.Fatalf("revision's supplier quote = %+v", sq)
	}
	for _, f := range sq.Files {
		if f.Removable && f.Filename == "cotizacion.pdf" {
			t.Error("the issued quote's file is removable on the revision")
		}
	}
	body = page(rev.Folio, "")
	if !strings.Contains(body, "cotizacion.pdf") || !strings.Contains(body, `value="Ingram Micro México"`) {
		t.Error("revision page doesn't show the chain's supplier quote")
	}
	revBase := "/cotizaciones/" + rev.Folio
	if rec = postSupplier(t, a, ana, q.Proveedor, revBase+"/proveedor", rev.Folio, map[string]string{"proveedor": "Ingram Micro México", "numero": "COT-7781-B"}, testUpload{"nueva.pdf", testPDF}); rec.Code != http.StatusSeeOther {
		t.Fatalf("revision upload = %d", rec.Code)
	}
	sq, _ = a.store.SupplierQuoteOf(ctx, ql.ID)
	if sq.Supplier != "Ingram Micro México" || len(sq.Files) != 4 {
		t.Fatalf("chain after revision edit = %+v", sq)
	}
	if rec = del(rev.Folio, pdfID); !strings.Contains(rec.Header().Get("Location"), "proveedor_error=bloqueado") {
		t.Errorf("deleting the issued file from the revision = %q", rec.Header().Get("Location"))
	}
}

// TestSupplierQuoteOptional: a QL quote issues with no supplier quote at all.
func TestSupplierQuoteOptional(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	ctx := context.Background()
	ana := createTestUser(t, a, "ana", "vendedor", "hunter2")
	customerID, _, _ := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, ana)
	ql, _ := a.store.CreateDraftQuote(ctx, customerID, ana, "QL")
	pv := map[string]string{"folio": ql.Folio}
	if rec := doForm(t, a, ana, q.Emitir, "POST", "/cotizaciones/"+ql.Folio+"/emitir", pv, qlIssueForm(), false); rec.Code != http.StatusSeeOther {
		t.Fatalf("Emitir = %d; body = %s", rec.Code, rec.Body.String())
	}
	if got, _ := a.store.QuoteByID(ctx, ql.ID); got.Status != "emitida" {
		t.Fatalf("status = %q", got.Status)
	}
	body := doForm(t, a, ana, q.Builder, "GET", "/cotizaciones/"+ql.Folio, pv, nil, false).Body.String()
	if !strings.Contains(body, "Todavía no hay archivos.") || !strings.Contains(body, `name="proveedor"`) {
		t.Error("issued quote without a supplier quote should show the empty section")
	}
}

// TestSupplierQuoteRequestTooLarge: a post over what nginx lets through is refused with
// a message rather than an error page.
func TestSupplierQuoteRequestTooLarge(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	ana := createTestUser(t, a, "ana", "vendedor", "hunter2")
	customerID, _, _ := seedQuoteBuilderFixtures(t, a)
	ql, _ := a.store.CreateDraftQuote(context.Background(), customerID, ana, "QL")
	chunk := append(append([]byte{}, testPDF...), make([]byte, 9<<20)...)
	rec := postSupplier(t, a, ana, q.Proveedor, "/cotizaciones/"+ql.Folio+"/proveedor", ql.Folio, nil,
		testUpload{"a.pdf", chunk}, testUpload{"b.pdf", chunk}, testUpload{"c.pdf", chunk}, testUpload{"d.pdf", chunk})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "proveedor_error=total") {
		t.Fatalf("40 MB post = %d %q", rec.Code, rec.Header().Get("Location"))
	}
}
