#!/usr/bin/env bash
# Primera vez (y después para cualquier cambio de infraestructura): crea los
# parámetros de /diseno/prod/ que falten y aplica deploy/infra.yaml.
#   AWS_PROFILE=ivan-admin deploy/bootstrap.sh
# Nunca imprime secretos, salvo la contraseña del administrador cuando la crea.
set -euo pipefail
cd "$(dirname "$0")"
export AWS_REGION=${AWS_REGION:-us-east-1} AWS_PAGER=""
STACK=${STACK:-diseno}
P=/diseno/prod

existe() { aws ssm get-parameter --name "$1" --query Parameter.Name --output text >/dev/null 2>&1; }
crear() { # nombre tipo valor
  aws ssm put-parameter --name "$P/$1" --type "$2" --value "$3" >/dev/null
  echo "Creado $P/$1"
}

# Hexadecimal: la contraseña de Postgres va dentro de DATABASE_URL.
existe "$P/POSTGRES_PASSWORD" || crear POSTGRES_PASSWORD SecureString "$(openssl rand -hex 24)"
existe "$P/SESSION_SECRET" || crear SESSION_SECRET SecureString "$(openssl rand -hex 32)"
existe "$P/ADMIN_USERNAME" || crear ADMIN_USERNAME String Admin
existe "$P/ORIGIN_DOMAIN" || crear ORIGIN_DOMAIN String design-origin.nexed.dev
# String y no SecureString: CloudFormation lo lee con {{resolve:ssm:...}} para CloudFront.
existe "$P/ORIGIN_SECRET" || crear ORIGIN_SECRET String "$(openssl rand -hex 32)"
if ! existe "$P/ADMIN_PASSWORD"; then
  admin_nueva=$(openssl rand -hex 12)
  crear ADMIN_PASSWORD SecureString "$admin_nueva"
  # Se muestra aquí y no al final, por si la pila falla después.
  echo "Contraseña inicial del administrador: $admin_nueva (este script no la vuelve a mostrar)"
fi

# CloudFormation vuelve a leer el parámetro del AMI en cada actualización; si AWS
# publicó uno nuevo, reemplazaría la instancia y con ella la base. Por eso el AMI
# se fija una vez en /diseno/ami y solo cambia a propósito (ver README).
if ! existe /diseno/ami; then
  ami=$(aws ssm get-parameter --name /aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64 \
    --query Parameter.Value --output text)
  aws ssm put-parameter --name /diseno/ami --type String --value "$ami" >/dev/null
  echo "AMI fijado en /diseno/ami: $ami"
fi

aws cloudformation deploy --stack-name "$STACK" --template-file infra.yaml \
  --capabilities CAPABILITY_NAMED_IAM --no-fail-on-empty-changeset \
  --parameter-overrides AmiId=/diseno/ami --tags Proyecto=diseno

aws cloudformation describe-stacks --stack-name "$STACK" \
  --query 'Stacks[0].Outputs[].[OutputKey,OutputValue]' --output table

cat <<EOF

Para leer la contraseña inicial del administrador:
  aws ssm get-parameter --name $P/ADMIN_PASSWORD --with-decryption --query Parameter.Value --output text
Siguiente paso: AWS_PROFILE=${AWS_PROFILE:-ivan-admin} deploy/release.sh
EOF
