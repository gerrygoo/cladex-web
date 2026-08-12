-- Acquisition cost, for products priced by cost + margin at quote time (the M2 pricing
-- engine), as opposed to unit_price_micros, which holds a flat final price for products
-- that don't need margin math (e.g. ABASTILUM's catalog prices). In practice a product
-- has one or the other, never both.
ALTER TABLE products ADD COLUMN cost_micros INTEGER; -- 1e-6 MXN
