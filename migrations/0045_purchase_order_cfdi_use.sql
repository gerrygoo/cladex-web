-- The uso de CFDI a supplier O.C. asks the supplier to put on its factura is chosen per
-- O.C. (Gerry, 2026-11-14): one purchase can be G01 (mercancías) and another G03 (gastos
-- en general). Configuración's "Datos de facturación en órdenes de compra" keeps a uso de
-- CFDI as the default a new borrador starts with.
--
--   purchase_orders.cfdi_use  c_UsoCFDI; NULL on O.C.s from before, whose confirmed
--                             document already carries the uso it was printed with, and
--                             on borradores, which take the default until saved.
--
-- Adding a column breaks nothing the previous build runs; the audit triggers are
-- recreated to record it.
ALTER TABLE purchase_orders ADD COLUMN cfdi_use TEXT;

DROP TRIGGER audit_purchase_orders_ai;
CREATE TRIGGER audit_purchase_orders_ai AFTER INSERT ON purchase_orders BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('purchase_orders', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('project_id', new.project_id, 'quote_id', new.quote_id,
                        'supersedes_id', new.supersedes_id, 'seq', new.seq,
                        'revision', new.revision, 'number', new.number, 'status', new.status,
                        'supplier_id', new.supplier_id, 'delivery_address', new.delivery_address,
                        'delivery_references', new.delivery_references, 'currency', new.currency,
                        'payment_form', new.payment_form, 'credit_days', new.credit_days,
                        'payment_other', new.payment_other, 'cfdi_use', new.cfdi_use, 'total', new.total,
                        'revision_reason', new.revision_reason, 'confirmed_at', new.confirmed_at));
END;

DROP TRIGGER audit_purchase_orders_au;
CREATE TRIGGER audit_purchase_orders_au AFTER UPDATE ON purchase_orders BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('purchase_orders', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('seq', old.seq, 'number', old.number, 'status', old.status,
                        'supplier_id', old.supplier_id, 'delivery_address', old.delivery_address,
                        'delivery_references', old.delivery_references, 'currency', old.currency,
                        'payment_form', old.payment_form, 'credit_days', old.credit_days,
                        'payment_other', old.payment_other, 'cfdi_use', old.cfdi_use, 'total', old.total,
                        'confirmed_at', old.confirmed_at, 'cancel_reason', old.cancel_reason,
                        'cancelled_by', old.cancelled_by, 'cancelled_at', old.cancelled_at),
            json_object('seq', new.seq, 'number', new.number, 'status', new.status,
                        'supplier_id', new.supplier_id, 'delivery_address', new.delivery_address,
                        'delivery_references', new.delivery_references, 'currency', new.currency,
                        'payment_form', new.payment_form, 'credit_days', new.credit_days,
                        'payment_other', new.payment_other, 'cfdi_use', new.cfdi_use, 'total', new.total,
                        'confirmed_at', new.confirmed_at, 'cancel_reason', new.cancel_reason,
                        'cancelled_by', new.cancelled_by, 'cancelled_at', new.cancelled_at));
END;
