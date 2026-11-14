-- Nombre del proyecto (issue #73): an optional free-text name, up to 40 characters
-- (checked by the app), to tell a client's proyectos apart at a glance, e.g.
-- "QCCA0053 · Capuchinas". NULL when it has none. Any user may set or change it at any
-- time; the audit triggers now record it.
--
-- Additive: the previous build doesn't read the column, so this isn't exclusive.
ALTER TABLE projects ADD COLUMN name TEXT;

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
                        'payment_ref', new.payment_ref, 'paid_at', new.paid_at,
                        'cancel_reason', new.cancel_reason, 'cancelled_from', new.cancelled_from,
                        'name', new.name));
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
                        'cancel_reason', old.cancel_reason, 'cancelled_from', old.cancelled_from,
                        'name', old.name),
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
                        'cancel_reason', new.cancel_reason, 'cancelled_from', new.cancelled_from,
                        'name', new.name));
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
                        'cancel_reason', old.cancel_reason, 'cancelled_from', old.cancelled_from,
                        'name', old.name));
END;
