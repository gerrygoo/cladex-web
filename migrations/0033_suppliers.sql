-- Proveedores (issue #64): the suppliers Cladex buys from, as the base for the
-- órdenes de compra to come (docs/planning/ordenes-de-compra-a-proveedores.md). Admins
-- keep the catalog; vendedores only look it up, and never see the bank data.
--
-- suppliers:
--   name          nombre comercial, how the app lists it
--   rfc           12 characters (persona moral) or 13 (persona física), upper case
--   legal_name    razón social
--   street .. postal_code  the fiscal address; all required, the CP five digits
--   tax_regime    c_RegimenFiscal code (internal/store/sat.go), NULL when unknown
--   credit_days   días de crédito, NULL when not agreed
--   currency      MXN or USD, what the supplier bills in; informational only, nothing
--                 is converted
--   payment_terms, lead_time  free text: condiciones de pago, tiempo de entrega habitual
--   bank, clabe, beneficiary  datos bancarios; the CLABE is 18 digits with a valid
--                 check digit (checked by the app)
--   inactive      1 keeps it (and its familias) but marks it out of use; a supplier
--                 that can't be deleted is made inactive instead
--
-- supplier_contacts: one row per contact. receives_po marks who an orden de compra is
-- sent to; it's only informative, the app sends no email.
--
-- product_families.default_supplier_id: a familia's default supplier, edited from the
-- supplier's form ("Familias que surte"). A supplier may supply several familias; a
-- familia has at most one, or none. The free-lines-only familia (QL) never gets one:
-- each QL quote may come from a different wholesaler.
--
-- Only adds tables, a nullable column, triggers and rows, so the previous build keeps
-- working while this applies: not exclusive.
CREATE TABLE suppliers (
    id            INTEGER PRIMARY KEY,
    name          TEXT NOT NULL,
    rfc           TEXT NOT NULL,
    legal_name    TEXT NOT NULL,
    street        TEXT NOT NULL,
    ext_number    TEXT NOT NULL,
    neighborhood  TEXT NOT NULL,
    municipality  TEXT NOT NULL,
    state         TEXT NOT NULL,
    postal_code   TEXT NOT NULL CHECK (length(postal_code) = 5),
    tax_regime    TEXT,
    notes         TEXT,
    credit_days   INTEGER CHECK (credit_days IS NULL OR credit_days >= 0),
    currency      TEXT CHECK (currency IS NULL OR currency IN ('MXN', 'USD')),
    payment_terms TEXT,
    lead_time     TEXT,
    bank          TEXT,
    clabe         TEXT CHECK (clabe IS NULL OR length(clabe) = 18),
    beneficiary   TEXT,
    inactive      INTEGER NOT NULL DEFAULT 0 CHECK (inactive IN (0, 1)),
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE supplier_contacts (
    id          INTEGER PRIMARY KEY,
    supplier_id INTEGER NOT NULL REFERENCES suppliers (id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    role        TEXT,
    phone       TEXT,
    email       TEXT,
    receives_po INTEGER NOT NULL DEFAULT 0 CHECK (receives_po IN (0, 1))
);
CREATE INDEX idx_supplier_contacts_supplier ON supplier_contacts (supplier_id, id);

ALTER TABLE product_families ADD COLUMN default_supplier_id INTEGER REFERENCES suppliers (id);
CREATE INDEX idx_product_families_default_supplier ON product_families (default_supplier_id);

-- The supplier's Historial reads these three tables' trails: its own row, its contacts
-- (found by supplier_id) and the familias that gained or lost it.
CREATE TRIGGER audit_suppliers_ai AFTER INSERT ON suppliers BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('suppliers', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', new.name, 'rfc', new.rfc, 'legal_name', new.legal_name,
                        'street', new.street, 'ext_number', new.ext_number,
                        'neighborhood', new.neighborhood, 'municipality', new.municipality,
                        'state', new.state, 'postal_code', new.postal_code,
                        'tax_regime', new.tax_regime, 'notes', new.notes,
                        'credit_days', new.credit_days, 'currency', new.currency,
                        'payment_terms', new.payment_terms, 'lead_time', new.lead_time,
                        'bank', new.bank, 'clabe', new.clabe, 'beneficiary', new.beneficiary,
                        'inactive', new.inactive));
END;

CREATE TRIGGER audit_suppliers_au AFTER UPDATE ON suppliers BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('suppliers', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', old.name, 'rfc', old.rfc, 'legal_name', old.legal_name,
                        'street', old.street, 'ext_number', old.ext_number,
                        'neighborhood', old.neighborhood, 'municipality', old.municipality,
                        'state', old.state, 'postal_code', old.postal_code,
                        'tax_regime', old.tax_regime, 'notes', old.notes,
                        'credit_days', old.credit_days, 'currency', old.currency,
                        'payment_terms', old.payment_terms, 'lead_time', old.lead_time,
                        'bank', old.bank, 'clabe', old.clabe, 'beneficiary', old.beneficiary,
                        'inactive', old.inactive),
            json_object('name', new.name, 'rfc', new.rfc, 'legal_name', new.legal_name,
                        'street', new.street, 'ext_number', new.ext_number,
                        'neighborhood', new.neighborhood, 'municipality', new.municipality,
                        'state', new.state, 'postal_code', new.postal_code,
                        'tax_regime', new.tax_regime, 'notes', new.notes,
                        'credit_days', new.credit_days, 'currency', new.currency,
                        'payment_terms', new.payment_terms, 'lead_time', new.lead_time,
                        'bank', new.bank, 'clabe', new.clabe, 'beneficiary', new.beneficiary,
                        'inactive', new.inactive));
END;

CREATE TRIGGER audit_suppliers_ad AFTER DELETE ON suppliers BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('suppliers', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', old.name, 'rfc', old.rfc, 'legal_name', old.legal_name,
                        'street', old.street, 'ext_number', old.ext_number,
                        'neighborhood', old.neighborhood, 'municipality', old.municipality,
                        'state', old.state, 'postal_code', old.postal_code,
                        'tax_regime', old.tax_regime, 'notes', old.notes,
                        'credit_days', old.credit_days, 'currency', old.currency,
                        'payment_terms', old.payment_terms, 'lead_time', old.lead_time,
                        'bank', old.bank, 'clabe', old.clabe, 'beneficiary', old.beneficiary,
                        'inactive', old.inactive));
