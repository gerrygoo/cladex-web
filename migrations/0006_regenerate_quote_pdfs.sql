-- Issued quote PDFs are no longer stored on disk: they are re-rendered on every
-- request from the quote's frozen row and lines. Everything the PDF prints therefore
-- has to be frozen at issue time. Lines, totals, terms, dates and FX already were;
-- the customer and salesperson names were still read live through joins, so a
-- renamed customer would have changed old PDFs. These two columns freeze them.
--
-- Existing issued quotes get backfilled from the current names (all mock data at this
-- point). pdf_sha256 stays as a column: from now on it is the hash of the PDF as
-- issued, and a reprint that no longer matches it means the template or Typst changed
-- since (see Quotes.PDF). The old hashes are cleared, since those PDFs embedded their
-- render time and no reprint could match them.
ALTER TABLE quotes ADD COLUMN customer_name_snapshot TEXT;
ALTER TABLE quotes ADD COLUMN vendedor_snapshot TEXT;

-- The backfill fires audit_quotes_au; log it as unattributed, not as whoever the
-- app last stamped.
UPDATE audit_actor SET user_id = NULL, source = NULL WHERE id = 1;
UPDATE quotes SET
    customer_name_snapshot = (SELECT name FROM customers WHERE id = quotes.customer_id),
    vendedor_snapshot      = (SELECT name FROM users WHERE id = quotes.user_id),
    pdf_sha256             = NULL
WHERE issued_at IS NOT NULL;

ALTER TABLE quotes DROP COLUMN pdf_path;
