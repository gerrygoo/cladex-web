package web

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// postDocumento sends the upload form as the browser does, multipart, through
// RequireAdmin as the router wires it, with a file when filename isn't empty.
func postDocumento(t *testing.T, a *Auth, d *Documentos, userID int64, fields map[string]string, filename string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("WriteField: %v", err)
		}
	}
	if filename != "" {
		fw, err := mw.CreateFormFile("archivo", filename)
		if err != nil {
			t.Fatalf("CreateFormFile: %v", err)
		}
		fw.Write(data)
	}
	mw.Close()
	req := httptest.NewRequest("POST", "/documentos", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec := httptest.NewRecorder()
	a.RequireAuth(a.RequireAdmin(http.HandlerFunc(d.Subir))).ServeHTTP(rec, req)
	return rec
}

func TestDocumentosOficiales(t *testing.T) {
	a := newTestAuth(t)
	d := NewDocumentos(a.store)
	admin := createTestUser(t, a, "gerry", "admin", "hunter2")
	vendedor := createTestUser(t, a, "emilio", "vendedor", "hunter2")
	ctx := context.Background()
	sections, err := a.store.ListDocumentSections(ctx)
	if err != nil || len(sections) != 2 {
		t.Fatalf("ListDocumentSections = %+v, %v", sections, err)
	}
	comerciales := sections[0].ID
	pdf := []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n")
	page := func(userID int64) string {
		return doForm(t, a, userID, d.List, "GET", "/documentos", nil, nil, false).Body.String()
	}

	// Both starting sections show while empty; a vendedor gets no upload form.
	body := page(vendedor)
	for _, want := range []string{"Documentos comerciales", "Educación", "Todavía no hay documentos.", `<a href="/documentos">Documentos oficiales</a>`} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
	if strings.Contains(body, `action="/documentos"`) {
		t.Error("a vendedor sees the upload form")
	}
	// An admin gets it folded behind "Subir documento" until they open it.
	if adminBody := page(admin); !strings.Contains(adminBody, `<details class="disclosure" id="subir"><summary>Subir documento</summary>`) {
		t.Error("an admin doesn't see the folded upload form")
	}

	// Only admins upload.
	fields := map[string]string{"titulo": "Ficha técnica Copperclad (CCS)", "seccion": itoa(comerciales)}
	if rec := postDocumento(t, a, d, vendedor, fields, "ficha.pdf", pdf); rec.Code != http.StatusForbidden {
		t.Fatalf("vendedor upload status = %d, want 403", rec.Code)
	}
	// The type comes from the bytes: a script named .pdf is refused, and so is no file.
	if rec := postDocumento(t, a, d, admin, fields, "ficha.pdf", []byte("<script>alert(1)</script>")); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "El archivo debe ser PDF o una imagen") {
		t.Fatalf("html upload = %d: %s", rec.Code, rec.Body.String())
	} else if !strings.Contains(rec.Body.String(), `<details class="disclosure" id="subir" open>`) {
		t.Error("a refused upload comes back with the form folded")
	}
	if rec := postDocumento(t, a, d, admin, fields, "", nil); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Elige el archivo") {
		t.Fatalf("upload without a file = %d", rec.Code)
	}
	if rec := postDocumento(t, a, d, admin, fields, "ficha.pdf", pdf); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/documentos?guardado=subido" {
		t.Fatalf("admin upload = %d %q: %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	// A new section by name, and the title from the file name when left blank.
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	if rec := postDocumento(t, a, d, admin, map[string]string{"seccion": itoa(comerciales), "seccion_nueva": "Logos"}, "isotipo cladex.png", png); rec.Code != http.StatusSeeOther {
		t.Fatalf("upload to a new section = %d: %s", rec.Code, rec.Body.String())
	}

	sections, _ = a.store.ListDocumentSections(ctx)
	if len(sections) != 3 || sections[2].Name != "Logos" || len(sections[2].Documents) != 1 || sections[2].Documents[0].Title != "isotipo cladex" {
		t.Fatalf("sections = %+v", sections)
	}
	doc := sections[0].Documents[0]
	body = page(vendedor)
	for _, want := range []string{"Ficha técnica Copperclad (CCS)", "/documentos/" + itoa(doc.ID) + "?descargar=1", "PDF · 1 KB", "isotipo cladex"} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
	if strings.Contains(body, "/eliminar") {
		t.Error("a vendedor sees the remove button")
	}

	// Opening it: inline and sandboxed; ?descargar=1 downloads it.
	pv := map[string]string{"id": itoa(doc.ID)}
	rec := doForm(t, a, vendedor, d.Archivo, "GET", "/documentos/"+itoa(doc.ID), pv, nil, false)
	if rec.Code != http.StatusOK || rec.Body.String() != string(pdf) || rec.Header().Get("Content-Type") != "application/pdf" ||
		!strings.HasPrefix(rec.Header().Get("Content-Disposition"), "inline") || rec.Header().Get("Content-Security-Policy") != uploadCSP {
		t.Fatalf("Archivo = %d %v", rec.Code, rec.Header())
	}
	rec = doForm(t, a, vendedor, d.Archivo, "GET", "/documentos/"+itoa(doc.ID)+"?descargar=1", pv, nil, false)
	if !strings.HasPrefix(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Errorf("descargar Content-Disposition = %q", rec.Header().Get("Content-Disposition"))
	}

	// Only admins remove; then it's gone for everyone.
	remove := func(userID int64) int {
		return doForm(t, a, userID, a.RequireAdmin(http.HandlerFunc(d.Eliminar)).ServeHTTP, "POST", "/documentos/"+itoa(doc.ID)+"/eliminar", pv, nil, false).Code
	}
	if code := remove(vendedor); code != http.StatusForbidden {
		t.Fatalf("vendedor remove = %d, want 403", code)
	}
	if code := remove(admin); code != http.StatusSeeOther {
		t.Fatalf("admin remove = %d, want 303", code)
	}
	if code := remove(admin); code != http.StatusNotFound {
		t.Errorf("second remove = %d, want 404", code)
	}
	if rec := doForm(t, a, vendedor, d.Archivo, "GET", "/documentos/"+itoa(doc.ID), pv, nil, false); rec.Code != http.StatusNotFound {
		t.Errorf("removed document still served: %d", rec.Code)
	}
}
