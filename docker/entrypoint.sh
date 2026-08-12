#!/bin/sh
set -e

# Restore the database from the replica if the local volume is empty.
# -if-db-not-exists  → no-op on normal restarts (DB already on the volume)
# -if-replica-exists → no-op on the very first boot (no backup yet)
litestream restore -if-db-not-exists -if-replica-exists -config /etc/litestream.yml /data/db.sqlite

# Replicate continuously while supervising the app. Litestream forwards
# signals and exits when tt exits.
exec litestream replicate -config /etc/litestream.yml -exec "tt --data-dir /data --port 8080"
