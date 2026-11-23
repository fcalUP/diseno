# Despliegue de diseno

Para retomar el desarrollo, empieza por [AGENTS.md](../AGENTS.md), la
[guía de validación](../docs/validacion.md) y el
[registro de estado y decisiones](../docs/estado-y-decisiones.md).

Todo vive en la cuenta 113380982662 (perfil `ivan-admin`), región `us-east-1`, en una sola pila de
CloudFormation llamada `diseno` ([`infra.yaml`](infra.yaml)). La instancia va en la VPC por defecto
(`vpc-07a9d9ca49e59006d`), aparte de las VPC de Portal.

## Cómo está armado

```
navegador ──HTTPS──► design.nexed.dev   CloudFront + WAF compartido
                       ├─ /media/*   ──► S3 diseno-media-113380982662/media/   (con caché)
                       ├─ /assets/*  ──┐                                       (con caché)
                       └─ lo demás   ──┴─► design-origin.nexed.dev:443         (sin caché)
                                             IP elástica; el 443 solo acepta IP de CloudFront
                                             │
                          EC2 t4g.small, Amazon Linux 2023, Docker Compose
                            caddy   revisa X-Origin-Verify; certificado de Let's Encrypt
                            app     Go y el front compilado (imagen de ECR)
                            db      Postgres 18 en un volumen de Docker
                          09:00 UTC ──► s3://diseno-media-113380982662/backups/db/

panel de administración ──PUT firmado──► S3 media/u/   (las imágenes no pasan por el API)
```

Lo que conviene tener en la cabeza:

- Al 443 de la instancia solo llegan las IP de CloudFront, y Caddy además contesta 403 si falta
  `X-Origin-Verify`. El valor vive en `/diseno/prod/ORIGIN_SECRET`; `bootstrap.sh` se lo pasa a
  CloudFormation como parámetro NoEcho (`OriginSecret`) para la distribución y `deploy.sh` a Caddy.
- El 80 queda abierto para que Let's Encrypt valide el certificado de `design-origin.nexed.dev`. La
  cuenta ACME va sin correo; si quieres uno, crea `/diseno/prod/ACME_EMAIL` y vuelve a desplegar.
- El grupo de seguridad no permite entrada por SSH; la operación usa Session Manager.
  El servicio `sshd` puede seguir escuchando dentro del host.
- La plantilla de la política de peticiones de esta distribución reenvía `Sec-Fetch-Site`
  para la comprobación CSRF adicional del API. Si se actualiza desde una plantilla anterior,
  este encabezado requiere aplicar el cambio de CloudFormation; publicar únicamente
  una imagen no actualiza la política. Se conservan las comprobaciones de `Origin`
  y JSON aunque el encabezado no esté presente.
- El WAF compartido bloquea cuerpos de más de 8 KB, curl y los navegadores sin interfaz, y le pone
  CAPTCHA a las IP de nube, incluidos los runners de GitHub. Por eso la prueba de humo corre dentro
  de la instancia y nunca a través de CloudFront.
