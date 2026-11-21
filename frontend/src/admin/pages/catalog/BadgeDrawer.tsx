import { useId, useState, type FormEvent } from 'react'
import { Trash2 } from 'lucide-react'
import { api, errorMessage } from '../../../lib/api'
import type { AdminBadge, AdminCourse, BadgeInput } from '../../../lib/types'
import {
  Button,
  Drawer,
  ImageUpload,
  NumberField,
  Spinner,
  TextField,
  Toggle,
  useConfirm,
  useToast,
  useUndo,
} from '../../ui'
import { CheckList } from './CheckList'
import { LoadError } from './PageState'
import { CATALOG_INT_MAX, catalogCount, catalogKeywordHit, catalogSameIds, catalogSlug } from './utils'

export interface BadgeDrawerProps {
  /** null = insignia nueva. */
  badge: AdminBadge | null
  /** Todas las insignias, para avisar de nombres repetidos. */
  badges: AdminBadge[]
  /** Cursos para la visibilidad; undefined mientras cargan o si fallaron. */
  courses: AdminCourse[] | undefined
  coursesError: string
  onRetryCourses: () => void
  onClose: () => void
  onSaved: (badge: AdminBadge) => void
  onDeleted: (id: number) => void
  /** Se llama después de deshacer desde un aviso, para recargar la lista. */
  onUndone: () => void
}

interface Form {
  name: string
  cost: number
  stock: number
  imageUrl: string
  active: boolean
}

const sectionTitle = 'text-sm font-semibold uppercase tracking-wide text-gray-500'

function countError(n: number): string {
  return Number.isInteger(n) && n >= 0 && n <= CATALOG_INT_MAX ? '' : 'Usa un número entero de 0 en adelante.'
}

