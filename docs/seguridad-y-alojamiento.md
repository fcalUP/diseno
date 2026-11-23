# Revisión de seguridad y alojamiento

Revisión del **23 de noviembre de 2026**, aproximadamente **16:03–16:15 de Ciudad de México**
(`22:03–22:15 UTC`). Las consultas se hicieron con el perfil `ivan-admin`, en `us-east-1`.

La EC2 inspeccionada tiene carga baja y aloja únicamente los tres contenedores de `diseno`
observados, junto con servicios del sistema. No se encontraron contenedores de otros proyectos.
El acceso público y la ejecución de despliegues están acotados a los recursos del sitio. Esto
reduce el alcance de una falla, pero los contenedores comparten host, red e identidad de EC2:
no representan una separación completa entre aplicación, proxy y base de datos.

Se consultaron configuraciones y métricas y se ejecutaron comandos de inspección por SSM.
No se aplicaron cambios en AWS, reinicios, parches, modificaciones de red, IAM o WAF, ni lecturas
de valores de Parameter Store, cookies, credenciales o contenido de `.env`. Los cambios de
aplicación descritos más abajo corresponden al código local; esta auditoría no acredita su
publicación ni identifica la versión de Go del binario que ya corre en producción.

Se aplicaron las guías locales `performing-security-headers-audit`,
`testing-api-authentication-weaknesses`, `implementing-api-rate-limiting-and-throttling` y
`testing-api-security-with-owasp-top-10` de la colección de ciberseguridad del usuario, junto
con las guías de EC2, IAM y CloudFormation para revisar el alojamiento.

## Recursos y acceso observados

| Recurso | Evidencia actual |
|---|---|
| Pila | `diseno`, región `us-east-1`. |
| EC2 | `i-0f7f5f1381ebf8b63`, `t4g.small`, `running`, perfil `diseno-ec2`, protección contra terminación activada. |
| Red | VPC por defecto `vpc-07a9d9ca49e59006d`; no es una VPC exclusiva del proyecto. SG `sg-0da38cec8343e2ca7`. |
| Entrada al host | TCP 80 público para ACME; TCP 443 limitado a la lista administrada de orígenes CloudFront `pl-3b927c52`. Sin entrada autorizada a 22, 8080 ni 5432. |
| Escucha local | Caddy publica 80/443. El proceso `sshd` sí escucha en 22, aunque el SG impide su acceso de red. App y PostgreSQL no publican puertos al host. |
| Salida | El SG permite todo el tráfico IPv4 saliente. No se cambió esta configuración. |
| Metadatos | IMDSv2 obligatorio, hop limit 2, etiquetas de metadatos desactivadas. El límite 2 permite al SDK obtener credenciales desde la red de contenedores. |
| Disco | `vol-02b87983b324819fc`, gp3 cifrado, 20 GiB, 3000 IOPS y 125 MiB/s; `DeleteOnTermination=true`. |
| CloudFront | `E19NYJLKCDQGPU`, habilitado y `Deployed`; redirección a HTTPS y política mínima `TLSv1.2_2021`. WAF compartido asociado, sin modificaciones. |
| Caché | Comportamiento predeterminado con `Managed-CachingDisabled`; `/assets/*` y `/media/*` con `Managed-CachingOptimized`. El API queda fuera de esos dos patrones. |
| Fetch Metadata | La política actual de solicitudes al origen no reenvía `Sec-Fetch-Site`. El cambio local del template debe publicarse por separado antes de contar con esa señal en el backend de producción. |
| S3 | `diseno-media-113380982662`, cuatro bloqueos de acceso público activos y versionado habilitado. |
| Archivos operativos | `/opt/diseno` modo 700 y `/opt/diseno/.env` modo 600. Solo se consultaron permisos. |

La configuración de Caddy en el repositorio exige el encabezado del origen y utiliza HTTPS
hacia CloudFront; la revisión no imprimió su valor. La entrada por una IP de CloudFront y el
encabezado cumplen funciones complementarias. No se probó eludir estas protecciones.

## Capacidad y aislamiento del host

La inspección de las 22:03 UTC mostró 1840 MiB de RAM total, 348 MiB usada y 1298 MiB
disponible; swap de 1023 MiB sin uso. El disco raíz ocupaba 3.7 GiB de 20 GiB, aproximadamente
19 %. App y PostgreSQL estaban `healthy`; Caddy estaba en ejecución.

