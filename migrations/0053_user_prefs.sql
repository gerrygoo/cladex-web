-- #90: what each user chose to see on their own screens, remembered between visits.
-- hide_completados hides Mis pendientes' Completados. A per-user convenience, so it isn't
-- audited and staging starts it empty.
CREATE TABLE user_prefs (
    user_id          INTEGER PRIMARY KEY REFERENCES users (id),
    hide_completados INTEGER NOT NULL DEFAULT 0 CHECK (hide_completados IN (0, 1))
);
