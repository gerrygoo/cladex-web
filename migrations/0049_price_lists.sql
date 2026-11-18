-- Listas de precios (#82, slice L3 of docs/planning/leads-y-listas-de-precios.md): a PDF
-- of every product of one familia at one margin, with its vigencia and the tipo's terms.
-- Prices are worked out exactly as a quote of that tipo would (same margins, same
-- materials), and the PDF is kept as it was generated: Emilio's rule is that a list's
-- price holds for its whole vigencia, even if the price per kilo changes later. No folio;
-- the lead it was sent to, if any, gets an entry in its history.
--
--   quote_type_id   the tipo whose familia, margins, terms and letterhead it used
--   lead_id         the lead it was made for, NULL for none
--   recipient       who it's for, as printed ("Para:")
--   margin_name     the margin it was priced at, and margin_micros its value
--   valid_until     the last day its prices hold (YYYY-MM-DD)
CREATE TABLE price_lists (
    id            INTEGER PRIMARY KEY,
    quote_type_id INTEGER NOT NULL REFERENCES quote_types (id),
    family_name   TEXT NOT NULL,
    lead_id       INTEGER REFERENCES leads (id),
    recipient     TEXT NOT NULL DEFAULT '',
    user_id       INTEGER NOT NULL REFERENCES users (id),
    margin_name   TEXT NOT NULL,
    margin_micros INTEGER NOT NULL,
    valid_until   TEXT NOT NULL,
    lines         INTEGER NOT NULL,
    pdf           BLOB NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_price_lists_lead ON price_lists (lead_id, created_at);
