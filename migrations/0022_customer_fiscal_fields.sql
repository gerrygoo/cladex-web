-- Receptor data for CFDI 4.0 (issue #17; PAC chosen in #20: Facturapi):
--   legal_name  the razón social exactly as registered with the SAT, without the
--               corporate regime ("S.A. de C.V."), which is how the PAC asks for it.
--               name stays the trade name printed on quotes.
--   cfdi_use    the customer's default uso de CFDI (c_UsoCFDI, e.g. G03); a factura can
--               still pick another.
-- Both optional: a customer may be a prospect with no fiscal data. Completeness is
-- checked when a factura is validated, not on the customer form. The fiscal address is
-- the postal_code already on the row (0004); CFDI 4.0 asks for nothing more.
ALTER TABLE customers ADD COLUMN legal_name TEXT;
ALTER TABLE customers ADD COLUMN cfdi_use TEXT;

-- tax_regime was free text ("601 - General de Ley Personas Morales"). It becomes the
-- c_RegimenFiscal code alone, picked from a list. A value that starts with a valid code
-- keeps just the code; anything else is cleared and kept in the notes, so it can be
-- entered again by hand. (The SAT codes are the ones in internal/store/sat.go.)
UPDATE customers
SET notes = CASE WHEN notes IS NULL OR trim(notes) = ''
                 THEN 'Régimen fiscal anterior: ' || trim(tax_regime)
                 ELSE notes || char(10) || 'Régimen fiscal anterior: ' || trim(tax_regime) END,
    tax_regime = NULL
WHERE tax_regime IS NOT NULL AND trim(tax_regime) != ''
  AND substr(trim(tax_regime), 1, 3) NOT IN ('601', '603', '605', '606', '607', '608',
      '610', '611', '612', '614', '615', '616', '620', '621', '622', '623', '624', '625', '626');

UPDATE customers SET tax_regime = substr(trim(tax_regime), 1, 3)
WHERE tax_regime IS NOT NULL AND trim(tax_regime) != '';

UPDATE customers SET tax_regime = NULL WHERE trim(tax_regime) = '';

-- The audit triggers list their columns: recreate them with every fiscal field (0004
-- predates postal_code and tax_regime, which were never in the trail).
DROP TRIGGER audit_customers_ai;
DROP TRIGGER audit_customers_au;
DROP TRIGGER audit_customers_ad;

CREATE TRIGGER audit_customers_ai AFTER INSERT ON customers BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('customers', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', new.name, 'rfc', new.rfc, 'contact_name', new.contact_name,
                        'phone', new.phone, 'email', new.email, 'address', new.address,
                        'postal_code', new.postal_code, 'tax_regime', new.tax_regime,
                        'legal_name', new.legal_name, 'cfdi_use', new.cfdi_use,
                        'notes', new.notes, 'deleted_at', new.deleted_at));
END;

CREATE TRIGGER audit_customers_au AFTER UPDATE ON customers BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('customers', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', old.name, 'rfc', old.rfc, 'contact_name', old.contact_name,
                        'phone', old.phone, 'email', old.email, 'address', old.address,
                        'postal_code', old.postal_code, 'tax_regime', old.tax_regime,
                        'legal_name', old.legal_name, 'cfdi_use', old.cfdi_use,
                        'notes', old.notes, 'deleted_at', old.deleted_at),
            json_object('name', new.name, 'rfc', new.rfc, 'contact_name', new.contact_name,
                        'phone', new.phone, 'email', new.email, 'address', new.address,
                        'postal_code', new.postal_code, 'tax_regime', new.tax_regime,
                        'legal_name', new.legal_name, 'cfdi_use', new.cfdi_use,
                        'notes', new.notes, 'deleted_at', new.deleted_at));
END;

CREATE TRIGGER audit_customers_ad AFTER DELETE ON customers BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('customers', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', old.name, 'rfc', old.rfc, 'contact_name', old.contact_name,
                        'phone', old.phone, 'email', old.email, 'address', old.address,
                        'postal_code', old.postal_code, 'tax_regime', old.tax_regime,
                        'legal_name', old.legal_name, 'cfdi_use', old.cfdi_use,
                        'notes', old.notes, 'deleted_at', old.deleted_at));
END;