// Alta y edición de una insignia. Se monta al abrir y se desmonta al cerrar.
export function BadgeDrawer({
  badge,
  badges,
  courses,
  coursesError,
  onRetryCourses,
  onClose,
  onSaved,
  onDeleted,
  onUndone,
}: BadgeDrawerProps) {
  const formId = useId()
  const toast = useToast()
  const confirm = useConfirm()
  const undo = useUndo()
  const [form, setForm] = useState<Form>(() => ({
    name: badge?.name ?? '',
    cost: badge?.cost ?? 0,
    stock: badge?.stock ?? 0,
    imageUrl: badge?.imageUrl ?? '',
    active: badge?.active ?? true,
  }))
  const [hidden, setHidden] = useState<ReadonlySet<number>>(() => new Set(badge?.hiddenCourseIds ?? []))
  const [showErrors, setShowErrors] = useState(false)
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [error, setError] = useState('')

  const set = <K extends keyof Form>(key: K, value: Form[K]) => setForm((f) => ({ ...f, [key]: value }))

  const name = form.name.trim()
  const slug = catalogSlug(name)
  const errors = {
    name: !name
      ? 'Escribe el nombre de la insignia.'
      : name.length > 120
        ? 'Máximo 120 caracteres.'
        : !slug
          ? 'Usa al menos una letra o un número.'
          : badges.some((b) => b.id !== badge?.id && b.slug === slug)
            ? 'Ya hay una insignia con ese nombre (o uno que sólo cambia en acentos o signos).'
            : '',
    cost: countError(form.cost),
    stock: countError(form.stock),
  }
  const invalid = Boolean(errors.name || errors.cost || errors.stock)

  // Al crear, los cursos con palabras clave que aparecen en el nombre la ocultan solos.
  const keywordHits = new Map<number, string>()
  if (!badge) {
    for (const c of courses ?? []) {
      const hit = catalogKeywordHit(name, c.hiddenBadgeKeywords)
      if (hit !== null) keywordHits.set(c.id, hit)
    }
  }
  const hiddenToSave = [...new Set([...hidden, ...keywordHits.keys()])]

  const changes: BadgeInput = {}
  if (badge) {
    if (name !== badge.name) changes.name = name
    if (form.cost !== badge.cost) changes.cost = form.cost
    if (form.stock !== badge.stock) changes.stock = form.stock
    if (form.imageUrl !== badge.imageUrl) changes.imageUrl = form.imageUrl
    if (form.active !== badge.active) changes.active = form.active
    if (courses !== undefined && !catalogSameIds(hidden, badge.hiddenCourseIds)) changes.hiddenCourseIds = [...hidden]
  }
  const dirty = badge
    ? Object.keys(changes).length > 0
    : Boolean(name || form.cost || form.stock || form.imageUrl || !form.active || hidden.size)

  const undoThenReload = async (auditId: number) => {
    if (await undo({ auditId })) onUndone()
  }

  const submit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    setShowErrors(true)
    if (saving || deleting || invalid) return
    if (badge && Object.keys(changes).length === 0) {
      toast.info('No había cambios que guardar.')
      onClose()
      return
    }
    setSaving(true)
    setError('')
    try {
      const r = badge
        ? await api.admin.updateBadge(badge.id, changes)
        : await api.admin.createBadge({
            name,
            cost: form.cost,
            stock: form.stock,
            imageUrl: form.imageUrl,
            active: form.active,
            hiddenCourseIds: hiddenToSave,
          })
      onSaved(r.badge)
      toast.success(badge ? `Insignia «${r.badge.name}» guardada.` : `Insignia «${r.badge.name}» creada.`, {
        undo: () => undoThenReload(r.auditId),
      })
      onClose()
    } catch (err) {
      setError(errorMessage(err))
      setSaving(false)
    }
  }

  const remove = async () => {
    if (!badge) return
    const ok = await confirm({
      title: `¿Eliminar «${badge.name}»?`,
      message: (
        <>
          <p>
            Las compras que ya se hicieron conservan el nombre de la insignia y siguen en el historial
            {badge.purchasedCount > 0
              ? ` (lleva ${catalogCount(badge.purchasedCount, 'unidad vendida', 'unidades vendidas')})`
              : ''}
            .
          </p>
          <p className="mt-2">Si sólo quieres quitarla de la tienda, mejor apaga «Activa».</p>
        </>
      ),
      confirmLabel: 'Eliminar insignia',
      danger: true,
    })
    if (!ok) return
    setDeleting(true)
    setError('')
    try {
      const { auditId } = await api.admin.deleteBadge(badge.id)
      onDeleted(badge.id)
      toast.success(`Insignia «${badge.name}» eliminada.`, { undo: () => undoThenReload(auditId) })
      onClose()
    } catch (err) {
      setError(errorMessage(err))
      setDeleting(false)
    }
  }

  const requestClose = async () => {
    if (saving || deleting) return
    if (
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

  let courseList
  if (courses) {
    const visible = new Set(courses.filter((c) => !hidden.has(c.id) && !keywordHits.has(c.id)).map((c) => c.id))
    courseList = (
      <CheckList
        legend="Visible por curso"
        hint="Las desmarcadas no aparecen en la tienda de ese curso."
        items={courses.map((c) => {
          const hit = keywordHits.get(c.id)
          return {
            id: c.id,
            label: c.name,
            disabled: hit !== undefined,
            note:
              hit !== undefined
                ? `Se oculta sola por la palabra «${hit}»; podrás mostrarla después de crearla.`
                : c.active
                  ? undefined
                  : 'Curso oculto en el login',
          }
        })}
        checked={visible}
        onChange={(next) => setHidden(new Set(courses.filter((c) => !next.has(c.id)).map((c) => c.id)))}
        allLabel="Mostrar en todos"
        noneLabel="Ocultar en todos"
        summary={(n, total) => `Visible en ${n} de ${total} cursos`}
        empty="Todavía no hay cursos."
      />
    )
  } else if (coursesError) {
    courseList = <LoadError message={`No se pudieron cargar los cursos: ${coursesError}`} onRetry={onRetryCourses} />
  } else {
    courseList = <Spinner label="Cargando cursos…" showLabel />
  }

  const deleteHintId = `${formId}-delete-hint`

  return (
    <Drawer
      open
      onClose={() => void requestClose()}
      title={badge ? 'Editar insignia' : 'Nueva insignia'}
      description={badge ? badge.name : 'Los alumnos la compran con sus monedas en la tienda.'}
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
          <Button type="submit" form={formId} loading={saving} disabled={deleting}>
            {badge ? 'Guardar cambios' : 'Crear insignia'}
          </Button>
        </>
      }
    >
      <form id={formId} noValidate onSubmit={(e) => void submit(e)} className="space-y-8">
        <section aria-labelledby={`${formId}-data`} className="space-y-4">
          <h3 id={`${formId}-data`} className={sectionTitle}>
            Datos
          </h3>
          <TextField
            label="Nombre"
            value={form.name}
            onChange={(e) => set('name', e.target.value)}
            error={showErrors ? errors.name : undefined}
            hint={
              badge && badge.purchasedCount > 0
                ? 'Las compras anteriores conservan el nombre con el que se compraron.'
                : undefined
            }
            maxLength={120}
            required
            autoComplete="off"
            data-autofocus
          />
          <div className="grid gap-4 sm:grid-cols-2">
            <NumberField
              label="Costo (monedas)"
              value={form.cost}
              onChange={(v) => set('cost', v)}
              error={showErrors ? errors.cost : undefined}
              min={0}
              step={1}
              inputMode="numeric"
            />
            <NumberField
              label="Stock"
              value={form.stock}
              onChange={(v) => set('stock', v)}
              error={showErrors ? errors.stock : undefined}
              hint="Unidades que quedan para comprar."
              min={0}
              step={1}
              inputMode="numeric"
            />
          </div>
          <ImageUpload
            label="Imagen"
            value={form.imageUrl}
            onChange={(url) => set('imageUrl', url)}
            previewClassName="h-48 w-full"
          />
          <Toggle
            checked={form.active}
            onChange={(v) => set('active', v)}
            label="Activa"
            description="Las inactivas no aparecen en la tienda de ningún curso."
          />
          {badge && (
            <p className="text-sm text-gray-600">
              Vendidas:{' '}
              <span className="font-semibold">{catalogCount(badge.purchasedCount, 'unidad', 'unidades')}</span> (sin
              contar compras deshechas).
            </p>
          )}
        </section>

        <section aria-labelledby={`${formId}-courses`} className="space-y-4">
          <h3 id={`${formId}-courses`} className={sectionTitle}>
            Cursos
          </h3>
          {courseList}
        </section>

        {badge && (
          <section aria-labelledby={`${formId}-danger`} className="rounded-2xl border border-red-100 bg-red-50/60 p-4">
            <h3 id={`${formId}-danger`} className="text-sm font-semibold text-red-700">
              Eliminar insignia
            </h3>
            <p id={deleteHintId} className="mt-1 text-sm text-gray-600">
              Desaparece de la tienda y del panel. Las compras anteriores conservan su nombre. Puedes deshacerlo justo
              después.
            </p>
            <Button
              variant="danger"
              size="sm"
              icon={Trash2}
              className="mt-3"
              onClick={() => void remove()}
              disabled={saving}
              loading={deleting}
              aria-describedby={deleteHintId}
            >
              Eliminar insignia
            </Button>
          </section>
        )}
      </form>
    </Drawer>
  )
}
