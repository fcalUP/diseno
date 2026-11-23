# Guía para desarrollar con una IA

## Antes de modificar

1. Lee [README.md](README.md), [arquitectura](docs/arquitectura.md) y
   [decisiones y estado conocido](docs/estado-y-decisiones.md).
2. Ejecuta `git status --short --branch`. Conserva el trabajo que ya exista; no hagas reset,
   checkout de archivos o limpieza de datos para conseguir una base aparentemente limpia.
3. Identifica el flujo que cambia y consulta su código, sus pruebas y el
   [contrato HTTP](docs/routes_contract.md) cuando toque el API. Usa
   [validación](docs/validacion.md) para escoger las comprobaciones necesarias.
4. Las instrucciones actuales del usuario mandan sobre estas memorias. Si el código y un
   documento difieren, verifica el comportamiento, corrige el documento y explica el cambio;
   no restaures automáticamente una implementación antigua para cumplir una descripción vieja.

## Qué sistema estás modificando

React + Vite + TypeScript + ESLint en `frontend/`; Go con `net/http` y `pgx/v5` en `backend/`;
PostgreSQL como fuente de datos. `legacy/` conserva el sistema anterior como referencia visual
y del formato de Sheets; no es el servidor de producción.

El alumno entra con matrícula y contraseña. Una materia abre directamente; varias muestran
tarjetas con nombre y logo. La barra permite cambiar de materia. El panel completo vive en
`/admin`; el acceso rápido de administrador desde el portal también sigue funcionando.

## Reglas que deben conservarse

- Conserva el diseño, colores y textos del portal salvo el cambio que se pida. Tailwind se
  mantiene en 3.4 por compatibilidad visual; `cssMinify: false` también es intencional.
- Usa [CustomSelect](frontend/src/components/CustomSelect.tsx) y los componentes del panel.
  No reintroduzcas `<select>` visibles ni pickers con apariencia nativa. Mantén teclado,
  Escape, foco, opciones deshabilitadas y uso táctil. Los archivos se eligen mediante los
  botones y zonas de carga existentes, con el input nativo oculto.
- La matrícula (`students.code`) identifica al alumno entre materias; `students.id` identifica
  una inscripción. No mezcles estos IDs ni monedas, compras o progreso entre inscripciones.
  Conserva la sesión anclada a la inscripción que autenticó la contraseña, su revocación y
  el estado de selección pendiente. Consulta el contrato antes de tocar login o cambio de materia.
- El servidor decide permisos, precios, existencias y saldos. Las operaciones de compra y sus
  reversiones deben seguir siendo transaccionales. No confíes en datos del cliente para autorizar.
- Mantén los contratos JSON: nombres camelCase, listas vacías como `[]` cuando el cliente
  espera arrays y los dos estilos de error existentes. El portal recibe `{error}`; el cliente
  del panel lanza `ApiError`. Actualiza Go, tipos, cliente y documentación juntos.
- Mantén autenticación, validación, CSRF, límites por cuenta/red, cupos de bcrypt y cookies
  seguras. Los helpers que solo leen la cookie no reemplazan `requireStudent` o `requireAdmin`.
- Los cambios de administración se registran mediante la auditoría y el mecanismo de
  deshacer/rehacer aplicable. Los cambios exclusivos de secretos no se pueden deshacer.
  Respeta conflictos 409 y las restricciones para no eliminar al último administrador activo.
- El WAF compartido limita los cuerpos JSON de producción a 8 KB. Conserva los límites por
  bytes y los lotes del cliente. Las imágenes van mediante PUT firmado a S3, no en JSON/base64
  ni como archivos a través del API de producción.
- Sheets es una fuente de importación manual; PostgreSQL es la fuente operativa. Conserva el
  formato original, la vista previa, la deduplicación y las compras históricas sin volver a
  descontar saldo. No introduzcas sincronización o escritura a la hoja sin que se solicite.
- Cursos, insignias, niveles, imágenes y ajustes se gestionan desde el panel. No reemplaces
  configuración de la base por constantes ni sobrescribas personalizaciones en un despliegue.

