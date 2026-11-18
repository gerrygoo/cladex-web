-- Leads (#82, slice L2 of docs/planning/leads-y-listas-de-precios.md): people who ask for
-- prices before there's a real proyecto. A lead never shows in Proyectos, Pronóstico or
-- the dashboards; it has its own list, history and seguimiento, and becomes a proyecto
-- only through "Convertir en proyecto", which creates the draft quote (quote_id) that
-- turns into the proyecto when it's issued.
--
-- leads:
--   contact_name        the person who asked (required)
--   customer_id         the customer they work for, when it already exists (a new
--                       contact of a known customer); set on conversion otherwise
--   company             the company typed in when there's no customer yet ('' if none)
--   phone, email        at least one (checked by the app)
--   user_id             the vendedor in charge: the one the seguimiento avisos go to
--   next_followup_date  when to follow up next (YYYY-MM-DD, Mexico City), NULL for none
--   status              abierto | perdido | convertido
--   lost_reason         why it was lost, when perdido
--   quote_id            the draft quote "Convertir en proyecto" created
CREATE TABLE leads (
    id                 INTEGER PRIMARY KEY,
    contact_name       TEXT NOT NULL CHECK (length(trim(contact_name)) > 0),
    customer_id        INTEGER REFERENCES customers (id),
    company            TEXT NOT NULL DEFAULT '',
    phone              TEXT NOT NULL DEFAULT '',
    email              TEXT NOT NULL DEFAULT '',
    user_id            INTEGER NOT NULL REFERENCES users (id),
    next_followup_date TEXT,
    status             TEXT NOT NULL DEFAULT 'abierto'
        CHECK (status IN ('abierto', 'perdido', 'convertido')),
    lost_reason        TEXT,
    quote_id           INTEGER REFERENCES quotes (id),
    created_by         INTEGER REFERENCES users (id),
    created_at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    closed_at          TEXT
);
CREATE INDEX idx_leads_followup ON leads (status, next_followup_date);
CREATE INDEX idx_leads_quote ON leads (quote_id);

-- A lead's history: notes, follow-ups (with the new date they set) and what happened
-- to it (perdido, convertido), in the words the app writes.
CREATE TABLE lead_comments (
    id         INTEGER PRIMARY KEY,
    lead_id    INTEGER NOT NULL REFERENCES leads (id),
    user_id    INTEGER NOT NULL REFERENCES users (id),
    body       TEXT NOT NULL CHECK (length(trim(body)) > 0),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_lead_comments_lead ON lead_comments (lead_id, created_at);

-- A seguimiento aviso about a lead names it, as one about a proyecto names the proyecto.
ALTER TABLE notifications ADD COLUMN lead_id INTEGER REFERENCES leads (id);

CREATE TRIGGER audit_leads_ai AFTER INSERT ON leads BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, new_values)
    VALUES ('leads', new.id, 'insert',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('contact_name', new.contact_name, 'customer_id', new.customer_id,
                        'company', new.company, 'phone', new.phone, 'email', new.email,
                        'user_id', new.user_id, 'next_followup_date', new.next_followup_date,
                        'status', new.status, 'lost_reason', new.lost_reason,
                        'quote_id', new.quote_id));
END;

CREATE TRIGGER audit_leads_au AFTER UPDATE ON leads BEGIN
    INSERT INTO audit_log (table_name, row_key, op, actor_id, source, old_values, new_values)
    VALUES ('leads', new.id, 'update',
            (SELECT user_id FROM audit_actor WHERE id = 1),
            (SELECT source FROM audit_actor WHERE id = 1),
            json_object('contact_name', old.contact_name, 'customer_id', old.customer_id,
                        'company', old.company, 'phone', old.phone, 'email', old.email,
                        'user_id', old.user_id, 'next_followup_date', old.next_followup_date,
                        'status', old.status, 'lost_reason', old.lost_reason,
                        'quote_id', old.quote_id),
            json_object('contact_name', new.contact_name, 'customer_id', new.customer_id,
                        'company', new.company, 'phone', new.phone, 'email', new.email,
                        'user_id', new.user_id, 'next_followup_date', new.next_followup_date,
                        'status', new.status, 'lost_reason', new.lost_reason,
                        'quote_id', new.quote_id));
END;
