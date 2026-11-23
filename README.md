# Gamificación de los cursos de diseño

Portal para los alumnos de la UP. Cada alumno entra con su matrícula y ve su avatar, su nivel, sus
monedas, la barra de experiencia, sus actividades y los comentarios del profesor. Puede canjear
monedas por insignias en la tienda y ver el leaderboard de su curso. El profesor administra todo
desde un panel.

El alumno entra con matrícula y contraseña, sin elegir materia. Si tiene varias inscripciones,
elige una desde tarjetas con nombre y logo; después puede cambiar de materia desde la barra
superior. Las monedas, actividades, compras y progreso corresponden a la materia abierta.
Los selectores del portal y del panel son personalizados, con navegación por teclado y móvil.

Antes los datos vivían en un Google Sheets y el servidor leía y escribía directo en la hoja. Ahora
hay base de datos propia (Postgres). La hoja sigue sirviendo como fuente: se importa desde el panel
cuando haga falta, con el mismo formato de columnas de siempre.

Sitio: https://design.nexed.dev · Panel: https://design.nexed.dev/admin

## Qué hay en el repo

| Carpeta | Qué es |
|---|---|
| `frontend/` | React 19 + Vite + TypeScript + ESLint. Tailwind 3 a propósito: el diseño original usa utilidades que la versión 4 quitó, y así se ve idéntico. |
| `backend/` | API en Go con Postgres 18. Sirve también el front ya compilado. |
| `deploy/` | Infraestructura en AWS, Caddy, Docker Compose y los scripts de despliegue. Detalle en [deploy/README.md](deploy/README.md). |
| `seed/media/legacy/` | Las imágenes que traía el proyecto (avatares, insignias, fondos, logos, el GIF de nivel). Se suben a S3 en cada despliegue. |
| `.github/workflows/` | Pruebas en cada push y despliegue automático desde `main`. |

## Lo que se puede hacer desde el panel

Nada de esto requiere tocar código:

- **Alumnos**: crear uno por uno o generar muchos pegando una lista (las contraseñas se generan
  solas y se pueden descargar en CSV), editar cualquier dato, sumar o restar tareas, monedas,
  asistencias, juegos y EXP en lote, activar o desactivar, mover de curso, restablecer contraseñas.
- **Avatares**: una imagen por nivel para cada alumno, más los avatares predeterminados listos
  desde el primer arranque y editables desde el panel.
- **Cursos**: cuáles aparecen en el login, en qué orden, su imagen lateral y su logo, y de qué
  pestaña del Sheets se importan.
- **Insignias**: nombre, costo, existencias, imagen y en qué cursos se ven.
- **Compras**: historial con filtros; cualquier compra se puede deshacer (devuelve monedas y
  existencias) o rehacer.
- **Niveles**: cuánta EXP pide cada nivel.
- **Ajustes**: abrir o cerrar el registro, la recuperación de contraseña, la tienda y el
  leaderboard; la animación de subir de nivel; el remitente y las copias de los correos; la regla de
  las contraseñas de alumnos y sus textos.
- **Actividad**: cada cambio queda registrado con el antes y el después, y se puede deshacer o
  rehacer, uno por uno o en lote (por ejemplo, una importación completa).
- **Administradores**: alta, baja, contraseñas.

El enlace *Administrator* del login sigue abriendo el panel rápido de siempre, con un acceso al
panel completo.

## Importar desde Google Sheets

1. En Google Cloud, una cuenta de servicio con su llave en JSON (la misma que usaba el servidor
   anterior sirve).
2. Comparte la hoja con el correo de esa cuenta, solo lectura.
3. En el panel, *Ajustes → Google Sheets*, sube el JSON. También se puede subir una sola vez desde
   *Importar* sin guardarlo.
4. *Importar → Vista previa* muestra qué se va a crear o actualizar. Luego *Importar*.

Se leen las mismas pestañas que antes: una por curso (columnas A a N), `Badges` y `Purchases`. Las
contraseñas, que en la hoja estaban en texto plano, aquí se guardan con bcrypt. Las compras importadas solo
quedan como historial, no vuelven a descontar monedas. La importación completa se puede deshacer
desde *Actividad*.

## Desarrollo local

Hace falta Go 1.26, Node 24 y Docker (o un Postgres propio).

```sh
docker compose up -d                      # Postgres 18 en 127.0.0.1:5433

cd backend
DATABASE_URL='postgres://diseno:diseno@localhost:5433/diseno?sslmode=disable' \
SESSION_SECRET="$(openssl rand -hex 32)" \
ADMIN_PASSWORD='una-contraseña-larga' \
MEDIA_SEED_DIR=../seed/media \
go run ./cmd/server                       # API en :8080

cd frontend && npm ci && npm run dev      # http://localhost:5173 y /admin
```

El primer arranque crea las tablas, los cursos y niveles iniciales y el usuario `Admin` con
`ADMIN_PASSWORD`. Las imágenes se guardan en disco (`MEDIA_DIR`) en vez de S3.

Antes de subir cambios:

```sh
cd frontend && npm run lint && npm run build
cd backend && gofmt -l . && go vet ./... && go test ./...   # con DATABASE_URL apunta a una base de prueba
```

## Producción

`design.nexed.dev` está en CloudFront con el WAF compartido de la cuenta. Detrás hay una EC2
t4g.small con Caddy, la app y Postgres en Docker Compose; las imágenes van a S3. Cada push a `main`
corre las pruebas y, si pasan, construye la imagen y la despliega por SSM (GitHub entra a AWS con
OIDC, sin llaves guardadas). Respaldos diarios de la base en S3. Todo el detalle, incluido cómo
entrar a la instancia y restaurar un respaldo, está en [deploy/README.md](deploy/README.md).
