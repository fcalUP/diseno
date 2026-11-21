import { useMemo, useState } from 'react'
import { useSearchParams } from 'react-router'
import { Download, Pencil, Plus, SearchX, UserPlus, Users } from 'lucide-react'
import { api, errorMessage } from '../../lib/api'
import { downloadCsv, toCsv } from '../../lib/csv'
import type { AdminStudent } from '../../lib/types'
import {
  Button,
  DataTable,
  EmptyState,
  IconButton,
  PageHeader,
  Pill,
  SearchInput,
  Select,
  Tabs,
  Toggle,
  useAsync,
  useToast,
  type Column,
  type TabItem,
} from '../ui'
import StudentDrawer from './StudentDrawer'
import BulkBar from './students/BulkBar'
import NewStudentModal from './students/NewStudentModal'
import { ErrorBanner, LinkButton, StudentAvatar } from './students/parts'
import { useUndoToast } from './students/useUndoToast'
import {
  STUDENT_CSV,
  alumnos,
  avatarKey,
  fileSlug,
  formatNumber,
  matchesStudent,
  sortCourses,
  today,
} from './students/util'

type Estado = 'all' | 'true' | 'false'

const ESTADOS = [
  { value: 'all', label: 'Todos los estados' },
  { value: 'true', label: 'Solo activos' },
  { value: 'false', label: 'Solo inactivos' },
]

const NO_STUDENTS: AdminStudent[] = []

