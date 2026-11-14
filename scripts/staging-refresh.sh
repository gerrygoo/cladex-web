#!/bin/sh
# Replace the staging database with a fresh copy of production's. Runs on the NAS, from
# anywhere: ssh cladex-nas 'sh /volume1/docker/cladex/staging-refresh.sh'
# (docs/NAS_OPERATIONS.md#staging). Whatever was done in staging is lost.
#
# The copy is sqlite3's online backup of the live database, so production keeps running.
# Sessions are dropped from the copy: production's login cookies don't open staging.
# Then the test data is added on top (seed-staging).
set -eu

APP_DIR=/volume1/docker/cladex
PROD_DB="$APP_DIR/data/cladex.db"
STAGING_DATA="$APP_DIR/staging/data"
NEW="$STAGING_DATA/cladex.db.refresh"

mkdir -p "$STAGING_DATA"
rm -f "$NEW"
sqlite3 "$PROD_DB" ".backup '$NEW'"
sqlite3 "$NEW" "PRAGMA integrity_check;" | grep -qx ok
sqlite3 "$NEW" "DELETE FROM sessions;"
# Staging stamps with the SAT's test CSD (ESCUELA KEMPER URGATE), so its emisor is that
# company, and its CFDIs get their own series so they never look like production's.
sqlite3 "$NEW" "INSERT INTO settings (key, value) VALUES
  ('emisor_rfc', 'EKU9003173C9'), ('emisor_nombre', 'ESCUELA KEMPER URGATE'), ('emisor_regimen', '601'),
  ('emisor_cp', '42501'), ('factura_serie', 'PRUEBAF'), ('pago_serie', 'PRUEBAP')
  ON CONFLICT (key) DO UPDATE SET value = excluded.value;"

cd "$APP_DIR"
docker compose stop cladex-staging
rm -f "$STAGING_DATA/cladex.db-wal" "$STAGING_DATA/cladex.db-shm"
mv "$NEW" "$STAGING_DATA/cladex.db"
# Add the test data (users, customers valid in the SAT's sandbox, suppliers, products and
# proyectos in several states; cladex seed-staging, web.SeedStaging) so staging is a
# superset of production. The test users' password is printed here. It refuses to run
# unless the service's APP_ENV is staging.
# A failed seed must not leave staging down: warn, start it anyway, and re-run the seed
# by hand (it resumes): docker compose run --rm --no-deps cladex-staging seed-staging
docker compose run --rm --no-deps cladex-staging seed-staging || echo "AVISO: los datos de prueba no se agregaron completos"
docker compose up -d cladex-staging
echo "staging refreshed from production at $(date '+%Y-%m-%d %H:%M')"
