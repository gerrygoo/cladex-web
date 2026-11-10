-- Avisos (issue #69; docs/planning/notificaciones-y-trabajos.md §3). Which avisos each
-- user gets, and which scheduled rounds have already gone out.
--
-- user_avisos: a row turns one kind of aviso on for one user; none means off, so every
-- aviso starts off for everyone. Only an admin adds or removes rows (Usuarios → Avisos),
-- so they are audited like a user's role. The channels are still the user's own
-- notify_email / notify_whatsapp: a kind on and no channel on sends nothing.
--
-- aviso_rounds: one row per round of a scheduled aviso (e.g. 'seguimiento 2026-11-12T09:00'),
-- written in the same transaction as the notifications it queued, so a round retried
-- after a crash never sends twice. Rows are never deleted.
--
-- Two new tables leave the previous build's queries working, so this isn't exclusive.
CREATE TABLE user_avisos (
    user_id    INTEGER NOT NULL REFERENCES users (id),
    kind       TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (user_id, kind)
);
CREATE INDEX idx_user_avisos_kind ON user_avisos (kind);

CREATE TRIGGER audit_user_avisos_ai AFTER INSERT ON user_avisos BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('user_avisos', new.user_id || ':' || new.kind, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('user_id', new.user_id, 'kind', new.kind));
END;

CREATE TRIGGER audit_user_avisos_ad AFTER DELETE ON user_avisos BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('user_avisos', old.user_id || ':' || old.kind, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('user_id', old.user_id, 'kind', old.kind));
END;

CREATE TABLE aviso_rounds (
    round      TEXT PRIMARY KEY,
    queued     INTEGER NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
