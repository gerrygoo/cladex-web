-- Issue #12, second round: a QL quote is priced in a currency the salesperson picks, and
-- carries a validity date the salesperson types.
--
-- quotes.currency is 'MXN' or 'USD', NULL until chosen. It is only asked for when the
-- series' terms hold the token {moneda}; prices are typed in that currency and nothing is
-- converted. It is copied onto a revision. The validity date is quotes.valid_until, which
-- a draft of a free-lines-only series now holds before it is issued (it was NULL until
-- issue), so it needs no column.
ALTER TABLE quotes ADD COLUMN currency TEXT CHECK (currency IN ('MXN', 'USD'));

-- QL's first term names the currency chosen on each quote instead of assuming pesos. Only
-- rewrite it while it is still the 0014 wording: an admin's later edit at /familias wins.
UPDATE product_families SET terms = 'Precios en {moneda}, no incluyen IVA
Precios sujetos a cambios sin previo aviso
Tiempo de entrega: {tiempo_de_entrega}
Pago por adelantado para colocar OC'
WHERE name = 'QL' AND terms = 'Precios en pesos mexicanos (MXN), no incluyen IVA (a menos que se haya indicado que esa cotización se emitirá en USD)
Precios sujetos a cambios sin previo aviso
Tiempo de entrega: {tiempo_de_entrega}
Pago por adelantado para colocar OC';

-- The audit triggers list their columns, so recreate them with currency.
DROP TRIGGER audit_quotes_ai;
DROP TRIGGER audit_quotes_au;
DROP TRIGGER audit_quotes_ad;

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
                        'custom_margin_micros', new.custom_margin_micros,
                        'delivery_time', new.delivery_time,
                        'currency', new.currency));
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
                        'custom_margin_micros', old.custom_margin_micros,
                        'delivery_time', old.delivery_time,
                        'currency', old.currency),
            json_object('folio', new.folio, 'customer_id', new.customer_id,
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
                        'currency', new.currency));
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
                        'custom_margin_micros', old.custom_margin_micros,
                        'delivery_time', old.delivery_time,
                        'currency', old.currency));
END;
