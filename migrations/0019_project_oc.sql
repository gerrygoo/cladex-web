-- Recibir la O.C. (docs/planning/hitos-proyectos-y-facturas.md, slice 4.4). A proyecto
-- enters oc_recibida with the client's purchase order on record: its number, its date
-- (a plain day, YYYY-MM-DD) and the forma de pago agreed for it, P.U.E. or P.P.D., which
-- is what the payment state of slice 4.5 branches on. They are NULL until then, and on
-- the proyectos that were already in oc_recibida before this migration.
ALTER TABLE projects ADD COLUMN oc_number TEXT;
ALTER TABLE projects ADD COLUMN oc_date TEXT;
ALTER TABLE projects ADD COLUMN payment_method TEXT CHECK (payment_method IN ('PUE', 'PPD'));

-- Files attached to a proyecto, for now only the client's purchase order (kind 'oc').
-- The bytes live in the database so the nightly backup and the Litestream replica, which
-- only cover the database, carry them too; the app caps a file at 10 MB. Rows are never
-- updated or deleted: attaching a corrected purchase order adds a row and the newest is
-- the one in force. Like quote_comments, that makes the table its own log, so it has no
-- audit triggers: uploaded_by and uploaded_at already say who added what and when.
CREATE TABLE project_files (
    id           INTEGER PRIMARY KEY,
    project_id   INTEGER NOT NULL REFERENCES projects (id),
    kind         TEXT NOT NULL CHECK (kind IN ('oc')),
    filename     TEXT NOT NULL CHECK (length(trim(filename)) > 0),
    content_type TEXT NOT NULL,
    size         INTEGER NOT NULL,
    data         BLOB NOT NULL,
    uploaded_by  INTEGER NOT NULL REFERENCES users (id),
    uploaded_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_project_files_project_id ON project_files (project_id, kind, id);

-- The audit triggers list their columns, so recreate them with the purchase order's.
DROP TRIGGER audit_projects_ai;
DROP TRIGGER audit_projects_au;
DROP TRIGGER audit_projects_ad;

CREATE TRIGGER audit_projects_ai AFTER INSERT ON projects BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('projects', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('folio', new.folio, 'customer_id', new.customer_id,
                        'user_id', new.user_id, 'status', new.status,
                        'probability', new.probability,
                        'lost_reason', new.lost_reason, 'lost_from', new.lost_from,
                        'expected_oc_date', new.expected_oc_date,
                        'next_followup_date', new.next_followup_date,
                        'oc_number', new.oc_number, 'oc_date', new.oc_date,
                        'payment_method', new.payment_method));
END;

CREATE TRIGGER audit_projects_au AFTER UPDATE ON projects BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('projects', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('folio', old.folio, 'customer_id', old.customer_id,
                        'user_id', old.user_id, 'status', old.status,
                        'probability', old.probability,
                        'lost_reason', old.lost_reason, 'lost_from', old.lost_from,
                        'expected_oc_date', old.expected_oc_date,
                        'next_followup_date', old.next_followup_date,
                        'oc_number', old.oc_number, 'oc_date', old.oc_date,
                        'payment_method', old.payment_method),
            json_object('folio', new.folio, 'customer_id', new.customer_id,
                        'user_id', new.user_id, 'status', new.status,
                        'probability', new.probability,
                        'lost_reason', new.lost_reason, 'lost_from', new.lost_from,
                        'expected_oc_date', new.expected_oc_date,
                        'next_followup_date', new.next_followup_date,
                        'oc_number', new.oc_number, 'oc_date', new.oc_date,
                        'payment_method', new.payment_method));
END;

CREATE TRIGGER audit_projects_ad AFTER DELETE ON projects BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('projects', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('folio', old.folio, 'customer_id', old.customer_id,
                        'user_id', old.user_id, 'status', old.status,
                        'probability', old.probability,
                        'lost_reason', old.lost_reason, 'lost_from', old.lost_from,
                        'expected_oc_date', old.expected_oc_date,
                        'next_followup_date', old.next_followup_date,
                        'oc_number', old.oc_number, 'oc_date', old.oc_date,
                        'payment_method', old.payment_method));
END;
