-- cladex:foreign-keys-off
-- cladex:exclusive
-- Estados del proyecto (issues #66, #67; docs/planning/estados-del-proyecto.md,
-- "Decisions 2026-11-10"). The single line of stages after the client's O.C.
-- (oc_recibida → facturado → en_entrega → cerrado) gives way to one stored stage and
-- three derived states:
--
--   projects.status   prospecto / en_curso / perdido / cancelado. oc_recibida, facturado,
--                     en_entrega and cerrado all become en_curso. Cerrado · Ganado is no
--                     longer stored: it is en_curso with the three states complete (the
--                     project_states view below), so a correction that breaks one reopens
--                     the proyecto by itself.
--   cancel_reason, cancelled_from, cancelled_at
--                     Cancelado (admin only): why, the stage it was cancelled from (where
--                     Reactivar sends it back) and when. NULL on any proyecto that isn't
--                     cancelado, like lost_* on perdido.
--   lost_from         a proyecto lost with its O.C. in was lost from oc_recibida; that is
--                     en_curso now.
--
-- The hand-typed invoice and payment columns (invoice_ref, invoice_date, payment_status,
-- payment_ref, paid_at) stay for now: the typed-folio Factura form stays until #63's
-- manual mode replaces it, and a typed invoice_ref still counts as facturada.
--
-- projects can't change a CHECK in place, so it is rebuilt (foreign keys off, checked
-- before commit), and so is project_files, whose kind gains 'pago' (a comprobante de
-- pago, #67) and 'entrega' (a comprobante de entrega, #66, in now so that slice needs no
-- rebuild). The audit triggers go with the old tables and come back after the copy, so
-- the copied rows aren't logged; the history of each proyecto's old stages stays in the
-- audit log as it was (the old status names included). Old code can't run against the
-- new stage names: exclusive.
CREATE TABLE projects_new (
    id          INTEGER PRIMARY KEY,
    folio       TEXT NOT NULL UNIQUE,
    customer_id INTEGER NOT NULL REFERENCES customers (id),
    user_id     INTEGER NOT NULL REFERENCES users (id),
    status      TEXT NOT NULL DEFAULT 'prospecto'
        CHECK (status IN ('prospecto', 'en_curso', 'perdido', 'cancelado')),
    probability INTEGER NOT NULL DEFAULT 10 CHECK (probability IN (10, 25, 50, 75, 90)),
    probability_updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    lost_reason TEXT,
    lost_from   TEXT CHECK (lost_from IN ('prospecto', 'en_curso')),
    lost_at     TEXT,
    expected_oc_date   TEXT,
    next_followup_date TEXT,
    oc_number   TEXT,
    oc_date     TEXT,
    payment_method TEXT CHECK (payment_method IN ('PUE', 'PPD')),
    invoice_ref TEXT,
    invoice_date TEXT,
    payment_status TEXT CHECK (payment_status IN ('facturado_anticipo', 'pagado')),
    payment_ref TEXT,
    paid_at     TEXT,
    cancel_reason  TEXT,
    cancelled_from TEXT CHECK (cancelled_from IN ('prospecto', 'en_curso')),
    cancelled_at   TEXT
);
INSERT INTO projects_new (id, folio, customer_id, user_id, status, probability,
        probability_updated_at, created_at, lost_reason, lost_from, lost_at,
        expected_oc_date, next_followup_date, oc_number, oc_date, payment_method,
        invoice_ref, invoice_date, payment_status, payment_ref, paid_at)
    SELECT id, folio, customer_id, user_id,
        CASE WHEN status IN ('oc_recibida', 'facturado', 'en_entrega', 'cerrado') THEN 'en_curso' ELSE status END,
        probability, probability_updated_at, created_at, lost_reason,
        CASE WHEN lost_from = 'oc_recibida' THEN 'en_curso' ELSE lost_from END,
        lost_at, expected_oc_date, next_followup_date, oc_number, oc_date, payment_method,
        invoice_ref, invoice_date, payment_status, payment_ref, paid_at
    FROM projects ORDER BY id;
DROP TABLE projects;
ALTER TABLE projects_new RENAME TO projects;
CREATE INDEX idx_projects_customer_id ON projects (customer_id);
CREATE INDEX idx_projects_user_id ON projects (user_id);
CREATE INDEX idx_projects_status ON projects (status);

