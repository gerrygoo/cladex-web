-- Pendientes libres (issue #72; docs/planning/dashboard.md): the to-dos a user writes on
-- Mis pendientes, with a priority they pick. They are private: every query is scoped to
-- the signed-in user and no screen lists anyone else's, admins included.
--
-- Deliberately NOT audited: the audit log is readable by admins, so an audit trigger here
-- would copy each pendiente's text where others can read it. Whoever runs the server or
-- holds the backups can still read this table, as with any other.
CREATE TABLE pendientes (
    id           INTEGER PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users (id),
    body         TEXT NOT NULL CHECK (length(trim(body)) > 0),
    priority     TEXT NOT NULL DEFAULT 'media' CHECK (priority IN ('baja', 'media', 'alta', 'muy_alta')),
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    completed_at TEXT
);

CREATE INDEX idx_pendientes_user ON pendientes (user_id, completed_at);
