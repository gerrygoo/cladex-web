package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// DocumentSection is a heading on the Documentos oficiales page with its documents,
// ordered by title (see migrations/0023_official_documents.sql).
type DocumentSection struct {
	ID        int64
	Name      string
	Documents []OfficialDocument
}

// OfficialDocument is an official_documents row without its bytes.
type OfficialDocument struct {
	ID             int64
	SectionID      int64
	Title          string
	Filename       string
	ContentType    string
	Size           int64
	UploadedByName string
	UploadedAt     string
}

// ErrBadDocument is returned when a document lacks its section, title or bytes.
var ErrBadDocument = errors.New("store: document needs a section, a title and a file")

// ListDocumentSections returns every section not removed in the order they were
// created, each with its documents (not removed) by title, ignoring case. An empty
// section is still listed.
func (s *Store) ListDocumentSections(ctx context.Context) ([]DocumentSection, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name FROM document_sections WHERE deleted_at IS NULL ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: document sections: %w", err)
	}
	var sections []DocumentSection
	index := map[int64]int{}
	for rows.Next() {
		var sec DocumentSection
		if err := rows.Scan(&sec.ID, &sec.Name); err != nil {
			rows.Close()
			return nil, fmt.Errorf("store: document sections: %w", err)
		}
		index[sec.ID] = len(sections)
		sections = append(sections, sec)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: document sections: %w", err)
	}

	rows, err = s.db.QueryContext(ctx, `
		SELECT d.id, d.section_id, d.title, d.filename, d.content_type, d.size, u.name, d.uploaded_at
		FROM official_documents d JOIN users u ON u.id = d.uploaded_by
		WHERE d.deleted_at IS NULL
		ORDER BY d.title COLLATE NOCASE, d.id`)
	if err != nil {
		return nil, fmt.Errorf("store: official documents: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var d OfficialDocument
		if err := rows.Scan(&d.ID, &d.SectionID, &d.Title, &d.Filename, &d.ContentType, &d.Size, &d.UploadedByName, &d.UploadedAt); err != nil {
			return nil, fmt.Errorf("store: official documents: %w", err)
		}
		if i, ok := index[d.SectionID]; ok {
			sections[i].Documents = append(sections[i].Documents, d)
		}
	}
	return sections, rows.Err()
}

// AddOfficialDocument stores a document under an existing section (sectionID), or under
// newSection when that is not blank: the section with that name, ignoring case, created
// if there is none. A removed section counts as none. It returns the new document's id
// and the section it went to.
func (s *Store) AddOfficialDocument(ctx context.Context, sectionID int64, newSection, title string, file NewFile, userID int64) (docID, inSection int64, err error) {
	newSection, title = strings.TrimSpace(newSection), strings.TrimSpace(title)
	if title == "" || strings.TrimSpace(file.Filename) == "" || len(file.Data) == 0 || newSection == "" && sectionID == 0 {
		return 0, 0, ErrBadDocument
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("store: add official document: %w", err)
	}
	defer tx.Rollback()
	// A new section is audited (migrations/0027_document_sections_edit.sql).
	if err := stampActor(ctx, tx); err != nil {
		return 0, 0, err
	}
	if newSection != "" {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO document_sections (name) VALUES (?)
			ON CONFLICT (name) WHERE deleted_at IS NULL DO NOTHING`, newSection); err != nil {
			return 0, 0, fmt.Errorf("store: add official document: section: %w", err)
		}
		err = tx.QueryRowContext(ctx, `SELECT id FROM document_sections WHERE name = ? AND deleted_at IS NULL`, newSection).Scan(&sectionID)
	} else {
		err = tx.QueryRowContext(ctx, `SELECT id FROM document_sections WHERE id = ? AND deleted_at IS NULL`, sectionID).Scan(&sectionID)
	}
	if err == sql.ErrNoRows {
		return 0, 0, ErrBadDocument
	}
	if err != nil {
		return 0, 0, fmt.Errorf("store: add official document: section: %w", err)
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO official_documents (section_id, title, filename, content_type, size, data, uploaded_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sectionID, title, strings.TrimSpace(file.Filename), file.ContentType, len(file.Data), file.Data, userID)
	if err != nil {
		return 0, 0, fmt.Errorf("store: add official document: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, 0, fmt.Errorf("store: add official document: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("store: add official document: %w", err)
	}
	return id, sectionID, nil
}

var (
	// ErrSectionNameTaken is returned when another section on the page already has the
	// name, ignoring case.
	ErrSectionNameTaken = errors.New("store: a document section with that name exists")
	// ErrSectionNotEmpty is returned when removing a section that still has documents.
	ErrSectionNotEmpty = errors.New("store: document section still has documents")
	// ErrNoSection is returned when there is no section with that id, or it was removed.
	ErrNoSection = errors.New("store: no such document section")
)

// RenameDocumentSection renames a section on the page. The name is trimmed and must not
// be blank (ErrBadDocument) nor another section's, ignoring case (ErrSectionNameTaken);
// changing only its case is fine.
func (s *Store) RenameDocumentSection(ctx context.Context, id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrBadDocument
	}
	res, err := s.exec(ctx, `UPDATE document_sections SET name = ? WHERE id = ? AND deleted_at IS NULL`, name, id)
	if isUniqueViolation(err, "document_sections.name") {
		return ErrSectionNameTaken
	}
	if err != nil {
		return fmt.Errorf("store: rename document section %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: rename document section %d: %w", id, err)
	}
	if n == 0 {
		return ErrNoSection
	}
	return nil
}

// RemoveDocumentSection takes an empty section off the page, recording who did it. It
// refuses (ErrSectionNotEmpty) while the section has documents that aren't removed; the
// removed ones keep pointing at it, which is why the row stays (see
// migrations/0027_document_sections_edit.sql).
func (s *Store) RemoveDocumentSection(ctx context.Context, id, userID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: remove document section %d: %w", id, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return err
	}
	var documents int
	err = tx.QueryRowContext(ctx, `
		SELECT (SELECT count(*) FROM official_documents WHERE section_id = s.id AND deleted_at IS NULL)
		FROM document_sections s WHERE s.id = ? AND s.deleted_at IS NULL`, id).Scan(&documents)
	if err == sql.ErrNoRows {
		return ErrNoSection
	}
	if err != nil {
		return fmt.Errorf("store: remove document section %d: %w", id, err)
	}
	if documents > 0 {
		return ErrSectionNotEmpty
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE document_sections
		SET deleted_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), deleted_by = ?
		WHERE id = ?`, nullIfZero(userID), id); err != nil {
		return fmt.Errorf("store: remove document section %d: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: remove document section %d: %w", id, err)
	}
	return nil
}

