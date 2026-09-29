-- Follow-up stages after a quote is issued (issue #11): the client shows interest
-- (pipeline), sends the purchase order and it goes to the manufacturer (oc_emitida),
-- the cable is delivered (entregada), and finally invoiced and collected (cerrada).
-- Borrador, emitida and revisada are unchanged; a revision still supersedes an emitida.
--
-- SQLite can't alter a CHECK constraint, so quotes is rebuilt with the wider list: park
-- the rows, drop the table, recreate it under its own name, put the rows back. Dropping
-- it counts every row of quote_lines, quote_comments and supersedes_quote_id as a
-- dangling reference; those are checked at commit instead, and inserting the parent
-- rows back under the same name is what clears them (a rename would not).
PRAGMA defer_foreign_keys = ON;

CREATE TEMP TABLE quotes_parked AS SELECT * FROM quotes ORDER BY id;
DROP TABLE quotes;

CREATE TABLE quotes (
    id                  INTEGER PRIMARY KEY,
    folio               TEXT NOT NULL UNIQUE,
    prefix              TEXT NOT NULL CHECK (prefix IN ('QA', 'QS', 'QI')),
    customer_id         INTEGER NOT NULL REFERENCES customers (id),
    user_id             INTEGER NOT NULL REFERENCES users (id),
    status              TEXT NOT NULL CHECK (status IN
        ('borrador', 'emitida', 'revisada', 'pipeline', 'oc_emitida', 'entregada', 'cerrada')),
    subtotal            INTEGER NOT NULL DEFAULT 0, -- centavos
    iva                 INTEGER NOT NULL DEFAULT 0, -- centavos
    total               INTEGER NOT NULL DEFAULT 0, -- centavos
    terms_snapshot      TEXT, -- per-family terms block, frozen at issue time
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
        CHECK (custom_margin_micros IS NULL OR (custom_margin_micros >= 0 AND custom_margin_micros < 1000000))
);

INSERT INTO quotes (id, folio, prefix, customer_id, user_id, status, subtotal, iva, total,
        terms_snapshot, created_at, issued_at, valid_until, supersedes_quote_id, pdf_sha256,
        customer_name_snapshot, vendedor_snapshot, margin_option_id, margin_name_snapshot,
        margin_snapshot_micros, custom_margin_micros)
SELECT id, folio, prefix, customer_id, user_id, status, subtotal, iva, total,
        terms_snapshot, created_at, issued_at, valid_until, supersedes_quote_id, pdf_sha256,
        customer_name_snapshot, vendedor_snapshot, margin_option_id, margin_name_snapshot,
        margin_snapshot_micros, custom_margin_micros
FROM quotes_parked ORDER BY id;
DROP TABLE quotes_parked;

-- The audit triggers went with the old table and come back only now, so the copied rows
-- are not logged as inserts.
CREATE INDEX idx_quotes_customer_id ON quotes (customer_id);
CREATE INDEX idx_quotes_user_id ON quotes (user_id);
CREATE INDEX idx_quotes_status ON quotes (status);

CREATE TRIGGER audit_quotes_ai AFTER INSERT ON quotes BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('quotes', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('folio', new.folio, 'customer_id', new.customer_id,
                        'user_id', new.user_id, 'status', new.status,
                        'subtotal', new.subtotal, 'iva', new.iva, 'total', new.total,
                        'issued_at', new.issued_at, 'valid_until', new.valid_until,
                        'supersedes_quote_id', new.supersedes_quote_id,
                        'pdf_sha256', new.pdf_sha256,
                        'margin_option_id', new.margin_option_id,
                        'margin_name_snapshot', new.margin_name_snapshot,
                        'margin_snapshot_micros', new.margin_snapshot_micros,
                        'custom_margin_micros', new.custom_margin_micros));
END;

CREATE TRIGGER audit_quotes_au AFTER UPDATE ON quotes BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('quotes', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('folio', old.folio, 'customer_id', old.customer_id,
                        'user_id', old.user_id, 'status', old.status,
                        'subtotal', old.subtotal, 'iva', old.iva, 'total', old.total,
                        'issued_at', old.issued_at, 'valid_until', old.valid_until,
                        'supersedes_quote_id', old.supersedes_quote_id,
                        'pdf_sha256', old.pdf_sha256,
                        'margin_option_id', old.margin_option_id,
                        'margin_name_snapshot', old.margin_name_snapshot,
                        'margin_snapshot_micros', old.margin_snapshot_micros,
                        'custom_margin_micros', old.custom_margin_micros),
            json_object('folio', new.folio, 'customer_id', new.customer_id,
                        'user_id', new.user_id, 'status', new.status,
                        'subtotal', new.subtotal, 'iva', new.iva, 'total', new.total,
                        'issued_at', new.issued_at, 'valid_until', new.valid_until,
                        'supersedes_quote_id', new.supersedes_quote_id,
                        'pdf_sha256', new.pdf_sha256,
                        'margin_option_id', new.margin_option_id,
                        'margin_name_snapshot', new.margin_name_snapshot,
                        'margin_snapshot_micros', new.margin_snapshot_micros,
                        'custom_margin_micros', new.custom_margin_micros));
END;

CREATE TRIGGER audit_quotes_ad AFTER DELETE ON quotes BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('quotes', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('folio', old.folio, 'customer_id', old.customer_id,
                        'user_id', old.user_id, 'status', old.status,
                        'subtotal', old.subtotal, 'iva', old.iva, 'total', old.total,
                        'issued_at', old.issued_at, 'valid_until', old.valid_until,
                        'supersedes_quote_id', old.supersedes_quote_id,
                        'pdf_sha256', old.pdf_sha256,
                        'margin_option_id', old.margin_option_id,
                        'margin_name_snapshot', old.margin_name_snapshot,
                        'margin_snapshot_micros', old.margin_snapshot_micros,
                        'custom_margin_micros', old.custom_margin_micros));
END;
