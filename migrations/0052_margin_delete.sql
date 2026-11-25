-- #86: an admin deletes a retired margin, and it leaves Márgenes for good. The row stays
-- (deleted_at), since quotes issued with it point at it; their name and value were frozen
-- at issue anyway. Its name is free again in its scope, so the unique index skips
-- deleted rows. Drafts still on it are moved to "Otro margen" with its value by the app.
ALTER TABLE margin_options ADD COLUMN deleted_at TEXT;

DROP INDEX idx_margin_options_name;
CREATE UNIQUE INDEX idx_margin_options_name ON margin_options (COALESCE(quote_type_id, 0), name)
    WHERE deleted_at IS NULL;

DROP TRIGGER audit_margin_options_au;
CREATE TRIGGER audit_margin_options_au AFTER UPDATE ON margin_options BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('margin_options', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('quote_type_id', old.quote_type_id, 'name', old.name,
                        'value_micros', old.value_micros, 'is_default', old.is_default,
                        'retired_at', old.retired_at, 'deleted_at', old.deleted_at),
            json_object('quote_type_id', new.quote_type_id, 'name', new.name,
                        'value_micros', new.value_micros, 'is_default', new.is_default,
                        'retired_at', new.retired_at, 'deleted_at', new.deleted_at));
END;
