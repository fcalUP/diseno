#!/usr/bin/env bash
# Primera vez (y después para cualquier cambio de infraestructura): crea lo que
# falte en /diseno/prod/, aplica deploy/infra.yaml y deja los secretos como
# SecureString con la llave de la pila (alias/diseno). Repetirlo es seguro.
#   AWS_PROFILE=ivan-admin deploy/bootstrap.sh
# Nunca imprime secretos, salvo la contraseña del administrador cuando la crea.
set -euo pipefail
cd "$(dirname "$0")"
export AWS_REGION=${AWS_REGION:-us-east-1} AWS_PAGER=""
STACK=${STACK:-diseno}
P=/diseno/prod
LLAVE=alias/diseno
err=$(mktemp)
trap 'rm -f "$err"' EXIT

existe() { aws ssm get-parameter --name "$1" --query Parameter.Name --output text >/dev/null 2>&1; }
# Nombre, tipo y llave de lo que ya hay en $P, sin valores.
meta=$(aws ssm describe-parameters --parameter-filters "Key=Path,Values=$P" \
  --query 'Parameters[].[Name,Type,KeyId]' --output text)
tipo() { awk -v n="$P/$1" '$1 == n { print $2 }' <<<"$meta"; }
llave() { awk -v n="$P/$1" '$1 == n { print $3 }' <<<"$meta"; }
leer() { aws ssm get-parameter --name "$P/$1" --with-decryption --query Parameter.Value --output text; }
poner() { # nombre tipo valor [más opciones de put-parameter]
  local n=$1 t=$2 v=$3
  shift 3
  # --value=: un valor que empiece con guion no se toma por opción.
  aws ssm put-parameter --name "$P/$n" --type "$t" --value="$v" "$@" >/dev/null
}
crear() { poner "$@" && echo "Creado $P/$1"; }

# (a) Lo que no es secreto.
[ -n "$(tipo ADMIN_USERNAME)" ] || crear ADMIN_USERNAME String Admin
[ -n "$(tipo ORIGIN_DOMAIN)" ] || crear ORIGIN_DOMAIN String design-origin.nexed.dev

# CloudFormation vuelve a leer el parámetro del AMI en cada actualización; si AWS
# publicó uno nuevo, reemplazaría la instancia y con ella la base. Por eso el AMI
# se fija una vez en /diseno/ami y solo cambia a propósito (ver README).
if ! existe /diseno/ami; then
  ami=$(aws ssm get-parameter --name /aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64 \
    --query Parameter.Value --output text)
  aws ssm put-parameter --name /diseno/ami --type String --value "$ami" >/dev/null
  echo "AMI fijado en /diseno/ami: $ami"
fi

# (b) El secreto del origen: el que ya hay (String en versiones anteriores, o
# SecureString) o uno nuevo, que se guarda en (d) porque la llave nace en (c).
if [ -n "$(tipo ORIGIN_SECRET)" ]; then
  origin_secret=$(leer ORIGIN_SECRET)
else
  origin_secret=$(openssl rand -hex 32)
fi

# (c) La pila. CloudFront recibe el secreto como parámetro NoEcho.
aws cloudformation deploy --stack-name "$STACK" --template-file infra.yaml \
  --capabilities CAPABILITY_NAMED_IAM --no-fail-on-empty-changeset \
  --parameter-overrides AmiId=/diseno/ami "OriginSecret=$origin_secret" --tags Proyecto=diseno

# (d) Los secretos, como SecureString con alias/diseno. Para cambiar de tipo o de
# llave se borra y se vuelve a crear: el tipo no cambia de otra forma, y así se van
# también las versiones anteriores, que seguían cifradas con aws/ssm y cualquier rol
# de la cuenta lee como "nombre:1".
cifrar() { # nombre valor [--overwrite]
  local i
  # KMS es de consistencia eventual: el alias recién creado puede tardar en verse.
  for i in 1 2 3 4 5 6; do
    if poner "$1" SecureString "$2" --key-id "$LLAVE" ${3:+"$3"} 2>"$err"; then return; fi
    [ "$i" = 6 ] || sleep 5
  done
  cat "$err" >&2
  return 1
}
borrar() { # si un reintento del CLI llega después de un borrado que sí ocurrió, responde ParameterNotFound
  aws ssm delete-parameter --name "$P/$1" 2>"$err" || grep -q ParameterNotFound "$err" || {
    cat "$err" >&2
    exit 1
  }
}
asegurar() { # nombre valor_si_falta
  local t k v
  t=$(tipo "$1") k=$(llave "$1")
  if [ -z "$t" ]; then
    cifrar "$1" "$2"
    echo "Creado $P/$1"
  elif [ "$t" != SecureString ] || [ "$k" != "$LLAVE" ]; then
    v=$(leer "$1")
    borrar "$1"
    # --overwrite: si un intento anterior sí quedó guardado, el siguiente no falla.
    if ! cifrar "$1" "$v" --overwrite; then
      echo "No se pudo guardar $P/$1 con $LLAVE; se restaura como estaba." >&2
      if [ "$t" = SecureString ]; then poner "$1" "$t" "$v" --key-id "$k"; else poner "$1" "$t" "$v"; fi ||
        echo "Revisa $P/$1: si ya no existe, su valor sigue en /opt/diseno/.env de la instancia." >&2
      exit 1
    fi
    echo "Migrado $P/$1 a SecureString con $LLAVE"
  fi
}
# Hexadecimal: la contraseña de Postgres va dentro de DATABASE_URL.
asegurar POSTGRES_PASSWORD "$(openssl rand -hex 24)"
asegurar SESSION_SECRET "$(openssl rand -hex 32)"
asegurar ORIGIN_SECRET "$origin_secret"
admin_nueva=$(openssl rand -hex 12)
asegurar ADMIN_PASSWORD "$admin_nueva"
if [ -z "$(tipo ADMIN_PASSWORD)" ]; then
  echo "Contraseña inicial del administrador: $admin_nueva (este script no la vuelve a mostrar)"
fi

aws cloudformation describe-stacks --stack-name "$STACK" \
  --query 'Stacks[0].Outputs[].[OutputKey,OutputValue]' --output table

cat <<EOF

Para leer la contraseña inicial del administrador:
  aws ssm get-parameter --name $P/ADMIN_PASSWORD --with-decryption --query Parameter.Value --output text
Siguiente paso: AWS_PROFILE=${AWS_PROFILE:-ivan-admin} deploy/release.sh
EOF
