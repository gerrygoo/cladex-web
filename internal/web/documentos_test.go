package web

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	// Back to the page with the section it went to unfolded.
	wantAt := "/documentos?guardado=subido&seccion=" + itoa(comerciales) + "#seccion-" + itoa(comerciales)
	if rec := postDocumento(t, a, d, admin, fields, "ficha.pdf", pdf); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != wantAt {
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
		!strings.HasPrefix(rec.Header().Get("Content-Disposition"), "inline") || rec.Header().Get("Content-Security-Policy") != "sandbox" {
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

// Issue #44: sections fold to their name and count; admins rename sections, edit and
// move documents, and remove empty sections.
func TestDocumentosEditar(t *testing.T) {
	a := newTestAuth(t)
	d := NewDocumentos(a.store)
	admin := createTestUser(t, a, "gerry", "admin", "hunter2")
	vendedor := createTestUser(t, a, "emilio", "vendedor", "hunter2")
	ctx := context.Background()
	sections, _ := a.store.ListDocumentSections(ctx)
	comerciales, educacion := sections[0].ID, sections[1].ID
	pdf := []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n")
	for _, title := range []string{"Ficha CCS", "Ficha CCA"} {
		if rec := postDocumento(t, a, d, admin, map[string]string{"titulo": title, "seccion": itoa(comerciales)}, "ficha.pdf", pdf); rec.Code != http.StatusSeeOther {
			t.Fatalf("upload = %d: %s", rec.Code, rec.Body.String())
		}
	}
	sections, _ = a.store.ListDocumentSections(ctx)
	ccs := sections[0].Documents[1] // by title: CCA, CCS
	page := func(userID int64, path string) string {
		return doForm(t, a, userID, d.List, "GET", path, nil, nil, false).Body.String()
	}
	post := func(userID int64, h http.HandlerFunc, path string, pv map[string]string, form url.Values) *httptest.ResponseRecorder {
		return doForm(t, a, userID, a.RequireAdmin(h).ServeHTTP, "POST", path, pv, form, false)
	}
	comercialesOpen := `<details class="document-section" id="seccion-` + itoa(comerciales) + `" open>`

	// Folded by default: each section's name with its count; a vendedor sees no edit forms.
	body := page(vendedor, "/documentos")
	for _, want := range []string{
		`<details class="document-section" id="seccion-` + itoa(comerciales) + `"><summary><h2>Documentos comerciales <span class="document-count">(2)</span></h2></summary>`,
		`<details class="document-section" id="seccion-` + itoa(educacion) + `"><summary><h2>Educación <span class="document-count">(0)</span></h2></summary>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
	for _, unwanted := range []string{"/editar", "Editar sección", "/secciones/"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("a vendedor sees %q", unwanted)
		}
	}
	// Reached after an action, that section comes back open; an admin gets the forms, and
	// a remove button only on the empty section.
	body = page(admin, "/documentos?guardado=subido&seccion="+itoa(comerciales))
	if !strings.Contains(body, comercialesOpen) || !strings.Contains(body, "Documento subido.") {
		t.Error("the section just uploaded to isn't open")
	}
	for _, want := range []string{
		`action="/documentos/` + itoa(ccs.ID) + `/editar"`,
		`action="/documentos/secciones/` + itoa(comerciales) + `/editar"`,
		`action="/documentos/secciones/` + itoa(educacion) + `/eliminar"`,
		"primero mueve sus documentos",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("admin page missing %q", want)
		}
	}
	if strings.Contains(body, `action="/documentos/secciones/`+itoa(comerciales)+`/eliminar"`) {
		t.Error("a section with documents offers a remove button")
	}

	// Only admins edit, rename or remove.
	docPV := map[string]string{"id": itoa(ccs.ID)}
	comercialesPV := map[string]string{"id": itoa(comerciales)}
	educacionPV := map[string]string{"id": itoa(educacion)}
	for name, rec := range map[string]*httptest.ResponseRecorder{
		"edit":   post(vendedor, d.Editar, "/documentos/"+itoa(ccs.ID)+"/editar", docPV, url.Values{"titulo": {"X"}, "seccion": {itoa(educacion)}}),
		"rename": post(vendedor, d.RenombrarSeccion, "/documentos/secciones/"+itoa(comerciales)+"/editar", comercialesPV, url.Values{"nombre": {"X"}}),
		"remove": post(vendedor, d.EliminarSeccion, "/documentos/secciones/"+itoa(educacion)+"/eliminar", educacionPV, nil),
	} {
		if rec.Code != http.StatusForbidden {
			t.Errorf("vendedor %s = %d, want 403", name, rec.Code)
		}
	}
	if sections, _ := a.store.ListDocumentSections(ctx); len(sections) != 2 || sections[0].Name != "Documentos comerciales" || len(sections[0].Documents) != 2 {
		t.Fatalf("a vendedor changed something: %+v", sections)
	}

	// A section with documents can't be removed: the page says why, with it open.
	rec := post(admin, d.EliminarSeccion, "/documentos/secciones/"+itoa(comerciales)+"/eliminar", comercialesPV, nil)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "No se puede eliminar una sección que tiene documentos") || !strings.Contains(rec.Body.String(), comercialesOpen) {
		t.Errorf("removing a section with documents = %d: %s", rec.Code, rec.Body.String())
	}

	// Renaming to another section's name, in any case, is a field error, with what was typed.
	rec = post(admin, d.RenombrarSeccion, "/documentos/secciones/"+itoa(educacion)+"/editar", educacionPV, url.Values{"nombre": {"documentos COMERCIALES"}})
	if body := rec.Body.String(); rec.Code != http.StatusBadRequest || !strings.Contains(body, "Ya hay una sección con ese nombre") ||
		!strings.Contains(body, `value="documentos COMERCIALES"`) || !strings.Contains(body, `id="seccion-`+itoa(educacion)+`" open>`) {
		t.Errorf("rename to a taken name = %d: %s", rec.Code, body)
	}
	rec = post(admin, d.RenombrarSeccion, "/documentos/secciones/"+itoa(educacion)+"/editar", educacionPV, url.Values{"nombre": {" "}})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Escribe el nombre de la sección.") {
		t.Errorf("rename to blank = %d", rec.Code)
	}
	rec = post(admin, d.RenombrarSeccion, "/documentos/secciones/"+itoa(educacion)+"/editar", educacionPV, url.Values{"nombre": {"Capacitación"}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/documentos?guardado=seccion-renombrada&seccion="+itoa(educacion)+"#seccion-"+itoa(educacion) {
		t.Errorf("rename = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec := post(admin, d.RenombrarSeccion, "/documentos/secciones/999/editar", map[string]string{"id": "999"}, url.Values{"nombre": {"X"}}); rec.Code != http.StatusNotFound {
		t.Errorf("rename a missing section = %d, want 404", rec.Code)
	}

	// Editing a document: a blank title is a field error in its own form, with the
	// section it is in open; then a new title and section.
	rec = post(admin, d.Editar, "/documentos/"+itoa(ccs.ID)+"/editar", docPV, url.Values{"titulo": {" "}, "seccion": {itoa(educacion)}})
	if body := rec.Body.String(); rec.Code != http.StatusBadRequest || !strings.Contains(body, "Escribe el título del documento.") ||
		!strings.Contains(body, comercialesOpen) || !strings.Contains(body, `<details class="document-edit" open>`) {
		t.Errorf("blank title = %d: %s", rec.Code, body)
	}
	rec = post(admin, d.Editar, "/documentos/"+itoa(ccs.ID)+"/editar", docPV, url.Values{"titulo": {"Ficha técnica Copperclad (CCS)"}, "seccion": {itoa(educacion)}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/documentos?guardado=editado&seccion="+itoa(educacion)+"#seccion-"+itoa(educacion) {
		t.Errorf("edit = %d %q: %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	if rec := post(admin, d.Editar, "/documentos/999/editar", map[string]string{"id": "999"}, url.Values{"titulo": {"X"}, "seccion": {itoa(educacion)}}); rec.Code != http.StatusNotFound {
		t.Errorf("edit a missing document = %d, want 404", rec.Code)
	}
	sections, _ = a.store.ListDocumentSections(ctx)
	if sections[1].Name != "Capacitación" || len(sections[1].Documents) != 1 || sections[1].Documents[0].Title != "Ficha técnica Copperclad (CCS)" {
		t.Fatalf("after edit: %+v", sections)
	}

	// Once its last document is moved out, the section can be removed.
	cca := sections[0].Documents[0]
	rec = post(admin, d.Editar, "/documentos/"+itoa(cca.ID)+"/editar", map[string]string{"id": itoa(cca.ID)}, url.Values{"titulo": {cca.Title}, "seccion": {itoa(educacion)}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("move = %d: %s", rec.Code, rec.Body.String())
	}
	rec = post(admin, d.EliminarSeccion, "/documentos/secciones/"+itoa(comerciales)+"/eliminar", comercialesPV, nil)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/documentos?guardado=seccion-eliminada" {
		t.Errorf("remove an empty section = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if body := page(vendedor, "/documentos?guardado=seccion-eliminada"); strings.Contains(body, "Documentos comerciales") || !strings.Contains(body, "Sección eliminada.") ||
		!strings.Contains(body, `Capacitación <span class="document-count">(2)</span>`) {
		t.Errorf("after removing the section: %s", body)
	}
	if rec := post(admin, d.EliminarSeccion, "/documentos/secciones/"+itoa(comerciales)+"/eliminar", comercialesPV, nil); rec.Code != http.StatusNotFound {
		t.Errorf("removing twice = %d, want 404", rec.Code)
	}
}
