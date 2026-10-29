-- Tipos de cotización, slice T2 (docs/planning/tipos-de-cotizacion.md): admins can now
-- change a quote type's behavior, so an issued quote freezes the behavior it was issued
-- with, as it already freezes its terms (terms_snapshot). Its PDF is regenerated from
-- the frozen row and checked against the SHA-256 recorded at issue, so it must not
-- follow later edits of the type. NULL while the quote is a draft, which follows its
-- type; set by IssueQuote. Adding nullable columns leaves the previous build's queries
-- working, so this migration isn't exclusive.
ALTER TABLE quotes ADD COLUMN letterhead_snapshot TEXT;
ALTER TABLE quotes ADD COLUMN hide_address_snapshot INTEGER;
ALTER TABLE quotes ADD COLUMN kg_priced_snapshot INTEGER;
ALTER TABLE quotes ADD COLUMN copper_price_margin_snapshot INTEGER;

-- Quotes issued so far were issued with their type's current behavior: nothing has been
-- able to change it since 0029 derived it. The backfill isn't an edit, so the update
-- trigger is dropped around it (as 0024 did for units) and recreated unchanged.
DROP TRIGGER audit_quotes_au;
UPDATE quotes SET
    letterhead_snapshot = (SELECT letterhead FROM quote_types t WHERE t.id = quotes.quote_type_id),
    hide_address_snapshot = (SELECT hide_address FROM quote_types t WHERE t.id = quotes.quote_type_id),
    kg_priced_snapshot = (SELECT kg_priced FROM quote_types t WHERE t.id = quotes.quote_type_id),
    copper_price_margin_snapshot = (SELECT copper_price_margin FROM quote_types t WHERE t.id = quotes.quote_type_id)
WHERE issued_at IS NOT NULL;

CREATE TRIGGER audit_quotes_au AFTER UPDATE ON quotes BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('quotes', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('folio', old.folio, 'quote_type_id', old.quote_type_id,
                        'customer_id', old.customer_id,
                        'user_id', old.user_id, 'status', old.status,
                        'subtotal', old.subtotal, 'iva', old.iva, 'total', old.total,
                        'issued_at', old.issued_at, 'valid_until', old.valid_until,
                        'supersedes_quote_id', old.supersedes_quote_id,
                        'pdf_sha256', old.pdf_sha256,
                        'margin_option_id', old.margin_option_id,
                        'margin_name_snapshot', old.margin_name_snapshot,
                        'margin_snapshot_micros', old.margin_snapshot_micros,
                        'custom_margin_micros', old.custom_margin_micros,
                        'delivery_time', old.delivery_time,
                        'currency', old.currency,
                        'project_id', old.project_id),
            json_object('folio', new.folio, 'quote_type_id', new.quote_type_id,
                        'customer_id', new.customer_id,
                        'user_id', new.user_id, 'status', new.status,
                        'subtotal', new.subtotal, 'iva', new.iva, 'total', new.total,
                        'issued_at', new.issued_at, 'valid_until', new.valid_until,
                        'supersedes_quote_id', new.supersedes_quote_id,
                        'pdf_sha256', new.pdf_sha256,
                        'margin_option_id', new.margin_option_id,
                        'margin_name_snapshot', new.margin_name_snapshot,
                        'margin_snapshot_micros', new.margin_snapshot_micros,
                        'custom_margin_micros', new.custom_margin_micros,
                        'delivery_time', new.delivery_time,
                        'currency', new.currency,
                        'project_id', new.project_id));
END;
