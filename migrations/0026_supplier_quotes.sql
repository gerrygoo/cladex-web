-- Cotización del proveedor (issue #39): a QL quote is put together from a quote a
-- supplier sent Cladex, kept here as evidence: who sent it, its number, and the files
-- (PDF or images). None of it is printed on the client's PDF.
--
-- It belongs to the revision chain, not to one quote: QL0003-R1 and -R2, and the
-- proyecto QL0003, see what QL0003 has, without copying. The chain is named by its
-- first quote (the one that revises nothing), which exists from the first draft on,
-- before there is a proyecto, and never changes; see store.QuoteChainRoot.
--
-- One row per chain that has a supplier or a number typed in. It is edited in place,
-- so it is audited like quotes.
CREATE TABLE supplier_quotes (
    root_quote_id INTEGER PRIMARY KEY REFERENCES quotes (id),
    supplier      TEXT NOT NULL DEFAULT '',
    number        TEXT NOT NULL DEFAULT '',
    updated_by    INTEGER NOT NULL REFERENCES users (id),
    updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TRIGGER audit_supplier_quotes_ai AFTER INSERT ON supplier_quotes BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('supplier_quotes', new.root_quote_id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('supplier', new.supplier, 'number', new.number));
END;

CREATE TRIGGER audit_supplier_quotes_au AFTER UPDATE ON supplier_quotes BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('supplier_quotes', new.root_quote_id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('supplier', old.supplier, 'number', old.number),
            json_object('supplier', new.supplier, 'number', new.number));
END;

CREATE TRIGGER audit_supplier_quotes_ad AFTER DELETE ON supplier_quotes BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('supplier_quotes', old.root_quote_id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('supplier', old.supplier, 'number', old.number));
END;

-- The files, with their bytes in the database as in official_documents (0023); the app
-- caps a file at 10 MB and accepts PDF, JPG and PNG, by what the bytes are. Removing one
-- only stamps deleted_at/deleted_by, and only while the chain's current quote is a
-- borrador and the file came after the chain's last emission: what an issued quote was
-- based on stays. Rows are otherwise never updated, so like official_documents the
-- table has no audit triggers.
CREATE TABLE supplier_quote_files (
    id            INTEGER PRIMARY KEY,
    root_quote_id INTEGER NOT NULL REFERENCES quotes (id),
    filename      TEXT NOT NULL CHECK (length(trim(filename)) > 0),
    content_type  TEXT NOT NULL,
    size          INTEGER NOT NULL,
    data          BLOB NOT NULL,
    uploaded_by   INTEGER NOT NULL REFERENCES users (id),
    uploaded_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    deleted_at    TEXT,
    deleted_by    INTEGER REFERENCES users (id)
);
CREATE INDEX idx_supplier_quote_files_root ON supplier_quote_files (root_quote_id, id) WHERE deleted_at IS NULL;
