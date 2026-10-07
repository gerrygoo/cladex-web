-- Estado de pago (docs/planning/hitos-proyectos-y-facturas.md, slice 4.5). Facturas are
-- still issued outside the app, so the proyecto records them by hand: the folio and day
-- of the purchase order's factura (for a P.P.D. order, the factura de anticipo), where
-- the payment stands, and the folio of the comprobante de pago and the day it was paid.
-- payment_status is NULL while nothing has been invoiced. All of it is NULL on every
-- existing proyecto: none is past oc_recibida with these on record.
--
-- With this the 'facturado' stage, in the CHECK since 0016, comes into use between
-- oc_recibida and en_entrega. No existing row changes stage.
ALTER TABLE projects ADD COLUMN invoice_ref TEXT;
ALTER TABLE projects ADD COLUMN invoice_date TEXT;
ALTER TABLE projects ADD COLUMN payment_status TEXT CHECK (payment_status IN ('facturado_anticipo', 'pagado'));
ALTER TABLE projects ADD COLUMN payment_ref TEXT;
ALTER TABLE projects ADD COLUMN paid_at TEXT;

-- The audit triggers list their columns, so recreate them with the payment's.
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
                        'payment_method', new.payment_method,
                        'invoice_ref', new.invoice_ref, 'invoice_date', new.invoice_date,
                        'payment_status', new.payment_status,
                        'payment_ref', new.payment_ref, 'paid_at', new.paid_at));
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
                        'payment_ref', old.payment_ref, 'paid_at', old.paid_at),
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
                        'payment_ref', new.payment_ref, 'paid_at', new.paid_at));
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
                        'payment_ref', old.payment_ref, 'paid_at', old.paid_at));
END;