export default function Students() {
  const toast = useToast()
  const done = useUndoToast()
  const [params, setParams] = useSearchParams()
  const studentsQ = useAsync(() => api.admin.students(), [])
  const coursesQ = useAsync(() => api.admin.courses(), [])
  // Un solo pedido de avatares por carga: con él se arma la miniatura de cada fila.
  const avatarsQ = useAsync(() => api.admin.avatars(), [])

  const [query, setQuery] = useState('')
  const [estado, setEstado] = useState<Estado>('all')
  const [selected, setSelected] = useState<Set<number>>(() => new Set())
  const [openId, setOpenId] = useState<number | null>(null)
  const [creating, setCreating] = useState(false)
  const [toggling, setToggling] = useState<ReadonlySet<number>>(() => new Set())

  const students = studentsQ.data?.students ?? NO_STUDENTS
  const courses = useMemo(() => sortCourses(coursesQ.data?.courses ?? []), [coursesQ.data])

  // El curso va en la URL (?curso=ID) para que el Resumen pueda enlazar aquí ya filtrado.
  const wanted = Number(params.get('curso'))
  const courseId =
    Number.isInteger(wanted) && wanted > 0 && (!coursesQ.data || courses.some((c) => c.id === wanted)) ? wanted : null
  const course = courses.find((c) => c.id === courseId)

  const counts = useMemo(() => {
    const m = new Map<number, number>()
    for (const s of students) m.set(s.courseId, (m.get(s.courseId) ?? 0) + 1)
    return m
  }, [students])

  const rows = useMemo(
    () =>
      students.filter(
        (s) =>
          (courseId === null || s.courseId === courseId) &&
          (estado === 'all' || String(s.active) === estado) &&
          matchesStudent(s, query),
      ),
    [students, courseId, estado, query],
  )

  const selectedRows = useMemo(() => rows.filter((s) => selected.has(s.id)), [rows, selected])

  const avatarUrls = useMemo(
    () => new Map((avatarsQ.data?.avatars ?? []).map((a) => [avatarKey(a.code, a.level), a.url])),
    [avatarsQ.data],
  )

  const tabs: TabItem<string>[] = [
    { value: 'todos', label: 'Todos', count: students.length },
    ...courses.map((c) => ({
      value: String(c.id),
      label: c.active ? c.name : `${c.name} (oculto)`,
      count: counts.get(c.id) ?? 0,
    })),
  ]

  const reloadAll = () => {
    studentsQ.reload()
    avatarsQ.reload()
  }

  const pickCourse = (value: string) => {
    setSelected(new Set())
    setParams(value === 'todos' ? {} : { curso: value }, { replace: true })
  }

  const clearFilters = () => {
    setQuery('')
    setEstado('all')
    pickCourse('todos')
  }

  const toggleActive = async (s: AdminStudent) => {
    setToggling((prev) => new Set(prev).add(s.id))
    try {
      const res = await api.admin.updateStudent(s.id, { active: !s.active })
      studentsQ.setData((d) => d && { ...d, students: d.students.map((x) => (x.id === s.id ? res.student : x)) })
      done(
        res.student.active ? `${s.name} quedó activo.` : `${s.name} quedó inactivo.`,
        { auditId: res.auditId },
        studentsQ.reload,
      )
    } catch (e) {
      toast.error(errorMessage(e))
    } finally {
      setToggling((prev) => {
        const next = new Set(prev)
        next.delete(s.id)
        return next
      })
    }
  }

  const exportRows = () => {
    const name = course ? `alumnos-${fileSlug(course.name)}` : 'alumnos'
    downloadCsv(`${name}-${today()}.csv`, toCsv(rows, STUDENT_CSV))
  }

  const thumb = (s: AdminStudent) =>
    s.avatarLevels?.includes(s.level) ? avatarUrls.get(avatarKey(s.code, s.level)) : undefined

  const num = (key: 'level' | 'exp' | 'monedas' | 'tareas' | 'asistencias' | 'games' | 'badges', header: string) => ({
    key,
    header,
    align: 'right' as const,
    className: 'tabular-nums',
    cell: (s: AdminStudent) => formatNumber(s[key]),
  })

  // Avatar, nombre e ID van en una sola celda y el interruptor justo después: así la tabla
  // cabe en una laptop sin tener que desplazarla para activar o desactivar.
  const columns: Column<AdminStudent>[] = [
    {
      key: 'student',
      header: 'Alumno',
      cell: (s) => (
        <div className="flex min-w-[13rem] items-center gap-3">
          <StudentAvatar name={s.name} url={thumb(s)} />
          <div className="min-w-0">
            <p className="font-medium text-gray-800">
              {s.name}
              {!s.active && (
                <Pill tone="gray" className="ml-2 align-middle">
                  Inactivo
                </Pill>
              )}
            </p>
            <p className="text-xs text-gray-500">
              <span className="font-mono">{s.code}</span>
              {courseId === null && ` · ${s.courseName}`}
            </p>
          </div>
        </div>
      ),
    },
    {
      key: 'active',
      header: 'Activo',
      align: 'center',
      cell: (s) => (
        <Toggle
          label={`${s.name} activo`}
          hideLabel
          checked={s.active}
          disabled={toggling.has(s.id)}
          onChange={() => void toggleActive(s)}
        />
      ),
    },
    num('level', 'Nivel'),
    num('exp', 'EXP'),
    num('monedas', 'Monedas'),
    num('tareas', 'Tareas'),
    num('asistencias', 'Asistencias'),
    num('games', 'Juegos'),
    num('badges', 'Insignias'),
    {
      key: 'actions',
      header: <span className="sr-only">Acciones</span>,
      align: 'right',
      cell: (s) => <IconButton icon={Pencil} label={`Editar a ${s.name}`} size="sm" onClick={() => setOpenId(s.id)} />,
    },
  ]

  const filtered = query.trim() !== '' || estado !== 'all' || courseId !== null
  const empty =
    students.length === 0 ? (
      <EmptyState
        icon={Users}
        title="Aún no hay alumnos"
        description="Agrega uno, genera varios desde una lista o impórtalos desde Google Sheets."
        action={
          <div className="flex flex-wrap justify-center gap-2">
            <Button icon={Plus} onClick={() => setCreating(true)} disabled={courses.length === 0}>
              Nuevo alumno
            </Button>
            <LinkButton to="/admin/importar">Importar</LinkButton>
          </div>
        }
      />
    ) : (
      <EmptyState
        icon={SearchX}
        title="Sin resultados"
        description="Ningún alumno coincide con los filtros."
        action={
          filtered && (
            <Button variant="ghost" onClick={clearFilters}>
              Limpiar filtros
            </Button>
          )
        }
      />
    )

  const activeCount = useMemo(() => students.filter((s) => s.active).length, [students])

  return (
    <>
      <PageHeader
        title="Alumnos"
        subtitle={
          studentsQ.data
            ? `${alumnos(students.length)}, ${formatNumber(activeCount)} ${activeCount === 1 ? 'activo' : 'activos'}.`
            : 'Datos, contadores, avatares y contraseñas de cada alumno.'
        }
        actions={
          <>
            <LinkButton to={courseId ? `/admin/generar?curso=${courseId}` : '/admin/generar'}>
              <UserPlus aria-hidden="true" className="h-4 w-4" />
              Generar alumnos
            </LinkButton>
            <Button icon={Plus} onClick={() => setCreating(true)} disabled={courses.length === 0}>
              Nuevo alumno
            </Button>
          </>
        }
      />

      {coursesQ.error && (
        <ErrorBanner
          className="mb-4"
          message={`No se pudieron cargar los cursos: ${errorMessage(coursesQ.error)}`}
          onRetry={coursesQ.reload}
        />
      )}

      <Tabs
        label="Filtrar por curso"
        items={tabs}
        value={courseId === null ? 'todos' : String(courseId)}
        onChange={pickCourse}
        className="mb-4"
      />

      <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center">
        <SearchInput
          value={query}
          onChange={setQuery}
          placeholder="Buscar por ID, nombre o correo"
          label="Buscar alumnos"
          className="sm:flex-1"
        />
        <Select
          label="Estado"
          hideLabel
          value={estado}
          onChange={(v) => setEstado(v as Estado)}
          options={ESTADOS}
          className="sm:w-48"
        />
        <Button variant="ghost" icon={Download} onClick={exportRows} disabled={rows.length === 0}>
          Exportar CSV
        </Button>
      </div>

      {studentsQ.error && (
        <ErrorBanner
          className="mb-4"
          message={
            studentsQ.data
              ? `No se pudo actualizar la lista: ${errorMessage(studentsQ.error)}`
              : `No se pudieron cargar los alumnos: ${errorMessage(studentsQ.error)}`
          }
          onRetry={studentsQ.reload}
        />
      )}

      {selectedRows.length > 0 && (
        <BulkBar rows={selectedRows} courses={courses} onClear={() => setSelected(new Set())} onDone={reloadAll} />
      )}

      {!(studentsQ.error && !studentsQ.data) && (
        <>
          <DataTable
            rows={rows}
            columns={columns}
            rowKey={(s) => s.id}
            selected={selected}
            onSelectionChange={setSelected}
            onRowClick={(s) => setOpenId(s.id)}
            rowLabel={(s) => `${s.name} (${s.code})`}
            loading={studentsQ.loading}
            empty={empty}
            caption={course ? `Alumnos de ${course.name}` : 'Alumnos'}
            className="relative max-h-[70vh]"
          />
          {studentsQ.data && rows.length > 0 && (
            <p className="mt-2 text-xs text-gray-500" aria-live="polite">
              {rows.length === students.length
                ? `Mostrando ${alumnos(rows.length)}.`
                : `Mostrando ${formatNumber(rows.length)} de ${alumnos(students.length)}.`}
            </p>
          )}
        </>
      )}

      <StudentDrawer
        studentId={openId}
        initial={students.find((s) => s.id === openId)}
        onClose={() => setOpenId(null)}
        onChanged={reloadAll}
      />
      <NewStudentModal
        open={creating}
        courses={courses}
        defaultCourseId={courseId}
        onClose={() => setCreating(false)}
        onCreated={studentsQ.reload}
      />
    </>
  )
}
