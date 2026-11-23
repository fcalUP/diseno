#!/bin/bash
# Corre EN la instancia, como root, mandado por release.sh vía SSM:
#   deploy.sh TAG REPO_URI BUCKET
# Baja el paquete de la versión, arma .env con los parámetros de /diseno/prod/,
# levanta los contenedores y, si la app no queda sana, vuelve a la versión anterior.
set -euo pipefail

if [ $# -ne 3 ]; then
  echo "uso: deploy.sh TAG REPO_URI BUCKET" >&2
  exit 2
fi
TAG=$1 REPO_URI=$2 BUCKET=$3
export AWS_REGION=us-east-1 AWS_PAGER=""

# En una instancia recién creada el user data puede seguir instalando Docker.
cloud-init status --wait >/dev/null 2>&1 || true

cd /opt/diseno
# Un despliegue a la vez: el de CI y un release.sh a mano pueden coincidir.
exec 9>.lock
if ! flock -n 9; then
  echo "Hay otro despliegue en curso en la instancia; reintenta cuando termine." >&2
  exit 1
fi
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

for f in compose.yml Caddyfile backup.sh smoke.py; do
  aws s3 cp --only-show-errors "s3://$BUCKET/deploy/$TAG/$f" "$tmp/$f"
done

aws ssm get-parameters-by-path --path /diseno/prod/ --with-decryption \
  --query 'Parameters[].[Name,Value]' --output text > "$tmp/params"
param() { awk -F'\t' -v k="/diseno/prod/$1" '$1 == k { print $2 }' "$tmp/params"; }

for nombre in POSTGRES_PASSWORD SESSION_SECRET ADMIN_PASSWORD ORIGIN_DOMAIN ORIGIN_SECRET; do
  if [ -z "$(param "$nombre")" ]; then
    echo "Falta $nombre en /diseno/prod/ (corre deploy/bootstrap.sh)." >&2
    exit 1
  fi
done
ORIGIN_DOMAIN=$(param ORIGIN_DOMAIN)
ORIGIN_SECRET=$(param ORIGIN_SECRET)
# Los valores van entre comillas simples, que Compose toma literales. No caben
# comillas simples, saltos de línea ni tabuladores, y el nombre debe servir de variable.
if grep -q "'" "$tmp/params" ||
  awk -F'\t' 'NF != 2 || $1 !~ /^\/diseno\/prod\/[A-Za-z_][A-Za-z0-9_]*$/ { m = 1 } END { exit !m }' "$tmp/params"; then
  echo "Un parámetro de /diseno/prod/ trae comilla simple, salto de línea o tabulador, o su nombre no sirve de variable; no cabe en .env." >&2
  exit 1
fi
{
  awk -F'\t' '{ n = $1; sub(/.*\//, "", n); printf "%s=\047%s\047\n", n, $2 }' "$tmp/params"
  echo "APP_IMAGE='$REPO_URI:$TAG'"
  echo "S3_BUCKET='$BUCKET'"
} > "$tmp/.env"

# Valida antes de reemplazar la versión en uso. En particular, producción siempre
# necesita ADMIN_PASSWORD: si falta, el backend generaría una y la escribiría al log.
docker compose --env-file "$tmp/.env" -f "$tmp/compose.yml" config --quiet

aws ecr get-login-password | docker login --username AWS --password-stdin "${REPO_URI%%/*}" >/dev/null

# Lo que había queda en *.prev para volver si la versión nueva falla.
primera=1
if [ -f .env ]; then
  primera=0
  for f in .env compose.yml Caddyfile backup.sh; do
    if [ -f "$f" ]; then cp -p "$f" "$f.prev"; fi
  done
fi
install -m 600 "$tmp/.env" .env
install -m 644 "$tmp/compose.yml" compose.yml
install -m 644 "$tmp/Caddyfile" Caddyfile
install -m 755 "$tmp/backup.sh" backup.sh

volver() {
  echo "Regreso a la versión anterior." >&2
  # Esta salida acaba en el log público de GitHub Actions: solo va la app, cuyo
  # registro no trae IP. El de Caddy sí las trae (CloudFront-Viewer-Address).
  docker compose logs --no-color --tail 60 app >&2 || true
  if [ "$primera" = 0 ]; then
    for f in .env compose.yml Caddyfile backup.sh; do
      if [ -f "$f.prev" ]; then mv -f "$f.prev" "$f"; fi
    done
    docker compose up -d --remove-orphans || true
    docker compose restart caddy || true
  fi
  exit 1
}

docker compose pull --quiet || volver
docker compose up -d --remove-orphans || volver
# Compose no ve cambios dentro de archivos montados: si cambió el Caddyfile, se reinicia Caddy.
if [ "$primera" = 0 ] && ! cmp -s Caddyfile Caddyfile.prev; then
  docker compose restart caddy || volver
fi

# Hasta 150 s para que el healthcheck de la app diga healthy.
estado=
for _ in $(seq 30); do
  cid=$(docker compose ps -q app)
  estado=$(docker inspect -f '{{.State.Health.Status}}' "$cid" 2>/dev/null || true)
  if [ "$estado" = healthy ]; then break; fi
  sleep 5
done
if [ "$estado" != healthy ]; then
  echo "La app no quedó sana (estado: ${estado:-desconocido})." >&2
  volver
fi
echo "App sana con $REPO_URI:$TAG"

# De punta a punta por Caddy, como llega CloudFront. En la primera versión el
# certificado puede seguir en trámite: ahí solo se avisa.
caddy_ok=
for _ in $(seq 24); do
  if curl -fsS -o /dev/null --max-time 5 --resolve "$ORIGIN_DOMAIN:443:127.0.0.1" \
    -H "X-Origin-Verify: $ORIGIN_SECRET" "https://$ORIGIN_DOMAIN/api/health" 2>/dev/null; then
    caddy_ok=1
    break
  fi
  sleep 5
done
if [ -n "$caddy_ok" ]; then
  echo "Caddy responde por HTTPS en $ORIGIN_DOMAIN"
  SMOKE_BASE_URL="https://$ORIGIN_DOMAIN" SMOKE_CONNECT_HOST=127.0.0.1 \
    SMOKE_PUBLIC_URL=https://design.nexed.dev SMOKE_ORIGIN_SECRET="$ORIGIN_SECRET" \
    SMOKE_REQUIRE_WEB=1 python3 "$tmp/smoke.py" || volver
elif [ "$primera" = 1 ]; then
  echo "AVISO: Caddy aún no responde por HTTPS; quizá el certificado sigue en trámite (docker compose logs caddy)." >&2
else
  echo "Caddy no responde por HTTPS en $ORIGIN_DOMAIN; lee su registro por Session Manager (docker compose logs caddy, ver deploy/README.md)." >&2
  volver
fi

docker image prune -af --filter until=168h >/dev/null || true
