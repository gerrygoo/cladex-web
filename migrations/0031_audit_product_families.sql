-- Admins can now rename a familia on /familias (issue #55), as they can rename a unit,
-- so product_families gets the same audit triggers units has (0004): who renamed it or
-- changed its SAT code, and when. Adding triggers leaves the previous build's queries
-- working, so this migration isn't exclusive.
CREATE TRIGGER audit_product_families_ai AFTER INSERT ON product_families BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('product_families', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', new.name, 'sat_product_code', new.sat_product_code));
END;

CREATE TRIGGER audit_product_families_au AFTER UPDATE ON product_families BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('product_families', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', old.name, 'sat_product_code', old.sat_product_code),
            json_object('name', new.name, 'sat_product_code', new.sat_product_code));
END;

CREATE TRIGGER audit_product_families_ad AFTER DELETE ON product_families BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('product_families', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', old.name, 'sat_product_code', old.sat_product_code));
END;
