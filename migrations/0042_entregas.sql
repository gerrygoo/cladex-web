-- cladex:exclusive
-- Estado de entrega (issue #66; docs/planning/estados-del-proyecto.md). Delivery is
-- marked per supplier O.C., and an admin can cancel a confirmed supplier O.C.:
--
--   purchase_orders.cancel_reason, cancelled_by, cancelled_at
--                   an admin cancels a confirmed O.C. (status cancelada, reserved since
--                   0034) with a motivo, also in the proyecto's history. A cancelled O.C.
--                   no longer counts: not for Entrega, nor for the blocks on cancelling
--                   the proyecto or undoing its O.C. recibida, which only look at
--                   confirmada. NULL on any O.C. that isn't cancelada.
--
--   deliveries      one row each time a supplier O.C. is marked as delivered ("Marcar
--                   como entregado", the proyecto's vendedor or an admin):
--     project_id, seq   the O.C. it is for: the proyecto and the O.C.'s seq, which its
--                   revisions keep. Delivery belongs to the O.C., not to one revision of
--                   it, so a revision confirmed after the delivery is still delivered
--     purchase_order_id the revision that was vigente when it was marked
--     delivered_on  the day it was delivered (YYYY-MM-DD, México City)
--     file_id       the comprobante de entrega (a project_files row of kind 'entrega',
--                   stored like the O.C.'s file), if any. It may be added later, once
--     created_by/at who marked it and when
--     undone_*      an admin takes a delivery marked by mistake back to En proceso de
--                   entrega with a motivo: the row stays, out of every state, and the
--                   motivo goes to the history. The O.C. can be marked again (a new row)
--   At most one delivery in force per O.C.
--
-- project_states.entrega now reads them: sin_oc without a confirmed supplier O.C.,
-- entregado when every confirmed one has its delivery in force, else en_proceso. With
-- entregado, Cerrado · Ganado can finally happen.
--
-- Proyectos that were en_entrega or cerrado when 0041 replaced the old line of stages
-- had been delivered under the old flow; their confirmed supplier O.C.s get a delivery
-- from the audit log (see below). In production that is QA0037.
--
-- Only adds columns, a table and rows, and replaces the view with one that has the same
-- columns: the previous build could run against it. It is exclusive because SQLite
-- replaces a view by dropping it (TestBreakingMigrationsAreExclusive can't tell), and the
-- pause it costs is a few seconds.
ALTER TABLE purchase_orders ADD COLUMN cancel_reason TEXT;
ALTER TABLE purchase_orders ADD COLUMN cancelled_by INTEGER REFERENCES users (id);
ALTER TABLE purchase_orders ADD COLUMN cancelled_at TEXT;

DROP TRIGGER audit_purchase_orders_au;
CREATE TRIGGER audit_purchase_orders_au AFTER UPDATE ON purchase_orders BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('purchase_orders', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('seq', old.seq, 'number', old.number, 'status', old.status,
                        'supplier_id', old.supplier_id, 'delivery_address', old.delivery_address,
                        'delivery_references', old.delivery_references, 'currency', old.currency,
                        'payment_form', old.payment_form, 'credit_days', old.credit_days,
                        'payment_other', old.payment_other, 'total', old.total,
                        'confirmed_at', old.confirmed_at, 'cancel_reason', old.cancel_reason,
                        'cancelled_by', old.cancelled_by, 'cancelled_at', old.cancelled_at),
            json_object('seq', new.seq, 'number', new.number, 'status', new.status,
                        'supplier_id', new.supplier_id, 'delivery_address', new.delivery_address,
                        'delivery_references', new.delivery_references, 'currency', new.currency,
                        'payment_form', new.payment_form, 'credit_days', new.credit_days,
                        'payment_other', new.payment_other, 'total', new.total,
                        'confirmed_at', new.confirmed_at, 'cancel_reason', new.cancel_reason,
                        'cancelled_by', new.cancelled_by, 'cancelled_at', new.cancelled_at));
END;

CREATE TABLE deliveries (
    id                INTEGER PRIMARY KEY,
    project_id        INTEGER NOT NULL REFERENCES projects (id),
    seq               INTEGER NOT NULL CHECK (seq >= 1),
    purchase_order_id INTEGER NOT NULL REFERENCES purchase_orders (id),
    delivered_on      TEXT NOT NULL,
    file_id           INTEGER REFERENCES project_files (id),
    created_by        INTEGER NOT NULL REFERENCES users (id),
    created_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    undone_by         INTEGER REFERENCES users (id),
    undone_at         TEXT,
    undo_reason       TEXT,
    CHECK ((undone_at IS NULL) = (undone_by IS NULL) AND (undone_at IS NULL) = (undo_reason IS NULL))
);
CREATE UNIQUE INDEX idx_deliveries_in_force ON deliveries (project_id, seq) WHERE undone_at IS NULL;
CREATE INDEX idx_deliveries_purchase_order ON deliveries (purchase_order_id);

CREATE TRIGGER audit_deliveries_ai AFTER INSERT ON deliveries BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('deliveries', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('project_id', new.project_id, 'seq', new.seq,
                        'purchase_order_id', new.purchase_order_id, 'delivered_on', new.delivered_on,
                        'file_id', new.file_id, 'created_by', new.created_by));
END;

CREATE TRIGGER audit_deliveries_au AFTER UPDATE ON deliveries BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('deliveries', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('project_id', old.project_id, 'seq', old.seq, 'delivered_on', old.delivered_on,
                        'file_id', old.file_id, 'undone_by', old.undone_by,
                        'undone_at', old.undone_at, 'undo_reason', old.undo_reason),
            json_object('project_id', new.project_id, 'seq', new.seq, 'delivered_on', new.delivered_on,
                        'file_id', new.file_id, 'undone_by', new.undone_by,
                        'undone_at', new.undone_at, 'undo_reason', new.undo_reason));
END;

CREATE TRIGGER audit_deliveries_ad AFTER DELETE ON deliveries BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('deliveries', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('project_id', old.project_id, 'seq', old.seq, 'delivered_on', old.delivered_on,
                        'file_id', old.file_id, 'undone_at', old.undone_at));
END;

-- Delivered under the old flow. A proyecto whose last stage before 0041 (its newest
-- audit row from before 0041 was applied) was en_entrega or cerrado had its goods
-- delivered: each of its confirmed supplier O.C.s gets a delivery, on the México City day
-- of the audit row that last moved it to en_entrega (or to cerrado, when it never went
-- through en_entrega in the log), attributed to whoever made that move (else its
-- vendedor), without a comprobante. Logged as the system's doing. Nothing matches, nothing
-- is written. México City has had no daylight saving time since 2022: UTC-6.
UPDATE audit_actor SET user_id = NULL, source = NULL WHERE id = 1;
WITH cutoff (at) AS (
    SELECT applied_at FROM schema_migrations WHERE filename = '0041_project_states.sql'
),
moves AS (
    SELECT a.id, CAST(a.row_key AS INTEGER) AS project_id, a.actor_id, a.at,
        json_extract(a.new_values, '$.status') AS status,
        CASE WHEN a.op = 'update' THEN json_extract(a.old_values, '$.status') END AS old_status,
        ROW_NUMBER() OVER (PARTITION BY a.row_key ORDER BY a.id DESC) AS newest
    FROM audit_log a, cutoff
    WHERE a.table_name = 'projects' AND a.op IN ('insert', 'update') AND a.at <= cutoff.at
),
legacy AS (
    SELECT l.project_id, COALESCE(
        (SELECT m.id FROM moves m WHERE m.project_id = l.project_id
            AND m.status = 'en_entrega' AND m.old_status IS NOT 'en_entrega' ORDER BY m.id DESC LIMIT 1),
        (SELECT m.id FROM moves m WHERE m.project_id = l.project_id
            AND m.status = 'cerrado' AND m.old_status IS NOT 'cerrado' ORDER BY m.id DESC LIMIT 1)) AS move_id
    FROM moves l
    WHERE l.newest = 1 AND l.status IN ('en_entrega', 'cerrado')
)
INSERT INTO deliveries (project_id, seq, purchase_order_id, delivered_on, created_by, created_at)
    SELECT po.project_id, po.seq, po.id, date(a.at, '-6 hours'), COALESCE(a.actor_id, p.user_id), a.at
    FROM legacy l
    JOIN audit_log a ON a.id = l.move_id
    JOIN projects p ON p.id = l.project_id
    JOIN purchase_orders po ON po.project_id = l.project_id AND po.status = 'confirmada'
    ORDER BY po.id;

-- project_states as in 0041, with entrega derived from the deliveries: sin_oc without a
-- confirmed supplier O.C. (borradores, replaced and cancelled ones don't count),
-- entregado when every confirmed one has a delivery in force for its seq, else
-- en_proceso. cerrado (Cerrado · Ganado) reads it as before.
DROP VIEW project_states;
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
        CASE
            WHEN NOT EXISTS (SELECT 1 FROM purchase_orders po WHERE po.project_id = p.id AND po.status = 'confirmada')
                THEN 'sin_oc'
            WHEN EXISTS (SELECT 1 FROM purchase_orders po WHERE po.project_id = p.id AND po.status = 'confirmada'
                    AND NOT EXISTS (SELECT 1 FROM deliveries d
                        WHERE d.project_id = po.project_id AND d.seq = po.seq AND d.undone_at IS NULL))
                THEN 'en_proceso'
            ELSE 'entregado' END AS entrega,
        COALESCE((SELECT SUM(pm.amount) FROM payments pm WHERE pm.project_id = p.id AND pm.deleted_at IS NULL), 0) AS paid,
        COALESCE((SELECT MAX(pm.paid_on) FROM payments pm WHERE pm.project_id = p.id AND pm.deleted_at IS NULL), '') AS last_paid_on
    FROM projects p
    LEFT JOIN quotes q ON q.project_id = p.id AND q.status != 'revisada'
);
