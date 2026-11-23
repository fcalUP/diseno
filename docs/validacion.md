# Validación de cambios

Ejecuta las comprobaciones que correspondan al cambio y registra sus resultados actuales.
Un build correcto no demuestra un flujo de negocio ni confirma un despliegue. Para orientarte,
consulta [AGENTS.md](../AGENTS.md), el [README](../README.md) y el
[contrato HTTP](routes_contract.md). Todos los comandos siguientes parten de la raíz del repo,
salvo donde se indica otra cosa.

## Herramientas y entorno local

Usa Node 24, la versión de Go declarada en [go.mod](../backend/go.mod) y PostgreSQL 18.
El [Compose local](../docker-compose.yml) publica Postgres exclusivamente en `127.0.0.1:5433`;
el puerto 5432 puede pertenecer a otro Postgres de la máquina.

```sh
docker compose up -d db
docker compose exec -T db pg_isready -U diseno -d diseno
```

La suite puede usar la base local `diseno`: los helpers crean esquemas aislados. Si prefieres
una base dedicada, créala una sola vez y usa su URL en los comandos de Go:

```sh
docker compose exec -T db createdb -U diseno diseno_test
```

Si ya existe, omite `createdb`. No borres volúmenes ni bases para conseguir una prueba limpia.
**Nunca apuntes `DATABASE_URL` de las pruebas a producción.**
[freshSchema](../backend/internal/app/app_test.go) necesita crear esquemas `t_<aleatorio>`,
establece su `search_path` y los elimina al finalizar. `isolatedDB` aplica las migraciones y
semillas dentro de cada esquema; `isolatedApp` usa además almacenamiento temporal. Este
aislamiento no justifica dar a la suite acceso a una base de producción.

## Frontend

Los scripts reales están en [package.json](../frontend/package.json):

```sh
(cd frontend && npm ci && npm run lint && npm run typecheck && npm run build)
```

`npm ci` respeta el lockfile; `lint` ejecuta ESLint; `typecheck` ejecuta TypeScript; `build`
vuelve a comprobar tipos y genera `frontend/dist`. No hay `npm test`, `npm run e2e` ni una
suite de navegador versionada. No presentes esos comandos como validación existente.

