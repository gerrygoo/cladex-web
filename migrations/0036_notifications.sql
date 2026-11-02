-- Notificaciones (#18; docs/planning/notificaciones-y-trabajos.md §2, slice N2).
--
-- Where a user wants to be told things: an email, a WhatsApp number (E.164, "+52…"), and
-- whether each is on. A user edits their own on Mi cuenta; an admin, anyone's on Usuarios.
-- The users audit triggers list their columns, so they're recreated with these.
ALTER TABLE users ADD COLUMN email TEXT;
ALTER TABLE users ADD COLUMN whatsapp TEXT;
ALTER TABLE users ADD COLUMN notify_email INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN notify_whatsapp INTEGER NOT NULL DEFAULT 0;

DROP TRIGGER audit_users_ai;
DROP TRIGGER audit_users_au;
DROP TRIGGER audit_users_ad;

CREATE TRIGGER audit_users_ai AFTER INSERT ON users BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('users', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('username', new.username, 'name', new.name, 'role', new.role,
                        'disabled_at', new.disabled_at, 'email', new.email,
                        'whatsapp', new.whatsapp, 'notify_email', new.notify_email,
                        'notify_whatsapp', new.notify_whatsapp));
END;

CREATE TRIGGER audit_users_au AFTER UPDATE ON users BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('users', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('username', old.username, 'name', old.name, 'role', old.role,
                        'disabled_at', old.disabled_at, 'email', old.email,
                        'whatsapp', old.whatsapp, 'notify_email', old.notify_email,
                        'notify_whatsapp', old.notify_whatsapp),
            json_object('username', new.username, 'name', new.name, 'role', new.role,
                        'disabled_at', new.disabled_at, 'email', new.email,
                        'whatsapp', new.whatsapp, 'notify_email', new.notify_email,
                        'notify_whatsapp', new.notify_whatsapp,
                        'password_changed',
                        CASE WHEN old.password_hash <> new.password_hash THEN 1 ELSE 0 END));
END;

CREATE TRIGGER audit_users_ad AFTER DELETE ON users BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values)
    VALUES ('users', old.id, 'delete',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('username', old.username, 'name', old.name, 'role', old.role,
                        'disabled_at', old.disabled_at, 'email', old.email,
                        'whatsapp', old.whatsapp, 'notify_email', old.notify_email,
                        'notify_whatsapp', old.notify_whatsapp));
END;

-- A notification is one message, to one recipient, on one channel. It is written in the
-- same transaction as whatever caused it, together with the job that sends it
-- (kind 'notificacion'), and delivered by that job with its retries:
--
--   por_enviar → enviando → aceptada → entregada (provider webhook, from N4/N5)
--        ↑   ↖ temporary error ↙   ↘ fallida (permanent error, out of attempts, or a bounce)
--        └──────── retried by hand ───────┘
--   por_enviar → cancelada (no longer applies, or outside production to an address not
--                           on NOTIFY_ALLOW)
--
-- - address is the email or phone as it was when queued; the recipient is user_id or
--   customer_id (one of them, or neither for an "Enviar prueba" to a typed address).
-- - Email carries subject, body_text and body_html; WhatsApp, template and params (JSON).
-- - provider is who took it ('resend', 'smtp', 'fake', 'whatsapp'), provider_message_id
--   their id for it.
-- - job_id is a plain reference without a foreign key: the notification's record never
--   depends on the job row.
-- Notifications and their events are never deleted.
CREATE TABLE notifications (
    id                  INTEGER PRIMARY KEY,
    kind                TEXT NOT NULL,
    channel             TEXT NOT NULL CHECK (channel IN ('email', 'whatsapp')),
    user_id             INTEGER REFERENCES users (id),
    customer_id         INTEGER REFERENCES customers (id),
    address             TEXT NOT NULL,
    project_id          INTEGER REFERENCES projects (id),
    comprobante_id      INTEGER REFERENCES comprobantes (id),
    subject             TEXT,
    body_text           TEXT,
    body_html           TEXT,
    template            TEXT,
    params              TEXT,
    status              TEXT NOT NULL DEFAULT 'por_enviar' CHECK (status IN
                            ('por_enviar', 'enviando', 'aceptada', 'entregada', 'fallida', 'cancelada')),
    provider            TEXT,
    provider_message_id TEXT,
    error               TEXT,
    job_id              INTEGER,
    created_by          INTEGER REFERENCES users (id),
    created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    sent_at             TEXT
);

CREATE INDEX notifications_created ON notifications (created_at);

-- Each attempt and outcome, in the spirit of comprobante_events: queued, sending,
-- accepted, error, failed, cancelled, retried (by hand, with user_id).
CREATE TABLE notification_events (
    id              INTEGER PRIMARY KEY,
    notification_id INTEGER NOT NULL REFERENCES notifications (id),
    event           TEXT NOT NULL,
    detail          TEXT,
    user_id         INTEGER REFERENCES users (id),
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX notification_events_notification ON notification_events (notification_id);
