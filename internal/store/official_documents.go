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

// ListDocumentSections returns every section in the order they were created, each with
// its documents (not removed) by title, ignoring case. An empty section is still listed.
func (s *Store) ListDocumentSections(ctx context.Context) ([]DocumentSection, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name FROM document_sections ORDER BY id`)
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
// if there is none. It returns the new document's id.
func (s *Store) AddOfficialDocument(ctx context.Context, sectionID int64, newSection, title string, file NewFile, userID int64) (int64, error) {
	newSection, title = strings.TrimSpace(newSection), strings.TrimSpace(title)
	if title == "" || strings.TrimSpace(file.Filename) == "" || len(file.Data) == 0 || newSection == "" && sectionID == 0 {
		return 0, ErrBadDocument
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store: add official document: %w", err)
	}
	defer tx.Rollback()
	if newSection != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO document_sections (name) VALUES (?) ON CONFLICT (name) DO NOTHING`, newSection); err != nil {
			return 0, fmt.Errorf("store: add official document: section: %w", err)
		}
		err = tx.QueryRowContext(ctx, `SELECT id FROM document_sections WHERE name = ?`, newSection).Scan(&sectionID)
	} else {
		err = tx.QueryRowContext(ctx, `SELECT id FROM document_sections WHERE id = ?`, sectionID).Scan(&sectionID)
	}
	if err == sql.ErrNoRows {
		return 0, ErrBadDocument
	}
	if err != nil {
		return 0, fmt.Errorf("store: add official document: section: %w", err)
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO official_documents (section_id, title, filename, content_type, size, data, uploaded_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sectionID, title, strings.TrimSpace(file.Filename), file.ContentType, len(file.Data), file.Data, userID)
	if err != nil {
		return 0, fmt.Errorf("store: add official document: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: add official document: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: add official document: %w", err)
	}
	return id, nil
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
// stay in the database. It reports whether there was such a document to remove.
func (s *Store) RemoveOfficialDocument(ctx context.Context, id, userID int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE official_documents
		SET deleted_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), deleted_by = ?
		WHERE id = ? AND deleted_at IS NULL`, userID, id)
	if err != nil {
		return false, fmt.Errorf("store: remove official document %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: remove official document %d: %w", id, err)
	}
	return n == 1, nil
}
