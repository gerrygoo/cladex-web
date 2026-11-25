-- #87: a payment recorded by hand ("Registrar pago") says how the client paid: its SAT
-- forma de pago (c_FormaPago, e.g. 03 Transferencia), which its complemento de pago starts
-- with. Required for new manual payments; NULL on the ones recorded before, and on those a
-- factura or complemento recorded (theirs is the comprobante's).
ALTER TABLE payments ADD COLUMN forma_pago TEXT;

DROP TRIGGER audit_payments_ai;
CREATE TRIGGER audit_payments_ai AFTER INSERT ON payments BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('payments', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('project_id', new.project_id, 'amount', new.amount, 'paid_on', new.paid_on,
                        'source', new.source, 'comprobante_id', new.comprobante_id,
                        'file_id', new.file_id, 'forma_pago', new.forma_pago, 'created_by', new.created_by));
END;

DROP TRIGGER audit_payments_au;
CREATE TRIGGER audit_payments_au AFTER UPDATE ON payments BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('payments', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('project_id', old.project_id, 'amount', old.amount, 'paid_on', old.paid_on,
                        'source', old.source, 'comprobante_id', old.comprobante_id,
                        'file_id', old.file_id, 'forma_pago', old.forma_pago, 'deleted_by', old.deleted_by,
                        'deleted_at', old.deleted_at, 'delete_reason', old.delete_reason),
            json_object('project_id', new.project_id, 'amount', new.amount, 'paid_on', new.paid_on,
                        'source', new.source, 'comprobante_id', new.comprobante_id,
                        'file_id', new.file_id, 'forma_pago', new.forma_pago, 'deleted_by', new.deleted_by,
                        'deleted_at', new.deleted_at, 'delete_reason', new.delete_reason));
END;
