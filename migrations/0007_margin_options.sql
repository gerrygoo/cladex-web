-- M3.1: margins become a menu of named options, one chosen per quote, replacing the
-- single default_margin setting. See docs/PLAN.md, M3.
--
-- value_micros is a fraction of the sale price in micros (123400 = 12.34%), the same
-- cost / (1 - margin) convention default_margin used. Options are never deleted, only
-- retired: issued quotes snapshot the option's name and value, and audit_log still names
-- it. Exactly one active option is the default a new quote starts on; the partial
-- unique index enforces "at most one", and the store keeps it on an active option.
CREATE TABLE margin_options (
    id           INTEGER PRIMARY KEY,
    name         TEXT NOT NULL UNIQUE,
    value_micros INTEGER NOT NULL CHECK (value_micros >= 0 AND value_micros < 1000000),
    is_default   INTEGER NOT NULL DEFAULT 0 CHECK (is_default IN (0, 1)),
    retired_at   TEXT,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_by   INTEGER REFERENCES users (id)
);
CREATE UNIQUE INDEX idx_margin_options_default ON margin_options (is_default) WHERE is_default = 1;

-- Drafts follow their option live; IssueQuote freezes its name and value into the two
-- snapshot columns, so an issued quote keeps saying which margin it went out with even
-- after the option is renamed, repriced or retired.
ALTER TABLE quotes ADD COLUMN margin_option_id INTEGER REFERENCES margin_options (id);
ALTER TABLE quotes ADD COLUMN margin_name_snapshot TEXT;
ALTER TABLE quotes ADD COLUMN margin_snapshot_micros INTEGER;

CREATE TRIGGER audit_margin_options_ai AFTER INSERT ON margin_options BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('margin_options', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', new.name, 'value_micros', new.value_micros,
                        'is_default', new.is_default, 'retired_at', new.retired_at));
END;

CREATE TRIGGER audit_margin_options_au AFTER UPDATE ON margin_options BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('margin_options', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', old.name, 'value_micros', old.value_micros,
                        'is_default', old.is_default, 'retired_at', old.retired_at),
            json_object('name', new.name, 'value_micros', new.value_micros,
                        'is_default', new.is_default, 'retired_at', new.retired_at));
END;

CREATE TRIGGER audit_margin_options_ad AFTER DELETE ON margin_options BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('margin_options', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', old.name, 'value_micros', old.value_micros,
                        'is_default', old.is_default, 'retired_at', old.retired_at));
END;

-- The quotes triggers from 0004 list their columns explicitly; recreate them so a
-- change of margin on a draft, and the issue-time margin snapshot, are audited too.
DROP TRIGGER audit_quotes_ai;
DROP TRIGGER audit_quotes_au;
DROP TRIGGER audit_quotes_ad;

CREATE TRIGGER audit_quotes_ai AFTER INSERT ON quotes BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('quotes', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('folio', new.folio, 'customer_id', new.customer_id,
                        'user_id', new.user_id, 'status', new.status,
                        'currency', new.currency,
                        'fx_rate_used_micros', new.fx_rate_used_micros,
                        'subtotal', new.subtotal, 'iva', new.iva, 'total', new.total,
                        'issued_at', new.issued_at, 'valid_until', new.valid_until,
                        'supersedes_quote_id', new.supersedes_quote_id,
                        'pdf_sha256', new.pdf_sha256,
                        'margin_option_id', new.margin_option_id,
                        'margin_name_snapshot', new.margin_name_snapshot,
                        'margin_snapshot_micros', new.margin_snapshot_micros));
END;

CREATE TRIGGER audit_quotes_au AFTER UPDATE ON quotes BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('quotes', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('folio', old.folio, 'customer_id', old.customer_id,
                        'user_id', old.user_id, 'status', old.status,
                        'currency', old.currency,
                        'fx_rate_used_micros', old.fx_rate_used_micros,
                        'subtotal', old.subtotal, 'iva', old.iva, 'total', old.total,
                        'issued_at', old.issued_at, 'valid_until', old.valid_until,
                        'supersedes_quote_id', old.supersedes_quote_id,
                        'pdf_sha256', old.pdf_sha256,
                        'margin_option_id', old.margin_option_id,
                        'margin_name_snapshot', old.margin_name_snapshot,
                        'margin_snapshot_micros', old.margin_snapshot_micros),
            json_object('folio', new.folio, 'customer_id', new.customer_id,
                        'user_id', new.user_id, 'status', new.status,
                        'currency', new.currency,
                        'fx_rate_used_micros', new.fx_rate_used_micros,
                        'subtotal', new.subtotal, 'iva', new.iva, 'total', new.total,
                        'issued_at', new.issued_at, 'valid_until', new.valid_until,
                        'supersedes_quote_id', new.supersedes_quote_id,
                        'pdf_sha256', new.pdf_sha256,
                        'margin_option_id', new.margin_option_id,
                        'margin_name_snapshot', new.margin_name_snapshot,
                        'margin_snapshot_micros', new.margin_snapshot_micros));
END;

CREATE TRIGGER audit_quotes_ad AFTER DELETE ON quotes BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('quotes', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('folio', old.folio, 'customer_id', old.customer_id,
                        'user_id', old.user_id, 'status', old.status,
                        'currency', old.currency,
                        'fx_rate_used_micros', old.fx_rate_used_micros,
                        'subtotal', old.subtotal, 'iva', old.iva, 'total', old.total,
                        'issued_at', old.issued_at, 'valid_until', old.valid_until,
                        'supersedes_quote_id', old.supersedes_quote_id,
                        'pdf_sha256', old.pdf_sha256,
                        'margin_option_id', old.margin_option_id,
                        'margin_name_snapshot', old.margin_name_snapshot,
                        'margin_snapshot_micros', old.margin_snapshot_micros));
END;

-- Seed, backfill and cleanup below fire audit triggers; log them as unattributed, not
-- as whoever the app last stamped.
UPDATE audit_actor SET user_id = NULL, source = NULL WHERE id = 1;

-- The four options the user settled on (docs/PLAN.md, M3). 12.34% is today's CCA
-- margin and becomes the default; 16.56% and 20.28% are the workbook's other two CCA
-- price columns; 29.55% reproduces CCS & AC's copper-derived margin once 3.2 moves it
-- onto material cost. Names deliberately leave out the percentage, which would go
-- stale the first time an admin edits a value; the UI always shows it next to the name.
-- Admins rename them in /ajustes.
INSERT INTO margin_options (name, value_micros, is_default) VALUES
    ('Estándar', 123400, 1),
    ('Medio', 165600, 0),
    ('Alto', 202800, 0),
    ('CCS & AC', 295500, 0);

-- Existing drafts get the option matching their series; QI's old 20% has no exact
-- option, so it gets the closest. Issued quotes keep a NULL option: production quotes
-- are all test data (docs/PLAN.md, M3), and their prices are frozen either way.
UPDATE quotes SET margin_option_id = (
    SELECT id FROM margin_options WHERE value_micros = CASE quotes.prefix
        WHEN 'QS' THEN 295500
        WHEN 'QI' THEN 202800
        ELSE 123400
    END)
WHERE status = 'borrador';

DELETE FROM settings WHERE key = 'default_margin';
