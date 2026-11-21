-- Esquema inicial. Cada fila de students equivale a una fila de la hoja de su curso
-- en el Google Sheets original; quien lleva dos cursos tiene dos filas.

create table courses (
  id                    bigserial primary key,
  name                  text not null unique,
  sheet_name            text not null unique,
  sheet_aliases         text[] not null default '{}',
  side_image_url        text not null default '',
  logo_url              text not null default '',
  -- Insignias nuevas cuyo nombre contenga alguna de estas palabras nacen ocultas aquí.
  hidden_badge_keywords text[] not null default '{}',
  active                boolean not null default true,
  sort_order            integer not null default 0,
  created_at            timestamptz not null default now(),
  updated_at            timestamptz not null default now()
);

create table students (
  id               bigserial primary key,
  course_id        bigint not null references courses(id) on delete restrict,
  code             text not null check (code <> ''),
  name             text not null,
  sexo             text not null default 'Hombre',
  email            text not null default '',
  password_hash    text not null,
  tareas           integer not null default 0,
  monedas          integer not null default 0,
  asistencias      integer not null default 0,
  exp              integer not null default 0,
  games            integer not null default 0,
  last_login_level integer not null default 0,
  comments         text not null default '',
  active           boolean not null default true,
  last_login_at    timestamptz,
  created_at       timestamptz not null default now(),
  updated_at       timestamptz not null default now(),
  unique (course_id, code)
);
create index students_code_idx on students (code);

create table badges (
  id         bigserial primary key,
  name       text not null,
  slug       text not null unique,
  stock      integer not null default 0 check (stock >= 0),
  cost       integer not null default 0 check (cost >= 0),
  image_url  text not null default '',
  active     boolean not null default true,
  sort_order integer not null default 0,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

-- Fila presente = la insignia no se ve ni se compra en ese curso.
create table course_hidden_badges (
  course_id bigint not null references courses(id) on delete cascade,
  badge_id  bigint not null references badges(id) on delete cascade,
  primary key (course_id, badge_id)
);

create table purchases (
  id           bigserial primary key,
  student_id   bigint not null references students(id) on delete cascade,
  badge_id     bigint references badges(id) on delete set null,
  badge_name   text not null,
  quantity     integer not null check (quantity > 0),
  unit_cost    integer not null default 0,
  total_cost   integer not null default 0,
  coins_before integer,
  coins_after  integer,
  source       text not null default 'app' check (source in ('app', 'sheets', 'admin')),
  -- Huella de la fila importada de Sheets, para que reimportar no duplique.
  source_ref   text,
  created_at   timestamptz not null default now(),
  reverted_at  timestamptz
);
create index purchases_student_idx on purchases (student_id);
create index purchases_badge_idx on purchases (badge_id);
create unique index purchases_source_ref_idx on purchases (source_ref) where source_ref is not null;

-- code es la matrícula o DefaultH / DefaultF, igual que los archivos de avatars/.
create table avatars (
  code       text not null,
  level      integer not null check (level >= 0),
  url        text not null,
  updated_at timestamptz not null default now(),
  primary key (code, level)
);

create table levels (
  level   integer primary key check (level >= 0),
  min_exp integer not null unique check (min_exp >= 0)
);

-- Solo se guardan los valores que el admin cambió; los predeterminados viven en settings.go.
create table settings (
  key        text primary key,
  value      jsonb not null,
  updated_at timestamptz not null default now()
);

create table admins (
  id            bigserial primary key,
  username      text not null unique,
  password_hash text not null,
  active        boolean not null default true,
  last_login_at timestamptz,
  created_at    timestamptz not null default now(),
  updated_at    timestamptz not null default now()
);

create table audit_log (
  id          bigserial primary key,
  at          timestamptz not null default now(),
  actor_type  text not null check (actor_type in ('admin', 'student', 'system')),
  actor_id    bigint,
  actor_label text not null default '',
  action      text not null,
  entity      text not null,
  entity_id   text,
  summary     text not null,
  before      jsonb,
  after       jsonb,
  batch_id    uuid,
  undoable    boolean not null default true,
  undone_at   timestamptz,
  undone_by   bigint,
  reverts_id  bigint references audit_log(id) on delete set null
);
create index audit_log_entity_idx on audit_log (entity, entity_id);
create index audit_log_batch_idx on audit_log (batch_id) where batch_id is not null;

create table reset_codes (
  student_id bigint primary key references students(id) on delete cascade,
  code_hash  text not null,
  expires_at timestamptz not null,
  attempts   integer not null default 0
);
