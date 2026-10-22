package store

import (
	"context"
	"errors"
	"testing"
)

// TestSupplierQuoteChain: the cotización del proveedor belongs to the revision chain,
// its data changes only while the chain's current quote is a borrador, and a file stays
// once a quote was issued after it.
func TestSupplierQuoteChain(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	customerID, userID, _, _ := seedQuoteFixtures(t, s, ctx)
	q, err := s.CreateDraftQuote(ctx, customerID, userID, "QL")
	if err != nil {
		t.Fatal(err)
	}
	pdf := NewFile{Filename: "cot.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.7")}

	if err := s.SetSupplierQuoteInfo(ctx, q.ID, "  ", "", userID); err != nil {
		t.Fatal(err)
	}
	var rows int
	s.db.QueryRowContext(ctx, `SELECT count(*) FROM supplier_quotes`).Scan(&rows)
	if rows != 0 {
		t.Error("blank data on a chain without a row created one")
	}
	if err := s.SetSupplierQuoteInfo(ctx, q.ID, " CT Internacional ", "Q-1", userID); err != nil {
		t.Fatal(err)
	}
	if err := s.AddSupplierQuoteFiles(ctx, q.ID, []NewFile{pdf}, userID); err != nil {
		t.Fatal(err)
	}
	// Uploaded before the quote is issued (testIssue's date is in the past).
	s.db.ExecContext(ctx, `UPDATE supplier_quote_files SET uploaded_at = '2026-08-01T00:00:00.000Z'`)
	if err := s.IssueQuote(ctx, q.ID, testIssue("sha1")); err != nil {
		t.Fatal(err)
	}
	// The data stays editable after emission, so a forgotten supplier can be filled in.
	if err := s.SetSupplierQuoteInfo(ctx, q.ID, "CT Internacional", "Q-2", userID); err != nil {
		t.Fatalf("editing an issued chain = %v", err)
	}
	if sq, _ := s.SupplierQuoteOf(ctx, q.ID); sq.Number != "Q-2" {
		t.Fatalf("number after editing an issued chain = %q, want Q-2", sq.Number)
	}

	rev, err := s.CreateRevision(ctx, q.ID, userID)
	if err != nil {
		t.Fatal(err)
	}
	if root, _ := s.QuoteChainRoot(ctx, rev.ID); root != q.ID {
		t.Fatalf("chain root of the revision = %d, want %d", root, q.ID)
	}
	// Uploaded on the revision, after the last emission: removable while it's a borrador.
	if err := s.AddSupplierQuoteFiles(ctx, rev.ID, []NewFile{pdf, pdf}, userID); err != nil {
		t.Fatal(err)
	}
	sq, err := s.SupplierQuoteOf(ctx, rev.ID)
	if err != nil || sq.Supplier != "CT Internacional" || sq.Number != "Q-2" || len(sq.Files) != 3 {
		t.Fatalf("revision sees %+v, %v", sq, err)
	}
	if sq.Files[0].Removable || !sq.Files[1].Removable {
		t.Fatalf("removable = %v, %v; want the issued file locked and the new one not", sq.Files[0].Removable, sq.Files[1].Removable)
	}
	if found, err := s.RemoveSupplierQuoteFile(ctx, rev.ID, sq.Files[0].ID, userID); !found || !errors.Is(err, ErrSupplierFileLocked) {
		t.Fatalf("removing the issued file = %v, %v", found, err)
	}
	if found, err := s.RemoveSupplierQuoteFile(ctx, q.ID, sq.Files[1].ID, userID); !found || err != nil {
		t.Fatalf("removing a new file through the original's folio = %v, %v", found, err)
	}
	if found, _ := s.RemoveSupplierQuoteFile(ctx, rev.ID, sq.Files[1].ID, userID); found {
		t.Error("a removed file was found again")
	}
	if f, _, _ := s.SupplierQuoteFileData(ctx, rev.ID, sq.Files[1].ID); f != nil {
		t.Error("a removed file is still served")
	}
	var deletedBy int64
	if err := s.db.QueryRowContext(ctx, `SELECT deleted_by FROM supplier_quote_files WHERE id = ? AND length(data) = 8`, sq.Files[1].ID).Scan(&deletedBy); err != nil || deletedBy != userID {
		t.Errorf("removed file keeps its bytes and remover: %d, %v", deletedBy, err)
	}

	// Once the revision is issued too, nothing can be removed.
	if err := s.IssueQuote(ctx, rev.ID, testIssue("sha2")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RemoveSupplierQuoteFile(ctx, rev.ID, sq.Files[2].ID, userID); !errors.Is(err, ErrSupplierFileLocked) {
		t.Fatalf("removing with the chain issued = %v", err)
	}

	var audited int
	s.db.QueryRowContext(ctx, `SELECT count(*) FROM audit_log WHERE table_name = 'supplier_quotes'`).Scan(&audited)
	if audited != 2 {
		t.Errorf("supplier_quotes audit rows = %d, want 2 (the insert and the edit after emission)", audited)
	}
}