- Los secretos están en Parameter Store bajo `/diseno/prod/`, cifrados con la llave KMS de la pila
  (`alias/diseno`). En cada despliegue `deploy.sh` los escribe en `/opt/diseno/.env` (modo 600). La
  instancia no puede leer parámetros de otros proyectos y los roles de otros proyectos no pueden
  descifrar estos (ver [Llave y secretos](#llave-y-secretos)). Cada valor va en una sola línea y sin
  comillas simples; si alguno no cabe en `.env`, `deploy.sh` se detiene antes de tocar nada.
- `/media/*` sale con su propia política de encabezados (`diseno-media`): CSP
  `default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; sandbox`, `nosniff`, HSTS,
  `X-Frame-Options: DENY` y `Referrer-Policy: same-origin`. Así un SVG o un HTML subido desde el
  panel no corre código en el dominio del sitio.

| Archivo | Qué hace |
|---|---|
| `infra.yaml` | Bucket, ECR, roles, llave KMS, grupo de seguridad, instancia, IP elástica, DNS, certificado y CloudFront. |
| `bootstrap.sh` | Crea los parámetros de `/diseno/prod/` que falten, aplica la pila y deja los secretos cifrados con `alias/diseno`. Se corre a mano. |
| `release.sh` | Construye la imagen arm64, la sube a ECR, sincroniza `seed/media/legacy` con `media/legacy/`, sube el paquete a `deploy/<etiqueta>/` y manda instalar por SSM. Lo usa CI y también sirve desde tu máquina. |
| `instance/deploy.sh` | En la instancia: arma `.env`, baja la imagen, levanta Compose, espera a que la app esté sana y prueba Caddy de punta a punta. Si algo falla, vuelve a la versión anterior. |
| `instance/backup.sh` | `pg_dump` de la base al bucket. |
| `instance/smoke.py` | Comprueba API, autenticación requerida, SPA, assets y encabezados a través de Caddy. |
| `compose.prod.yml`, `Caddyfile` | Lo que corre en la instancia. |

## Primera vez

Necesitas el AWS CLI con el perfil `ivan-admin`, Docker con buildx y `openssl`.

```sh
AWS_PROFILE=ivan-admin deploy/bootstrap.sh
AWS_PROFILE=ivan-admin deploy/release.sh
```

`bootstrap.sh` tarda de 10 a 20 minutos la primera vez; el certificado y CloudFront son lo lento.
Crea en `/diseno/prod/` solo lo que falta y nunca cambia un valor que ya existe:

| Parámetro | Tipo |
|---|---|
| `POSTGRES_PASSWORD`, `SESSION_SECRET`, `ADMIN_PASSWORD`, `ORIGIN_SECRET` | SecureString con `alias/diseno`, al azar |
| `ADMIN_USERNAME` (`Admin`), `ORIGIN_DOMAIN` | String |

Los secretos se crean después de aplicar la pila, porque la llave nace con ella.

La contraseña del administrador se imprime una sola vez, cuando se crea. Después la lees con:

```sh
aws ssm get-parameter --name /diseno/prod/ADMIN_PASSWORD --with-decryption --query Parameter.Value --output text
```

Solo sirve para crear el primer administrador con la base vacía. Si la cambias en el panel, el
parámetro se queda con la vieja.

En el primer `release.sh` es normal ver un aviso de que Caddy aún no responde: está pidiendo el
certificado del origen. En un par de minutos https://design.nexed.dev debe cargar.

## CI/CD

[`.github/workflows/ci-cd.yml`](../.github/workflows/ci-cd.yml) corre en cada push y en cada pull request:

- `frontend`: `npm ci`, `npm run lint` y `npm run build`.
- `backend`: `gofmt`, `go vet` y `go test ./...` contra un Postgres 18 de servicio.
- `deployment-config`: sintaxis y ShellCheck de los scripts, Compose de producción y validación de Caddy.
- `deploy`: solo en push a `main` y si los tres anteriores pasaron. Asume `diseno-github-deploy` por OIDC,
  sin llaves de AWS guardadas en GitHub, y corre `release.sh` con la etiqueta
  `<sha de 12>-<run_number>-<run_attempt>`. Las etiquetas de ECR son inmutables; por eso reintentar un
  trabajo genera una nueva.

Nunca corren dos despliegues de CI a la vez (grupo `deploy-prod`). Si un `release.sh` a mano llega
mientras otro despliegue sigue en la instancia, `deploy.sh` lo rechaza. El rol solo acepta la rama
`main` de `fcalUP/diseno`.

Si la app no queda sana en 150 s, `deploy.sh` regresa `.env`, `compose.yml` y el `Caddyfile`
anteriores, levanta la versión previa y el trabajo sale en rojo. Para volver atrás a propósito, haz
`git revert` del commit y empuja a `main`.

Ese rollback restaura código y configuración, **no deshace migraciones SQL** ya confirmadas ni
escrituras de usuarios. Una migración nueva debe conservar la compatibilidad necesaria con la
imagen anterior; recuperar datos exige un respaldo y el procedimiento de restauración.

Un push que solo cambie Markdown también dispara este flujo. Para una entrega exclusivamente
documental se puede omitir ese CI con `[skip ci]` en el mensaje del commit, según la
[documentación de GitHub](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/skip-workflow-runs).
No lo uses para cambios de código o infraestructura. En un PR con checks obligatorios, omitir el
workflow los deja pendientes y puede impedir el merge; usa el flujo normal en ese caso.

Las acciones van fijadas al commit, con la versión en un comentario (`@<sha> # v7.0.1`): una etiqueta
se puede mover y el trabajo `deploy` corre con el rol de AWS. Para subir una de versión, busca el
commit de la etiqueta nueva y cambia el SHA y el comentario:

```sh
gh api repos/actions/checkout/git/ref/tags/v7.0.2 --jq '.object.type + " " + .object.sha'
# Si el tipo es "tag" (etiqueta anotada), el commit sale de:
gh api repos/actions/checkout/git/tags/<sha> --jq .object.sha
```

### Protección de `main` (pendiente: la configura un administrador del repo)

Quien pueda empujar a `main` despliega, porque el rol de AWS confía en esa rama. Alguien con permisos
de administrador en `fcalUP/diseno` debería:

1. Proteger `main` (Settings → Rules o Branches): pull request con al menos una aprobación, `frontend`,
   `backend` y `deployment-config` como checks obligatorios, sin force-push y sin borrado.
2. Si se quiere una aprobación antes de cada despliegue, crear el environment `production` con
   revisores obligatorios y solo `main` como rama permitida. Después, en un mismo cambio, agregar
   `environment: production` al trabajo `deploy` y cambiar en `infra.yaml` el `sub` que acepta
   `DeployRole` a `repo:fcalUP/diseno:environment:production` (y su variante con ids): con environment,
   GitHub firma el token con ese `sub` y ya no con la rama. No agregues ese `sub` antes de que el
   environment exista con sus reglas, porque un flujo que nombra un environment inexistente lo crea
   sin protección.

## Registros

Entra a la instancia con Session Manager (hace falta el plugin de Session Manager del AWS CLI):

```sh
export AWS_PROFILE=ivan-admin
aws ssm start-session --target "$(aws cloudformation describe-stacks --stack-name diseno \
  --query "Stacks[0].Outputs[?OutputKey=='InstanceId'].OutputValue" --output text)"
sudo -i
cd /opt/diseno
docker compose ps
docker compose logs -f app        # o caddy, o db
```

El primer arranque queda en `/var/log/cloud-init-output.log`. Docker rota solo los registros de los
contenedores. La salida de cada despliegue aparece en el trabajo de GitHub y en el historial de Run
Command de SSM.

Si un despliegue falla, el trabajo de GitHub muestra las últimas líneas de la app (método, ruta,
estado y duración, sin IP). Las de Caddy no salen ahí: traen la IP de cada visitante y el log de
Actions es público. Léelas desde esta sesión con `docker compose logs caddy`.

Los parches del sistema se aplican desde esa misma sesión con `dnf upgrade --releasever=latest`.

## Respaldos

Cada día a las 09:00 UTC (03:00 en Ciudad de México) `diseno-backup.timer` corre `backup.sh`, que deja
`s3://diseno-media-113380982662/backups/db/AAAA-MM-DD.dump`. Se conservan 35 días, más 30 como
versión anterior porque el bucket tiene versionado. Para uno a mano, en la instancia:
`systemctl start diseno-backup`.

Para restaurar, como root en la instancia:

```sh
cd /opt/diseno
aws s3 cp s3://diseno-media-113380982662/backups/db/2026-11-20.dump /tmp/respaldo.dump
docker compose stop app
docker compose exec -T db sh -c 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --single-transaction' < /tmp/respaldo.dump
docker compose start app
```

Los medios no necesitan respaldo aparte: S3 guarda 30 días lo que se borra o se reemplaza.

## Cambios de infraestructura

Edita `infra.yaml` y vuelve a correr `bootstrap.sh` después de revisar el cambio propuesto.
Repetirlo sin cambios conserva los valores existentes; una edición de infraestructura puede
reemplazar recursos o interrumpir el servicio. Revisa especialmente la instancia y su disco.

El `UserData` de la instancia solo corre en el primer arranque. Si lo cambias, CloudFormation detiene
la instancia y la vuelve a arrancar (un par de minutos sin servicio) y aun así no lo ejecuta: el
cambio solo aplica a una instancia nueva.

El AMI queda fijo en `/diseno/ami`. CloudFormation vuelve a leer ese parámetro en cada actualización, y
si apuntara al AMI más reciente de AWS, cualquier cambio a la pila podría reemplazar la instancia, con
la base adentro. Para cambiar de AMI a propósito:

1. Haz un respaldo.
2. Quita la protección contra terminación de la instancia actual
   (`aws ec2 modify-instance-attribute --instance-id <id> --no-disable-api-termination`); si no,
   CloudFormation no puede borrar la vieja.
3. Pon el AMI nuevo en `/diseno/ami` y corre `bootstrap.sh`. La instancia se reemplaza.
4. Corre `release.sh` y restaura el respaldo.

Para cambiar `ORIGIN_SECRET`, mira [Rotar `ORIGIN_SECRET`](#rotar-origin_secret).

Si algún día borras la pila, el bucket y la llave KMS se conservan y la instancia no se deja terminar
hasta que le quites la protección.

## Llave y secretos

Con la llave `aws/ssm` de la cuenta, cualquier rol que pueda leer un parámetro también lo descifra, y
en esta cuenta hay roles de EC2 de otros proyectos con `AmazonSSMManagedInstanceCore`, que deja leer
cualquier parámetro. Por eso los secretos de `/diseno/prod/` van cifrados con una llave propia,
`alias/diseno`, que crea `infra.yaml`:

- Su política deja administrarla por IAM (la declaración estándar de la cuenta) y deja descifrar, vía
  SSM, al rol `diseno-ec2`. A cualquier otro rol le niega `kms:Decrypt` aunque su política de IAM se lo
  permita: algunos roles de otros proyectos tienen `kms:Decrypt` sobre todas las llaves. Los usuarios
  de IAM con permiso de KMS, como el del perfil `ivan-admin`, sí descifran. Si algún día administras
  con un rol (SSO, por ejemplo), agrégalo a la excepción de la declaración `NingunOtroRol`.
- La rotación automática está prendida (una vez al año); lo cifrado antes se sigue descifrando solo.
- Si borras la pila, la llave se conserva, porque sin ella los parámetros ya no se leen. Para
  eliminarla, borra primero los parámetros y después programa su borrado con
  `aws kms schedule-key-deletion`.

`bootstrap.sh` revisa los cuatro secretos en cada corrida. Si alguno es String o está cifrado con otra
llave, lo lee, lo borra y lo vuelve a crear con el mismo valor; si no logra crearlo, lo restaura como
estaba. Borra en lugar de sobrescribir porque `put-parameter --overwrite` conserva las versiones
anteriores, que seguirían cifradas con `aws/ssm` y legibles como `/diseno/prod/NOMBRE:1`. Durante ese
cambio el parámetro no existe por un momento: no corras `bootstrap.sh` a la vez que un despliegue.

Un secreto nuevo se crea siempre con la llave:

```sh
aws ssm put-parameter --name /diseno/prod/NOMBRE --type SecureString --key-id alias/diseno --value '...'
```

### Rotar `ORIGIN_SECRET`

```sh
export AWS_PROFILE=ivan-admin
aws ssm delete-parameter --name /diseno/prod/ORIGIN_SECRET
deploy/bootstrap.sh   # genera uno nuevo, se lo da a CloudFront y lo guarda cifrado
deploy/release.sh     # Caddy empieza a exigirlo
```

Entre `bootstrap.sh` y `release.sh` el sitio contesta 403 (CloudFront ya manda el nuevo y Caddy aún
espera el viejo), así que córrelos seguidos. Ningún script imprime el valor y CloudFormation lo guarda
como NoEcho, así que tampoco sale en `describe-stacks`. Sí se ve en la configuración de la
distribución (`aws cloudfront get-distribution-config`), porque CloudFront lo necesita en claro.

## Costo

Aproximado al mes, en USD y sin impuestos:

| Concepto | USD |
|---|---|
| EC2 t4g.small todo el mes | 12.3 |
| IPv4 pública (la IP elástica) | 3.7 |
| Disco gp3 de 20 GB | 1.6 |
| S3, ECR, la alarma y las consultas de DNS | ~0.5 |
| Llave KMS `alias/diseno` | 1 (2 tras la primera rotación anual y 3 desde la segunda) |
| CloudFront | 0 dentro de la capa gratuita (1 TB y 10 millones de solicitudes) |
| WAF | centavos: el ACL ya existe, solo se pagan las solicitudes |
| **Total** | **~19** |

La t4g está en modo de créditos ilimitado: si la CPU pasara mucho tiempo arriba del 20 %, AWS cobra
el excedente. Con el tráfico de un curso no debería pasar.

## Llaves de Google y Resend

Lo normal es ponerlas en el panel, en **Ajustes**: la API key de Resend y el JSON de la cuenta de
servicio de Google se guardan cifrados en la base.

Si prefieres que Resend venga de SSM, crea el parámetro y vuelve a desplegar. Lo que esté en Ajustes
tiene prioridad sobre esta variable.

```sh
aws ssm put-parameter --name /diseno/prod/RESEND_API_KEY --type SecureString --key-id alias/diseno --value 're_...'
AWS_PROFILE=ivan-admin deploy/release.sh    # o un push a main
```

Para trabajar en local, mira el comentario al inicio de [`docker-compose.yml`](../docker-compose.yml).
