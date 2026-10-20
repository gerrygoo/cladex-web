package store

import (
	"context"
	"errors"
	"testing"
)

func TestOfficialDocuments(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID, err := s.CreateUser(ctx, "rodolfo", "Rodolfo Flores", "hashedpw", "admin")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// The two starting sections are there, empty.
	sections, err := s.ListDocumentSections(ctx)
	if err != nil || len(sections) != 2 || sections[0].Name != "Documentos comerciales" || sections[1].Name != "Educación" || len(sections[0].Documents) != 0 {
		t.Fatalf("ListDocumentSections = %+v, %v", sections, err)
	}
	comerciales := sections[0].ID
	pdf := NewFile{Filename: "ficha.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.4")}

	ccsID, err := s.AddOfficialDocument(ctx, comerciales, "", "Ficha técnica Copperclad (CCS)", pdf, userID)
	if err != nil {
		t.Fatalf("AddOfficialDocument: %v", err)
	}
	if _, err := s.AddOfficialDocument(ctx, comerciales, "", "ficha técnica Conduccasa (CCA)", pdf, userID); err != nil {
		t.Fatalf("AddOfficialDocument: %v", err)
	}
	// A new section by name; the same name in another case reuses it.
	if _, err := s.AddOfficialDocument(ctx, 0, "Presentaciones", "Presentación Copperclad", pdf, userID); err != nil {
		t.Fatalf("AddOfficialDocument(new section): %v", err)
	}
	if _, err := s.AddOfficialDocument(ctx, 0, " presentaciones ", "Presentación Cladex", pdf, userID); err != nil {
		t.Fatalf("AddOfficialDocument(same section): %v", err)
	}
	for name, add := range map[string]func() (int64, error){
		"no title":    func() (int64, error) { return s.AddOfficialDocument(ctx, comerciales, "", " ", pdf, userID) },
		"no section":  func() (int64, error) { return s.AddOfficialDocument(ctx, 0, "", "X", pdf, userID) },
		"bad section": func() (int64, error) { return s.AddOfficialDocument(ctx, 999, "", "X", pdf, userID) },
		"empty file": func() (int64, error) {
			return s.AddOfficialDocument(ctx, comerciales, "", "X", NewFile{Filename: "x.pdf"}, userID)
		},
	} {
		if _, err := add(); !errors.Is(err, ErrBadDocument) {
			t.Errorf("%s: err = %v, want ErrBadDocument", name, err)
		}
	}

	sections, err = s.ListDocumentSections(ctx)
	if err != nil || len(sections) != 3 || sections[2].Name != "Presentaciones" || len(sections[2].Documents) != 2 {
		t.Fatalf("ListDocumentSections = %+v, %v", sections, err)
	}
	// By title, ignoring case: Conduccasa before Copperclad.
	if docs := sections[0].Documents; len(docs) != 2 || docs[0].Title != "ficha técnica Conduccasa (CCA)" || docs[1].UploadedByName != "Rodolfo Flores" {
		t.Errorf("Documentos comerciales = %+v", docs)
	}

	doc, data, err := s.OfficialDocumentData(ctx, ccsID)
	if err != nil || doc == nil || string(data) != "%PDF-1.4" || doc.Filename != "ficha.pdf" || doc.Size != 8 {
		t.Fatalf("OfficialDocumentData = %+v, %q, %v", doc, data, err)
	}

	if ok, err := s.RemoveOfficialDocument(ctx, ccsID, userID); !ok || err != nil {
		t.Fatalf("RemoveOfficialDocument = %v, %v", ok, err)
	}
	if ok, _ := s.RemoveOfficialDocument(ctx, ccsID, userID); ok {
		t.Error("removing twice reported a removal")
	}
	if doc, _, err := s.OfficialDocumentData(ctx, ccsID); doc != nil || err != nil {
		t.Errorf("removed document still served: %+v, %v", doc, err)
	}
	sections, _ = s.ListDocumentSections(ctx)
	if len(sections[0].Documents) != 1 {
		t.Errorf("removed document still listed: %+v", sections[0].Documents)
	}
	// The bytes stay, with who removed it.
	var deletedBy int64
	if err := s.db.QueryRowContext(ctx, `SELECT deleted_by FROM official_documents WHERE id = ? AND length(data) = 8`, ccsID).Scan(&deletedBy); err != nil || deletedBy != userID {
		t.Errorf("removed row = %d, %v; want its bytes kept and deleted_by %d", deletedBy, err, userID)
	}
}
