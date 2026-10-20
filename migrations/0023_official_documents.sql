-- Documentos oficiales (issue #32): files every user can open from the top menu (fichas
-- técnicas, presentations, logos, training), grouped in sections. Only admins add or
-- remove them.
--
-- A section is a heading on the page. The two Cladex asked for exist from the start, so
-- Educación shows even while it is empty; an admin creates more while uploading. Names are
-- unique regardless of case, so "educación" picks the existing section.
CREATE TABLE document_sections (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE COLLATE NOCASE CHECK (length(trim(name)) > 0),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
INSERT INTO document_sections (name) VALUES ('Documentos comerciales'), ('Educación');

-- The bytes live in the database, as in project_files (0019), so the nightly backup and
-- the Litestream replica carry them; the app caps a file at 25 MB and accepts only PDF and
-- images, by what the bytes are. These are uploaded documents made outside the app, so
-- there is nothing to re-render them from; documents the app makes itself (quotes,
-- facturas) keep their source data and render on demand instead.
--
-- Removing one only stamps deleted_at/deleted_by and keeps the bytes, so a mistaken
-- delete can be undone by hand. Rows are otherwise never updated, and the columns say
-- who added and who removed what, so like project_files the table has no audit triggers.
CREATE TABLE official_documents (
    id           INTEGER PRIMARY KEY,
    section_id   INTEGER NOT NULL REFERENCES document_sections (id),
    title        TEXT NOT NULL CHECK (length(trim(title)) > 0),
    filename     TEXT NOT NULL CHECK (length(trim(filename)) > 0),
    content_type TEXT NOT NULL,
    size         INTEGER NOT NULL,
    data         BLOB NOT NULL,
    uploaded_by  INTEGER NOT NULL REFERENCES users (id),
    uploaded_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    deleted_at   TEXT,
    deleted_by   INTEGER REFERENCES users (id)
);
CREATE INDEX idx_official_documents_section ON official_documents (section_id) WHERE deleted_at IS NULL;
