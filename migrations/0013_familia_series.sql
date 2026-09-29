-- cladex:foreign-keys-off
-- Issue #12: a familia owns its quote series. Until now the QA/QS/QI series were
-- hardcoded in three places (the CHECK constraints on quotes.prefix and
-- folio_sequences.prefix, the web layer's prefix whitelist, and the PDF terms map), so
-- adding a series meant a migration plus a code change. This moves the series onto
-- product_families so admins can create a familia, and with it a series, from /familias.
--
--   series          the folio prefix ("QA"); unique, null only for a familia that has none
--   series_label    what the series picker shows next to the prefix ("Cable CCA")
--   terms           the "Términos y condiciones" block printed on that series' PDFs, one
--                   term per line; issuing still freezes a copy on the quote
--   free_lines_only the familia holds no catalog products (QL: líneas libres only), so it
--                   is left out of the product form's familia list
ALTER TABLE product_families ADD COLUMN series TEXT;
ALTER TABLE product_families ADD COLUMN series_label TEXT;
ALTER TABLE product_families ADD COLUMN terms TEXT NOT NULL DEFAULT '';
ALTER TABLE product_families ADD COLUMN free_lines_only INTEGER NOT NULL DEFAULT 0
    CHECK (free_lines_only IN (0, 1));
CREATE UNIQUE INDEX idx_product_families_series ON product_families (series)
    WHERE series IS NOT NULL;

-- The three catalog familias are created here if the importer never ran (a fresh
-- database), and otherwise just gain their series. Production already has all three.
INSERT INTO product_families (name) VALUES ('CCA') ON CONFLICT (name) DO NOTHING;
UPDATE product_families SET series = 'QA', series_label = 'Cable CCA', terms = 'Precios en pesos mexicanos (MXN), no incluyen IVA
Precios sujetos a cambios sin previo aviso
Tiempo de entrega inmediato salvo previa venta
Pago por adelantado para colocar OC
Empaques de 100, 500, 1000 mt y especiales bajo pedido
Las propiedades del conductor de Resistencia Eléctrica y Elongación están basadas en ASTM B566, UL-83, UL-1581
La capacidad eléctrica fue determinada en cables bajo pruebas en condiciones controladas a 30°C de temperatura
UL 477453, cumple con NOM
Valor de las ampacidades conforme a NEC 2014 CAP 310 conductores eléctricos tabla 310.15(B)(16) al aire'
WHERE name = 'CCA';

INSERT INTO product_families (name) VALUES ('CCS & AC') ON CONFLICT (name) DO NOTHING;
UPDATE product_families SET series = 'QS', series_label = 'Cable CCS & AC', terms = 'Precios en pesos mexicanos (MXN), no incluyen IVA
Precios sujetos a cambios sin previo aviso
Tiempo de entrega inmediata salvo previa venta
El flete no está incluido en la cotización
Pago por adelantado para colocar OC
Sigla 03, lo que indica CFE norma 33
Empaque 500 KG + o - 5% tolerancia de embarque
CCS = Copper Clad Steel
Especificaciones Copperclad: CFE-E0000-33, ANCE, ASTM B227, ASTM B228, ASTM B229, ASTM B452, ASTM B910, UL-854, UL-1581'
WHERE name = 'CCS & AC';

INSERT INTO product_families (name) VALUES ('ABASTILUM') ON CONFLICT (name) DO NOTHING;
UPDATE product_families SET series = 'QI', series_label = 'Alumbrado', terms = 'Precios en pesos mexicanos (MXN), no incluyen IVA
Precios sujetos a cambios sin previo aviso
Tiempo de entrega inmediato
Pago por adelantado para colocar OC
Flete se cotiza por separado
Los postes fondeados en primer rojo óxido'
WHERE name = 'ABASTILUM';

-- QL: líneas libres only. The terms are a placeholder copied from the generic lines of
-- the other series, to be replaced with the real text (docs/PLAN.md, Backlog).
INSERT INTO product_families (name, series, series_label, terms, free_lines_only) VALUES (
    'QL', 'QL', 'Líneas libres',
    'Precios en pesos mexicanos (MXN), no incluyen IVA
Precios sujetos a cambios sin previo aviso
Pago por adelantado para colocar OC
Flete se cotiza por separado',
    1);

-- Drop the QA/QS/QI CHECKs: any series a familia owns is now valid. SQLite can't alter a
-- constraint, so both tables are rebuilt. quote_lines and quote_comments reference
-- quotes, so the runner applies this file with foreign keys off (the marker on line 1)
-- and checks them before committing. Rebuilding quotes drops its indexes and audit
-- triggers with it, so they are recreated below.

CREATE TABLE folio_sequences_new (
    prefix      TEXT PRIMARY KEY,
    next_number INTEGER NOT NULL DEFAULT 1
);
INSERT INTO folio_sequences_new SELECT prefix, next_number FROM folio_sequences;
DROP TABLE folio_sequences;
ALTER TABLE folio_sequences_new RENAME TO folio_sequences;

CREATE TABLE quotes_new (
    id                  INTEGER PRIMARY KEY,
    folio               TEXT NOT NULL UNIQUE,
    prefix              TEXT NOT NULL,
    customer_id         INTEGER NOT NULL REFERENCES customers (id),
    user_id             INTEGER NOT NULL REFERENCES users (id),
    status              TEXT NOT NULL CHECK (status IN
        ('borrador', 'emitida', 'revisada', 'pipeline', 'oc_emitida', 'entregada', 'cerrada')),
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
        CHECK (custom_margin_micros IS NULL OR (custom_margin_micros >= 0 AND custom_margin_micros < 1000000))
);
INSERT INTO quotes_new (id, folio, prefix, customer_id, user_id, status, subtotal, iva, total,
        terms_snapshot, created_at, issued_at, valid_until, supersedes_quote_id, pdf_sha256,
        customer_name_snapshot, vendedor_snapshot, margin_option_id, margin_name_snapshot,
        margin_snapshot_micros, custom_margin_micros)
    SELECT id, folio, prefix, customer_id, user_id, status, subtotal, iva, total,
        terms_snapshot, created_at, issued_at, valid_until, supersedes_quote_id, pdf_sha256,
        customer_name_snapshot, vendedor_snapshot, margin_option_id, margin_name_snapshot,
        margin_snapshot_micros, custom_margin_micros
    FROM quotes;
DROP TABLE quotes;
ALTER TABLE quotes_new RENAME TO quotes;
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
