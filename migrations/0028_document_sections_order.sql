-- Orden de las secciones de documentos oficiales (issue #44): the page lists sections by
-- position instead of by when they were created, and admins move a section up or down.
-- A new section goes last.
ALTER TABLE document_sections ADD COLUMN position INTEGER NOT NULL DEFAULT 0;

-- The order Emilio asked for. Names are matched loosely (case, spaces, dashes and the
-- accent on "Educación" ignored) since they were typed in the app; any other section
-- keeps its creation order after these. Removed sections get a position too, so one
-- brought back by hand doesn't jump to the top. This runs before the audit triggers are
-- replaced below, so seeding the column doesn't write audit rows.
DROP TRIGGER audit_document_sections_ai;
DROP TRIGGER audit_document_sections_au;
DROP TRIGGER audit_document_sections_ad;
UPDATE document_sections SET position =
    CASE lower(replace(replace(replace(replace(replace(name, ' ', ''), '-', ''), '–', ''), 'ó', 'o'), 'Ó', 'o'))
        WHEN 'documentoscladex' THEN 1
        WHEN 'educacion' THEN 2
        WHEN 'rodolfodocumentoscomerciales' THEN 3
        WHEN 'gerardodocumentoscomerciales' THEN 4
        WHEN 'documentoscomercialesotros' THEN 5
        ELSE 100 + id
    END;

-- The triggers of 0027, now with the position: reordering is an edit like renaming.
CREATE TRIGGER audit_document_sections_ai AFTER INSERT ON document_sections BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('document_sections', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', new.name, 'position', new.position, 'deleted_at', new.deleted_at));
END;

CREATE TRIGGER audit_document_sections_au AFTER UPDATE ON document_sections BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('document_sections', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', old.name, 'position', old.position, 'deleted_at', old.deleted_at),
            json_object('name', new.name, 'position', new.position, 'deleted_at', new.deleted_at));
END;

CREATE TRIGGER audit_document_sections_ad AFTER DELETE ON document_sections BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('document_sections', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', old.name, 'position', old.position, 'deleted_at', old.deleted_at));
END;
