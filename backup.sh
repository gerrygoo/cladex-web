#!/bin/sh
set -eu

# Nightly local backup of the Cladex SQLite DB.
# Runs on the NAS via cron. Point NAS-level cloud sync at $BACKUP_DIR once
# that's set up, for an offsite copy.
#
# This is the archival copy, not the recovery window: the litestream service in
# compose.yaml replicates the DB continuously into $BACKUP_DIR/litestream, so a lost
# disk costs seconds rather than up to a day. What this script adds is a
# self-contained tarball that restores without any tooling. Quote PDFs aren't stored
# (they're regenerated from the DB), so the DB is everything.

APP_DIR=/volume1/docker/cladex
DATA_DIR="$APP_DIR/data"
BACKUP_DIR="$APP_DIR/backups"
KEEP_DAYS=14

DATE=$(date +%Y-%m-%d)
STAGE="$BACKUP_DIR/.stage-$DATE"
ARCHIVE="$BACKUP_DIR/cladex-$DATE.tar.gz"

mkdir -p "$BACKUP_DIR"
rm -rf "$STAGE"
mkdir -p "$STAGE"

sqlite3 "$DATA_DIR/cladex.db" ".backup '$STAGE/cladex.db'"
sqlite3 "$STAGE/cladex.db" "PRAGMA integrity_check;" | grep -qx ok

tar -C "$STAGE" -czf "$ARCHIVE" .
rm -rf "$STAGE"

find "$BACKUP_DIR" -maxdepth 1 -name 'cladex-*.tar.gz' -mtime "+$KEEP_DAYS" -delete

echo "backup $DATE ok: $ARCHIVE"
