package store

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gerrygoo/cladex-web"
)

func TestOfficialDocuments(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID, err := s.CreateUser(ctx, "rodolfo", "Rodolfo Flores", "hashedpw", "admin")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// The two starting sections are there, empty, Educación first (migrations/0028).
	sections, err := s.ListDocumentSections(ctx)
	if err != nil || len(sections) != 2 || sections[0].Name != "Educación" || sections[1].Name != "Documentos comerciales" || len(sections[0].Documents) != 0 {
		t.Fatalf("ListDocumentSections = %+v, %v", sections, err)
	}
	comerciales := sections[1].ID
	pdf := NewFile{Filename: "ficha.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.4")}

	ccsID, _, err := s.AddOfficialDocument(ctx, comerciales, "", "Ficha técnica Copperclad (CCS)", pdf, userID)
	if err != nil {
		t.Fatalf("AddOfficialDocument: %v", err)
	}
	if _, _, err := s.AddOfficialDocument(ctx, comerciales, "", "ficha técnica Conduccasa (CCA)", pdf, userID); err != nil {
		t.Fatalf("AddOfficialDocument: %v", err)
	}
	// A new section by name; the same name in another case reuses it.
	if _, _, err := s.AddOfficialDocument(ctx, 0, "Presentaciones", "Presentación Copperclad", pdf, userID); err != nil {
		t.Fatalf("AddOfficialDocument(new section): %v", err)
	}
	if _, _, err := s.AddOfficialDocument(ctx, 0, " presentaciones ", "Presentación Cladex", pdf, userID); err != nil {
		t.Fatalf("AddOfficialDocument(same section): %v", err)
	}
	for name, add := range map[string]func() error{
		"no title":    func() error { _, _, err := s.AddOfficialDocument(ctx, comerciales, "", " ", pdf, userID); return err },
		"no section":  func() error { _, _, err := s.AddOfficialDocument(ctx, 0, "", "X", pdf, userID); return err },
		"bad section": func() error { _, _, err := s.AddOfficialDocument(ctx, 999, "", "X", pdf, userID); return err },
		"empty file": func() error {
			_, _, err := s.AddOfficialDocument(ctx, comerciales, "", "X", NewFile{Filename: "x.pdf"}, userID)
			return err
		},
	} {
		if err := add(); !errors.Is(err, ErrBadDocument) {
			t.Errorf("%s: err = %v, want ErrBadDocument", name, err)
		}
	}

	sections, err = s.ListDocumentSections(ctx)
	if err != nil || len(sections) != 3 || sections[2].Name != "Presentaciones" || len(sections[2].Documents) != 2 {
		t.Fatalf("ListDocumentSections = %+v, %v", sections, err)
	}
	// By title, ignoring case: Conduccasa before Copperclad.
	if docs := sections[1].Documents; len(docs) != 2 || docs[0].Title != "ficha técnica Conduccasa (CCA)" || docs[1].UploadedByName != "Rodolfo Flores" {
		t.Errorf("Documentos comerciales = %+v", docs)
	}

	doc, data, err := s.OfficialDocumentData(ctx, ccsID)
	if err != nil || doc == nil || string(data) != "%PDF-1.4" || doc.Filename != "ficha.pdf" || doc.Size != 8 {
		t.Fatalf("OfficialDocumentData = %+v, %q, %v", doc, data, err)
	}

	if sec, err := s.RemoveOfficialDocument(ctx, ccsID, userID); sec != comerciales || err != nil {
		t.Fatalf("RemoveOfficialDocument = %v, %v; want section %d", sec, err, comerciales)
	}
	if sec, _ := s.RemoveOfficialDocument(ctx, ccsID, userID); sec != 0 {
		t.Error("removing twice reported a removal")
	}
	if doc, _, err := s.OfficialDocumentData(ctx, ccsID); doc != nil || err != nil {
		t.Errorf("removed document still served: %+v, %v", doc, err)
	}
	sections, _ = s.ListDocumentSections(ctx)
	if len(sections[1].Documents) != 1 {
		t.Errorf("removed document still listed: %+v", sections[1].Documents)
	}
	// The bytes stay, with who removed it.
	var deletedBy int64
	if err := s.db.QueryRowContext(ctx, `SELECT deleted_by FROM official_documents WHERE id = ? AND length(data) = 8`, ccsID).Scan(&deletedBy); err != nil || deletedBy != userID {
		t.Errorf("removed row = %d, %v; want its bytes kept and deleted_by %d", deletedBy, err, userID)
	}
}

