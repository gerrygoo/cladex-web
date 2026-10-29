-- cladex:foreign-keys-off
-- cladex:exclusive
-- Tipos de cotización (issue #53; docs/planning/tipos-de-cotizacion.md, slice T1). The
-- quote series moves off product_families into a table of its own, and the prefix text
-- stops being a key: a quote points at its type by id, the folio counter lives on the
-- type, and what used to be decided by comparing a prefix or a familia name ("QG",
-- "QL"/"QI", "CCS & AC", "CCA"...) is a flag on the type. Nothing a user sees changes.
--
-- quotes.folio and quotes.prefix stay, as what was printed: written once at creation and
-- never re-rendered, so renaming a type's prefix later never changes an existing folio.
-- folio_number is the folio's number on its own; with quote_type_id it is the folio's
-- identity (revisions share their first quote's number).
--
-- quote_types:
--   prefix              the folio prefix new quotes get (unique; the app also refuses
--                       any prefix a quote was ever issued with)
--   name                what the new-quote picker shows next to it ("Cable CCA")
--   terms               the "Términos y condiciones" block, one term per line
--   next_number         the next folio number; continues across prefix renames
--   family_id           the familia whose products this type quotes: the builder's
--                       product search defaults to it, and a factura's lines take its
--                       SAT code. Not part of anyone's identity.
--   free_lines_only     quotes hold only free lines (QL)
--   hide_address        the PDF leaves the Cladex address off its letterhead (#51)
--   sort_last           listed after the others when no sort is chosen (QI)
--   kg_priced           products are costed per kg and quoted per metre (QG, #43)
--   copper_price_margin "Otro margen" can be typed as the CCS 30% price per kg (#38)
--   letterhead          which logo block the PDF prints (#16, #47, #48, #51)
CREATE TABLE quote_types (
    id                  INTEGER PRIMARY KEY,
    prefix              TEXT NOT NULL UNIQUE,
    name                TEXT NOT NULL DEFAULT '',
    terms               TEXT NOT NULL DEFAULT '',
    next_number         INTEGER NOT NULL DEFAULT 1 CHECK (next_number >= 1),
    family_id           INTEGER REFERENCES product_families (id),
    free_lines_only     INTEGER NOT NULL DEFAULT 0 CHECK (free_lines_only IN (0, 1)),
    hide_address        INTEGER NOT NULL DEFAULT 0 CHECK (hide_address IN (0, 1)),
    sort_last           INTEGER NOT NULL DEFAULT 0 CHECK (sort_last IN (0, 1)),
    kg_priced           INTEGER NOT NULL DEFAULT 0 CHECK (kg_priced IN (0, 1)),
    copper_price_margin INTEGER NOT NULL DEFAULT 0 CHECK (copper_price_margin IN (0, 1)),
    letterhead          TEXT NOT NULL DEFAULT 'cladex' CHECK (letterhead IN
        ('cladex', 'cladex_large', 'conduccasa', 'copperclad', 'alumoclad')),
    created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- One type per familia that has a series, in the familias' order (the new-quote form's).
-- The flags reproduce exactly what the code decided from prefixes and names until now.
-- The counter is the larger of the stored sequence and one past the highest folio used,
-- so it can never hand out a folio that exists.
INSERT INTO quote_types (prefix, name, terms, next_number, family_id, free_lines_only,
        hide_address, sort_last, kg_priced, copper_price_margin, letterhead, created_at)
SELECT f.series, COALESCE(f.series_label, ''), f.terms,
    max(COALESCE((SELECT next_number FROM folio_sequences s WHERE s.prefix = f.series), 1),
        COALESCE((SELECT max(CAST(substr(q.folio, length(f.series) + 1, 4) AS INTEGER)) + 1
                  FROM quotes q WHERE q.prefix = f.series), 1)),
    f.id, f.free_lines_only,
    f.series IN ('QL', 'QI'),
    f.series = 'QI',
    f.series = 'QG',
    f.name = 'CCS & AC',
    CASE
        WHEN f.series IN ('QL', 'QI') THEN 'cladex_large'
        WHEN f.name = 'CCA' THEN 'conduccasa'
        WHEN f.name = 'CCS & AC' THEN 'copperclad'
        WHEN f.name = 'Alumoclad' THEN 'alumoclad'
        ELSE 'cladex'
    END,
    f.created_at
FROM product_families f
WHERE f.series IS NOT NULL
ORDER BY f.id;

-- quotes is rebuilt for the NOT NULL quote_type_id, the same way as in 0013 and 0016:
-- quote_lines, quote_comments, supplier quotes and comprobantes reference it, so the
-- runner applies this file with foreign keys off (line 1) and checks them before
-- committing. The prefix join below is the last one that will ever run.
CREATE TABLE quotes_new (
    id                  INTEGER PRIMARY KEY,
    folio               TEXT NOT NULL UNIQUE,
    prefix              TEXT NOT NULL, -- as printed in folio; never joined on
    quote_type_id       INTEGER NOT NULL REFERENCES quote_types (id),
    folio_number        INTEGER NOT NULL CHECK (folio_number >= 1),
    customer_id         INTEGER NOT NULL REFERENCES customers (id),
    user_id             INTEGER NOT NULL REFERENCES users (id),
    status              TEXT NOT NULL CHECK (status IN ('borrador', 'emitida', 'revisada')),
    subtotal            INTEGER NOT NULL DEFAULT 0, -- centavos
    iva                 INTEGER NOT NULL DEFAULT 0, -- centavos
    total               INTEGER NOT NULL DEFAULT 0, -- centavos
    terms_snapshot      TEXT, -- the type's terms block, frozen at issue time
    created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    issued_at           TEXT, -- set once; freezes the row
    valid_until         TEXT,
    supersedes_quote_id INTEGER REFERENCES quotes (id),
    pdf_sha256          TEXT,
    customer_name_snapshot TEXT,
    vendedor_snapshot   TEXT,
    margin_option_id    INTEGER REFERENCES margin_options (id),
    margin_name_snapshot TEXT,
    margin_snapshot_micros INTEGER,
    custom_margin_micros INTEGER
        CHECK (custom_margin_micros IS NULL OR (custom_margin_micros >= 0 AND custom_margin_micros < 1000000)),
    delivery_time       TEXT,
    currency            TEXT CHECK (currency IN ('MXN', 'USD')),
    project_id          INTEGER REFERENCES projects (id), -- NULL until the quote is issued
    extra_terms         TEXT,
    custom_copper_price_micros INTEGER
        CHECK (custom_copper_price_micros IS NULL OR custom_copper_price_micros > 0)
);
INSERT INTO quotes_new (id, folio, prefix, quote_type_id, folio_number, customer_id, user_id,
        status, subtotal, iva, total, terms_snapshot, created_at, issued_at, valid_until,
        supersedes_quote_id, pdf_sha256, customer_name_snapshot, vendedor_snapshot,
        margin_option_id, margin_name_snapshot, margin_snapshot_micros, custom_margin_micros,
        delivery_time, currency, project_id, extra_terms, custom_copper_price_micros)
    SELECT q.id, q.folio, q.prefix, t.id, CAST(substr(q.folio, length(q.prefix) + 1, 4) AS INTEGER),
        q.customer_id, q.user_id, q.status, q.subtotal, q.iva, q.total, q.terms_snapshot,
        q.created_at, q.issued_at, q.valid_until, q.supersedes_quote_id, q.pdf_sha256,
        q.customer_name_snapshot, q.vendedor_snapshot, q.margin_option_id,
        q.margin_name_snapshot, q.margin_snapshot_micros, q.custom_margin_micros,
        q.delivery_time, q.currency, q.project_id, q.extra_terms, q.custom_copper_price_micros
    FROM quotes q LEFT JOIN quote_types t ON t.prefix = q.prefix
    ORDER BY q.id;
DROP TABLE quotes;
ALTER TABLE quotes_new RENAME TO quotes;
CREATE INDEX idx_quotes_customer_id ON quotes (customer_id);
CREATE INDEX idx_quotes_user_id ON quotes (user_id);
CREATE INDEX idx_quotes_status ON quotes (status);
CREATE INDEX idx_quotes_quote_type_id ON quotes (quote_type_id);
CREATE UNIQUE INDEX idx_quotes_live_per_project ON quotes (project_id)
    WHERE project_id IS NOT NULL AND status != 'revisada';
-- A folio's identity: its type and number. Revisions repeat their first quote's number.
CREATE UNIQUE INDEX idx_quotes_type_number ON quotes (quote_type_id, folio_number)
    WHERE supersedes_quote_id IS NULL;

-- The audit triggers went with the old table; they come back with quote_type_id.
CREATE TRIGGER audit_quotes_ai AFTER INSERT ON quotes BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('quotes', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('folio', new.folio, 'quote_type_id', new.quote_type_id,
                        'customer_id', new.customer_id,
                        'user_id', new.user_id, 'status', new.status,
                        'subtotal', new.subtotal, 'iva', new.iva, 'total', new.total,
                        'issued_at', new.issued_at, 'valid_until', new.valid_until,
                        'supersedes_quote_id', new.supersedes_quote_id,
                        'pdf_sha256', new.pdf_sha256,
                        'margin_option_id', new.margin_option_id,
                        'margin_name_snapshot', new.margin_name_snapshot,
                        'margin_snapshot_micros', new.margin_snapshot_micros,
                        'custom_margin_micros', new.custom_margin_micros,
                        'delivery_time', new.delivery_time,
                        'currency', new.currency,
                        'project_id', new.project_id));
END;

CREATE TRIGGER audit_quotes_au AFTER UPDATE ON quotes BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('quotes', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('folio', old.folio, 'quote_type_id', old.quote_type_id,
                        'customer_id', old.customer_id,
                        'user_id', old.user_id, 'status', old.status,
                        'subtotal', old.subtotal, 'iva', old.iva, 'total', old.total,
                        'issued_at', old.issued_at, 'valid_until', old.valid_until,
                        'supersedes_quote_id', old.supersedes_quote_id,
                        'pdf_sha256', old.pdf_sha256,
                        'margin_option_id', old.margin_option_id,
                        'margin_name_snapshot', old.margin_name_snapshot,
                        'margin_snapshot_micros', old.margin_snapshot_micros,
                        'custom_margin_micros', old.custom_margin_micros,
                        'delivery_time', old.delivery_time,
                        'currency', old.currency,
                        'project_id', old.project_id),
            json_object('folio', new.folio, 'quote_type_id', new.quote_type_id,
                        'customer_id', new.customer_id,
                        'user_id', new.user_id, 'status', new.status,
                        'subtotal', new.subtotal, 'iva', new.iva, 'total', new.total,
                        'issued_at', new.issued_at, 'valid_until', new.valid_until,
                        'supersedes_quote_id', new.supersedes_quote_id,
                        'pdf_sha256', new.pdf_sha256,
                        'margin_option_id', new.margin_option_id,
                        'margin_name_snapshot', new.margin_name_snapshot,
                        'margin_snapshot_micros', new.margin_snapshot_micros,
                        'custom_margin_micros', new.custom_margin_micros,
                        'delivery_time', new.delivery_time,
                        'currency', new.currency,
                        'project_id', new.project_id));
END;

CREATE TRIGGER audit_quotes_ad AFTER DELETE ON quotes BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('quotes', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('folio', old.folio, 'quote_type_id', old.quote_type_id,
                        'customer_id', old.customer_id,
                        'user_id', old.user_id, 'status', old.status,
                        'subtotal', old.subtotal, 'iva', old.iva, 'total', old.total,
                        'issued_at', old.issued_at, 'valid_until', old.valid_until,
                        'supersedes_quote_id', old.supersedes_quote_id,
                        'pdf_sha256', old.pdf_sha256,
                        'margin_option_id', old.margin_option_id,
                        'margin_name_snapshot', old.margin_name_snapshot,
                        'margin_snapshot_micros', old.margin_snapshot_micros,
                        'custom_margin_micros', old.custom_margin_micros,
                        'delivery_time', old.delivery_time,
                        'currency', old.currency,
                        'project_id', old.project_id));
END;

-- The series columns now live on quote_types. free_lines_only stays on the familia too:
-- there it means the familia holds no catalog products, so the product form leaves it
-- out (the QL familia also keeps its SAT code, which QL facturas use).
DROP INDEX idx_product_families_series;
ALTER TABLE product_families DROP COLUMN series;
ALTER TABLE product_families DROP COLUMN series_label;
ALTER TABLE product_families DROP COLUMN terms;

-- folio_sequences now only holds CFDI counters ("cfdi:" + serie); they move to a table
-- of their own, keyed by the serie as registered with the PAC.
CREATE TABLE cfdi_folio_sequences (
    serie       TEXT PRIMARY KEY,
    next_number INTEGER NOT NULL DEFAULT 1
);
INSERT INTO cfdi_folio_sequences (serie, next_number)
    SELECT substr(prefix, 6), next_number FROM folio_sequences WHERE prefix LIKE 'cfdi:%';
DROP TABLE folio_sequences;
