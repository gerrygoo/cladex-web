-- Seguimiento del prospecto (docs/planning/hitos-proyectos-y-facturas.md, slice 4.3).
-- Two dates a salesperson keeps on a prospecto, both optional and both plain days
-- (YYYY-MM-DD, no time): when the client's purchase order is expected, which orders the
-- Pronóstico view, and when to follow up next, which the app flags once it is due.
ALTER TABLE projects ADD COLUMN expected_oc_date TEXT;
ALTER TABLE projects ADD COLUMN next_followup_date TEXT;

-- The audit triggers list their columns, so recreate them with the two dates.
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
                        'next_followup_date', new.next_followup_date));
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
                        'next_followup_date', old.next_followup_date),
            json_object('folio', new.folio, 'customer_id', new.customer_id,
                        'user_id', new.user_id, 'status', new.status,
                        'probability', new.probability,
                        'lost_reason', new.lost_reason, 'lost_from', new.lost_from,
                        'expected_oc_date', new.expected_oc_date,
                        'next_followup_date', new.next_followup_date));
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
                        'next_followup_date', old.next_followup_date));
END;
