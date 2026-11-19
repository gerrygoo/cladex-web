-- Follow-ups to #82's listas de precios.
--
-- 1. quote_types.validity_days: how long a quote or price list of the type is valid, in
--    days from the day it's issued. NULL keeps the rule every catalog type has had (issue
--    #26): until the last day of that month. Emilio: "de momento ... hasta el último día
--    del mes ... El administrador debe poder cambiar la vigencia de cada familia". QL
--    types ignore it; their vigencia is typed in.
--
-- 2. Products without a unit get the one Emilio said their list is priced in, so the
--    lists (and the builder) say "1 m" or "1 pza" instead of a bare 1: metres for the
--    cable brands (ConduCCAsa = CCA, copperclad = CCS & AC, Alumoclad), pieces for
--    Exceline and Alumbrado (the large-Cladex catalog type, QI). Astro's products already
--    have Caja (0039). Only a NULL unit is filled; a unit someone chose is left alone.
ALTER TABLE quote_types ADD COLUMN validity_days INTEGER
    CHECK (validity_days IS NULL OR (validity_days BETWEEN 1 AND 365));

UPDATE products SET unit_id = (SELECT id FROM units WHERE code = 'm')
WHERE unit_id IS NULL AND deleted_at IS NULL
  AND family_id IN (SELECT family_id FROM quote_types
                    WHERE free_lines_only = 0 AND family_id IS NOT NULL
                      AND letterhead IN ('conduccasa', 'copperclad', 'alumoclad'));

UPDATE products SET unit_id = (SELECT id FROM units WHERE code = 'pza')
WHERE unit_id IS NULL AND deleted_at IS NULL
  AND family_id IN (SELECT family_id FROM quote_types
                    WHERE free_lines_only = 0 AND family_id IS NOT NULL
                      AND letterhead IN ('exceline', 'cladex_large'));
