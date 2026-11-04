-- Datos para facturar por producto (issues #62 and #54): each product gets its own SAT
-- codes, independent of its familia and of its internal unit.
--
--   sat_product_code  c_ClaveProdServ, 8 digits
--   sat_unit_code     c_ClaveUnidad, as the SAT writes it (MTR, KGM, H87…)
--
-- Both NULL until someone captures them: they're optional to create and quote a product.
-- The app checks them against the SAT's full catalogs (internal/satcat), which are too
-- big and too often revised for a CHECK here. They aren't seeded from the familia's code
-- or the unit's: the point is that someone confirms each SKU, and the product list's
-- "Sin datos para facturar" filter finds the ones still to do. Until then a factura draft
-- falls back to those defaults (CreateFacturaDraft).
--
-- Adding nullable columns leaves the previous build's queries working (its INSERT names
-- its columns), so this migration isn't exclusive. The products audit triggers list
-- their columns, so they're recreated with the new ones.
ALTER TABLE products ADD COLUMN sat_product_code TEXT;
ALTER TABLE products ADD COLUMN sat_unit_code TEXT;

DROP TRIGGER audit_products_ai;
DROP TRIGGER audit_products_au;
DROP TRIGGER audit_products_ad;

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
                        'sat_product_code', new.sat_product_code,
                        'sat_unit_code', new.sat_unit_code,
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
                        'sat_product_code', old.sat_product_code,
                        'sat_unit_code', old.sat_unit_code,
                        'deleted_at', old.deleted_at),
            json_object('family_id', new.family_id, 'sku', new.sku,
                        'description', new.description,
                        'kg_per_m_micros', new.kg_per_m_micros,
                        'cost_micros', new.cost_micros,
                        'active', new.active, 'unit_id', new.unit_id,
                        'sat_product_code', new.sat_product_code,
                        'sat_unit_code', new.sat_unit_code,
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
                        'sat_product_code', old.sat_product_code,
                        'sat_unit_code', old.sat_unit_code,
                        'deleted_at', old.deleted_at));
END;