## Dónde hacer cada cambio

| Cambio | Archivos que revisar juntos |
|---|---|
| Petición, respuesta o ruta | `backend/internal/app/app.go`, handler correspondiente, `frontend/src/lib/types.ts`, `frontend/src/lib/api.ts`, `docs/routes_contract.md` |
| Login, sesión o materias | `auth.go`, `student.go`, `student_courses_test.go`, `security_test.go`, `frontend/src/student/StudentApp.tsx`, `CourseSelector.tsx`, `CourseSwitcher.tsx` |
| Panel y formularios | `frontend/src/admin/pages/`, `frontend/src/admin/ui/`, `frontend/src/components/CustomSelect.tsx`, handler y validaciones Go correspondientes |
| Ajuste configurable | `settings.go`, validación/vista en `admin_misc.go`, tipos y página `Settings.tsx`; invalida el caché después del commit de la transacción |
| Datos persistentes | Nueva migración numerada en `backend/internal/app/migrations/`; revisar auditoría, importador y deshacer si cambia una entidad |
| Imágenes o importación | `storage.go`, `sheets.go`, `importer.go`, `ImageUpload.tsx`, `Import.tsx` y configuración S3/CSP/CORS si corresponde |
| Infraestructura o entrega | `deploy/README.md`, `deploy/infra.yaml`, scripts de `deploy/`, `Dockerfile`, `.github/workflows/ci-cd.yml` |

Las rutas abreviadas de Go están bajo `backend/internal/app/`. Las marcas BF/B1/B2/FF que
quedan en algunos comentarios son historia de la migración, no propietarios vigentes ni
una prohibición para editar archivos relacionados.

## Base de datos y secretos

- Agrega una migración nueva; no edites las migraciones ya aplicadas. El arranque ejecuta las
  pendientes y no hay migraciones de bajada automáticas. Planifica cambios compatibles con
  la versión anterior cuando se dependa del rollback de la aplicación.
- Usa una base local dedicada para pruebas. `freshSchema` crea y elimina esquemas; nunca
  apuntes la suite a producción. Sin `DATABASE_URL`, muchas pruebas se omiten.
- No escribas contraseñas reales, JSON de Google, cookies, tokens, `.env` o valores de SSM en
  el código, Markdown, capturas o logs. No necesitas conocer la contraseña de `fcal` para
  desarrollar: crea un administrador y datos propios en tu entorno local.
- `SESSION_SECRET` firma sesiones y deriva la llave que cifra los secretos guardados en
  PostgreSQL. No la regeneres en cada arranque ni la rotes como si solo cerrara sesiones.
  Revisa la estrategia de recifrado antes de cambiarla en una instalación existente.

## Validar y entregar

- Sigue [docs/validacion.md](docs/validacion.md): frontend, Go con PostgreSQL real y QA de los
  flujos afectados. Usa `-race` cuando cambien sesiones, concurrencia, importación o compras.
- Las pruebas deben demostrar el comportamiento o la regresión que importa. No añadas
  dependencias o pruebas que solo repitan la implementación para una edición documental.
- No existe una suite E2E de navegador versionada ni un comando `npm test`. Los resultados
  históricos no sustituyen ejecutar las comprobaciones pertinentes al cambio actual.
- Un push a `main` dispara CI y despliega si pasa. Publicar código e infraestructura son
  acciones reales: distingue la validación local de la publicación solicitada. No ejecutes
  `bootstrap.sh` o `release.sh` únicamente para comprobar un cambio de documentación.
- Conserva la única EC2 y su disco de PostgreSQL. No reemplaces la instancia, cambies el AMI,
  borres volúmenes ni modifiques el WAF compartido como arreglo rutinario. Revisa respaldos y
  el plan de cambio en [deploy/README.md](deploy/README.md).
- Antes de terminar, revisa `git diff --check` y actualiza las memorias que el cambio deje
  obsoletas. Informa qué cambió, qué se verificó y qué quedó sin comprobar; no declares
  desplegada una versión solo porque compiló localmente.