Para comprobar la interfaz, inicia el API siguiendo el [README](../README.md#desarrollo-local)
y, en otra terminal, ejecuta `(cd frontend && npm run dev)`. Vite sirve el portal y `/admin`
en `http://localhost:5173` y envía `/api` y `/media` al puerto 8080, según
[vite.config.ts](../frontend/vite.config.ts).

Usa un administrador y alumnos locales de prueba. Conserva `SESSION_SECRET` entre arranques,
fuera de Git: también deriva la llave de cifrado de los secretos de Ajustes. Para imágenes
mantén `MEDIA_DIR=../.data/media` y `MEDIA_SEED_DIR=../seed/media`, como en el README.
No copies credenciales, cookies o JSON de Google a capturas o resultados compartidos.

## Backend con PostgreSQL real

Comprueba formato, análisis estático y suite con una URL local explícita:

```sh
(cd backend && test -z "$(gofmt -l .)" && go vet ./... && \
  DATABASE_URL='postgres://diseno:diseno@localhost:5433/diseno_test?sslmode=disable' \
  go test -count=1 -v ./...)
```

Si usas la base local predeterminada, reemplaza únicamente `diseno_test` por `diseno`.
`-count=1` evita reutilizar resultados de la caché; `-v` permite revisar qué se ejecutó.
Sin `DATABASE_URL`, las pruebas que usan `freshSchema` se omiten con
`DATABASE_URL no definida`: un resultado `ok` en esas condiciones no valida PostgreSQL,
migraciones, transacciones, importación ni sesiones contra filas reales.

Durante una corrección puedes ejecutar primero la regresión relevante. Estos patrones
corresponden a nombres de pruebas que sí existen en `backend/internal/app/`:

| Cambio | Selección inicial para `go test -run` |
|---|---|
| Login y materias | `TestStu(AutomaticSingleCourse|MultipleCoursesAndPersistentSelection|CoursesExcludeInactiveAndOtherStudents|CourseSessionRevokesWithAuthenticatingEnrollment|LoginLimitSharedAcrossCourseAndAutomatic)` |
| Sesiones, cookies y límites | `Test(TokenTamper|SessionCookie|RequireActiveRow|SecSessionsFollowPassword|SecStudentLoginBurst|SecAdminLoginBurst|LimiterAllowConcurrent)` |
| Alumnos y generación | `TestAdm(StudentCreate|StudentsGenerate|StudentUpdate|StudentDelete|StudentPassword|StudentsBulk)` y `TestSecIDAndNameRules` |
| Imágenes y migraciones | `Test(AdmUpload|CatAvatars|LocalUpload|LocalStorageTraversal|MigrateTwice|MigrateDefaultAvatars)` |
| Tienda | `TestStu(Purchase|PurchaseConcurrent|Badges)` y `TestAdmPurchasesRevertRestore` |
| Importación y deshacer | `TestImport`, `TestImp` y `TestUndo` |
| Autenticación del panel | `TestAdm(AuthRequired|Login|Admins)` |

Ejemplo desde la raíz, con la misma base dedicada:

```sh
(cd backend && \
  DATABASE_URL='postgres://diseno:diseno@localhost:5433/diseno_test?sslmode=disable' \
  go test -count=1 -v ./internal/app \
  -run 'TestStu(AutomaticSingleCourse|MultipleCoursesAndPersistentSelection|CourseSessionRevokesWithAuthenticatingEnrollment)')
```

Si cambian sesiones, estado compartido, límites, importación, compras o concurrencia, ejecuta
también el detector de carreras. Necesita un entorno con compilador C y soporte de `-race`:

```sh
(cd backend && \
  DATABASE_URL='postgres://diseno:diseno@localhost:5433/diseno_test?sslmode=disable' \
  go test -race -count=1 ./...)
```

Puedes acotar `-race -run` mientras investigas; antes de entregar un cambio transversal corre
la suite completa. Las pruebas de mayor interés están en
[student_courses_test.go](../backend/internal/app/student_courses_test.go),
[security_test.go](../backend/internal/app/security_test.go),
[student_test.go](../backend/internal/app/student_test.go),
[importer_test.go](../backend/internal/app/importer_test.go) y
[undo_test.go](../backend/internal/app/undo_test.go). El detector de carreras complementa las
aserciones: la prueba `TestStuPurchaseConcurrent` verifica además que dos compras de la última
unidad produzcan una sola venta, existencias cero y ninguna sobreventa.

## Configuración de despliegue sin publicar

La referencia ejecutable es [ci-cd.yml](../.github/workflows/ci-cd.yml). Requiere Bash,
ShellCheck, Python 3 y Docker; `actionlint` permite revisar adicionalmente el workflow.
Estos comandos validan archivos y usan valores ficticios; no ejecutan un despliegue:

```sh
for script in deploy/bootstrap.sh deploy/release.sh deploy/instance/deploy.sh deploy/instance/backup.sh; do
  bash -n "$script"
done
shellcheck deploy/bootstrap.sh deploy/release.sh deploy/instance/deploy.sh deploy/instance/backup.sh
python3 -c "import ast; ast.parse(open('deploy/instance/smoke.py').read())"
actionlint .github/workflows/ci-cd.yml
```

Valida la interpolación de [compose.prod.yml](../deploy/compose.prod.yml) sin imprimir la
configuración resuelta ni leer un archivo `.env`:

```sh
POSTGRES_PASSWORD=solo-validacion \
SESSION_SECRET=solo-validacion-012345678901234567890123 \
ADMIN_PASSWORD=solo-validacion \
APP_IMAGE=diseno:validacion \
S3_BUCKET=diseno-validacion \
ORIGIN_DOMAIN=design-origin.example.com \
ORIGIN_SECRET=solo-validacion-012345678901234567890123 \
docker compose --env-file /dev/null -f deploy/compose.prod.yml config --quiet
```

Valida el [Caddyfile](../deploy/Caddyfile) con la misma versión de Caddy usada por CI:

```sh
docker run --rm \
  -e ORIGIN_DOMAIN=design-origin.example.com \
  -e ORIGIN_SECRET=solo-validacion-012345678901234567890123 \
  -v "$PWD/deploy/Caddyfile:/etc/caddy/Caddyfile:ro" \
  caddy:2.11-alpine caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
```

No ejecutes `bootstrap.sh` o `release.sh` para revisar sintaxis o documentación. Sus efectos
y el flujo de publicación se describen en [deploy/README.md](../deploy/README.md).

## Prueba de humo HTTP

[smoke.py](../deploy/instance/smoke.py) usa la biblioteca estándar de Python y se configura
mediante **variables de entorno**, no mediante argumentos `--url` o `--admin`:

```sh
SMOKE_BASE_URL=http://localhost:8080 python3 deploy/instance/smoke.py
```

Por defecto comprueba salud, configuración, rechazo sin sesión y ruta API inexistente,
incluidos encabezados de seguridad y caché. No crea alumnos ni realiza compras.

| Variable | Comportamiento real |
|---|---|
| `SMOKE_BASE_URL` | Origen `http(s)` sin ruta; por defecto `http://localhost:8080`. |
| `SMOKE_REQUIRE_WEB=1` | Revisa HTML de `/`, rutas SPA `/admin` y `/admin/alumnos`, assets Vite y sus encabezados de caché. |
| `SMOKE_ADMIN_PASSWORD` | Habilita login de admin, cookie, consultas del panel, logout y rechazo posterior sin sesión. Usa solo una credencial propia del entorno. |
| `SMOKE_ADMIN_USERNAME` | Usuario para esa comprobación; por defecto `Admin`. |
| `SMOKE_PUBLIC_URL` | Valor de `Origin` para CSRF en peticiones de escritura; por defecto el origen de `SMOKE_BASE_URL`. |
| `SMOKE_CONNECT_HOST` | Host al que conectar; conserva el dominio de la URL para Host/SNI y valida TLS. |
| `SMOKE_ORIGIN_SECRET` | Añade `X-Origin-Verify` para Caddy; no se imprime. |

Para incluir la SPA local, compila primero el frontend e inicia Go con
`WEB_DIR=../frontend/dist`, además de las variables del README. Después:

```sh
SMOKE_BASE_URL=http://localhost:8080 SMOKE_REQUIRE_WEB=1 \
python3 deploy/instance/smoke.py
```

No apuntes esta prueba web a Vite: espera los encabezados de assets que sirve Go. Para probar
admin, aporta `SMOKE_ADMIN_PASSWORD` desde el entorno local sin guardar ni publicar su valor.
La prueba no cambia datos de negocio, aunque el login puede generar actividad de autenticación.

En producción [deploy.sh](../deploy/instance/deploy.sh) la ejecuta dentro de la instancia,
a través de Caddy, con el host de conexión local y el secreto del origen. El WAF compartido
puede retar o bloquear curl y navegadores automáticos externos; un reto no demuestra que
el API esté fallando. No deshabilites el WAF para conseguir que una prueba externa pase.

## QA de los flujos afectados

Reproduce en navegador los casos de la fila pertinente con datos locales. Revisa escritorio
y un ancho móvil de aproximadamente 390 px, consola sin errores, imágenes cargadas y ausencia
de desbordamiento horizontal. Guarda evidencia del resultado y anota las comprobaciones que
no pudiste realizar.

| Área modificada | Casos y resultado esperado |
|---|---|
| Login y materias | Entrar con matrícula y contraseña sin selector previo. Una inscripción entra directo; varias muestran tarjetas con nombre y logo antes del dashboard. Refrescar conserva la selección pendiente o la materia abierta. Cambiar desde navbar carga los contadores y compras de esa inscripción. Denegar materias ajenas o inactivas. Cambiar la clave o desactivar la inscripción que autenticó revoca la sesión incluso al estar en otra materia; cambiar la clave de otra inscripción no la revoca. |
| Pickers personalizados | Abrir con ratón y toque; recorrer con flechas, Home/End y búsqueda por texto; elegir con Enter y cerrar con Escape o clic fuera. Conservar foco; Escape cierra el picker sin cerrar el modal padre. Saltar opciones deshabilitadas, manejar lista vacía y control deshabilitado. Dentro de modal/drawer, comprobar que el menú no se recorta y que funciona al desplazar y en móvil. No introducir `<select>` visibles ni apariencia nativa. |
| Alumnos y generación | Alta individual y edición; errores de matrícula/nombre, campos obligatorios y límites numéricos. Altas admiten Hombre/Mujer; edición puede conservar un valor heredado fijo. Pegar lista con encabezados, H/M, duplicados y filas inválidas; crear únicamente válidas y descargar CSV con claves generadas. Comprobar lote parcial, reintento y deshacer. |
| Avatares e imágenes | Subir formato permitido y guardar; rechazar formato/tamaño inválido; esperar subida antes de guardar/cerrar. Ver imagen tras recargar. Una alta nueva sin importación debe tener avatar predeterminado; comprobar hombres y mujeres en distintos niveles y conservar personalizaciones. |
| Tienda y contexto | Saldo suficiente/insuficiente, existencias agotadas y doble clic. Probar dos alumnos comprando la última unidad: solo una venta y saldo/existencias coherentes. Verificar revertir/rehacer. Cambiar materia limpia carrito y modales; respuestas tardías de tienda/leaderboard no reabren el contexto anterior; no permitir switch durante compra o cambio en curso. |
| Importación | Vista previa sin escrituras; importar sobre una hoja de prueba compartida con la cuenta de servicio. Reimportar no duplica compras ni vuelve a descontar monedas históricas. Probar modos actualizar/solo nuevos, errores de credenciales y progreso. Deshacer el lote restaura el estado anterior; revisar conflictos y auditoría. Las pruebas Go usan un servidor Sheets simulado, no garantizan acceso a una hoja real. |
| Administración | Rutas privadas sin cookie devuelven 401; login correcto/incorrecto, límites, logout y sesión revocada por cambio de clave. Comprobar panel completo y acceso rápido. No permitir eliminar/desactivar al último admin activo. Verificar errores y que no se muestren secretos guardados. |

La revisión anterior usó Playwright y capturas bajo `/tmp/diseno-qa-frontend/` y otros
directorios temporales. **Esos scripts y resultados no están versionados, pueden haber
desaparecido y no constituyen una suite reproducible del repo.** Repite los casos manualmente
o añade una prueba de regresión pertinente al cambio cuando haga falta; no prometas una
automatización de navegador que todavía no existe. Los componentes a revisar son
[CustomSelect](../frontend/src/components/CustomSelect.tsx),
[StudentApp](../frontend/src/student/StudentApp.tsx),
[generación](../frontend/src/admin/pages/GenerateStudents.tsx),
[carga de imágenes](../frontend/src/admin/ui/ImageUpload.tsx) e
[importación](../frontend/src/admin/pages/Import.tsx).

## Evidencia para entregar

Antes de terminar, ejecuta `git diff --check`. Indica comandos ejecutados, base local utilizada,
pruebas que realmente pasaron y casos manuales revisados. Distingue pruebas omitidas,
credenciales externas no disponibles y bloqueos del WAF de un resultado comprobado. Para
una edición exclusivamente documental, comprueba enlaces y diff; no hace falta ejecutar
toda la suite ni publicar la aplicación.