// Issue #44: renaming sections, renaming and moving documents, removing empty sections,
// all on the audit trail.
func TestEditOfficialDocuments(t *testing.T) {
	s := newTestStore(t)
	userID, err := s.CreateUser(context.Background(), "rodolfo", "Rodolfo Flores", "hashedpw", "admin")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	ctx := WithActor(context.Background(), Actor{UserID: userID, Source: SourceWeb})
	key := func(id int64) string { return strconv.FormatInt(id, 10) }
	sections, _ := s.ListDocumentSections(ctx)
	comerciales, educacion := sections[1].ID, sections[0].ID
	pdf := NewFile{Filename: "ficha.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.4")}
	docID, inSection, err := s.AddOfficialDocument(ctx, comerciales, "", "Ficha CCS", pdf, userID)
	if err != nil || inSection != comerciales {
		t.Fatalf("AddOfficialDocument = %d, %d, %v", docID, inSection, err)
	}

	// Rename: another section's name in any case is taken; its own name in another case
	// is fine; blank is refused; a missing section is reported.
	if err := s.RenameDocumentSection(ctx, educacion, " documentos COMERCIALES "); !errors.Is(err, ErrSectionNameTaken) {
		t.Errorf("rename to a taken name = %v, want ErrSectionNameTaken", err)
	}
	if err := s.RenameDocumentSection(ctx, educacion, "  "); !errors.Is(err, ErrBadDocument) {
		t.Errorf("rename to blank = %v, want ErrBadDocument", err)
	}
	if err := s.RenameDocumentSection(ctx, 999, "X"); !errors.Is(err, ErrNoSection) {
		t.Errorf("rename a missing section = %v, want ErrNoSection", err)
	}
	if err := s.RenameDocumentSection(ctx, educacion, "EDUCACIÓN"); err != nil {
		t.Errorf("rename to its own name in another case: %v", err)
	}
	if err := s.RenameDocumentSection(ctx, educacion, " Capacitación "); err != nil {
		t.Fatalf("RenameDocumentSection: %v", err)
	}

	// Edit a document: a blank title or a missing section is refused.
	if _, err := s.UpdateOfficialDocument(ctx, docID, educacion, " "); !errors.Is(err, ErrBadDocument) {
		t.Errorf("blank title = %v, want ErrBadDocument", err)
	}
	if _, err := s.UpdateOfficialDocument(ctx, docID, 999, "X"); !errors.Is(err, ErrBadDocument) {
		t.Errorf("missing section = %v, want ErrBadDocument", err)
	}
	if ok, err := s.UpdateOfficialDocument(ctx, 999, educacion, "X"); ok || err != nil {
		t.Errorf("missing document = %v, %v", ok, err)
	}

	// A section with documents can't be removed; once they're moved out, it can.
	if err := s.RemoveDocumentSection(ctx, comerciales, userID); !errors.Is(err, ErrSectionNotEmpty) {
		t.Fatalf("remove a section with documents = %v, want ErrSectionNotEmpty", err)
	}
	if ok, err := s.UpdateOfficialDocument(ctx, docID, educacion, " Ficha técnica Copperclad (CCS) "); !ok || err != nil {
		t.Fatalf("UpdateOfficialDocument = %v, %v", ok, err)
	}
	sections, _ = s.ListDocumentSections(ctx)
	if sections[0].Name != "Capacitación" || len(sections[0].Documents) != 1 || sections[0].Documents[0].Title != "Ficha técnica Copperclad (CCS)" || len(sections[1].Documents) != 0 {
		t.Fatalf("after rename and move: %+v", sections)
	}
	if err := s.RemoveDocumentSection(ctx, comerciales, userID); err != nil {
		t.Fatalf("RemoveDocumentSection: %v", err)
	}

	// It's gone from the page and takes nothing new, but the row stays with who removed
	// it, and its name is free again.
	if err := s.RemoveDocumentSection(ctx, comerciales, userID); !errors.Is(err, ErrNoSection) {
		t.Errorf("removing twice = %v, want ErrNoSection", err)
	}
	if err := s.RenameDocumentSection(ctx, comerciales, "Otra"); !errors.Is(err, ErrNoSection) {
		t.Errorf("renaming a removed section = %v, want ErrNoSection", err)
	}
	if _, err := s.UpdateOfficialDocument(ctx, docID, comerciales, "X"); !errors.Is(err, ErrBadDocument) {
		t.Errorf("moving to a removed section = %v, want ErrBadDocument", err)
	}
	if _, _, err := s.AddOfficialDocument(ctx, comerciales, "", "X", pdf, userID); !errors.Is(err, ErrBadDocument) {
		t.Errorf("uploading to a removed section = %v, want ErrBadDocument", err)
	}
	sections, _ = s.ListDocumentSections(ctx)
	if len(sections) != 1 || sections[0].ID != educacion {
		t.Fatalf("after removal: %+v", sections)
	}
	var deletedBy int64
	if err := s.db.QueryRowContext(ctx, `SELECT deleted_by FROM document_sections WHERE id = ?`, comerciales).Scan(&deletedBy); err != nil || deletedBy != userID {
		t.Errorf("removed section deleted_by = %d, %v", deletedBy, err)
	}
	if err := s.RenameDocumentSection(ctx, educacion, "documentos comerciales"); err != nil {
		t.Errorf("a removed section's name should be free: %v", err)
	}
	// Typing a live section's name while uploading picks it, not the removed one.
	if _, sec, err := s.AddOfficialDocument(ctx, 0, "Documentos Comerciales", "Y", pdf, userID); err != nil || sec != educacion {
		t.Errorf("new section by a live name = %d, %v; want %d", sec, err, educacion)
	}

	// Who did what is on the audit trail.
	entries, err := s.AuditLog(ctx, AuditFilter{TableName: "official_documents", RowKey: key(docID)})
	if err != nil || len(entries) != 1 || entries[0].Op != "update" || entries[0].ActorID == nil || *entries[0].ActorID != userID ||
		entries[0].OldValues["title"] != "Ficha CCS" || entries[0].NewValues["title"] != "Ficha técnica Copperclad (CCS)" ||
		entries[0].NewValues["section_id"] != float64(educacion) {
		t.Errorf("document audit = %+v, %v", entries, err)
	}
	entries, err = s.AuditLog(ctx, AuditFilter{TableName: "document_sections", RowKey: key(comerciales)})
	if err != nil || len(entries) != 1 || entries[0].NewValues["deleted_at"] == nil || entries[0].ActorID == nil || *entries[0].ActorID != userID {
		t.Errorf("section removal audit = %+v, %v", entries, err)
	}
	entries, err = s.AuditLog(ctx, AuditFilter{TableName: "document_sections", RowKey: key(educacion)})
	if err != nil || len(entries) != 3 || entries[0].NewValues["name"] != "documentos comerciales" || entries[2].OldValues["name"] != "Educación" {
		t.Errorf("section rename audit = %+v, %v", entries, err)
	}
	// A section made while uploading is audited too.
	_, presentaciones, err := s.AddOfficialDocument(ctx, 0, "Presentaciones", "Z", pdf, userID)
	if err != nil {
		t.Fatalf("AddOfficialDocument(new section): %v", err)
	}
	entries, err = s.AuditLog(ctx, AuditFilter{TableName: "document_sections", RowKey: key(presentaciones)})
	if err != nil || len(entries) != 1 || entries[0].Op != "insert" || entries[0].ActorID == nil || *entries[0].ActorID != userID {
		t.Errorf("new section audit = %+v, %v", entries, err)
	}
}

