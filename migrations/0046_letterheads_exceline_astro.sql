-- cladex:foreign-keys-off
-- cladex:exclusive
-- Exceline and Astro logos on their quotes (issues #60, #68). Emilio asked for the QEXL
-- (Exceline) and QAST (Astro) series to print their own logo instead of Cladex's, like
-- Alumoclad's QG (#48). quote_types.letterhead gains 'exceline' and 'astro', and those
-- two series switch to them; a database without them (staging, tests) just gains the
-- values. Issued quotes keep the letterhead they were issued with (letterhead_snapshot).
--
-- SQLite can't change a CHECK in place, so quote_types is rebuilt; quotes references it,
-- so the runner applies this file with foreign keys off and checks them before
-- committing. quote_types has no triggers or extra indexes to recreate. A rebuild drops
-- the table under the previous build, so it's exclusive: deploy.sh pauses production.
CREATE TABLE quote_types_new (
    id                  INTEGER PRIMARY KEY,
    prefix              TEXT NOT NULL UNIQUE,
    name                TEXT NOT NULL DEFAULT '',
    terms               TEXT NOT NULL DEFAULT '',
    next_number         INTEGER NOT NULL DEFAULT 1 CHECK (next_number >= 1),
    family_id           INTEGER REFERENCES product_families (id),
    free_lines_only     INTEGER NOT NULL DEFAULT 0 CHECK (free_lines_only IN (0, 1)),
    hide_address        INTEGER NOT NULL DEFAULT 0 CHECK (hide_address IN (0, 1)),
    sort_last           INTEGER NOT NULL DEFAULT 0 CHECK (sort_last IN (0, 1)),
    kg_priced           INTEGER NOT NULL DEFAULT 0 CHECK (kg_priced IN (0, 1)),
    copper_price_margin INTEGER NOT NULL DEFAULT 0 CHECK (copper_price_margin IN (0, 1)),
    letterhead          TEXT NOT NULL DEFAULT 'cladex' CHECK (letterhead IN
        ('cladex', 'cladex_large', 'conduccasa', 'copperclad', 'alumoclad', 'exceline', 'astro')),
    created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
INSERT INTO quote_types_new (id, prefix, name, terms, next_number, family_id, free_lines_only,
        hide_address, sort_last, kg_priced, copper_price_margin, letterhead, created_at)
SELECT id, prefix, name, terms, next_number, family_id, free_lines_only,
        hide_address, sort_last, kg_priced, copper_price_margin, letterhead, created_at
FROM quote_types;
DROP TABLE quote_types;
ALTER TABLE quote_types_new RENAME TO quote_types;

UPDATE quote_types SET letterhead = 'exceline' WHERE prefix = 'QEXL';
UPDATE quote_types SET letterhead = 'astro' WHERE prefix = 'QAST';