CREATE TABLE project_files_new (
    id           INTEGER PRIMARY KEY,
    project_id   INTEGER NOT NULL REFERENCES projects (id),
    kind         TEXT NOT NULL CHECK (kind IN ('oc', 'pago', 'entrega')),
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
                        'payment_method', new.payment_method,
                        'invoice_ref', new.invoice_ref, 'invoice_date', new.invoice_date,
                        'payment_status', new.payment_status,
                        'payment_ref', new.payment_ref, 'paid_at', new.paid_at,
                        'cancel_reason', new.cancel_reason, 'cancelled_from', new.cancelled_from));
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
                        'payment_method', old.payment_method,
                        'invoice_ref', old.invoice_ref, 'invoice_date', old.invoice_date,
                        'payment_status', old.payment_status,
                        'payment_ref', old.payment_ref, 'paid_at', old.paid_at,
                        'cancel_reason', old.cancel_reason, 'cancelled_from', old.cancelled_from),
            json_object('folio', new.folio, 'customer_id', new.customer_id,
                        'user_id', new.user_id, 'status', new.status,
                        'probability', new.probability,
                        'lost_reason', new.lost_reason, 'lost_from', new.lost_from,
                        'expected_oc_date', new.expected_oc_date,
                        'next_followup_date', new.next_followup_date,
                        'oc_number', new.oc_number, 'oc_date', new.oc_date,
                        'payment_method', new.payment_method,
                        'invoice_ref', new.invoice_ref, 'invoice_date', new.invoice_date,
                        'payment_status', new.payment_status,
                        'payment_ref', new.payment_ref, 'paid_at', new.paid_at,
                        'cancel_reason', new.cancel_reason, 'cancelled_from', new.cancelled_from));
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
                        'payment_method', old.payment_method,
                        'invoice_ref', old.invoice_ref, 'invoice_date', old.invoice_date,
                        'payment_status', old.payment_status,
                        'payment_ref', old.payment_ref, 'paid_at', old.paid_at,
                        'cancel_reason', old.cancel_reason, 'cancelled_from', old.cancelled_from));
END;

