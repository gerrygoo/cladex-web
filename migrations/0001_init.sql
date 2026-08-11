-- Cladex initial schema.
--
-- Money columns are always fixed-point integers: unit prices in micro-pesos
-- (unit_price_micros, 1e-6 MXN), everything actually charged in centavos (1e-2 MXN).
-- Physical quantities that feed pricing math (kg_per_m, qty) are fixed-point integers
-- too, for the same reason: floats must never sit upstream of a rounding step.
-- Timestamps are ISO-8601 UTC text.

CREATE TABLE users (
    id            INTEGER PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL CHECK (role IN ('admin', 'vendedor')),
    disabled_at   TEXT,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE sessions (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users (id),
    token_hash TEXT NOT NULL UNIQUE, -- SHA-256 of the session cookie token
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    expires_at TEXT NOT NULL
);
CREATE INDEX idx_sessions_user_id ON sessions (user_id);

CREATE TABLE login_attempts (
    id           INTEGER PRIMARY KEY,
    username     TEXT NOT NULL,
    ip           TEXT NOT NULL,
    success      INTEGER NOT NULL CHECK (success IN (0, 1)),
    attempted_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_login_attempts_username ON login_attempts (username, attempted_at);
CREATE INDEX idx_login_attempts_ip ON login_attempts (ip, attempted_at);

CREATE TABLE customers (
    id           INTEGER PRIMARY KEY,
    name         TEXT NOT NULL,
    rfc          TEXT,
    contact_name TEXT,
    phone        TEXT,
    email        TEXT,
    address      TEXT,
    notes        TEXT,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    deleted_at   TEXT
);

-- One row per catalog sheet (the workbook's four "Cotizador" families).
CREATE TABLE product_families (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    sheet_name TEXT, -- source spreadsheet tab, kept for import traceability
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE products (
    id               INTEGER PRIMARY KEY,
    family_id        INTEGER NOT NULL REFERENCES product_families (id),
    sku              TEXT NOT NULL UNIQUE,
    description      TEXT NOT NULL,
    kg_per_m_micros  INTEGER, -- cable weight, 1e-6 kg/m; null for non-cable products
    unit_price_micros INTEGER, -- flat catalog price, 1e-6 MXN; null when priced from metal + margin
    currency         TEXT NOT NULL DEFAULT 'MXN' CHECK (currency IN ('MXN', 'USD')),
    active           INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    deleted_at       TEXT
);
CREATE INDEX idx_products_family_id ON products (family_id);

-- Quantity-tiered pricing, e.g. a lower per-unit price at 100m+.
CREATE TABLE price_breaks (
    id                INTEGER PRIMARY KEY,
    product_id        INTEGER NOT NULL REFERENCES products (id),
    min_qty_milli     INTEGER NOT NULL, -- 1e-3 units; qty >= this tier's threshold
    unit_price_micros INTEGER NOT NULL
);
CREATE INDEX idx_price_breaks_product_id ON price_breaks (product_id);

-- Admin-editable knobs: FX rate, metal prices, default margins. Generic key/value so
-- new settings don't require a migration; numeric values are stored as fixed-point
-- integer text (micros) and parsed by internal/money callers.
CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_by INTEGER REFERENCES users (id)
);

CREATE TABLE quotes (
    id                  INTEGER PRIMARY KEY,
    folio               TEXT NOT NULL UNIQUE,
    prefix              TEXT NOT NULL CHECK (prefix IN ('QA', 'QS', 'QI')),
    customer_id         INTEGER NOT NULL REFERENCES customers (id),
    user_id             INTEGER NOT NULL REFERENCES users (id),
    status              TEXT NOT NULL CHECK (status IN ('borrador', 'emitida', 'revisada')),
    currency            TEXT NOT NULL DEFAULT 'MXN' CHECK (currency IN ('MXN', 'USD')),
    fx_rate_used_micros INTEGER, -- USD/MXN at issue time; null while a draft
    subtotal            INTEGER NOT NULL DEFAULT 0, -- centavos
    iva                 INTEGER NOT NULL DEFAULT 0, -- centavos
    total                INTEGER NOT NULL DEFAULT 0, -- centavos
    terms_snapshot      TEXT, -- per-family terms block, frozen at issue time
    created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    issued_at           TEXT, -- set once; freezes the row
    valid_until          TEXT,
    supersedes_quote_id INTEGER REFERENCES quotes (id),
    pdf_path            TEXT,
    pdf_sha256          TEXT
);
CREATE INDEX idx_quotes_customer_id ON quotes (customer_id);
CREATE INDEX idx_quotes_user_id ON quotes (user_id);
CREATE INDEX idx_quotes_status ON quotes (status);

CREATE TABLE quote_lines (
    id                  INTEGER PRIMARY KEY,
    quote_id            INTEGER NOT NULL REFERENCES quotes (id),
    line_no             INTEGER NOT NULL,
    product_id          INTEGER REFERENCES products (id), -- null: free-text "Cotizador libre" line
    description_snapshot TEXT NOT NULL,
    qty_milli           INTEGER NOT NULL, -- 1e-3 units
    unit_price_micros   INTEGER NOT NULL,
    line_total          INTEGER NOT NULL, -- centavos; the one rounding point
    pricing_inputs      TEXT, -- JSON: kg_per_m, margin, metal_price, fx_rate at issue time
    source              TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'rfp_extraction')),
    rfp_extraction_id   INTEGER -- reserved for backlog LLM ingestion; unused until then
);
CREATE UNIQUE INDEX idx_quote_lines_quote_id_line_no ON quote_lines (quote_id, line_no);
