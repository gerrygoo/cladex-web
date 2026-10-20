#!/bin/sh
# Replace the staging database with a fresh copy of production's. Runs on the NAS, from
# anywhere: ssh cladex-nas 'sh /volume1/docker/cladex/staging-refresh.sh'
# (docs/NAS_OPERATIONS.md#staging). Whatever was done in staging is lost.
#
# The copy is sqlite3's online backup of the live database, so production keeps running.
# Sessions are dropped from the copy: production's login cookies don't open staging.
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

cd "$APP_DIR"
docker compose stop cladex-staging
rm -f "$STAGING_DATA/cladex.db-wal" "$STAGING_DATA/cladex.db-shm"
mv "$NEW" "$STAGING_DATA/cladex.db"
docker compose up -d cladex-staging
echo "staging refreshed from production at $(date '+%Y-%m-%d %H:%M')"