| Contenedor | RAM observada | Límite de RAM | CPU/PIDs | Usuario configurado |
|---|---:|---:|---|---|
| `diseno-app-1` | 7.2 MiB | Sin límite | Sin cuota de CPU ni límite de PIDs explícitos | UID 65532 |
| `diseno-caddy-1` | 12.6 MiB | Sin límite | Sin cuota de CPU ni límite de PIDs explícitos | Valor vacío en la configuración de imagen |
| `diseno-db-1` | 56.4 MiB | 512 MiB | Sin cuota de CPU ni límite de PIDs explícitos | Valor vacío en la configuración de imagen |

Los tres utilizan `diseno_default`, no son privilegiados y no montan el socket Docker. Sus
sistemas de archivos raíz permiten escritura y no tienen `SecurityOpt` ni `CapDrop` explícitos;
siguen aplicando los valores predeterminados de Docker. Un usuario vacío en la configuración
no demuestra el UID de cada proceso después del entrypoint; no se inspeccionó ese detalle.

Sin límites explícitos, un contenedor puede competir por los recursos disponibles del host.
Por ello una fuga de memoria o exceso de procesos de la app puede afectar a su propio proxy
y base de datos. No se observaron otros servicios de aplicación en esta instancia, pero esto
no equivale a haber auditado todos los recursos de la cuenta.
[Referencia de Docker sobre límites de recursos](https://docs.docker.com/engine/containers/resource_constraints/).

CloudWatch devolvió 288 muestras completas de cinco minutos entre el 22 de noviembre a las
22:10 UTC y el 23 a las 22:10 UTC:

| Métrica | Resultado de las 24 horas |
|---|---:|
| CPU media de las muestras `Average` | 1.39 % |
| Mayor valor de `CPUUtilization Maximum` | 7.32 % |
| `CPUCreditBalance` mínimo/máximo | 576 / 576 créditos |
| `CPUSurplusCreditBalance` máximo | 0 |
| `CPUSurplusCreditsCharged` suma | 0 |

La instancia está en modo `unlimited`. No se detectó consumo facturado de créditos excedentes
en esa ventana. Estos datos describen el uso observado; no son una prueba de carga ni permiten
garantizar capacidad ante importaciones o ráfagas de bcrypt.
[Definición de las métricas de créditos](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/burstable-performance-instances-monitoring-cpu-credits.html).

## Permisos y separación entre proyectos

Se leyeron las políticas de confianza, políticas inline y políticas administradas asociadas a
`diseno-ec2` y `diseno-github-deploy`; no se modificaron.

- **EC2:** confía en `ec2.amazonaws.com`. Lee imágenes únicamente del repositorio ECR `diseno`;
  en el bucket propio puede leer `media/*` y `deploy/*`, escribir `media/u/*` y `backups/*`, y
  listar el bucket. No se encontró permiso para eliminar objetos ni operar EC2/IAM/CloudFront/WAF.
- **Parameter Store:** la política inline permite leer `/diseno/prod` y sus descendientes y
  contiene una denegación explícita de lectura e historial fuera de ese prefijo. Esto acota
  también el permiso amplio `GetParameter/GetParameters` de `AmazonSSMManagedInstanceCore`.
- **KMS:** `alias/diseno` está activa, con rotación automática cada 365 días. Su política permite
  descifrar al rol `diseno-ec2` mediante SSM y deniega `Decrypt/ReEncryptFrom` a otros roles de
  la cuenta. La administración por identidades IAM autorizadas sigue siendo posible.
- **Mejora pendiente de KMS:** la política de identidad de EC2 permite `kms:Decrypt` sobre
  `key/*`, condicionado a `kms:ViaService=ssm.us-east-1.amazonaws.com`. La denegación de
  Parameter Store reduce su alcance práctico; conviene sustituir ese comodín por el ARN exacto
  de la llave propia, conservando y comprobando el despliegue antes de aplicarlo.
- **GitHub:** la confianza OIDC exige audiencia `sts.amazonaws.com` y el repositorio `fcalUP/diseno`
  en `refs/heads/main`, también con la variante de identificadores estables. Permite publicar
  solo en ECR `diseno`, escribir los prefijos `deploy/*` y `media/legacy/*` del bucket propio,
  consultar la pila y ejecutar `AWS-RunShellScript` únicamente en la instancia indicada.
  No se encontró permiso para modificar otros hosts, WAF, CloudFront, IAM o leer secretos SSM.

Hay una excepción relevante al aislamiento de lecturas: el rol de GitHub tiene
`ssm:GetCommandInvocation` con `Resource: "*"`. AWS no admite un ARN de recurso para restringir
esa acción, por lo que el alcance de `SendCommand` a esta instancia no restringe también
la lectura de resultados. Con identificadores conocidos, el rol podría consultar resultados
de comandos de otros proyectos de la misma cuenta, sujeto a las demás políticas aplicables.
Es una capacidad de lectura, no permiso para ejecutar comandos en esos hosts. No se intentó
leer ninguna invocación ajena. Las salidas de Run Command deben conservarse libres de secretos;
si se requiere separación estricta de esos registros, hay que revisar el diseño de entrega.
[Acciones y recursos admitidos por Systems Manager](https://docs.aws.amazon.com/service-authorization/latest/reference/list_ssm.html).

IMDSv2 no asigna un rol distinto por contenedor. Una aplicación que alcance el servicio de
metadatos puede utilizar la identidad de esta EC2; por eso importan el alcance del rol y la
separación de cargas. La revisión no solicitó ni mostró las credenciales de metadatos.
[Credenciales de roles de EC2](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/instance-metadata-security-credentials.html),
[condición `kms:ViaService`](https://docs.aws.amazon.com/kms/latest/developerguide/conditions-kms.html#conditions-kms-via-service).

El rol de despliegue tiene capacidad de ejecutar comandos como parte de la entrega: un cambio
autorizado en `main` puede cambiar esta aplicación y sus datos. La protección de rama y las
identidades humanas con privilegios de cuenta deben revisarse por separado. No se efectuó una
auditoría de todas las políticas de recursos, usuarios, SCP, registros CloudTrail o permisos
de los demás proyectos.

## Respaldo y recuperación

El timer `diseno-backup.timer` estaba activo. El servicio terminó el 23 de noviembre a las
09:00:52 UTC con `Result=success` y código 0. S3 confirma:

| Objeto | Fecha UTC | Tamaño |
|---|---|---:|
| `backups/db/2026-11-23.dump` | 23/nov 09:00:53 | 34 728 bytes |
| `backups/db/2026-11-22.dump` | 22/nov 09:00:53 | 34 311 bytes |

El objeto más reciente tiene cifrado SSE-S3 `AES256` y versión asignada. Se comprobaron
metadatos y existencia; no se descargó ni restauró el contenido. El éxito del job y un objeto
no vacío no demuestran recuperación. Sigue pendiente una restauración aislada y su validación.

PostgreSQL reside en el único disco de esta instancia. La protección contra terminación está
activada, pero el volumen conserva `DeleteOnTermination=true`: si se quita esa protección y se
termina o reemplaza la instancia, el disco puede eliminarse. Debe mantenerse el procedimiento
de respaldo y restauración de [deploy/README.md](../deploy/README.md#respaldos) antes de cualquier
reemplazo, cambio de AMI o intervención sobre el volumen.

## Refuerzos de la aplicación y validación local

El trabajo local de este cambio conserva las autorizaciones de servidor, auditoría, cookies
separadas y revocación de sesiones, y agrega o ajusta:

- Acceso único con detección de rol por credenciales comprobadas en el servidor; límites de
  autenticación compartidos con el endpoint compatible para impedir saltárselos alternando rutas.
- JSON con un solo valor hasta EOF: rechazo de valores concatenados, basura y cuerpos que
  exceden el límite mediante espacios finales. Se mantiene el cuerpo vacío compatible.
- Rechazo de mutaciones API marcadas `Sec-Fetch-Site: cross-site`, manteniendo las comprobaciones
  de `Origin` y `Content-Type` para clientes sin Fetch Metadata.
- CSP `object-src 'none'` y `Permissions-Policy` para deshabilitar cámara, micrófono y
  geolocalización. Las excepciones necesarias para cargas S3 y el SDK del WAF se conservan.
- Go mínimo y builder Docker fijados en **1.26.11**. CI lee esa versión desde `go.mod`.
- Vistas previas, importaciones reales y de medios comparten un solo turno por proceso;
  las peticiones concurrentes reciben 409 antes de cargar la hoja y sus instantáneas en
  memoria. El turno se libera también ante errores, cancelaciones y panic.

La defensa adicional de Fetch Metadata se comprobó localmente. En la distribución actual,
la lista de encabezados reenviados al origen **no incluye `Sec-Fetch-Site`**. Se preparó su
incorporación en la política específica de `diseno` dentro del template local, sin aplicar
la pila. Hace falta publicar y comprobar esa actualización de infraestructura, además del
backend, para que esta defensa tenga efecto a través de CloudFront. Las comprobaciones de
`Origin` y JSON no dependen de esa incorporación.

Se conserva un riesgo conocido de la política de alumnos: el valor predeterminado es
`^[A-Z0-9]{4}$`, cuatro caracteres de mayúsculas o dígitos. La regla es configurable y los
ajustes existentes no se sobrescribieron. Los límites de intentos y bcrypt no convierten
esa longitud en una contraseña fuerte. Conviene acordar una regla de mayor longitud desde
el panel y un restablecimiento gradual de las cuentas existentes; modificar la regla solo
no cambia las claves ya guardadas.

El escaneo con Go 1.26.4 señaló 25 avisos de biblioteca estándar, 21 con rutas de llamadas
alcanzables según el análisis; eso no prueba que todos fueran explotables en este despliegue.
Go 1.26.11 incorpora los parches pertinentes, entre ellos
[GO-2026-6859](https://pkg.go.dev/vuln/GO-2026-6859).
La versión se contrastó con el [historial oficial de Go](https://go.dev/doc/devel/release).

| Comprobación local del 23/nov | Resultado |
|---|---|
| Pruebas HTTP focales de JSON, CSRF y cabeceras | Pasan. |
| `go vet ./...`, `go test -count=1 ./...` con Go 1.26.11 | Pasan sobre PostgreSQL local `diseno_test`. |
| `go test -race -count=1 ./...` con Go 1.26.11 | Pasa sobre la misma base local; no se usó producción. |
| `go run golang.org/x/vuln/cmd/govulncheck@v1.10.0 ./...` con Go 1.26.11 | `No vulnerabilities found.` |
| `npm audit --json` | 0 avisos reportados, 257 dependencias en el inventario. |
| Frontend con Node 24.22.3: lint, typecheck y build | Pasan. |
| QA en navegador local, escritorio y móvil | Pasan 17 grupos del panel, cinco de acceso/alumno y dos regresiones de fallos de configuración al arrancar. Incluye altas y edición, imágenes, niveles, ajustes, auditoría y reversión de compras. |
| `aws cloudformation validate-template` sobre la plantilla local | Pasa; no crea ni actualiza recursos. |
| `docker compose --env-file /dev/null -f deploy/compose.prod.yml config --quiet` con valores ficticios | Pasa. |

La comprobación de CloudFormation valida la plantilla, no demuestra un despliegue ni evalúa
el efecto de un reemplazo. No se ejecutaron `cfn-lint`, `cfn-guard` (no disponibles en el
entorno) ni un change set. El manifiesto del builder en ECR público no pudo consultarse por
un error de autorización caducada del registro; la compilación local de Go sí usó 1.26.11.

La evidencia temporal de esa QA está en `/tmp/diseno-login-qa/`, en `admin-report.json`,
`student-report.json`, `boot-retry-report.json` y capturas del mismo directorio.
Es una comprobación local del cambio actual; no es una suite E2E
versionada ni una prueba contra el sitio público.

Los escáneres cubren avisos conocidos y los caminos analizados, no garantizan ausencia de
fallas. No se validó el nivel de parcheo del sistema operativo ni se hizo una prueba de
penetración externa, un ensayo de carga, una recuperación o un cambio del WAF compartido.
Una consulta adicional por SSM devolvió `Failed` porque dos nombres consultados con `rpm -q`
no estaban instalados; sus lecturas previas de Docker y systemd sí produjeron evidencia.

## Acciones propuestas; no aplicadas en AWS

| Prioridad | Acción y propósito |
|---|---|
| Alta | Publicar y comprobar el binario con Go parcheado mediante el flujo normal de entrega; el build local no actualiza la EC2. |
| Alta | Revisar y publicar la incorporación de `Sec-Fetch-Site` en la política de solicitudes al origen de esta distribución, y comprobar que llega al backend. No modificar el WAF compartido. |
| Alta | Probar una restauración de S3 en una base aislada y comprobar datos y acceso antes de cualquier sustitución de host/disco. |
| Alta | Definir una política de contraseñas de alumno más larga desde Ajustes y planear el restablecimiento gradual, conservando las personalizaciones actuales. |
| Media | Medir picos de login/importación y definir límites de RAM, CPU y PIDs por contenedor, reservando recursos para PostgreSQL y el host; probarlos antes de aplicarlos. |
| Media | Acotar `kms:Decrypt` al ARN de la llave de `diseno`, con revisión de políticas y prueba del despliegue. |
| Media | Revisar el alcance transversal de lectura de resultados SSM por `GetCommandInvocation`; conservar salidas sin secretos y diseñar una separación mayor si se necesita aislamiento estricto de registros. |
| Media | Revisar parches de Amazon Linux, Docker y SSM en una ventana de mantenimiento, sin cambiar automáticamente el AMI o reemplazar la instancia. |
| Media | Revisar si `sshd` debe seguir activo: el SG ya bloquea 22 y la operación usa SSM. Cualquier cambio debe conservar el acceso operativo. |
| Media | Ensayar `no-new-privileges`, capacidades mínimas y raíz de solo lectura para app/proxy, comprobando las rutas que requieren escritura. |
| Media | Confirmar protección de `main`, revisión de cambios y auditoría de operaciones administrativas, conforme al manual de despliegue. |

Estas propuestas se limitan al proyecto. No requieren desactivar ni modificar el WAF compartido,
alterar instancias de otros sistemas o ampliar permisos a sus datos.

## Comandos de lectura reproducibles

Usar el perfil y región indicados en cada consulta. Las proyecciones de CloudFront excluyen
los encabezados del origen. No sustituir estas consultas por un volcado completo de
configuración, variables de contenedor o parámetros descifrados.

```sh
AWS_PROFILE=ivan-admin aws ec2 describe-instances --region us-east-1 \
  --instance-ids i-0f7f5f1381ebf8b63 \
  --query 'Reservations[].Instances[].{Id:InstanceId,State:State.Name,Type:InstanceType,Metadata:MetadataOptions,Volumes:BlockDeviceMappings}'
AWS_PROFILE=ivan-admin aws ec2 describe-security-groups --region us-east-1 \
  --group-ids sg-0da38cec8343e2ca7 --query 'SecurityGroups[].{Ingress:IpPermissions,Egress:IpPermissionsEgress}'
AWS_PROFILE=ivan-admin aws cloudfront get-distribution --id E19NYJLKCDQGPU \
  --query '{Status:Distribution.Status,TLS:Distribution.DistributionConfig.ViewerCertificate.MinimumProtocolVersion,DefaultCache:Distribution.DistributionConfig.DefaultCacheBehavior.CachePolicyId}'
AWS_PROFILE=ivan-admin aws iam get-role-policy --role-name diseno-ec2 --policy-name diseno-instancia
AWS_PROFILE=ivan-admin aws iam get-role-policy --role-name diseno-github-deploy --policy-name diseno-despliegue
AWS_PROFILE=ivan-admin aws kms get-key-policy --region us-east-1 \
  --key-id 7f745deb-4f9b-487f-b517-5f787541e2b6 --policy-name default
AWS_PROFILE=ivan-admin aws s3api head-object --bucket diseno-media-113380982662 \
  --key backups/db/2026-11-23.dump \
  --query '{Bytes:ContentLength,Modified:LastModified,Encryption:ServerSideEncryption}'
AWS_PROFILE=ivan-admin aws cloudwatch get-metric-statistics --region us-east-1 \
  --namespace AWS/EC2 --metric-name CPUUtilization \
  --dimensions Name=InstanceId,Value=i-0f7f5f1381ebf8b63 \
  --start-time 2026-11-22T22:10:00Z --end-time 2026-11-23T22:10:00Z \
  --period 300 --statistics Average Maximum
AWS_PROFILE=ivan-admin aws ssm get-command-invocation --region us-east-1 \
  --command-id 2e1750c2-76bf-4897-a40f-4468bb49682e --instance-id i-0f7f5f1381ebf8b63 \
  --query '{Status:Status,Start:ExecutionStartDateTime,End:ExecutionEndDateTime}'
```

La inspección del host usó `uptime`, `free -m`, `df -h /`, `docker ps`, `docker stats --no-stream`,
`docker inspect --format` limitado a usuario, límites, privilegios y montajes, `ss -lnt`,
estado del timer/servicio de respaldo y `stat` sobre permisos. No se ejecutó `docker inspect`
completo ni se imprimieron `Config.Env`, archivos de secretos o registros de visitantes.
Referencias SSM: `2e1750c2-76bf-4897-a40f-4468bb49682e` y
`58539636-92af-4271-8ad8-0b2391fe1e08`.
