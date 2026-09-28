-- Follow-up notes on a quote. Any signed-in user can add one to any quote, in any
-- status (a draft included), and they are never edited or deleted: the table is an
-- append-only log, so it needs no audit triggers — user_id and created_at already say
-- who wrote what and when.
CREATE TABLE quote_comments (
    id         INTEGER PRIMARY KEY,
    quote_id   INTEGER NOT NULL REFERENCES quotes (id),
    user_id    INTEGER NOT NULL REFERENCES users (id),
    body       TEXT NOT NULL CHECK (length(trim(body)) > 0),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_quote_comments_quote_id ON quote_comments (quote_id, created_at);
