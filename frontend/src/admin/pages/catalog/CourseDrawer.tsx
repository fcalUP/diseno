import { useId, useState, type FormEvent } from 'react'
import { EyeOff, Trash2 } from 'lucide-react'
import { api, errorMessage } from '../../../lib/api'
import type { AdminBadge, AdminCourse, CourseInput } from '../../../lib/types'
import {
  Button,
  CLOSE_WHILE_UPLOADING,
  Drawer,
  ImageUpload,
  Spinner,
  TextField,
  Toggle,
  useConfirm,
  useToast,
  useUndo,
  useUploads,
} from '../../ui'
import { CheckList } from './CheckList'
import { ChipsInput } from './ChipsInput'
import { LoginPreview } from './LoginPreview'
import { LoadError } from './PageState'
import { catalogCount, catalogKeywordHit, catalogNormalize, catalogSameIds, catalogSameList } from './utils'

export interface CourseDrawerProps {
  /** null = curso nuevo. */
  course: AdminCourse | null
  /** Todos los cursos, para avisar de nombres u hojas repetidas. */
  courses: AdminCourse[]
  /** Insignias para la lista de visibles; undefined mientras cargan o si fallaron. */
  badges: AdminBadge[] | undefined
  badgesError: string
  onRetryBadges: () => void
  onClose: () => void
  onSaved: (course: AdminCourse) => void
  onDeleted: (id: number) => void
  /** Se llama después de deshacer desde un aviso, para recargar la lista. */
  onUndone: () => void
}

interface Form {
  name: string
  sheetName: string
  sheetAliases: string[]
  sideImageUrl: string
  logoUrl: string
  hiddenBadgeKeywords: string[]
  active: boolean
}

function toForm(c: AdminCourse | null): Form {
  return {
    name: c?.name ?? '',
    sheetName: c?.sheetName ?? '',
    sheetAliases: c?.sheetAliases ?? [],
    sideImageUrl: c?.sideImageUrl ?? '',
    logoUrl: c?.logoUrl ?? '',
    hiddenBadgeKeywords: c?.hiddenBadgeKeywords ?? [],
    active: c?.active ?? true,
  }
}

// Sólo los campos que cambiaron respecto a lo guardado (el PATCH es parcial).
function changesFrom(f: Form, base: AdminCourse): CourseInput {
  const out: CourseInput = {}
  const name = f.name.trim()
  const sheet = f.sheetName.trim()
  if (name !== base.name) out.name = name
  if (sheet !== base.sheetName) out.sheetName = sheet
  if (!catalogSameList(f.sheetAliases, base.sheetAliases)) out.sheetAliases = f.sheetAliases
  if (f.sideImageUrl !== base.sideImageUrl) out.sideImageUrl = f.sideImageUrl
  if (f.logoUrl !== base.logoUrl) out.logoUrl = f.logoUrl
  if (!catalogSameList(f.hiddenBadgeKeywords, base.hiddenBadgeKeywords)) out.hiddenBadgeKeywords = f.hiddenBadgeKeywords
  if (f.active !== base.active) out.active = f.active
  return out
}

const sectionTitle = 'text-sm font-semibold uppercase tracking-wide text-gray-500'