// UpdateOfficialDocument changes a document's title and moves it to sectionID, which
// must be a section on the page. It returns ErrBadDocument for a blank title or a
// missing section, and reports whether there was such a document (not removed).
func (s *Store) UpdateOfficialDocument(ctx context.Context, id, sectionID int64, title string) (bool, error) {
	title = strings.TrimSpace(title)
	if title == "" || sectionID == 0 {
		return false, ErrBadDocument
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("store: update official document %d: %w", id, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return false, err
	}
	var live int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM document_sections WHERE id = ? AND deleted_at IS NULL`, sectionID).Scan(&live); err != nil {
		return false, fmt.Errorf("store: update official document %d: %w", id, err)
	}
	if live == 0 {
		return false, ErrBadDocument
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE official_documents SET title = ?, section_id = ?
		WHERE id = ? AND deleted_at IS NULL`, title, sectionID, id)
	if err != nil {
		return false, fmt.Errorf("store: update official document %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: update official document %d: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("store: update official document %d: %w", id, err)
	}
	return n == 1, nil
}

// OfficialDocumentData returns a document with its bytes, or nil when there is none
// with that id or it was removed.
func (s *Store) OfficialDocumentData(ctx context.Context, id int64) (*OfficialDocument, []byte, error) {
	var d OfficialDocument
	var data []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT id, section_id, title, filename, content_type, size, uploaded_at, data
		FROM official_documents WHERE id = ? AND deleted_at IS NULL`, id).
		Scan(&d.ID, &d.SectionID, &d.Title, &d.Filename, &d.ContentType, &d.Size, &d.UploadedAt, &data)
	if err == sql.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("store: official document %d: %w", id, err)
	}
	return &d, data, nil
}

// RemoveOfficialDocument takes a document off the page, recording who did it; its bytes
// stay in the database. It returns the section the document was in, or 0 when there was
// no such document to remove.
func (s *Store) RemoveOfficialDocument(ctx context.Context, id, userID int64) (int64, error) {
	var sectionID int64
	err := s.db.QueryRowContext(ctx, `
		UPDATE official_documents
		SET deleted_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), deleted_by = ?
		WHERE id = ? AND deleted_at IS NULL
		RETURNING section_id`, userID, id).Scan(&sectionID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("store: remove official document %d: %w", id, err)
	}
	return sectionID, nil
}
