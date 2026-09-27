#!/bin/sh
# Daily Postgres dump, kept 7 days locally. Cron (as a user in the docker group):
#   15 2 * * * /path/to/repo/deploy/backup.sh >> /var/log/newsscore-backup.log 2>&1
# Set BACKUP_REMOTE (an rclone remote, e.g. "r2:newsscore-backups") to also copy
# off the VPS: a backup on the same disk does not survive losing the VPS.
set -eu

cd "$(dirname "$0")"
mkdir -p backups

name="backups/newsscore-$(date -u +%Y%m%dT%H%M%SZ).dump"

docker compose exec -T postgres sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "$name.partial"
mv "$name.partial" "$name"

find backups -name '*.dump' -mtime +7 -delete
find backups -name '*.partial' -delete

if [ -n "${BACKUP_REMOTE:-}" ]; then
	rclone copy backups "$BACKUP_REMOTE" --include '*.dump'
fi

echo "backup ok: $name"