// Alta y edición de un curso. Se monta al abrir y se desmonta al cerrar.
export function CourseDrawer({
  course,
  courses,
  badges,
  badgesError,
  onRetryBadges,
  onClose,
  onSaved,
  onDeleted,
  onUndone,
}: CourseDrawerProps) {
  const formId = useId()
  const toast = useToast()
  const confirm = useConfirm()
  const undo = useUndo()
  // Lo último que quedó guardado; tras un guardado parcial el formulario compara contra esto.
  const [base, setBase] = useState(course)
  const [form, setForm] = useState(() => toForm(course))
  const [hidden, setHidden] = useState<ReadonlySet<number>>(() => new Set(course?.hiddenBadgeIds ?? []))
  const [showErrors, setShowErrors] = useState(false)
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [error, setError] = useState('')
  // Imagen lateral y logo comparten la cuenta de subidas en curso.
  const [uploading, onUploadingChange] = useUploads()

  const set = <K extends keyof Form>(key: K, value: Form[K]) => setForm((f) => ({ ...f, [key]: value }))

  const name = form.name.trim()
  const sheet = form.sheetName.trim()
  // En el alta, sin hoja se usa el nombre del curso.
  const sheetToSave = sheet || (base ? '' : name)
  const others = courses.filter((c) => c.id !== base?.id)
  const errors = {
    name: !name
      ? 'Escribe el nombre del curso.'
      : name.length > 120
        ? 'Máximo 120 caracteres.'
        : others.some((c) => c.name === name)
          ? 'Ya hay un curso con ese nombre.'
          : '',
    // En el alta sin nombre ya avisa el campo de arriba.
    sheetName: !sheetToSave
      ? base
        ? 'Escribe el nombre de la pestaña.'
        : ''
      : sheetToSave.length > 100
        ? 'Máximo 100 caracteres.'
        : others.some((c) => c.sheetName === sheetToSave)
          ? 'Otro curso ya usa esa hoja.'
          : '',
  }

  const changes = base ? changesFrom(form, base) : {}
  const hiddenChanged = (saved: AdminCourse | null) =>
    badges !== undefined && !catalogSameIds(hidden, saved?.hiddenBadgeIds ?? [])
  const dirty = base
    ? Object.keys(changes).length > 0 || hiddenChanged(base)
    : Boolean(
        name ||
        sheet ||
        form.sheetAliases.length ||
        form.sideImageUrl ||
        form.logoUrl ||
        form.hiddenBadgeKeywords.length ||
        !form.active ||
        hidden.size,
      )

  // Deshace varios cambios del más nuevo al más viejo; se detiene si uno falla.
  const undoAll = async (auditIds: number[]) => {
    let any = false
    for (const auditId of [...auditIds].reverse()) {
      if (!(await undo({ auditId }))) break
      any = true
    }
    if (any) onUndone()
  }

  const submit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    setShowErrors(true)
    // Con una imagen a medio subir se guardaría la anterior.
    if (saving || deleting || uploading || errors.name || errors.sheetName) return
    setSaving(true)
    setError('')
    const done: number[] = []
    let saved = base
    try {
      if (!saved) {
        const r = await api.admin.createCourse({
          name,
          sheetName: sheetToSave,
          sheetAliases: form.sheetAliases,
          sideImageUrl: form.sideImageUrl,
          logoUrl: form.logoUrl,
          hiddenBadgeKeywords: form.hiddenBadgeKeywords,
          active: form.active,
        })
        saved = r.course
        done.push(r.auditId)
      } else if (Object.keys(changes).length > 0) {
        const r = await api.admin.updateCourse(saved.id, changes)
        saved = r.course
        done.push(r.auditId)
      }
    } catch (err) {
      setError(errorMessage(err))
      setSaving(false)
      return
    }
    if (saved && saved !== base) {
      setBase(saved)
      onSaved(saved)
    }

    if (saved && hiddenChanged(saved)) {
      try {
        const r = await api.admin.setCourseHiddenBadges(saved.id, [...hidden])
        saved = r.course
        done.push(r.auditId)
        setBase(saved)
        onSaved(saved)
      } catch (err) {
        setSaving(false)
        setError(`No se guardó qué insignias se ven: ${errorMessage(err)}`)
        if (done.length > 0 && saved) {
          toast.success(`Se guardaron los datos de «${saved.name}».`, { undo: () => undoAll(done) })
        }
        return
      }
    }

    setSaving(false)
    if (!saved || done.length === 0) {
      toast.info('No había cambios que guardar.')
      onClose()
      return
    }
    toast.success(base ? `Curso «${saved.name}» guardado.` : `Curso «${saved.name}» creado.`, {
      undo: () => undoAll(done),
    })
    onClose()
  }

  const remove = async () => {
    if (!base || base.studentCount > 0) return
    const ok = await confirm({
      title: `¿Eliminar «${base.name}»?`,
      message:
        'Se borra el curso con su configuración: imágenes, alias e insignias ocultas. Puedes deshacerlo desde el aviso o en Actividad.',
      confirmLabel: 'Eliminar curso',
      danger: true,
    })
    if (!ok) return
    setDeleting(true)
    setError('')
    try {
      const { auditId } = await api.admin.deleteCourse(base.id)
      onDeleted(base.id)
      toast.success(`Curso «${base.name}» eliminado.`, { undo: () => undoAll([auditId]) })
      onClose()
    } catch (err) {
      setError(errorMessage(err))
      setDeleting(false)
    }
  }

  const requestClose = async () => {
    if (saving || deleting) return
    if (uploading) {
      if (!(await confirm(CLOSE_WHILE_UPLOADING))) return
    } else if (
      dirty &&
      !(await confirm({
        title: '¿Descartar los cambios?',
        message: 'Lo que no guardaste se pierde.',
        confirmLabel: 'Descartar',
        danger: true,
      }))
    ) {
      return
    }
    onClose()
  }

  // Insignias que ya existen y que las palabras clave ocultarían si fueran nuevas.
  const keywordMatches = (badges ?? []).filter(
    (b) => !hidden.has(b.id) && catalogKeywordHit(b.name, form.hiddenBadgeKeywords) !== null,
  )
  const visible = new Set((badges ?? []).filter((b) => !hidden.has(b.id)).map((b) => b.id))

  let badgeList
  if (badges) {
    badgeList = (
      <CheckList
        legend="Insignias visibles en este curso"
        hint="Las desmarcadas no aparecen en la tienda de este curso. Las inactivas no salen en ninguna tienda."
        items={badges.map((b) => ({
          id: b.id,
          label: b.name,
          imageUrl: b.imageUrl,
          note: b.active ? undefined : 'Inactiva',
        }))}
        checked={visible}
        onChange={(next) => setHidden(new Set(badges.filter((b) => !next.has(b.id)).map((b) => b.id)))}
        allLabel="Mostrar todas"
        noneLabel="Ocultar todas"
        summary={(n, total) => `${n} de ${total} visibles`}
        empty="Todavía no hay insignias. Créalas en Insignias."
        withImages
        extra={
          keywordMatches.length > 0 && (
            <Button
              variant="ghost"
              size="sm"
              icon={EyeOff}
              title="Oculta en este curso las insignias que ya existen y cuyo nombre contiene alguna de las palabras de arriba."
              onClick={() => setHidden(new Set([...hidden, ...keywordMatches.map((b) => b.id)]))}
            >
              Ocultar las que coinciden ({keywordMatches.length})
            </Button>
          )
        }
      />
    )
  } else if (badgesError) {
    badgeList = <LoadError message={`No se pudieron cargar las insignias: ${badgesError}`} onRetry={onRetryBadges} />
  } else {
    badgeList = <Spinner label="Cargando insignias…" showLabel />
  }

  const deleteHintId = `${formId}-delete-hint`
  const hasStudents = (base?.studentCount ?? 0) > 0

  return (
    <Drawer
      open
      onClose={() => void requestClose()}
      title={base ? 'Editar curso' : 'Nuevo curso'}
      description={base ? base.name : 'Cuando está visible aparece en el selector del login y del registro.'}
      footer={
        <>
          {error && (
            <p role="alert" className="w-full text-sm text-red-600 sm:mr-auto sm:w-auto sm:flex-1 sm:self-center">
              {error}
            </p>
          )}
          <Button variant="ghost" onClick={() => void requestClose()} disabled={saving || deleting}>
            Cancelar
          </Button>
          <Button type="submit" form={formId} loading={saving} disabled={deleting || uploading}>
            {base ? 'Guardar cambios' : 'Crear curso'}
          </Button>
        </>
      }
    >
      <form id={formId} noValidate onSubmit={(e) => void submit(e)} className="space-y-8">
        <section aria-labelledby={`${formId}-data`} className="space-y-4">
          <h3 id={`${formId}-data`} className={sectionTitle}>
            Datos del curso
          </h3>
          <TextField
            label="Nombre"
            value={form.name}
            onChange={(e) => set('name', e.target.value)}
            error={showErrors ? errors.name : undefined}
            hint="Es lo que ven los alumnos en el selector del login."
            maxLength={120}
            required
            autoComplete="off"
            data-autofocus
          />
          <TextField
            label="Hoja de Google Sheets"
            value={form.sheetName}
            onChange={(e) => set('sheetName', e.target.value)}
            error={showErrors ? errors.sheetName : undefined}
            hint={
              base
                ? 'Nombre exacto de la pestaña de donde se importan sus alumnos.'
                : 'Nombre exacto de la pestaña de donde se importan sus alumnos. Si lo dejas vacío se usa el nombre del curso.'
            }
            placeholder={base ? undefined : name || undefined}
            maxLength={100}
            autoComplete="off"
          />
          <ChipsInput
            label="Otros nombres de la hoja (alias)"
            itemName="alias"
            values={form.sheetAliases}
            onChange={(v) => set('sheetAliases', v)}
            hint="También se reconocen al importar. Escribe uno y presiona Enter."
            placeholder="Ej. Análisis de datos"
          />
          <Toggle
            checked={form.active}
            onChange={(v) => set('active', v)}
            label="Visible en el login"
            description="Si lo apagas, el curso sale del selector del login y del registro; sus alumnos se conservan."
          />
        </section>

        <section aria-labelledby={`${formId}-images`} className="space-y-4">
          <h3 id={`${formId}-images`} className={sectionTitle}>
            Imágenes del login
          </h3>
          <div className="grid gap-4 sm:grid-cols-2">
            <ImageUpload
              label="Imagen lateral"
              value={form.sideImageUrl}
              onChange={(url) => set('sideImageUrl', url)}
              onUploadingChange={onUploadingChange}
            />
            <ImageUpload
              label="Logo"
              value={form.logoUrl}
              onChange={(url) => set('logoUrl', url)}
              onUploadingChange={onUploadingChange}
            />
          </div>
          <LoginPreview name={name} sideImageUrl={form.sideImageUrl} logoUrl={form.logoUrl} />
        </section>

        <section aria-labelledby={`${formId}-badges`} className="space-y-4">
          <h3 id={`${formId}-badges`} className={sectionTitle}>
            Insignias
          </h3>
          <ChipsInput
            label="Palabras para ocultar insignias nuevas"
            itemName="palabra"
            values={form.hiddenBadgeKeywords}
            onChange={(v) => set('hiddenBadgeKeywords', v)}
            normalize={catalogNormalize}
            hint="Si el nombre de una insignia nueva contiene alguna, se crea oculta en este curso (sin importar acentos ni mayúsculas). No cambia las que ya existen."
            placeholder="Ej. calculadora"
          />
          {badgeList}
        </section>

        {base && (
          <section aria-labelledby={`${formId}-danger`} className="rounded-2xl border border-red-100 bg-red-50/60 p-4">
            <h3 id={`${formId}-danger`} className="text-sm font-semibold text-red-700">
              Eliminar curso
            </h3>
            <p id={deleteHintId} className="mt-1 text-sm text-gray-600">
              {hasStudents
                ? `No se puede eliminar porque tiene ${catalogCount(base.studentCount, 'alumno', 'alumnos')}. Muévelos a otro curso desde Alumnos, o apaga «Visible en el login» para ocultarlo.`
                : 'Se borra el curso y su configuración. Puedes deshacerlo justo después.'}
            </p>
            <span
              className="mt-3 inline-block"
              title={hasStudents ? 'Tiene alumnos: muévelos o desactiva el curso.' : undefined}
            >
              <Button
                variant="danger"
                size="sm"
                icon={Trash2}
                onClick={() => void remove()}
                disabled={hasStudents || saving}
                loading={deleting}
                aria-describedby={deleteHintId}
                className="disabled:pointer-events-none"
              >
                Eliminar curso
              </Button>
            </span>
          </section>
        )}
      </form>
    </Drawer>
  )
}
