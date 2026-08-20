-- Kept separate from the free-text address field so the future invoicing/billing
-- portal (CFDI) integration can read them directly. Both optional: a customer may
-- still be a prospect with no fiscal data yet.
ALTER TABLE customers ADD COLUMN postal_code TEXT;
ALTER TABLE customers ADD COLUMN tax_regime TEXT;
