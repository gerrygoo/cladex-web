-- Per-prefix folio counters for quotes (QA/QS/QI — see 0001's quotes.prefix CHECK).
-- One row per prefix; next_number is the next folio number to hand out. Store.NextFolio
-- increments this with a single atomic UPSERT + RETURNING, so folio assignment has no
-- read-then-write race window even under concurrent draft creation.
CREATE TABLE folio_sequences (
    prefix      TEXT PRIMARY KEY CHECK (prefix IN ('QA', 'QS', 'QI')),
    next_number INTEGER NOT NULL DEFAULT 1
);
