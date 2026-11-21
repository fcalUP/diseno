#!/usr/bin/env bash
# Construye la imagen arm64, la sube a ECR, sincroniza los medios heredados y
# despliega en la instancia por SSM. Lo corre GitHub Actions en cada push a main
# y también sirve desde tu máquina:
#   AWS_PROFILE=ivan-admin deploy/release.sh
# Variables opcionales: TAG, STACK (diseno) y BUILD_CACHE (argumentos extra de caché
# para buildx, p. ej. --cache-from type=gha).
set -euo pipefail
cd "$(dirname "$0")/.."
export AWS_REGION=${AWS_REGION:-us-east-1} AWS_PAGER=""
STACK=${STACK:-diseno}
TAG=${TAG:-$(git rev-parse --short=12 HEAD)-$(date +%Y%m%d%H%M%S)}
if ! [[ $TAG =~ ^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$ ]]; then
  echo "TAG inválido: $TAG" >&2
  exit 1
fi

salida() {
  aws cloudformation describe-stacks --stack-name "$STACK" \
    --query "Stacks[0].Outputs[?OutputKey=='$1'].OutputValue" --output text
}
INSTANCE=$(salida InstanceId)
BUCKET=$(salida BucketName)
REPO=$(salida RepositoryUri)
for v in "$INSTANCE" "$BUCKET" "$REPO"; do
  if [ -z "$v" ] || [ "$v" = None ]; then
    echo "La pila $STACK no tiene las salidas esperadas; corre deploy/bootstrap.sh." >&2
    exit 1
  fi
done
echo "Versión $TAG → $INSTANCE"

# Las imágenes base vienen de ECR Public; con sesión no topamos con el límite anónimo.
aws ecr-public get-login-password --region us-east-1 |
  docker login --username AWS --password-stdin public.ecr.aws >/dev/null ||
  echo "Sin sesión en ECR Public; se baja como anónimo." >&2
aws ecr get-login-password | docker login --username AWS --password-stdin "${REPO%%/*}" >/dev/null

# El Dockerfile compila cruzado: no hace falta QEMU para arm64.
# shellcheck disable=SC2086 # BUILD_CACHE trae varios argumentos a propósito
docker buildx build --platform linux/arm64 --provenance=false --push \
  ${BUILD_CACHE:-} -t "$REPO:$TAG" .

aws s3 sync --size-only --only-show-errors seed/media/legacy "s3://$BUCKET/media/legacy"

for f in deploy/compose.prod.yml:compose.yml deploy/Caddyfile:Caddyfile \
  deploy/instance/backup.sh:backup.sh deploy/instance/deploy.sh:deploy.sh; do
  aws s3 cp --only-show-errors "${f%%:*}" "s3://$BUCKET/deploy/$TAG/${f##*:}"
done

cmd="mkdir -p /opt/diseno && aws s3 cp --only-show-errors s3://$BUCKET/deploy/$TAG/deploy.sh /opt/diseno/deploy.sh && bash /opt/diseno/deploy.sh $TAG $REPO $BUCKET"
params=$(printf '{"commands":["%s"],"executionTimeout":["900"]}' "$cmd")

# Una instancia recién creada tarda un par de minutos en registrarse con SSM.
err=$(mktemp)
trap 'rm -f "$err"' EXIT
for intento in $(seq 30); do
  if CMD_ID=$(aws ssm send-command --instance-ids "$INSTANCE" --document-name AWS-RunShellScript \
    --comment "diseno $TAG" --parameters "$params" --query Command.CommandId --output text 2>"$err"); then
    break
  fi
  if ! grep -q InvalidInstanceId "$err" || [ "$intento" = 30 ]; then
    cat "$err" >&2
    exit 1
  fi
  echo "La instancia aún no está en SSM; reintento en 10 s." >&2
  sleep 10
done

invocacion() {
  aws ssm get-command-invocation --command-id "$CMD_ID" --instance-id "$INSTANCE" --query "$1" --output text
}
estado=Pending
for _ in $(seq 240); do
  sleep 5
  estado=$(invocacion Status 2>/dev/null || echo Pending)
  case $estado in Pending | InProgress | Delayed) ;; *) break ;; esac
done
invocacion StandardOutputContent || true
invocacion StandardErrorContent >&2 || true
echo "Despliegue $TAG: $estado"
[ "$estado" = Success ]
