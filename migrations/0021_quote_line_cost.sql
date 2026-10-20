-- Cost on free lines (issue #25). A línea libre has no catalog product behind it, so until
-- now it carried only the price the salesperson typed and added nothing to the quote's
-- profit. cost_micros is its MXN cost per unit, never printed on the PDF:
--   - QL (free_lines_only) quotes require it, together with a margin per line kept in
--     pricing_inputs; the price is derived from both, as catalog lines are.
--   - Free lines on other series may carry it; the price is still typed.
-- It stays NULL on catalog lines (their cost is in pricing_inputs) and on every existing
-- row.
--
-- Costs change after a quote goes out, so the quote's owner or an admin may update
-- cost_micros on an issued quote. That is the one UPDATE quote_lines ever gets (drafts
-- are rewritten by delete + insert), and it changes the profit figures, so it is audited
-- even though quote_lines is otherwise not (see 0004_audit_log.sql).
ALTER TABLE quote_lines ADD COLUMN cost_micros INTEGER CHECK (cost_micros >= 0);

CREATE TRIGGER audit_quote_lines_cost_au AFTER UPDATE OF cost_micros ON quote_lines
WHEN old.cost_micros IS NOT new.cost_micros BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('quote_lines', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('quote_id', old.quote_id, 'line_no', old.line_no, 'cost_micros', old.cost_micros),
            json_object('quote_id', new.quote_id, 'line_no', new.line_no, 'cost_micros', new.cost_micros));
END;

-- Términos y condiciones adicionales (issue #27): on a QL quote the salesperson may add
-- terms of their own, one per line, printed as bullets right after the series' "Pago por
-- adelantado para colocar OC". NULL or empty adds nothing. Issuing folds them into
-- terms_snapshot, so this column only matters while the quote is a draft.
ALTER TABLE quotes ADD COLUMN extra_terms TEXT;