// Issue #44: 0028 puts the sections in the order Emilio asked for, matching the names as
// typed loosely, and leaves any other after them in creation order, without audit rows.
func TestMigration0028OrdersSections(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "up.db")
	old, err := Open(ctx, dsn, migrationsBefore(t, "0028"))
	if err != nil {
		t.Fatalf("open at 0027: %v", err)
	}
	// 1 and 2 come from 0023; Emilio renamed 1 and added the rest.
	for _, q := range []string{
		`UPDATE document_sections SET name = 'Rodolfo - Documentos comerciales' WHERE id = 1`,
		`INSERT INTO document_sections (id, name) VALUES
			(3, 'Presentaciones'), (4, 'Documentos comerciales – otros'), (5, 'Gerardo -Documentos Comerciales'),
			(6, 'DOCUMENTOS CLADEX'), (7, 'Logos')`,
	} {
		if _, err := old.db.ExecContext(ctx, q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	var auditBefore int
	if err := old.db.QueryRowContext(ctx, `SELECT count(*) FROM audit_log`).Scan(&auditBefore); err != nil {
		t.Fatalf("audit count: %v", err)
	}
	old.Close()

	s, err := Open(ctx, dsn, cladex.MigrationsFS)
	if err != nil {
		t.Fatalf("open at 0028: %v", err)
	}
	defer s.Close()
	sections, err := s.ListDocumentSections(ctx)
	if err != nil {
		t.Fatalf("ListDocumentSections: %v", err)
	}
	var names []string
	for _, sec := range sections {
		names = append(names, sec.Name)
	}
	want := "DOCUMENTOS CLADEX|Educación|Rodolfo - Documentos comerciales|Gerardo -Documentos Comerciales|Documentos comerciales – otros|Presentaciones|Logos"
	if got := strings.Join(names, "|"); got != want {
		t.Errorf("order = %s\nwant    %s", got, want)
	}
	var auditAfter int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM audit_log`).Scan(&auditAfter); err != nil || auditAfter != auditBefore {
		t.Errorf("audit_log rows = %d, were %d (%v)", auditAfter, auditBefore, err)
	}
}

// Issue #44: admins move a section up or down; a new section goes last; moving is on the
// audit trail.
func TestMoveDocumentSection(t *testing.T) {
	s := newTestStore(t)
	userID, err := s.CreateUser(context.Background(), "rodolfo", "Rodolfo Flores", "hashedpw", "admin")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	ctx := WithActor(context.Background(), Actor{UserID: userID, Source: SourceWeb})
	pdf := NewFile{Filename: "ficha.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.4")}
	if _, _, err := s.AddOfficialDocument(ctx, 0, "Logos", "Isotipo", pdf, userID); err != nil {
		t.Fatalf("AddOfficialDocument: %v", err)
	}
	order := func() (string, []int64) {
		sections, err := s.ListDocumentSections(ctx)
		if err != nil {
			t.Fatalf("ListDocumentSections: %v", err)
		}
		var names []string
		var ids []int64
		for _, sec := range sections {
			names, ids = append(names, sec.Name), append(ids, sec.ID)
		}
		return strings.Join(names, "|"), ids
	}
	got, ids := order()
	if got != "Educación|Documentos comerciales|Logos" {
		t.Fatalf("order = %s, want the new section last", got)
	}
	educacion, comerciales, logos := ids[0], ids[1], ids[2]

	for _, c := range []struct {
		id   int64
		by   int
		want string
	}{
		{logos, -1, "Educación|Logos|Documentos comerciales"},
		{logos, -1, "Logos|Educación|Documentos comerciales"},
		{logos, -1, "Logos|Educación|Documentos comerciales"},      // already first
		{comerciales, 1, "Logos|Educación|Documentos comerciales"}, // already last
		{logos, 1, "Educación|Logos|Documentos comerciales"},
	} {
		if err := s.MoveDocumentSection(ctx, c.id, c.by); err != nil {
			t.Fatalf("MoveDocumentSection(%d, %d): %v", c.id, c.by, err)
		}
		if got, _ := order(); got != c.want {
			t.Errorf("after moving %d by %d: %s, want %s", c.id, c.by, got, c.want)
		}
	}
	if err := s.MoveDocumentSection(ctx, 999, 1); !errors.Is(err, ErrNoSection) {
		t.Errorf("moving a missing section = %v, want ErrNoSection", err)
	}
	// A removed section doesn't count as a neighbour.
	if err := s.RemoveDocumentSection(ctx, educacion, userID); err != nil {
		t.Fatalf("RemoveDocumentSection: %v", err)
	}
	if err := s.MoveDocumentSection(ctx, logos, -1); err != nil {
		t.Fatal(err)
	}
	if got, _ := order(); got != "Logos|Documentos comerciales" {
		t.Errorf("after removing Educación: %s", got)
	}
	if err := s.MoveDocumentSection(ctx, comerciales, -1); err != nil {
		t.Fatal(err)
	}
	if got, _ := order(); got != "Documentos comerciales|Logos" {
		t.Errorf("moving past a removed section: %s", got)
	}

	entries, err := s.AuditLog(ctx, AuditFilter{TableName: "document_sections", RowKey: strconv.FormatInt(comerciales, 10)})
	if err != nil || len(entries) == 0 || entries[0].Op != "update" || entries[0].ActorID == nil || *entries[0].ActorID != userID ||
		entries[0].NewValues["position"] == entries[0].OldValues["position"] {
		t.Errorf("move audit = %+v, %v", entries, err)
	}
}
