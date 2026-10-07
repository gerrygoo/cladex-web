-- Perdido (docs/planning/hitos-proyectos-y-facturas.md, slice 4.2). A proyecto can be
-- marked lost while it is a prospecto or has its purchase order in. projects.status
-- already accepts 'perdido' (0016); these columns hold why, when, and the stage it was
-- lost from, which is where an admin's reopen sends it back to. They are NULL on any
-- proyecto that isn't lost.
--
-- There is no lost status on quotes: losing a quote and losing its proyecto are the same
-- thing, so it is stored once, here.
ALTER TABLE projects ADD COLUMN lost_reason TEXT;
ALTER TABLE projects ADD COLUMN lost_from TEXT CHECK (lost_from IN ('prospecto', 'oc_recibida'));
ALTER TABLE projects ADD COLUMN lost_at TEXT;

-- The audit triggers list their columns, so recreate them with the reason and stage.
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
                        'lost_reason', new.lost_reason, 'lost_from', new.lost_from));
END;

CREATE TRIGGER audit_projects_au AFTER UPDATE ON projects BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('projects', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('folio', old.folio, 'customer_id', old.customer_id,
                        'user_id', old.user_id, 'status', old.status,
                        'probability', old.probability,
                        'lost_reason', old.lost_reason, 'lost_from', old.lost_from),
            json_object('folio', new.folio, 'customer_id', new.customer_id,
                        'user_id', new.user_id, 'status', new.status,
                        'probability', new.probability,
                        'lost_reason', new.lost_reason, 'lost_from', new.lost_from));
END;

CREATE TRIGGER audit_projects_ad AFTER DELETE ON projects BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('projects', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('folio', old.folio, 'customer_id', old.customer_id,
                        'user_id', old.user_id, 'status', old.status,
                        'probability', old.probability,
                        'lost_reason', old.lost_reason, 'lost_from', old.lost_from));
END;
