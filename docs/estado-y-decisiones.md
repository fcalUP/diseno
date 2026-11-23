# Estado y decisiones para retomar el proyecto

Fecha de este registro: **23 de noviembre de 2026**, zona `America/Mexico_City`.
Este archivo es un punto de continuidad para desarrolladores y otras IA. Describe las decisiones
vigentes y una verificación histórica; no sustituye consultar el estado de producción al retomar.

El funcionamiento del producto se explica en el [README principal](../README.md). La operación,
la infraestructura, los respaldos y los procedimientos de despliegue están en
[deploy/README.md](../deploy/README.md). Los contratos de implementación están en
[spec.md](spec.md) y [routes_contract.md](routes_contract.md).

## Decisiones que deben conservarse

- **Stack solicitado:** React + Vite + TypeScript + ESLint en el frontend, Go en el backend y
  PostgreSQL como base de datos. El backend sirve también el frontend compilado. Se conserva
  Tailwind 3 para mantener las clases del diseño heredado.
- **Diseño:** conservar la apariencia original del portal. Las excepciones solicitadas son el
  acceso multicurso, los controles personalizados y el formulario único de acceso. No aprovechar
  una tarea de mantenimiento para rediseñar el resto de las pantallas.
- **Acceso único:** alumnos y administradores entran por el formulario de `/`, con matrícula o
  usuario y contraseña. El servidor detecta el rol. El administrador abre directamente
  `/admin/alumnos`, sin enlace *Administrator*, login separado, notificación de acceso exitoso,
  tablero rápido ni sección Resumen. `/admin` redirige a Alumnos; sin sesión de administrador,
  al salir o al expirar la sesión se vuelve al formulario único. Los endpoints heredados se
  conservan para compatibilidad; no deben reintroducir las pantallas retiradas.
- **Multicurso:** el alumno entra con matrícula y contraseña, sin elegir materia en el login. Si
  tiene una materia activa, entra directamente; si tiene varias, elige en tarjetas con nombre y
  logo. Puede cambiar desde la barra superior sin iniciar sesión de nuevo. El progreso, las
  monedas y las compras se mantienen separados por materia.
- **Identidad del alumno:** las inscripciones con la misma matrícula representan al mismo alumno.
  Validar la contraseña de una inscripción permite abrir sus otras materias activas, incluso si
  las contraseñas heredadas de Sheets difieren. La sesión conserva la inscripción que autenticó
  y la versión de su contraseña; cambiar de materia no debe reemplazar ese anclaje.
- **Controles personalizados:** selectores propios en portal y administración, con teclado,
  Escape, foco, clic fuera y soporte móvil. Checkbox y radio tienen apariencia propia; los campos
  numéricos no muestran spinners del navegador. Las cargas de archivos usan botones propios y un
  input oculto para abrir el selector de archivos del sistema.
- **Google Sheets:** conservar su formato original como fuente de importación manual desde el
  panel, con vista previa. PostgreSQL es la fuente de datos operativa. No hay sincronización
  continua ni escritura de regreso a la hoja. Las compras importadas son historial y no deben
  descontar monedas otra vez.
- **Administración:** crear y generar alumnos, importar, cargar imágenes y gestionar cursos,
  insignias, niveles, compras y ajustes desde el sistema, sin depender de editar Sheets.
- **AWS:** una sola EC2 `t4g.small` con Caddy, la aplicación y PostgreSQL en Docker Compose. La base
  vive en el volumen de la instancia; **no se cambió a RDS**. La elección responde al alcance
  indicado de unos 100 usuarios y hasta 20 simultáneos; no constituye una prueba de carga.
- **Entrada pública:** `design.nexed.dev` utiliza CloudFront y el WAF compartido ya existente;
  Caddy funciona como proxy HTTPS en el origen. Las imágenes se almacenan en S3. Se mantiene el
  perfil AWS `ivan-admin` para la administración de esta infraestructura.

## Evidencia del cierre de la migración

