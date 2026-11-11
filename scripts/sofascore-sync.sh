#!/usr/bin/env bash
# Sync SofaScore competitions into the prod database from this machine.
# SofaScore blocks the prod server's IP; this laptop (on a network it accepts)
# reads the API and writes straight to prod Postgres through an SSH tunnel.
# Upserts by sofascore_id, so it is safe to run repeatedly. Logos and photos:
# the prod ingestor mirrors them itself (img.sofascore.com is not blocked).
#
# Run by systemd: scripts/systemd/sofascore-sync.{service,timer}.
# Needs: ssh alias newsscore-root, Go, a network SofaScore accepts.
set -euo pipefail

SSH_HOST=${SSH_HOST:-newsscore-root}
APP_UUID=${APP_UUID:-ci1v0ttne7who7dd7ylp4ba3}
LOCAL_PORT=${LOCAL_PORT:-15432}
cd "$(dirname "$0")/.."

# Coolify renames containers on every deploy: look up the current one.
pg=$(ssh -o BatchMode=yes "$SSH_HOST" "docker ps -q --filter name=^postgres-$APP_UUID | head -1")
[ -n "$pg" ] || { echo "postgres container not found" >&2; exit 1; }
ip=$(ssh -o BatchMode=yes "$SSH_HOST" "docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' $pg")
# Credentials stay on the server; fetched per run, never written to disk here.
creds=$(ssh -o BatchMode=yes "$SSH_HOST" "docker exec $pg sh -c 'printf \"%s\n%s\n%s\" \"\$POSTGRES_USER\" \"\$POSTGRES_PASSWORD\" \"\$POSTGRES_DB\"'")
{ read -r user; read -r pass; read -r name; } <<<"$creds"

ssh -o BatchMode=yes -o ExitOnForwardFailure=yes -N -L "127.0.0.1:$LOCAL_PORT:$ip:5432" "$SSH_HOST" &
tunnel=$!
trap 'kill $tunnel 2>/dev/null' EXIT
for _ in $(seq 20); do (exec 3<>/dev/tcp/127.0.0.1/"$LOCAL_PORT") 2>/dev/null && break; sleep 0.5; done

# Real env wins over .env (viper AutomaticEnv), so only the DB points at prod.
DB_HOST=127.0.0.1 DB_PORT=$LOCAL_PORT DB_USER=$user DB_PASSWORD=$pass DB_NAME=$name \
DB_SSL_MODE=require DB_MAX_CONNS=2 DB_MIN_CONNS=0 \
  go run ./cmd/seed-sofascore
