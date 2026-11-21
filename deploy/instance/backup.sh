#!/bin/bash
# Respaldo diario de Postgres al bucket: s3://BUCKET/backups/db/AAAA-MM-DD.dump
# Lo corre diseno-backup.timer (09:00 UTC); también sirve a mano.
set -euo pipefail
export AWS_REGION=us-east-1 AWS_PAGER=""
cd /opt/diseno

bucket=$(sed -n "s/^S3_BUCKET='\(.*\)'$/\1/p" .env)
key="backups/db/$(date -u +%F).dump"
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT

# Primero a un archivo: si pg_dump falla no queda en S3 un respaldo a medias.
docker compose exec -T db sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "$tmp"
aws s3 cp --only-show-errors "$tmp" "s3://$bucket/$key"
echo "Respaldo en s3://$bucket/$key ($(stat -c %s "$tmp") bytes)"
