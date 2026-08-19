-- Units of measure (m, kg, pza, rollo, ...) and per-product conversion ratios between
-- them. A product's own unit_id is the unit its cost/price/qty are already denominated
-- in (kg_per_m_micros, cost_micros, unit_price_micros — see 0001/0002); conversions let
-- a quote line be entered in a different unit and converted to that base unit before
-- pricing math runs (internal/pricing.ConvertQty).

CREATE TABLE units (
    id         INTEGER PRIMARY KEY,
    code       TEXT NOT NULL UNIQUE, -- short form, e.g. 'm', 'kg', 'pza', 'rollo'
    name       TEXT NOT NULL,        -- display name, e.g. 'Metro', 'Kilogramo'
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

INSERT INTO units (code, name) VALUES
    ('m', 'Metro'),
    ('kg', 'Kilogramo'),
    ('pza', 'Pieza'),
    ('rollo', 'Rollo');

ALTER TABLE products ADD COLUMN unit_id INTEGER REFERENCES units (id);

-- Ratio between two units for one product, e.g. "1 rollo = 100 m" for a specific SKU.
-- rate_micros is the amount of to_unit per 1 from_unit, 1e-6 fixed-point (matching the
-- catalog's own precision for unit prices). Application code enforces that each
-- unordered {from_unit, to_unit} pair appears at most once per product; the unique
-- index below only catches exact-direction duplicates.
CREATE TABLE product_unit_conversions (
    id           INTEGER PRIMARY KEY,
    product_id   INTEGER NOT NULL REFERENCES products (id),
    from_unit_id INTEGER NOT NULL REFERENCES units (id),
    to_unit_id   INTEGER NOT NULL REFERENCES units (id),
    rate_micros  INTEGER NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    CHECK (from_unit_id != to_unit_id)
);
CREATE UNIQUE INDEX idx_product_unit_conversions_pair
    ON product_unit_conversions (product_id, from_unit_id, to_unit_id);
CREATE INDEX idx_product_unit_conversions_product_id ON product_unit_conversions (product_id);
