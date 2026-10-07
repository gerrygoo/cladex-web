-- cladex:foreign-keys-off
-- Proyectos (docs/planning/hitos-proyectos-y-facturas.md, slice 4.1). What happens to a
-- quote after it is issued stops being more values of quotes.status and becomes a row of
-- its own: a proyecto opens when a quote is issued, keeps the base folio of that quote
-- (QA0105) as its name, and is followed by every revision of it (QA0105-R1, -R2...). The
-- stage lives on the proyecto, so quotes.status goes back to borrador / emitida /
-- revisada.
--
-- A proyecto's current quote is not stored: it is the one quote of the proyecto that is
-- not 'revisada' (the emitida one, or the borrador revision being worked on). The unique
-- index at the end keeps it to one.
--
-- probability is the probabilidad de cierre, one of five fixed steps. It only means
-- something while the proyecto is a prospecto, and is kept afterwards so a proyecto sent
-- back to prospecto shows what it had. status lists all six stages of the plan although
-- only four are in use yet (facturado and perdido arrive with later slices), because
-- SQLite can't widen a CHECK without rebuilding the table.
CREATE TABLE projects (
    id          INTEGER PRIMARY KEY,
    folio       TEXT NOT NULL UNIQUE,
    customer_id INTEGER NOT NULL REFERENCES customers (id),
    user_id     INTEGER NOT NULL REFERENCES users (id),
    status      TEXT NOT NULL DEFAULT 'prospecto' CHECK (status IN
        ('prospecto', 'oc_recibida', 'facturado', 'en_entrega', 'cerrado', 'perdido')),
    probability INTEGER NOT NULL DEFAULT 10 CHECK (probability IN (10, 25, 50, 75, 90)),
    probability_updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_projects_customer_id ON projects (customer_id);
CREATE INDEX idx_projects_user_id ON projects (user_id);
CREATE INDEX idx_projects_status ON projects (status);

-- Every quote with the first quote of its revision chain (the one that revises nothing).
CREATE TEMP TABLE quote_roots AS
WITH RECURSIVE chain (quote_id, root_id) AS (
    SELECT id, id FROM quotes WHERE supersedes_quote_id IS NULL
    UNION ALL
    SELECT q.id, chain.root_id FROM quotes q JOIN chain ON q.supersedes_quote_id = chain.quote_id
)
SELECT quote_id, root_id FROM chain;

-- One proyecto per chain that was ever issued (a first quote still in borrador has no
-- proyecto yet). Its stage comes from the chain's live quote: the old emitida and
-- pipeline both become prospecto, told apart by the probability (pipeline is Alta, 75,
-- so it stays relevante para pronóstico), and the later stages map one to one.
INSERT INTO projects (id, folio, customer_id, user_id, status, probability, probability_updated_at, created_at)
SELECT root.id, root.folio, root.customer_id, root.user_id,
    CASE live.status
        WHEN 'oc_emitida' THEN 'oc_recibida'
        WHEN 'entregada' THEN 'en_entrega'
        WHEN 'cerrada' THEN 'cerrado'
        ELSE 'prospecto'
    END,
    CASE live.status WHEN 'pipeline' THEN 75 ELSE 10 END,
    COALESCE(root.issued_at, root.created_at),
    COALESCE(root.issued_at, root.created_at)
FROM quotes root
JOIN quote_roots r ON r.root_id = root.id
JOIN quotes live ON live.id = r.quote_id AND live.status != 'revisada'
WHERE root.supersedes_quote_id IS NULL AND root.status != 'borrador'
ORDER BY root.id;

-- quotes is rebuilt for the narrower status CHECK and the new project_id, the same way
-- as in 0013: quote_lines and quote_comments reference it, so the runner applies this
-- file with foreign keys off (the marker on line 1) and checks them before committing.
CREATE TABLE quotes_new (
    id                  INTEGER PRIMARY KEY,
    folio               TEXT NOT NULL UNIQUE,
    prefix              TEXT NOT NULL,
    customer_id         INTEGER NOT NULL REFERENCES customers (id),
    user_id             INTEGER NOT NULL REFERENCES users (id),
    status              TEXT NOT NULL CHECK (status IN ('borrador', 'emitida', 'revisada')),
    subtotal            INTEGER NOT NULL DEFAULT 0, -- centavos
    iva                 INTEGER NOT NULL DEFAULT 0, -- centavos
    total               INTEGER NOT NULL DEFAULT 0, -- centavos
    terms_snapshot      TEXT, -- the series' terms block, frozen at issue time
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
    project_id          INTEGER REFERENCES projects (id) -- NULL until the quote is issued
);
INSERT INTO quotes_new (id, folio, prefix, customer_id, user_id, status, subtotal, iva, total,
        terms_snapshot, created_at, issued_at, valid_until, supersedes_quote_id, pdf_sha256,
        customer_name_snapshot, vendedor_snapshot, margin_option_id, margin_name_snapshot,
        margin_snapshot_micros, custom_margin_micros, delivery_time, currency, project_id)
    SELECT q.id, q.folio, q.prefix, q.customer_id, q.user_id,
        CASE WHEN q.status IN ('pipeline', 'oc_emitida', 'entregada', 'cerrada') THEN 'emitida' ELSE q.status END,
        q.subtotal, q.iva, q.total,
        q.terms_snapshot, q.created_at, q.issued_at, q.valid_until, q.supersedes_quote_id, q.pdf_sha256,
        q.customer_name_snapshot, q.vendedor_snapshot, q.margin_option_id, q.margin_name_snapshot,
        q.margin_snapshot_micros, q.custom_margin_micros, q.delivery_time, q.currency,
        (SELECT p.id FROM projects p JOIN quote_roots r ON r.root_id = p.id WHERE r.quote_id = q.id)
    FROM quotes q ORDER BY q.id;
DROP TABLE quotes;
ALTER TABLE quotes_new RENAME TO quotes;
DROP TABLE quote_roots;
CREATE INDEX idx_quotes_customer_id ON quotes (customer_id);
CREATE INDEX idx_quotes_user_id ON quotes (user_id);
CREATE INDEX idx_quotes_status ON quotes (status);
CREATE UNIQUE INDEX idx_quotes_live_per_project ON quotes (project_id)
    WHERE project_id IS NOT NULL AND status != 'revisada';

-- The audit triggers went with the old quotes table and come back only now, with
-- project_id, so the copied rows are not logged as inserts. The same goes for the
-- proyectos created above.
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
            json_object('folio', old.folio, 'customer_id', old.customer_id,
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
            json_object('folio', new.folio, 'customer_id', new.customer_id,
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
            json_object('folio', old.folio, 'customer_id', old.customer_id,
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

CREATE TRIGGER audit_projects_ai AFTER INSERT ON projects BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('projects', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('folio', new.folio, 'customer_id', new.customer_id,
                        'user_id', new.user_id, 'status', new.status,
                        'probability', new.probability));
END;

CREATE TRIGGER audit_projects_au AFTER UPDATE ON projects BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('projects', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('folio', old.folio, 'customer_id', old.customer_id,
                        'user_id', old.user_id, 'status', old.status,
                        'probability', old.probability),
            json_object('folio', new.folio, 'customer_id', new.customer_id,
                        'user_id', new.user_id, 'status', new.status,
                        'probability', new.probability));
END;

CREATE TRIGGER audit_projects_ad AFTER DELETE ON projects BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('projects', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('folio', old.folio, 'customer_id', old.customer_id,
                        'user_id', old.user_id, 'status', old.status,
                        'probability', old.probability));
END;
