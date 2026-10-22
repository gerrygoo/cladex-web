-- "Otro margen" typed as the CCS 30% sale price per kg (issue #38): the price the
-- salesperson typed is kept exactly, and drives the draft's prices, instead of being
-- reconstructed from custom_margin_micros. NULL when the margin was typed as a
-- percentage (or the quote has no custom margin); custom_margin_micros still holds the
-- margin the price implies. Whole centavos per kg, like the price field.
ALTER TABLE quotes ADD COLUMN custom_copper_price_micros INTEGER
    CHECK (custom_copper_price_micros IS NULL OR custom_copper_price_micros > 0);
