-- cladex:foreign-keys-off
-- cladex:exclusive
-- Márgenes por tipo de cotización (#82, slice L1 of docs/planning/leads-y-listas-de-precios.md).
-- A margin option now belongs to a quote type (quote_type_id), next to its terms, or to
-- no type (NULL): the "generales", which any type without margins of its own uses. Names
-- are unique within a type, and each type (and the generales) has at most one default.
--
-- Every type with catalog products (not solo líneas libres) gets a copy of the active
-- generales, same names, values and default, so no price changes today. Drafts of those
-- types move to their type's copy of the option they were on; issued quotes keep pointing
-- at the general option, and their snapshots say what they went out with anyway.
--
-- The table is rebuilt for the new unique constraints; quotes reference it, hence
-- foreign keys off. The old build lists every option, so it can't serve this schema.
CREATE TABLE margin_options_new (
    id            INTEGER PRIMARY KEY,
    quote_type_id INTEGER REFERENCES quote_types (id),
    name          TEXT NOT NULL,
    value_micros  INTEGER NOT NULL CHECK (value_micros >= 0 AND value_micros < 1000000),
    is_default    INTEGER NOT NULL DEFAULT 0 CHECK (is_default IN (0, 1)),
    retired_at    TEXT,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_by    INTEGER REFERENCES users (id)
);

INSERT INTO margin_options_new (id, quote_type_id, name, value_micros, is_default, retired_at,
        created_at, updated_at, updated_by)
SELECT id, NULL, name, value_micros, is_default, retired_at, created_at, updated_at, updated_by
FROM margin_options;

DROP TABLE margin_options;
ALTER TABLE margin_options_new RENAME TO margin_options;

CREATE UNIQUE INDEX idx_margin_options_name ON margin_options (COALESCE(quote_type_id, 0), name);
CREATE UNIQUE INDEX idx_margin_options_default ON margin_options (COALESCE(quote_type_id, 0))
    WHERE is_default = 1;

INSERT INTO margin_options (quote_type_id, name, value_micros, is_default)
SELECT t.id, m.name, m.value_micros, m.is_default
FROM quote_types t, margin_options m
WHERE t.free_lines_only = 0 AND m.quote_type_id IS NULL AND m.retired_at IS NULL
ORDER BY t.id, m.id;

UPDATE quotes SET margin_option_id = (
    SELECT c.id FROM margin_options c, margin_options g
    WHERE g.id = quotes.margin_option_id AND c.quote_type_id = quotes.quote_type_id
      AND c.name = g.name)
WHERE status = 'borrador'
  AND EXISTS (
    SELECT 1 FROM margin_options c, margin_options g
    WHERE g.id = quotes.margin_option_id AND c.quote_type_id = quotes.quote_type_id
      AND c.name = g.name);

CREATE TRIGGER audit_margin_options_ai AFTER INSERT ON margin_options BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('margin_options', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('quote_type_id', new.quote_type_id, 'name', new.name,
                        'value_micros', new.value_micros, 'is_default', new.is_default,
                        'retired_at', new.retired_at));
END;

CREATE TRIGGER audit_margin_options_au AFTER UPDATE ON margin_options BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('margin_options', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('quote_type_id', old.quote_type_id, 'name', old.name,
                        'value_micros', old.value_micros, 'is_default', old.is_default,
                        'retired_at', old.retired_at),
            json_object('quote_type_id', new.quote_type_id, 'name', new.name,
                        'value_micros', new.value_micros, 'is_default', new.is_default,
                        'retired_at', new.retired_at));
END;

CREATE TRIGGER audit_margin_options_ad AFTER DELETE ON margin_options BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('margin_options', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('quote_type_id', old.quote_type_id, 'name', old.name,
                        'value_micros', old.value_micros, 'is_default', old.is_default,
                        'retired_at', old.retired_at));
END;
