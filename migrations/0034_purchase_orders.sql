-- Órdenes de compra a proveedor (issue #65; docs/planning/ordenes-de-compra-a-proveedores.md
-- and estados-del-proyecto.md). Once the client's O.C. is in, the proyecto generates
-- Cladex's own O.C. to the supplier from its current quote, at cost (no margin), as a
-- borrador; an admin confirms it, which numbers it and freezes what its PDF prints.
--
-- purchase_orders:
--   project_id      the proyecto it buys for
--   quote_id        the proyecto's quote the lines come from (its current one, issued;
--                   past prospecto it can't be revised, so it never changes under it)
--   supersedes_id   on a revision, the confirmed O.C. it corrects
--   seq             which O.C. of the proyecto it is: 1, or 2, 3... on QL, printed as a
--                   -2, -3 suffix. NULL on a new borrador until it is confirmed; a
--                   revision keeps its original's
--   revision        0, or n for the -Rn revision
--   number          OC-<proyecto>-<DDMMAA>[-seq][-Rn], set on confirming. The date is
--                   the first confirmation's (México City day); a revision keeps it
--   status          borrador, confirmada, or reemplazada once a confirmed revision
--                   replaces it. cancelada is reserved for cancelling a confirmed O.C.,
--                   which isn't built yet: it's in the CHECK now so adding it later
--                   needs no rebuild
--   supplier_id     who it is bought from; may be empty on a borrador, required to
--                   confirm (the app checks)
--   delivery_address, delivery_references  dirección de entrega; the address is required
--                   to confirm
--   currency        MXN or USD: only which condition is printed, nothing is converted
--   payment_form    transferencia (por adelantado), contado, credito (credit_days
--                   required) or otra (payment_other, its printed text, required)
--   subtotal, iva, total  centavos, from the lines
--   revision_reason the motivo an admin gave for a revision (also in the history)
--   created_by/at, confirmed_by/at  who generated and who confirmed it, and when
--   document        a confirmed O.C.'s PDF content as JSON, frozen at confirmation:
--                   reprints render from it, never from the live supplier, settings or
--                   proyecto. pdf_sha256 is that PDF's hash, as for quotes
--
-- purchase_order_lines: the partidas, one per quote line chosen (quote_line_no is its
-- partida in the quote), at cost without margin. unit_price_micros is that cost per
-- unit, rounded to the centavo as printed; on the types priced by weight (kg columns)
-- it is price_per_kg × kg_per_unit, rounded. line_total is quantity × unit price. They
-- are recomputed from the quote's frozen pricing inputs while the O.C. is a borrador
-- and fixed once confirmed. Not audited, like quote_lines: the O.C.'s own row is, and
-- its totals move with them.
--
-- Settings: the "Datos de facturación" block an O.C. prints. RFC and régimen are the
-- emisor's (emisor_rfc, emisor_regimen); the razón social with its régimen de capital,
-- the domicilio fiscal and the uso de CFDI are new keys. All are seeded with Cladex's
-- data only where nothing is set.
--
-- Only adds tables, indexes, triggers and settings rows, so the previous build keeps
-- working while this applies: not exclusive.
CREATE TABLE purchase_orders (
    id                  INTEGER PRIMARY KEY,
    project_id          INTEGER NOT NULL REFERENCES projects (id),
    quote_id            INTEGER NOT NULL REFERENCES quotes (id),
    supersedes_id       INTEGER REFERENCES purchase_orders (id),
    seq                 INTEGER CHECK (seq IS NULL OR seq >= 1),
    revision            INTEGER NOT NULL DEFAULT 0 CHECK (revision >= 0),
    number              TEXT UNIQUE,
    status              TEXT NOT NULL DEFAULT 'borrador'
        CHECK (status IN ('borrador', 'confirmada', 'reemplazada', 'cancelada')),
    supplier_id         INTEGER REFERENCES suppliers (id),
    delivery_address    TEXT NOT NULL DEFAULT '',
    delivery_references TEXT NOT NULL DEFAULT '',
    currency            TEXT NOT NULL DEFAULT 'MXN' CHECK (currency IN ('MXN', 'USD')),
    payment_form        TEXT NOT NULL DEFAULT 'transferencia'
        CHECK (payment_form IN ('transferencia', 'contado', 'credito', 'otra')),
    credit_days         INTEGER CHECK (credit_days IS NULL OR credit_days >= 0),
    payment_other       TEXT NOT NULL DEFAULT '',
    subtotal            INTEGER NOT NULL DEFAULT 0,
    iva                 INTEGER NOT NULL DEFAULT 0,
    total               INTEGER NOT NULL DEFAULT 0,
    revision_reason     TEXT,
    created_by          INTEGER NOT NULL REFERENCES users (id),
    created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    confirmed_by        INTEGER REFERENCES users (id),
    confirmed_at        TEXT,
    document            TEXT,
    pdf_sha256          TEXT,
    CHECK (status = 'borrador' OR (number IS NOT NULL AND seq IS NOT NULL AND document IS NOT NULL))
);
CREATE INDEX idx_purchase_orders_project ON purchase_orders (project_id, id);
CREATE INDEX idx_purchase_orders_supplier ON purchase_orders (supplier_id);
-- One confirmed O.C. per proyecto and seq at a time, and one revision per O.C.
CREATE UNIQUE INDEX idx_purchase_orders_confirmed ON purchase_orders (project_id, seq)
    WHERE status = 'confirmada';
