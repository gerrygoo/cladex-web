#!/bin/sh
set -eu

# Nightly local backup of the Cladex SQLite DB and stored quote PDFs.
# Runs on the NAS via cron. Point NAS-level cloud sync at $BACKUP_DIR once
# that's set up, for an offsite copy.

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

if [ -d "$DATA_DIR/quotes" ]; then
    cp -r "$DATA_DIR/quotes" "$STAGE/quotes"
fi

tar -C "$STAGE" -czf "$ARCHIVE" .
rm -rf "$STAGE"

find "$BACKUP_DIR" -maxdepth 1 -name 'cladex-*.tar.gz' -mtime "+$KEEP_DAYS" -delete

echo "backup $DATE ok: $ARCHIVE"
