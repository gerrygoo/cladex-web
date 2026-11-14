-- cladex:foreign-keys-off
-- cladex:exclusive
-- Facturas registradas a mano (issue #63; docs/planning/estados-del-proyecto.md,
-- "Decisions 2026-11-10"): a factura issued outside the app (the PAC's portal) is
-- recorded by uploading its stamped XML, with its PDF optionally. It is an ordinary
-- comprobante, status 'timbrada', so the project_states view, the cancel and undo
-- blockers and the Facturas screens treat it like a stamped one.
--
--   comprobantes.origen       'timbrado' (stamped by the app through the PAC, every row
--                             so far) or 'manual' (its XML uploaded). A manual one has no
--                             request, no serie/folio of the app's sequence (the XML has
--                             its own) and is cancelled outside the app, then marked here.
--   comprobantes.ext_folio    a manual one's serie and folio as its XML has them ("A-153"),
--                             how it is named; the app's own serie/folio stay NULL, so
--                             they never collide with its sequence.
--   comprobantes.pdf_file_id  the PDF uploaded with a manual one (project_files, kind
--                             'factura' or 'complemento'); NULL renders it from the XML.
--
-- project_files.kind gains 'factura' and 'complemento' (the latter for the complementos
-- de pago of P.P.D. facturas, so recording them needs no second rebuild), which takes a
-- rebuild: hence exclusive. Nothing else changes; payments and deliveries keep pointing
-- at the same ids.

CREATE TABLE project_files_new (
    id           INTEGER PRIMARY KEY,
    project_id   INTEGER NOT NULL REFERENCES projects (id),
    kind         TEXT NOT NULL CHECK (kind IN ('oc', 'pago', 'entrega', 'factura', 'complemento')),
    filename     TEXT NOT NULL CHECK (length(trim(filename)) > 0),
    content_type TEXT NOT NULL,
    size         INTEGER NOT NULL,
    data         BLOB NOT NULL,
    uploaded_by  INTEGER NOT NULL REFERENCES users (id),
    uploaded_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
INSERT INTO project_files_new (id, project_id, kind, filename, content_type, size, data, uploaded_by, uploaded_at)
    SELECT id, project_id, kind, filename, content_type, size, data, uploaded_by, uploaded_at
    FROM project_files ORDER BY id;
DROP TABLE project_files;
ALTER TABLE project_files_new RENAME TO project_files;
CREATE INDEX idx_project_files_project_id ON project_files (project_id, kind, id);

ALTER TABLE comprobantes ADD COLUMN origen TEXT NOT NULL DEFAULT 'timbrado' CHECK (origen IN ('timbrado', 'manual'));
ALTER TABLE comprobantes ADD COLUMN pdf_file_id INTEGER REFERENCES project_files (id);
ALTER TABLE comprobantes ADD COLUMN ext_folio TEXT;