-- Pagos del cliente (#67). One row per payment the client made, whatever recorded it:
--
--   amount          centavos, MXN, with IVA (what the client pays; compared with the
--                   current quote's total)
--   paid_on         the day the money came in (YYYY-MM-DD)
--   source          manual ("Registrar pago"), factura (the payment a P.U.E. factura
--                   records) or complemento (a P.P.D. factura's complemento de pago).
--                   Only manual is written yet; #63 writes the other two
--   comprobante_id  the factura or complemento it came from, NULL for a manual one
--   file_id         the comprobante de pago the user uploaded (a project_files row of
--                   kind 'pago', stored the way the O.C.'s file is), if any
--   deleted_*       a payment recorded by mistake is deleted by an admin with a motivo:
--                   it stays here, out of every sum, and the motivo goes into the
--                   proyecto's history. Nothing else ever changes on a row
CREATE TABLE payments (
    id             INTEGER PRIMARY KEY,
    project_id     INTEGER NOT NULL REFERENCES projects (id),
    amount         INTEGER NOT NULL CHECK (amount > 0),
    paid_on        TEXT NOT NULL,
    source         TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'factura', 'complemento')),
    comprobante_id INTEGER REFERENCES comprobantes (id),
    file_id        INTEGER REFERENCES project_files (id),
    created_by     INTEGER NOT NULL REFERENCES users (id),
    created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    deleted_by     INTEGER REFERENCES users (id),
    deleted_at     TEXT,
    delete_reason  TEXT,
    CHECK ((deleted_at IS NULL) = (deleted_by IS NULL) AND (deleted_at IS NULL) = (delete_reason IS NULL))
);
CREATE INDEX idx_payments_project ON payments (project_id, paid_on);

CREATE TRIGGER audit_payments_ai AFTER INSERT ON payments BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('payments', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('project_id', new.project_id, 'amount', new.amount, 'paid_on', new.paid_on,
                        'source', new.source, 'comprobante_id', new.comprobante_id,
                        'file_id', new.file_id, 'created_by', new.created_by));
END;

CREATE TRIGGER audit_payments_au AFTER UPDATE ON payments BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('payments', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('project_id', old.project_id, 'amount', old.amount, 'paid_on', old.paid_on,
                        'source', old.source, 'comprobante_id', old.comprobante_id,
                        'file_id', old.file_id, 'deleted_by', old.deleted_by,
                        'deleted_at', old.deleted_at, 'delete_reason', old.delete_reason),
            json_object('project_id', new.project_id, 'amount', new.amount, 'paid_on', new.paid_on,
                        'source', new.source, 'comprobante_id', new.comprobante_id,
                        'file_id', new.file_id, 'deleted_by', new.deleted_by,
                        'deleted_at', new.deleted_at, 'delete_reason', new.delete_reason));
END;

CREATE TRIGGER audit_payments_ad AFTER DELETE ON payments BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('payments', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('project_id', old.project_id, 'amount', old.amount, 'paid_on', old.paid_on,
                        'source', old.source, 'comprobante_id', old.comprobante_id,
                        'file_id', old.file_id, 'deleted_at', old.deleted_at,
                        'delete_reason', old.delete_reason));
END;

-- A proyecto marked pagado by hand becomes one payment for its current quote's total,
-- on the day it was paid, recorded by whoever marked it pagado (the audit log knows) or
-- else its vendedor. Logged as the system's doing.
UPDATE audit_actor SET user_id = NULL, source = NULL WHERE id = 1;
INSERT INTO payments (project_id, amount, paid_on, source, created_by, created_at)
    SELECT p.id, q.total,
        COALESCE(p.paid_at, p.invoice_date, substr(p.created_at, 1, 10)), 'manual',
        COALESCE((SELECT a.actor_id FROM audit_log a
            WHERE a.table_name = 'projects' AND a.row_key = CAST(p.id AS TEXT) AND a.op = 'update'
                AND json_extract(a.new_values, '$.payment_status') = 'pagado'
                AND json_extract(a.old_values, '$.payment_status') IS NOT 'pagado'
                AND a.actor_id IS NOT NULL
            ORDER BY a.id DESC LIMIT 1), p.user_id),
        COALESCE((SELECT a.at FROM audit_log a
            WHERE a.table_name = 'projects' AND a.row_key = CAST(p.id AS TEXT) AND a.op = 'update'
                AND json_extract(a.new_values, '$.payment_status') = 'pagado'
                AND json_extract(a.old_values, '$.payment_status') IS NOT 'pagado'
            ORDER BY a.id DESC LIMIT 1), strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
    FROM projects p JOIN quotes q ON q.project_id = p.id AND q.status != 'revisada'
    WHERE p.payment_status = 'pagado' AND q.total > 0
    ORDER BY p.id;

-- project_states: one row per proyecto with its three derived states and Cerrado ·
-- Ganado. Nothing here is stored, so it can't disagree with the records it reads.
--
--   facturacion  pendiente / pendiente_complemento / facturada. From the newest vigente
--                factura (timbrada, or timbrada with its cancellation pending): P.U.E. is
--                facturada; P.P.D. is pendiente_complemento until its vigente complementos
--                de pago add up to its total. A cancelled factura doesn't count, so
--                cancelling one puts the proyecto back to pendiente. Without one, a
--                hand-typed invoice_ref counts as facturada (until #63's manual mode).
--   entrega      sin_oc / en_proceso / entregado. en_proceso once a supplier O.C. is
--                confirmed. 'entregado' can't happen yet: #66 adds delivery records and
--                makes it "every active supplier O.C. delivered" here.
--   cobro        pendiente / parcial / pagado: the payments not deleted against the
--                current quote's total with IVA. paid and last_paid_on go with it.
--   cerrado      1 when the proyecto is en_curso with facturada, entregado and pagado:
--                Cerrado · Ganado.
CREATE VIEW project_states AS
SELECT project_id, facturacion, entrega, paid, last_paid_on,
    CASE WHEN paid <= 0 THEN 'pendiente' WHEN paid >= total THEN 'pagado' ELSE 'parcial' END AS cobro,
    (status = 'en_curso' AND facturacion = 'facturada' AND entrega = 'entregado' AND paid > 0 AND paid >= total) AS cerrado
FROM (
    SELECT p.id AS project_id, p.status, COALESCE(q.total, 0) AS total,
        COALESCE((
            SELECT CASE WHEN f.metodo_pago = 'PPD' AND COALESCE((
                    SELECT SUM(c.total) FROM comprobantes c
                    WHERE c.related_id = f.id AND c.tipo = 'pago' AND c.status IN ('timbrada', 'cancelacion_pendiente')
                ), 0) < f.total THEN 'pendiente_complemento' ELSE 'facturada' END
            FROM comprobantes f
            WHERE f.project_id = p.id AND f.tipo = 'factura' AND f.status IN ('timbrada', 'cancelacion_pendiente')
            ORDER BY f.id DESC LIMIT 1
        ), CASE WHEN COALESCE(p.invoice_ref, '') != '' THEN 'facturada' ELSE 'pendiente' END) AS facturacion,
        CASE WHEN EXISTS (SELECT 1 FROM purchase_orders po WHERE po.project_id = p.id AND po.status = 'confirmada')
            THEN 'en_proceso' ELSE 'sin_oc' END AS entrega,
        COALESCE((SELECT SUM(pm.amount) FROM payments pm WHERE pm.project_id = p.id AND pm.deleted_at IS NULL), 0) AS paid,
        COALESCE((SELECT MAX(pm.paid_on) FROM payments pm WHERE pm.project_id = p.id AND pm.deleted_at IS NULL), '') AS last_paid_on
    FROM projects p
    LEFT JOIN quotes q ON q.project_id = p.id AND q.status != 'revisada'
);
