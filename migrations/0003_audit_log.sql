-- Row-level audit trail: who changed what, when.
--
-- SQLite has no binlog and no logical replication, so the CDC tooling that exists for
-- MySQL/Postgres (Debezium, wal2json, pgaudit) does not apply here. Triggers are the
-- equivalent, and they have one property no application-level audit log has: they fire
-- for *every* writer of this file — the web app, `cladex user ...`, `cmd/import`, and
-- anyone who opens the DB with the sqlite3 shell on the NAS. A store method added later
-- that forgets to log is still audited.
--
-- What triggers cannot see is *who*: SQLite has no session user. The app supplies one
-- via audit_actor below.

-- Single-row scratch table holding the actor for the in-flight write. store.exec sets it
-- and performs the write in one transaction; SQLite permits only one write transaction
-- at a time (even in WAL mode), so the row cannot be read by another writer's trigger
-- mid-write. It is deliberately not a TEMP table: SQLite rejects triggers that reference
-- objects in the temp database ("cannot reference objects in database temp").
CREATE TABLE audit_actor (
    id      INTEGER PRIMARY KEY CHECK (id = 1),
    user_id INTEGER, -- users.id; no FK, see audit_log
    source  TEXT     -- 'web', 'cli', 'import', or NULL for an unattributed write
);
INSERT INTO audit_actor (id, user_id, source) VALUES (1, NULL, NULL);

-- The trail itself. Append-only (see the guard triggers at the bottom).
--
-- No foreign keys, by design: an audit row must never fail to write, and must survive
-- the row it describes. row_key is TEXT so tables keyed by something other than an
-- INTEGER id (settings.key) fit the same shape.
CREATE TABLE audit_log (
    id         INTEGER PRIMARY KEY,
    table_name TEXT NOT NULL,
    row_key    TEXT NOT NULL,
    op         TEXT NOT NULL CHECK (op IN ('insert', 'update', 'delete')),
    actor_id   INTEGER, -- users.id, or NULL for CLI/import/unattributed writes
    source     TEXT,
    old_values TEXT, -- JSON of the audited columns before; NULL on insert
    new_values TEXT, -- JSON of the audited columns after; NULL on delete
    at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_audit_log_row ON audit_log (table_name, row_key, id);
CREATE INDEX idx_audit_log_at ON audit_log (at);
CREATE INDEX idx_audit_log_actor ON audit_log (actor_id, id);

-- Audited tables are the ones where "who changed this" has money or security weight:
-- prices (products, price_breaks), pricing knobs (settings), invoicing data (customers),
-- and access (users). quotes is audited for its status transitions.
--
-- Deliberately NOT audited:
--   sessions, login_attempts -- churn, and login_attempts is already an audit trail
--   quote_lines              -- rewritten on every draft keystroke; the issued quote
--                               already snapshots price, description, and pricing inputs
--   schema_migrations        -- append-only by construction

CREATE TRIGGER audit_products_ai AFTER INSERT ON products BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('products', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('family_id', new.family_id, 'sku', new.sku,
                        'description', new.description,
                        'kg_per_m_micros', new.kg_per_m_micros,
                        'unit_price_micros', new.unit_price_micros,
                        'cost_micros', new.cost_micros, 'currency', new.currency,
                        'active', new.active, 'deleted_at', new.deleted_at));
END;

CREATE TRIGGER audit_products_au AFTER UPDATE ON products BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('products', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('family_id', old.family_id, 'sku', old.sku,
                        'description', old.description,
                        'kg_per_m_micros', old.kg_per_m_micros,
                        'unit_price_micros', old.unit_price_micros,
                        'cost_micros', old.cost_micros, 'currency', old.currency,
                        'active', old.active, 'deleted_at', old.deleted_at),
            json_object('family_id', new.family_id, 'sku', new.sku,
                        'description', new.description,
                        'kg_per_m_micros', new.kg_per_m_micros,
                        'unit_price_micros', new.unit_price_micros,
                        'cost_micros', new.cost_micros, 'currency', new.currency,
                        'active', new.active, 'deleted_at', new.deleted_at));
END;

CREATE TRIGGER audit_products_ad AFTER DELETE ON products BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('products', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('family_id', old.family_id, 'sku', old.sku,
                        'description', old.description,
                        'kg_per_m_micros', old.kg_per_m_micros,
                        'unit_price_micros', old.unit_price_micros,
                        'cost_micros', old.cost_micros, 'currency', old.currency,
                        'active', old.active, 'deleted_at', old.deleted_at));
END;

CREATE TRIGGER audit_price_breaks_ai AFTER INSERT ON price_breaks BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('price_breaks', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('product_id', new.product_id, 'min_qty_milli', new.min_qty_milli,
                        'unit_price_micros', new.unit_price_micros));
END;

CREATE TRIGGER audit_price_breaks_au AFTER UPDATE ON price_breaks BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('price_breaks', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('product_id', old.product_id, 'min_qty_milli', old.min_qty_milli,
                        'unit_price_micros', old.unit_price_micros),
            json_object('product_id', new.product_id, 'min_qty_milli', new.min_qty_milli,
                        'unit_price_micros', new.unit_price_micros));
END;

CREATE TRIGGER audit_price_breaks_ad AFTER DELETE ON price_breaks BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('price_breaks', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('product_id', old.product_id, 'min_qty_milli', old.min_qty_milli,
                        'unit_price_micros', old.unit_price_micros));
END;

CREATE TRIGGER audit_customers_ai AFTER INSERT ON customers BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('customers', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', new.name, 'rfc', new.rfc, 'contact_name', new.contact_name,
                        'phone', new.phone, 'email', new.email, 'address', new.address,
                        'notes', new.notes, 'deleted_at', new.deleted_at));
