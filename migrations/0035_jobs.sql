-- Trabajos en segundo plano (#19; docs/planning/notificaciones-y-trabajos.md §1): the
-- queue of the runner inside the server (internal/jobs). A job is a handler's kind plus
-- a JSON payload, due at run_at:
--
--   pendiente → en_curso → hecho
--       ↑  ↖ (error with attempts left, or an expired lease) ↙
--       |              en_curso → fallido → (retried by hand) pendiente
--   pendiente → cancelado (what it was for went away)
--
-- - en_curso: claimed by a runner until lease_until. A lease that runs out means the
--   process died mid-job, so the job goes back to pendiente (or fallido, out of attempts).
-- - attempts counts claims; a failure with attempts < max_attempts goes back to pendiente
--   with run_at pushed out by the backoff.
-- - result: short text from the handler, mainly why a job did nothing.
-- Rows are never pruned, so the record of every run stays. No audit triggers: it's
-- machine churn, and what matters to people is recorded by the handlers in their tables.
-- Times are UTC in strftime's '%Y-%m-%dT%H:%M:%fZ' shape, which compares as text.
CREATE TABLE jobs (
    id           INTEGER PRIMARY KEY,
    kind         TEXT NOT NULL,
    payload      TEXT NOT NULL DEFAULT '{}',
    dedupe_key   TEXT,
    status       TEXT NOT NULL DEFAULT 'pendiente' CHECK (status IN
                     ('pendiente', 'en_curso', 'hecho', 'fallido', 'cancelado')),
    run_at       TEXT NOT NULL,
    attempts     INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    lease_until  TEXT,
    last_error   TEXT,
    result       TEXT,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    finished_at  TEXT
);

-- A dedupe key is unique among a kind's live jobs only: once one is done, failed or
-- cancelled, the same key can be queued again.
CREATE UNIQUE INDEX jobs_live_dedupe ON jobs (kind, dedupe_key)
    WHERE dedupe_key IS NOT NULL AND status IN ('pendiente', 'en_curso');

-- The runner's claim looks for the oldest due pendiente job.
CREATE INDEX jobs_due ON jobs (status, run_at);
