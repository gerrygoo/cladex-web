-- cladex:foreign-keys-off
-- Editar documentos oficiales (issue #44): admins rename sections, rename documents and
-- move them to another section, and remove sections that have nothing left in them.
--
-- A removed section only gets deleted_at/deleted_by, like a removed document: removed
-- documents keep their bytes and still point at their section (so a mistaken delete can
-- be undone by hand), so the section row has to stay. The app refuses to remove a
-- section while it has documents that aren't removed.
--
-- Names stay unique ignoring case, but only among the sections on the page, so a removed
-- "Educación" doesn't block a new one; a partial index replaces the column's UNIQUE.
-- SQLite can't drop that constraint in place, so the table is rebuilt; official_documents
-- references it, so the runner applies this file with foreign keys off (the marker on
-- line 1) and checks them before committing.
CREATE TABLE document_sections_new (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL COLLATE NOCASE CHECK (length(trim(name)) > 0),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    deleted_at TEXT,
    deleted_by INTEGER REFERENCES users (id)
);
INSERT INTO document_sections_new (id, name, created_at)
    SELECT id, name, created_at FROM document_sections ORDER BY id;
DROP TABLE document_sections;
ALTER TABLE document_sections_new RENAME TO document_sections;
CREATE UNIQUE INDEX idx_document_sections_name ON document_sections (name) WHERE deleted_at IS NULL;

-- Sections and documents are now edited in place, so who renamed or moved what is kept
-- in the audit trail (0004). This replaces what 0023 says about official_documents never
-- being updated: a document's title and section change, and those two columns are
-- audited; its upload and removal are still told by uploaded_*/deleted_* on the row.
CREATE TRIGGER audit_document_sections_ai AFTER INSERT ON document_sections BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('document_sections', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', new.name, 'deleted_at', new.deleted_at));
END;

CREATE TRIGGER audit_document_sections_au AFTER UPDATE ON document_sections BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('document_sections', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', old.name, 'deleted_at', old.deleted_at),
            json_object('name', new.name, 'deleted_at', new.deleted_at));
END;

CREATE TRIGGER audit_document_sections_ad AFTER DELETE ON document_sections BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('document_sections', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', old.name, 'deleted_at', old.deleted_at));
END;

CREATE TRIGGER audit_official_documents_au AFTER UPDATE OF title, section_id ON official_documents
WHEN old.title IS NOT new.title OR old.section_id IS NOT new.section_id BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('official_documents', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('title', old.title, 'section_id', old.section_id),
            json_object('title', new.title, 'section_id', new.section_id));
END;
