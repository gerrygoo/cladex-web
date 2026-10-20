-- Facturación (#20; docs/planning/facturas-timbrado-y-envio.md, milestones M5–M8):
-- stamping CFDIs through Facturoporti. Secrets (token, CSD) never come here; they are
-- files on the NAS (internal/facturoporti). Emisor data lives in settings (emisor_*).

-- SAT codes a CFDI concepto needs. A unit's c_ClaveUnidad, seeded for the three units
-- whose code is unambiguous; and a familia's c_ClaveProdServ, the default for its
-- products and free lines, left for an admin to fill in with the contador. Both can be
-- changed on each line of a factura draft.
--
-- The units audit triggers list their columns, so they are recreated with sat_code; they
-- are dropped first so the seeding, which nobody did by hand, isn't logged as an edit.
DROP TRIGGER audit_units_ai;
DROP TRIGGER audit_units_au;
DROP TRIGGER audit_units_ad;
ALTER TABLE units ADD COLUMN sat_code TEXT;
UPDATE units SET sat_code = 'MTR' WHERE code = 'm';
UPDATE units SET sat_code = 'KGM' WHERE code = 'kg';
UPDATE units SET sat_code = 'H87' WHERE code = 'pza';
ALTER TABLE product_families ADD COLUMN sat_product_code TEXT;
CREATE TRIGGER audit_units_ai AFTER INSERT ON units BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('units', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('code', new.code, 'name', new.name, 'sat_code', new.sat_code));
END;
CREATE TRIGGER audit_units_au AFTER UPDATE ON units BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('units', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('code', old.code, 'name', old.name, 'sat_code', old.sat_code),
            json_object('code', new.code, 'name', new.name, 'sat_code', new.sat_code));
END;
CREATE TRIGGER audit_units_ad AFTER DELETE ON units BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('units', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('code', old.code, 'name', old.name, 'sat_code', old.sat_code));
END;

-- A comprobante is a CFDI Cladex issues: a factura (de ingreso) built from a proyecto's
-- quote, or a pago (comprobante de pago, Pagos 2.0) for a P.P.D. factura. Its fiscal
-- state machine is §1 and §3 of the facturas doc:
--
--   borrador → lista → timbrando → timbrada → (cancelacion_pendiente →) cancelada
--                        ↘ rechazada → borrador     ↘ incierto → timbrando (same request)
--
-- - borrador: editable. Receptor data is read live from the customer until validation.
-- - lista: validated; request holds the exact JSON that will be sent (without the CSD,
--   added at send time) and request_sha256 its hash. serie/folio are assigned at the
--   first validation and never change, even back in borrador.
-- - timbrando: a request is in flight; written before the call.
-- - incierto: no readable answer. Resent only with the same request bytes, which
--   Facturoporti answers with the stamp it already made, so it can't stamp twice.
-- - rechazada: Facturoporti or the SAT said no (error has why); nothing was stamped.
-- - timbrada: uuid and the stamped xml (the legal document; the PDF is rendered from it
--   on demand, never stored). Immutable from here on except for cancellation.
-- sandbox says which Facturoporti host stamped it: a sandbox stamp is never real.
CREATE TABLE comprobantes (
    id             INTEGER PRIMARY KEY,
    tipo           TEXT NOT NULL CHECK (tipo IN ('factura', 'pago')),
    project_id     INTEGER NOT NULL REFERENCES projects (id),
    quote_id       INTEGER REFERENCES quotes (id),       -- factura: the quote it bills
    related_id     INTEGER REFERENCES comprobantes (id), -- pago: the factura it pays; factura: the one it replaces
    status         TEXT NOT NULL DEFAULT 'borrador' CHECK (status IN
                       ('borrador', 'lista', 'timbrando', 'incierto', 'rechazada', 'timbrada',
                        'cancelacion_pendiente', 'cancelada')),
    serie          TEXT,
    folio          INTEGER,
    uso_cfdi       TEXT,
    metodo_pago    TEXT CHECK (metodo_pago IN ('PUE', 'PPD')),
    forma_pago     TEXT,           -- c_FormaPago; 99 for P.P.D. facturas
    moneda         TEXT NOT NULL DEFAULT 'MXN' CHECK (moneda IN ('MXN', 'USD')),
    tipo_cambio_micros INTEGER,    -- USD only
    recipients     TEXT,           -- emails Facturoporti sends it to on stamping, ";"-separated
    pago_fecha     TEXT,           -- pago: when the money came in (YYYY-MM-DD)
    subtotal       INTEGER NOT NULL DEFAULT 0, -- centavos
    iva            INTEGER NOT NULL DEFAULT 0,
    total          INTEGER NOT NULL DEFAULT 0,
    request        TEXT,
    request_sha256 TEXT,
    sandbox        INTEGER CHECK (sandbox IN (0, 1)),
    error          TEXT,
    uuid           TEXT UNIQUE,
    xml            TEXT,
    stamped_at     TEXT,
    email_sent     INTEGER CHECK (email_sent IN (0, 1)),
    cancel_motivo  TEXT,
    cancel_sustitucion TEXT,
    cancel_acuse   TEXT,
    cancelled_at   TEXT,
    created_by     INTEGER NOT NULL REFERENCES users (id),
    created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (serie, folio)
);
CREATE INDEX idx_comprobantes_project ON comprobantes (project_id, id);

-- A factura's lines, copied from its quote's when the draft is made. Amounts are the
-- quote's and don't change; descriptions and SAT codes can be edited while borrador.
CREATE TABLE comprobante_conceptos (
    id                INTEGER PRIMARY KEY,
    comprobante_id    INTEGER NOT NULL REFERENCES comprobantes (id),
    line_no           INTEGER NOT NULL,
    quote_line_id     INTEGER REFERENCES quote_lines (id),
    sat_product_code  TEXT,
    sat_unit_code     TEXT,
    unit_name         TEXT,
    description       TEXT NOT NULL,
    qty_milli         INTEGER NOT NULL,
    unit_price_micros INTEGER NOT NULL,
    importe           INTEGER NOT NULL, -- centavos, the quote line's total
    iva               INTEGER NOT NULL, -- centavos, 16% of importe
    UNIQUE (comprobante_id, line_no)
);

-- Append-only history of each comprobante: validations, every stamp attempt before and
-- after the call, cancellations, with Facturoporti's answers (never a secret). Like
-- quote_comments it is its own log, so no audit triggers.
CREATE TABLE comprobante_events (
    id             INTEGER PRIMARY KEY,
    comprobante_id INTEGER NOT NULL REFERENCES comprobantes (id),
    event          TEXT NOT NULL,
    detail         TEXT,
    user_id        INTEGER REFERENCES users (id),
    created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_comprobante_events ON comprobante_events (comprobante_id, id);