Al preparar este registro se dejó como referencia el commit
[`0a38d7b`](https://github.com/fcalUP/diseno/commit/0a38d7b),
«Completa migracion con acceso multicurso y selectores personalizados», y la ejecución
[`45120199200` de GitHub Actions](https://github.com/fcalUP/diseno/actions/runs/45120199200),
reportada como exitosa con validación de frontend, backend, configuración y despliegue.
Esta evidencia corresponde a ese cambio; un despliegue posterior puede haber sustituido la versión.

En ese cierre quedó creado el administrador **`fcal`**, gestionado desde el panel. Su contraseña
no pertenece a la documentación, a Git ni a los registros de CI. Los parámetros del arranque
inicial no son un mecanismo para consultar o cambiar su contraseña. Un despliegue normal conserva
las cuentas existentes en la base.

Los controles personalizados se verificaron en escritorio y móvil: navegación por teclado,
selección táctil, opciones vacías o deshabilitadas, y uso dentro de modales y paneles laterales.
La prueba de humo del despliegue comprueba API, SPA, assets, autenticación requerida y encabezados
a través de Caddy. Estos checks no acreditan por sí solos una importación real de la hoja ni la
entrega de correo.

## Validación local del acceso único

El cambio del 23 de noviembre de 2026 unifica el acceso, abre el panel directamente en
Alumnos, agrega el ojo de las contraseñas y carga cada sección administrativa por separado.
La restauración de rol usa `GET /api/session`, con validación completa y la configuración
pública en paralelo. Esta evidencia corresponde al entorno local y no acredita un despliegue.

Se usó PostgreSQL local en la base nueva `diseno_login_qa`, con cuentas y archivos de prueba
propios. Playwright y Chrome comprobaron:

- **17 grupos del panel:** formulario único y guardas, entrada directa sin popup, vistas
  iniciales de Compras e Importar, validación del ID de Sheets, alta de curso con imagen,
  alumno individual y edición, generación con duplicados y filas inválidas, insignia con
  imagen, validación y guardado de niveles, avatar, alta de otro administrador y restricciones
  de la propia cuenta, ajustes, deshacer/rehacer en Actividad, y una compra local con
  reversión/restauración y comprobación de saldos. Se recorrieron las once secciones del panel
  en escritorio; también menú, Escape/foco, selector en modal y navegación en móvil de 390 px,
  restauración de sesión y logout.
- **5 grupos del alumno y acceso:** ojo de contraseña y errores, alumno con una materia,
  restauración y permisos, varias materias con selección pendiente/recarga/cambio y saldos,
  revocación al cambiar la contraseña de origen y acceso/restauración/logout del administrador.
- **2 regresiones del arranque:** un fallo temporal de configuración conserva el aviso de
  nivel y no repite la consulta de sesión del alumno; una sesión admin abre el panel aunque
  falle la petición de configuración pública.

Frontend pasó `lint`, `typecheck` y `build` con Node 24.22.3. Los cinco grupos de acceso y
las dos regresiones del arranque se ejecutaron también sobre ese build final.

No hubo excepciones JavaScript ni respuestas 5xx en la QA del panel. Se revisaron capturas de
escritorio y móvil: no hubo desbordamiento horizontal de página y las tablas conservaron su
scroll interno. Los reportes y scripts son temporales en `/tmp/diseno-login-qa/`
(`admin-report.json`, `student-report.json`, `boot-retry-report.json`); no hay una suite E2E
versionada en el repositorio.
Las capturas del panel enmascaran campos y valores que pudieran mostrar contraseñas.

La comparación local de builds con las mismas dependencias redujo el módulo principal de
`AdminApp` de 234 338 a 38 504 bytes (aproximadamente 84 %).
Alumnos se carga aparte. Es una medida de tamaño del bundle, no de latencia ni de capacidad de
producción.

La [revisión de seguridad y alojamiento](seguridad-y-alojamiento.md) registra el alcance de la
inspección de AWS, los refuerzos de JSON/CSRF/cabeceras y Go 1.26.11, las verificaciones locales
y las acciones pendientes. La vista previa y las importaciones real/de medios comparten un
turno exclusivo; su rechazo 409 y liberación ante error/cancelación se cubren con regresiones
Go sobre Sheets simulado. No se validó una importación real de Google Sheets ni una entrega
de correo: la QA usó un entorno sin esas credenciales externas. Tampoco se aplicaron cambios
de infraestructura como parte de estas comprobaciones.

## Pendientes que no deben darse por terminados

- **Datos reales de Sheets:** el usuario hará la importación desde el panel. Antes de marcarla
  completa, comprobar la cuenta de servicio y el acceso de lectura, revisar la vista previa y el
  reporte de la importación, y contrastar los cursos, alumnos y compras resultantes. La existencia
  del importador o sus pruebas no significa que la hoja del usuario ya esté cargada.
- **Correo:** comprobar en Ajustes la configuración del proveedor y del remitente y verificar una
  entrega real con el usuario. No afirmar que recuperación de contraseña y notificaciones están
  operativas únicamente porque el código existe o porque aparece una clave configurada.
- **Protección de `main`:** la consulta a la rama durante esta documentación devolvió
  `protected: false`. Verificar de nuevo al retomar. Configurarla requiere una persona con permisos
  de administrador del repositorio; tener permiso para hacer push no acredita ese nivel de acceso.
  El procedimiento propuesto está en
  [Protección de main](../deploy/README.md#protección-de-main-pendiente-la-configura-un-administrador-del-repo).
- **Recuperación:** la existencia del script y del timer de respaldo no equivale a una restauración
  probada. Al retomar, comprobar respaldos recientes y registrar una prueba de recuperación en un
  entorno aislado antes de considerar validada esa capacidad.

## Límites y riesgos operativos

**Un push a `main` puede desplegar producción.** El workflow no filtra por carpeta: también un
cambio de documentación ejecuta las verificaciones y puede disparar el despliegue. Los pull
requests ejecutan CI; el trabajo de despliegue requiere push a `main` y éxito en los tres trabajos
previos. No hay que asumir una aprobación manual o protección de rama que no se haya comprobado.

**Los secretos tienen dos lugares y responsabilidades distintas.** Los de infraestructura viven
en SSM Parameter Store, cifrados con KMS; las credenciales de Google y correo capturadas en Ajustes
se guardan cifradas en PostgreSQL. `SESSION_SECRET` se usa para firmar sesiones y derivar la clave
de cifrado de esos ajustes. Cambiarlo invalida sesiones y puede dejar ilegibles los ajustes
existentes: debe planearse la recaptura o migración de credenciales. Restaurar únicamente un
`pg_dump` sin conservar el secreto compatible tampoco recupera esos valores. Consultar
[Llave y secretos](../deploy/README.md#llave-y-secretos) antes de cualquier rotación; no volcar
valores de SSM, cookies, `.env` ni credenciales en tickets o logs públicos.

**La instancia contiene la base y su disco es parte del estado del sistema.** El AMI está fijado
en un parámetro para evitar reemplazos involuntarios. No cambiar el AMI, sustituir la instancia,
eliminar volúmenes ni ejecutar operaciones de limpieza sobre los datos como parte de una
actualización rutinaria. Un reemplazo requiere respaldo, revisión explícita del cambio y un plan
de restauración. Las políticas de retención de otros recursos no convierten el disco de EC2 en
una base administrada. Ver [Cambios de infraestructura](../deploy/README.md#cambios-de-infraestructura).

**Volver a una imagen anterior no deshace SQL.** El script intenta restaurar imagen y configuración
previas si falla la salud de la aplicación o la prueba por Caddy; las migraciones de base que ya
se hayan confirmado permanecen. Tampoco revierte las escrituras de usuarios ocurridas entre
despliegues. Revisar compatibilidad de las migraciones antes de desplegar. Para recuperar datos
se necesita un respaldo adecuado y el procedimiento de restauración, con su correspondiente
interrupción y posible pérdida de cambios posteriores al respaldo. La función «Deshacer» del
panel solo cubre las operaciones que implementa la auditoría; no reemplaza los respaldos.
Consultar [Respaldos](../deploy/README.md#respaldos).

**El WAF es compartido con otros proyectos.** No cambiarlo ni desactivarlo para hacer pasar una
prueba de esta aplicación. Puede bloquear clientes automatizados y solicitar CAPTCHA a redes de
nube, incluidos runners de CI; por eso la prueba de humo del despliegue corre dentro de la
instancia a través de Caddy. El acceso al origen exige el encabezado secreto y la restricción de
red. Evitar publicar configuraciones completas de CloudFront, que pueden contener ese encabezado.

**El límite público del cuerpo es 8 KiB.** Las operaciones masivas deben conservar el envío por
partes; aumentar solo el límite de Go o Caddy no evita el filtro del WAF. Las imágenes se suben
directamente a S3 mediante una URL firmada y luego se guarda su referencia; no enviarlas como
base64 ni como archivo a través de las rutas JSON del API. Mantener las restricciones de tipo y
tamaño, CORS y los encabezados de seguridad de los medios.

## Cómo retomar sin confundir este registro con el estado actual

1. Leer este archivo, el [README principal](../README.md) y el
   [manual de despliegue](../deploy/README.md). Revisar cambios locales y documentación aplicable
   antes de editar; no borrar avances sin commit de otra persona o IA.
2. Consultar Git, las ejecuciones de Actions y la pila para conocer qué versión terminó desplegada.
   Un workflow exitoso del pasado no acredita la salud actual ni un workflow todavía en ejecución.
3. Separar el cambio solicitado de los pendientes de configuración y datos del usuario. Si una
   comprobación exige credenciales, usarlas por los canales previstos sin copiarlas al repositorio.
4. Para cambios de producto, revisar contratos y validar frontend y backend con una base de prueba.
   Para cambios de infraestructura, revisar el cambio propuesto y su efecto sobre instancia y disco
   antes de aplicar nada. Seguir el flujo de despliegue existente.
5. Después de verificar el resultado, actualizar la sección de evidencia con fecha, commit,
   ejecución y alcance comprobado. Actualizar también los contratos o el manual afectados, y
   conservar claramente qué quedó pendiente. No convertir una intención en una afirmación de hecho.

Estas consultas son de lectura y evitan mostrar valores secretos:

```sh
git status --short
git log -5 --oneline
gh run list --repo fcalUP/diseno --workflow ci-cd.yml --branch main --limit 5
gh run view 45120199200 --repo fcalUP/diseno --json status,conclusion,headSha,url

AWS_PROFILE=ivan-admin aws sts get-caller-identity
AWS_PROFILE=ivan-admin aws cloudformation describe-stacks \
  --region us-east-1 --stack-name diseno \
  --query 'Stacks[0].{Estado:StackStatus,Actualizada:LastUpdatedTime,Salidas:Outputs}'
```

Para leer contenedores, logs y el timer, utilizar el acceso de Session Manager descrito en el
manual de despliegue. No pegar su `.env` ni logs con información de visitantes en documentación
pública. Registrar solo el resultado necesario para la continuidad.
