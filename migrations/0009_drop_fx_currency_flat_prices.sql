-- M3.3: every product is priced as cost / (1 - the quote's margin); nothing is priced
-- flat, in USD, or by quantity tier any more. See docs/PLAN.md, M3 decisions 1, 5 and 8.
--
-- Data first, while the columns still exist; then the triggers that name the columns
-- being dropped (SQLite refuses to drop a column a trigger references); then the drops.

-- Data changes below fire audit triggers; log them as unattributed.
UPDATE audit_actor SET user_id = NULL, source = NULL WHERE id = 1;

-- Postes: production stores the workbook's raw poste cost (e.g. 1680 for the 4 m poste),
-- which the workbook prices as USD x 18. With currencies gone, the user chose
-- (2026-09-22) to convert it to MXN at that same frozen 18: the 4 m poste costs $30,240.
-- Only rows that hold a cost and no flat price, i.e. the raw-cost form production uses.
UPDATE products SET cost_micros = cost_micros * 18
WHERE family_id = (SELECT id FROM product_families WHERE name = 'ABASTILUM')
  AND description LIKE 'POSTE MET%'
  AND cost_micros IS NOT NULL
  AND unit_price_micros IS NULL;

-- A catalog imported by the old cmd/import holds ABASTILUM as flat, margin-included MXN
-- prices instead (postes already x 18). Back the cost out with the margin that produced
-- each price: the workbook's 12.34% for LEDVANCE, its 20% "Margen iluminación" for the
-- rest. Production has no such rows; this keeps older databases pricing sensibly.
UPDATE products SET cost_micros = CAST(ROUND(unit_price_micros *
        CASE WHEN description LIKE '%LEDVANCE%' THEN 0.8766 ELSE 0.8 END) AS INTEGER)
WHERE family_id = (SELECT id FROM product_families WHERE name = 'ABASTILUM')
  AND unit_price_micros IS NOT NULL
  AND cost_micros IS NULL;

DELETE FROM settings WHERE key = 'fx_rate';

DROP TRIGGER audit_products_ai;
DROP TRIGGER audit_products_au;
DROP TRIGGER audit_products_ad;
DROP TRIGGER audit_quotes_ai;
DROP TRIGGER audit_quotes_au;
DROP TRIGGER audit_quotes_ad;

-- price_breaks was never used (its one real case, ELECTRACLEAN, was never imported) and
-- overrode the price outright, skipping the margin. Its own audit triggers go with it.
DROP TABLE price_breaks;

ALTER TABLE products DROP COLUMN unit_price_micros;
ALTER TABLE products DROP COLUMN currency;
-- Every quote is MXN. fx_rate_used_micros only ever recorded the setting at issue time;
-- no production quote has a USD line, so no issued price depended on it.
ALTER TABLE quotes DROP COLUMN currency;
ALTER TABLE quotes DROP COLUMN fx_rate_used_micros;

CREATE TRIGGER audit_products_ai AFTER INSERT ON products BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('products', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('family_id', new.family_id, 'sku', new.sku,
                        'description', new.description,
                        'kg_per_m_micros', new.kg_per_m_micros,
                        'cost_micros', new.cost_micros,
                        'active', new.active, 'unit_id', new.unit_id,
                        'deleted_at', new.deleted_at));
END;

CREATE TRIGGER audit_products_au AFTER UPDATE ON products BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('products', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('family_id', old.family_id, 'sku', old.sku,
                        'description', old.description,
                        'kg_per_m_micros', old.kg_per_m_micros,
                        'cost_micros', old.cost_micros,
                        'active', old.active, 'unit_id', old.unit_id,
                        'deleted_at', old.deleted_at),
            json_object('family_id', new.family_id, 'sku', new.sku,
                        'description', new.description,
                        'kg_per_m_micros', new.kg_per_m_micros,
                        'cost_micros', new.cost_micros,
                        'active', new.active, 'unit_id', new.unit_id,
                        'deleted_at', new.deleted_at));
END;

CREATE TRIGGER audit_products_ad AFTER DELETE ON products BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('products', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('family_id', old.family_id, 'sku', old.sku,
                        'description', old.description,
                        'kg_per_m_micros', old.kg_per_m_micros,
                        'cost_micros', old.cost_micros,
                        'active', old.active, 'unit_id', old.unit_id,
                        'deleted_at', old.deleted_at));
END;

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
                        'margin_snapshot_micros', new.margin_snapshot_micros));
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
                        'margin_snapshot_micros', old.margin_snapshot_micros),
            json_object('folio', new.folio, 'customer_id', new.customer_id,
                        'user_id', new.user_id, 'status', new.status,
                        'subtotal', new.subtotal, 'iva', new.iva, 'total', new.total,
                        'issued_at', new.issued_at, 'valid_until', new.valid_until,
                        'supersedes_quote_id', new.supersedes_quote_id,
                        'pdf_sha256', new.pdf_sha256,
                        'margin_option_id', new.margin_option_id,
                        'margin_name_snapshot', new.margin_name_snapshot,
                        'margin_snapshot_micros', new.margin_snapshot_micros));
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
                        'margin_snapshot_micros', old.margin_snapshot_micros));
END;