CREATE UNIQUE INDEX idx_purchase_orders_supersedes ON purchase_orders (supersedes_id)
    WHERE supersedes_id IS NOT NULL;

CREATE TABLE purchase_order_lines (
    id                  INTEGER PRIMARY KEY,
    purchase_order_id   INTEGER NOT NULL REFERENCES purchase_orders (id) ON DELETE CASCADE,
    line_no             INTEGER NOT NULL CHECK (line_no >= 1),
    quote_line_no       INTEGER NOT NULL CHECK (quote_line_no >= 1),
    description         TEXT NOT NULL,
    qty_milli           INTEGER NOT NULL,
    kg_per_unit_micros  INTEGER,
    price_per_kg_micros INTEGER,
    unit_price_micros   INTEGER NOT NULL,
    line_total          INTEGER NOT NULL,
    UNIQUE (purchase_order_id, line_no)
);

CREATE TRIGGER audit_purchase_orders_ai AFTER INSERT ON purchase_orders BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('purchase_orders', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('project_id', new.project_id, 'quote_id', new.quote_id,
                        'supersedes_id', new.supersedes_id, 'seq', new.seq,
                        'revision', new.revision, 'number', new.number, 'status', new.status,
                        'supplier_id', new.supplier_id, 'delivery_address', new.delivery_address,
                        'delivery_references', new.delivery_references, 'currency', new.currency,
                        'payment_form', new.payment_form, 'credit_days', new.credit_days,
                        'payment_other', new.payment_other, 'total', new.total,
                        'revision_reason', new.revision_reason, 'confirmed_at', new.confirmed_at));
END;

CREATE TRIGGER audit_purchase_orders_au AFTER UPDATE ON purchase_orders BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('purchase_orders', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('seq', old.seq, 'number', old.number, 'status', old.status,
                        'supplier_id', old.supplier_id, 'delivery_address', old.delivery_address,
                        'delivery_references', old.delivery_references, 'currency', old.currency,
                        'payment_form', old.payment_form, 'credit_days', old.credit_days,
                        'payment_other', old.payment_other, 'total', old.total,
                        'confirmed_at', old.confirmed_at),
            json_object('seq', new.seq, 'number', new.number, 'status', new.status,
                        'supplier_id', new.supplier_id, 'delivery_address', new.delivery_address,
                        'delivery_references', new.delivery_references, 'currency', new.currency,
                        'payment_form', new.payment_form, 'credit_days', new.credit_days,
                        'payment_other', new.payment_other, 'total', new.total,
                        'confirmed_at', new.confirmed_at));
END;

CREATE TRIGGER audit_purchase_orders_ad AFTER DELETE ON purchase_orders BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('purchase_orders', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('project_id', old.project_id, 'number', old.number, 'status', old.status,
                        'supplier_id', old.supplier_id, 'total', old.total));
END;

-- The seeds fire the settings triggers; log them as unattributed ("Sistema").
UPDATE audit_actor SET user_id = NULL, source = NULL WHERE id = 1;

INSERT OR IGNORE INTO settings (key, value) VALUES ('emisor_rfc', 'CLA260326Q19');
INSERT OR IGNORE INTO settings (key, value) VALUES ('emisor_regimen', '601');
INSERT OR IGNORE INTO settings (key, value) VALUES ('oc_razon_social', 'CLADEX S.A. de C.V.');
INSERT OR IGNORE INTO settings (key, value) VALUES ('oc_domicilio_fiscal',
    'Calle Circuito Valle del Silencio 143, Col. Lomas de Valle Escondido, C.P. 52930, Atizapán de Zaragoza, Estado de México');
INSERT OR IGNORE INTO settings (key, value) VALUES ('oc_uso_cfdi', 'G01');