END;

CREATE TRIGGER audit_supplier_contacts_ai AFTER INSERT ON supplier_contacts BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('supplier_contacts', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('supplier_id', new.supplier_id, 'name', new.name, 'role', new.role,
                        'phone', new.phone, 'email', new.email,
                        'receives_po', new.receives_po));
END;

CREATE TRIGGER audit_supplier_contacts_au AFTER UPDATE ON supplier_contacts BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('supplier_contacts', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('supplier_id', old.supplier_id, 'name', old.name, 'role', old.role,
                        'phone', old.phone, 'email', old.email,
                        'receives_po', old.receives_po),
            json_object('supplier_id', new.supplier_id, 'name', new.name, 'role', new.role,
                        'phone', new.phone, 'email', new.email,
                        'receives_po', new.receives_po));
END;

CREATE TRIGGER audit_supplier_contacts_ad AFTER DELETE ON supplier_contacts BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('supplier_contacts', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('supplier_id', old.supplier_id, 'name', old.name, 'role', old.role,
                        'phone', old.phone, 'email', old.email,
                        'receives_po', old.receives_po));
END;

-- The familia triggers (0031) list their columns: recreate them with the default
-- supplier, so assigning or reassigning a familia is in the trail.
DROP TRIGGER audit_product_families_ai;
DROP TRIGGER audit_product_families_au;
DROP TRIGGER audit_product_families_ad;

CREATE TRIGGER audit_product_families_ai AFTER INSERT ON product_families BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('product_families', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', new.name, 'sat_product_code', new.sat_product_code,
                        'default_supplier_id', new.default_supplier_id));
END;

CREATE TRIGGER audit_product_families_au AFTER UPDATE ON product_families BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('product_families', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', old.name, 'sat_product_code', old.sat_product_code,
                        'default_supplier_id', old.default_supplier_id),
            json_object('name', new.name, 'sat_product_code', new.sat_product_code,
                        'default_supplier_id', new.default_supplier_id));
END;

CREATE TRIGGER audit_product_families_ad AFTER DELETE ON product_families BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('product_families', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', old.name, 'sat_product_code', old.sat_product_code,
                        'default_supplier_id', old.default_supplier_id));
END;

-- Carga inicial (issue #64): Cable Anti Robo, its three contacts (all receive the O.C.)
-- and the familias it supplies. Seeds fire audit triggers; log them as unattributed, so
-- the Historial says "Sistema".
UPDATE audit_actor SET user_id = NULL, source = NULL WHERE id = 1;

INSERT INTO suppliers (name, rfc, legal_name, street, ext_number, neighborhood,
        municipality, state, postal_code)
VALUES ('Cable Anti Robo', 'CAR191126QX6', 'CABLE ANTI ROBO', 'Camino Real de Carretas',
        '356', 'Milenio III', 'Querétaro', 'Querétaro', '76060');

INSERT INTO supplier_contacts (supplier_id, name, phone, email, receives_po)
SELECT id, 'Luis Manuel Enríquez (Sr.)', '+52 442 452 1950', 'direccion@cableantirobo.com', 1
FROM suppliers WHERE rfc = 'CAR191126QX6';
INSERT INTO supplier_contacts (supplier_id, name, phone, email, receives_po)
SELECT id, 'Luis Enríquez (Jr.)', '+52 442 157 7006', 'luis.enriquez@conduccasa.com', 1
FROM suppliers WHERE rfc = 'CAR191126QX6';
INSERT INTO supplier_contacts (supplier_id, name, role, email, receives_po)
SELECT id, 'Cotizaciones', 'Correo general', 'cotizaciones@cableantirobo.com', 1
FROM suppliers WHERE rfc = 'CAR191126QX6';

-- CCA, CCS & AC (renamed "CCS" in some databases, so also found by its QS quote type,
-- like CCA by QA) and Alumoclad. A familia that doesn't exist here is simply skipped;
-- ABASTILUM, Exceline, ASTRO and QL stay without a supplier.
UPDATE product_families
SET default_supplier_id = (SELECT id FROM suppliers WHERE rfc = 'CAR191126QX6')
WHERE free_lines_only = 0
  AND (name COLLATE NOCASE IN ('CCA', 'CCS & AC', 'CCS', 'Alumoclad')
       OR id IN (SELECT family_id FROM quote_types
                 WHERE prefix IN ('QA', 'QS') AND family_id IS NOT NULL));
