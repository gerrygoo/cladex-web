#!/bin/sh
# Rebuild the staging database (#81). Runs on the NAS, from anywhere:
# ssh cladex-nas 'sh /volume1/docker/cladex/staging-refresh.sh'
# (docs/NAS_OPERATIONS.md#staging). Whatever was done in staging is lost.
#
# Staging is not a copy of production. It is a new database with production's
# configuration copied in (catalog, quote types, margins, settings; product costs scaled
# by a random factor; documentos oficiales as placeholder PDFs) and synthetic data on
# top: test customers, suppliers and about twenty proyectos. Only the real users in the
# service's STAGING_USERS can sign in, each with a staging-only password that survives
# refreshes (a new user's is printed at the end). See store.StagingTables for what each table gets.
#
# The new database is built next to the live one, so a failed build leaves staging as
# it was. Production keeps running throughout: its configuration is read from sqlite3's
# online backup, which is deleted afterwards.
set -eu

APP_DIR=/volume1/docker/cladex
PROD_DB="$APP_DIR/data/cladex.db"
STAGING_DATA="$APP_DIR/staging/data"
# Paths inside the staging container, where staging/data is /data.
SOURCE=prod-config.db
NEW=cladex.db.new

cleanup() { rm -f "$STAGING_DATA/$SOURCE" "$STAGING_DATA/$SOURCE-wal" "$STAGING_DATA/$SOURCE-shm"; }
trap cleanup EXIT

mkdir -p "$STAGING_DATA"
cleanup
rm -f "$STAGING_DATA/$NEW" "$STAGING_DATA/$NEW-wal" "$STAGING_DATA/$NEW-shm"
sqlite3 "$PROD_DB" ".backup '$STAGING_DATA/$SOURCE'"
sqlite3 "$STAGING_DATA/$SOURCE" "PRAGMA integrity_check;" | grep -qx ok

cd "$APP_DIR"
# Builds and seeds the new database. Staging passwords carry over from the database it
# replaces; only a user new to staging gets one, printed here. It refuses unless the
# service's APP_ENV is staging, and unless the database it builds is new.
if ! docker compose run --rm --no-deps -e DB_PATH="/data/$NEW" cladex-staging seed-staging \
	--config-from "/data/$SOURCE" --keep-passwords-from /data/cladex.db; then
	rm -f "$STAGING_DATA/$NEW" "$STAGING_DATA/$NEW-wal" "$STAGING_DATA/$NEW-shm"
	echo "ERROR: staging was not rebuilt; it keeps its current data" >&2
	exit 1
fi
# Fold the new database's WAL into it, so the swap moves a single file.
sqlite3 "$STAGING_DATA/$NEW" "PRAGMA wal_checkpoint(TRUNCATE);" >/dev/null

docker compose stop cladex-staging
rm -f "$STAGING_DATA/cladex.db-wal" "$STAGING_DATA/cladex.db-shm" "$STAGING_DATA/$NEW-wal" "$STAGING_DATA/$NEW-shm"
mv "$STAGING_DATA/$NEW" "$STAGING_DATA/cladex.db"
docker compose up -d cladex-staging
echo "staging rebuilt from production's configuration at $(date '+%Y-%m-%d %H:%M')"
