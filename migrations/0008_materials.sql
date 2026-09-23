-- M3.2: materials with a pure-cost price per unit, and how much of each material one
-- unit of a product contains. A product's cost is its flat cost_micros plus the sum of
-- its materials' qty x price; the quote's margin then prices it like any other cost
-- product. This replaces the copper_price setting, which was really CCS & AC's *sale*
-- price per kg (margin baked in). See docs/PLAN.md, M3.
--
-- price_micros is MXN per one unit_id of the material (e.g. per kg), pure cost with no
-- margin. qty_per_unit_micros is the conversion constant from product to material:
-- e.g. 0.1723 kg of CCS 30% per m of ALAMBRE 4, in the product's own base unit.
CREATE TABLE materials (
    id           INTEGER PRIMARY KEY,
    name         TEXT NOT NULL UNIQUE,
    unit_id      INTEGER NOT NULL REFERENCES units (id),
    price_micros INTEGER NOT NULL CHECK (price_micros >= 0),
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_by   INTEGER REFERENCES users (id)
);

CREATE TABLE product_materials (
    id                  INTEGER PRIMARY KEY,
    product_id          INTEGER NOT NULL REFERENCES products (id),
    material_id         INTEGER NOT NULL REFERENCES materials (id),
    qty_per_unit_micros INTEGER NOT NULL CHECK (qty_per_unit_micros > 0),
    created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (product_id, material_id)
);
CREATE INDEX idx_product_materials_material_id ON product_materials (material_id);

-- Both are audited for the same reason prices and conversions are: a changed material
-- price or content moves every draft that quotes the product.
CREATE TRIGGER audit_materials_ai AFTER INSERT ON materials BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('materials', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', new.name, 'unit_id', new.unit_id,
                        'price_micros', new.price_micros));
END;

CREATE TRIGGER audit_materials_au AFTER UPDATE ON materials BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('materials', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', old.name, 'unit_id', old.unit_id,
                        'price_micros', old.price_micros),
            json_object('name', new.name, 'unit_id', new.unit_id,
                        'price_micros', new.price_micros));
END;

CREATE TRIGGER audit_materials_ad AFTER DELETE ON materials BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('materials', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', old.name, 'unit_id', old.unit_id,
                        'price_micros', old.price_micros));
END;

CREATE TRIGGER audit_product_materials_ai AFTER INSERT ON product_materials BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('product_materials', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('product_id', new.product_id, 'material_id', new.material_id,
                        'qty_per_unit_micros', new.qty_per_unit_micros));
END;

CREATE TRIGGER audit_product_materials_au AFTER UPDATE ON product_materials BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('product_materials', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('product_id', old.product_id, 'material_id', old.material_id,
                        'qty_per_unit_micros', old.qty_per_unit_micros),
            json_object('product_id', new.product_id, 'material_id', new.material_id,
                        'qty_per_unit_micros', new.qty_per_unit_micros));
END;

CREATE TRIGGER audit_product_materials_ad AFTER DELETE ON product_materials BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('product_materials', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('product_id', old.product_id, 'material_id', old.material_id,
                        'qty_per_unit_micros', old.qty_per_unit_micros));
END;

-- Seed and data fixes below fire audit triggers; log them as unattributed.
UPDATE audit_actor SET user_id = NULL, source = NULL WHERE id = 1;

-- CCS 30% costs $160/kg: the user confirmed (2026-09-22) that the $160 production's
-- copper_price held is the material's cost, not a sale price, and that it's unlikely to
-- change this year. (The workbook's own 'CCS & AC'!D4 said $155.) Each CCS & AC product
-- gets its existing kg/m as its CCS 30% content, so it now prices at
-- kg/m x 160 / (1 - the quote's margin) instead of kg/m x copper_price with no margin.
INSERT INTO materials (name, unit_id, price_micros)
    VALUES ('CCS 30%', (SELECT id FROM units WHERE code = 'kg'), 160000000);

INSERT INTO product_materials (product_id, material_id, qty_per_unit_micros)
SELECT p.id, (SELECT id FROM materials WHERE name = 'CCS 30%'), p.kg_per_m_micros
FROM products p
JOIN product_families f ON f.id = p.family_id
WHERE f.name = 'CCS & AC' AND p.kg_per_m_micros > 0;

-- A CCS & AC product's cost now *is* its material content. Any cost_micros left on one
-- is the workbook's "cost before margin" column that the pre-M2.3 importer wrongly
-- copied in (spot-fixed in production then; see docs/PLAN.md, footnote 10), and kept
-- would now be added on top of the materials, counting the material twice.
UPDATE products SET cost_micros = NULL
WHERE cost_micros IS NOT NULL
  AND id IN (SELECT product_id FROM product_materials);

-- CCA's weight column was imported as kg per km (18.11 for 14 AWG, where copper runs
-- ~18.5 kg/km) into a column that means kg per m. It never fed a price (CCA prices from
-- cost), so dividing in place fixes it and keeps any hand edit.
UPDATE products SET kg_per_m_micros = CAST(ROUND(kg_per_m_micros / 1000.0) AS INTEGER)
WHERE kg_per_m_micros IS NOT NULL
  AND family_id = (SELECT id FROM product_families WHERE name = 'CCA');

DELETE FROM settings WHERE key = 'copper_price';