END;

CREATE TRIGGER audit_customers_au AFTER UPDATE ON customers BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('customers', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', old.name, 'rfc', old.rfc, 'contact_name', old.contact_name,
                        'phone', old.phone, 'email', old.email, 'address', old.address,
                        'notes', old.notes, 'deleted_at', old.deleted_at),
            json_object('name', new.name, 'rfc', new.rfc, 'contact_name', new.contact_name,
                        'phone', new.phone, 'email', new.email, 'address', new.address,
                        'notes', new.notes, 'deleted_at', new.deleted_at));
END;

CREATE TRIGGER audit_customers_ad AFTER DELETE ON customers BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('customers', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('name', old.name, 'rfc', old.rfc, 'contact_name', old.contact_name,
                        'phone', old.phone, 'email', old.email, 'address', old.address,
                        'notes', old.notes, 'deleted_at', old.deleted_at));
END;

-- users.password_hash is never written to the trail — a bcrypt hash is a credential, and
-- the audit log is the most-read table in an incident. A boolean records that it changed.
CREATE TRIGGER audit_users_ai AFTER INSERT ON users BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('users', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('username', new.username, 'name', new.name, 'role', new.role,
                        'disabled_at', new.disabled_at));
END;

CREATE TRIGGER audit_users_au AFTER UPDATE ON users BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('users', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('username', old.username, 'name', old.name, 'role', old.role,
                        'disabled_at', old.disabled_at),
            json_object('username', new.username, 'name', new.name, 'role', new.role,
                        'disabled_at', new.disabled_at,
                        'password_changed',
                        CASE WHEN old.password_hash <> new.password_hash THEN 1 ELSE 0 END));
END;

CREATE TRIGGER audit_users_ad AFTER DELETE ON users BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('users', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('username', old.username, 'name', old.name, 'role', old.role,
                        'disabled_at', old.disabled_at));
END;

-- settings is keyed by TEXT, hence row_key holding the key rather than an id.
CREATE TRIGGER audit_settings_ai AFTER INSERT ON settings BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('settings', new.key, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('value', new.value, 'updated_by', new.updated_by));
END;

CREATE TRIGGER audit_settings_au AFTER UPDATE ON settings BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('settings', new.key, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('value', old.value, 'updated_by', old.updated_by),
            json_object('value', new.value, 'updated_by', new.updated_by));
END;

CREATE TRIGGER audit_settings_ad AFTER DELETE ON settings BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('settings', old.key, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('value', old.value, 'updated_by', old.updated_by));
END;

CREATE TRIGGER audit_quotes_ai AFTER INSERT ON quotes BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('quotes', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('folio', new.folio, 'customer_id', new.customer_id,
                        'user_id', new.user_id, 'status', new.status,
                        'currency', new.currency,
                        'fx_rate_used_micros', new.fx_rate_used_micros,
                        'subtotal', new.subtotal, 'iva', new.iva, 'total', new.total,
                        'issued_at', new.issued_at, 'valid_until', new.valid_until,
                        'supersedes_quote_id', new.supersedes_quote_id,
                        'pdf_sha256', new.pdf_sha256));
END;

CREATE TRIGGER audit_quotes_au AFTER UPDATE ON quotes BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('quotes', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('folio', old.folio, 'customer_id', old.customer_id,
                        'user_id', old.user_id, 'status', old.status,
                        'currency', old.currency,
                        'fx_rate_used_micros', old.fx_rate_used_micros,
                        'subtotal', old.subtotal, 'iva', old.iva, 'total', old.total,
                        'issued_at', old.issued_at, 'valid_until', old.valid_until,
                        'supersedes_quote_id', old.supersedes_quote_id,
                        'pdf_sha256', old.pdf_sha256),
            json_object('folio', new.folio, 'customer_id', new.customer_id,
                        'user_id', new.user_id, 'status', new.status,
                        'currency', new.currency,
                        'fx_rate_used_micros', new.fx_rate_used_micros,
                        'subtotal', new.subtotal, 'iva', new.iva, 'total', new.total,
                        'issued_at', new.issued_at, 'valid_until', new.valid_until,
                        'supersedes_quote_id', new.supersedes_quote_id,
                        'pdf_sha256', new.pdf_sha256));
END;

CREATE TRIGGER audit_quotes_ad AFTER DELETE ON quotes BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('quotes', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('folio', old.folio, 'customer_id', old.customer_id,
                        'user_id', old.user_id, 'status', old.status,
                        'currency', old.currency,
                        'fx_rate_used_micros', old.fx_rate_used_micros,
                        'subtotal', old.subtotal, 'iva', old.iva, 'total', old.total,
                        'issued_at', old.issued_at, 'valid_until', old.valid_until,
                        'supersedes_quote_id', old.supersedes_quote_id,
                        'pdf_sha256', old.pdf_sha256));
END;

-- Append-only guards. These stop a careless UPDATE/DELETE from the app or the sqlite3
-- shell; they are not a defence against someone with write access to the file, which no
-- in-database mechanism can be. A future retention policy has to DROP these first, which
-- is the point: pruning the trail should be a deliberate, reviewed migration.
CREATE TRIGGER audit_log_no_update BEFORE UPDATE ON audit_log BEGIN
    SELECT RAISE(ABORT, 'audit_log is append-only');
END;

CREATE TRIGGER audit_log_no_delete BEFORE DELETE ON audit_log BEGIN
    SELECT RAISE(ABORT, 'audit_log is append-only');
END;
