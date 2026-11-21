import { useId, useRef, useState, type FormEvent } from 'react'
import { KeyRound, PencilLine, Redo2, Trash2, Undo2 } from 'lucide-react'
import { api, errorMessage } from '../../lib/api'
import type { AdminPurchase, AdminStudent, StudentInput } from '../../lib/types'
import {
  Button,
  Drawer,
  ImageUpload,
  NumberField,
  Pill,
  Select,
  TextArea,
  TextField,
  Toggle,
  cx,
  useAsync,
  useConfirm,
  useToast,
} from '../ui'
import {
  ErrorBanner,
  LevelPreview,
  LoadingBlock,
  PasswordReveal,
  Section,
  SexoField,
  StudentAvatar,
} from './students/parts'
import { useAvatarActions } from './students/useAvatarActions'
import { useUndoToast } from './students/useUndoToast'
import {
  COUNTERS,
  MAX_COMMENTS,
  cleanForm,
  fetchStudent,
  formatDateTime,
  formatNumber,
  pl,
  sortCourses,
  toForm,
  validateStudent,
  type FormErrors,
  type StudentForm,
} from './students/util'

export interface StudentDrawerProps {
  studentId: number | null
  onClose: () => void
  /** Algo cambió (datos, avatares, compras): la página que lo abrió debe recargar. */
  onChanged: () => void
  /** La fila que ya tiene la lista; si no viene, el panel la pide al servidor. */
  initial?: AdminStudent
}

// Detalle de un alumno. Se vuelve a montar por cada alumno, así no se arrastran ediciones.
export default function StudentDrawer({ studentId, ...rest }: StudentDrawerProps) {
  if (studentId === null) return null
  return <StudentPanel key={studentId} studentId={studentId} {...rest} />
}

type PanelProps = Omit<StudentDrawerProps, 'studentId'> & { studentId: number }

function StudentPanel({ studentId, initial, onClose, onChanged }: PanelProps) {
  const toast = useToast()
  const confirm = useConfirm()
  const done = useUndoToast()
  const formId = useId()
  const formRef = useRef<HTMLFormElement>(null)

  const fetched = useAsync(
    () => (initial?.id === studentId ? Promise.resolve(null) : fetchStudent(studentId)),
    [studentId],
  )
  // Lo que devolvió el servidor tras un cambio hecho aquí; manda sobre lo demás.
  const [latest, setLatest] = useState<AdminStudent | null>(null)
  const student = latest ?? (initial?.id === studentId ? initial : null) ?? fetched.data ?? null

  const coursesQ = useAsync(() => api.admin.courses(), [])
  const levelsQ = useAsync(() => api.admin.levels(), [])
  const code = student?.code ?? ''
  // Sin código el endpoint devolvería todos los avatares; por eso la guarda.
  const avatarsQ = useAsync(() => (code ? api.admin.avatars(code) : Promise.resolve({ avatars: [] })), [code])
  const purchasesQ = useAsync(() => api.admin.purchases({ studentId, limit: 20 }), [studentId])

  // Ediciones sin guardar sobre los datos del servidor; se manda solo lo que cambió.
  const [edits, setEdits] = useState<Partial<StudentForm>>({})
  const [errors, setErrors] = useState<FormErrors>({})
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [avatarBusy, setAvatarBusy] = useState<number | null>(null)
  const [purchaseBusy, setPurchaseBusy] = useState<number | null>(null)
  const [pwOpen, setPwOpen] = useState(false)
  const [pwText, setPwText] = useState('')
  const [pwBusy, setPwBusy] = useState(false)
  const [newPassword, setNewPassword] = useState<string | null>(null)

  const avatarActions = useAvatarActions(() => {
    avatarsQ.reload()
    onChanged()
  })

  const base = student ? toForm(student) : null
  const form = base ? { ...base, ...edits } : null
  const changed = base ? (Object.keys(edits) as (keyof StudentForm)[]).filter((k) => edits[k] !== base[k]) : []

  const set = <K extends keyof StudentForm>(key: K, value: StudentForm[K]) => {
    setEdits((e) => ({ ...e, [key]: value }))
    setErrors((prev) => {
      if (!prev[key]) return prev
      const next = { ...prev }
      delete next[key]
      return next
    })
  }

  // Vuelve a leer al alumno (p. ej. sus monedas tras deshacer una compra).
  const refresh = () => {
    fetchStudent(studentId, student ?? undefined).then(setLatest, (e: unknown) => toast.error(errorMessage(e)))
  }

  const requestClose = async () => {
    if (changed.length > 0) {
      const ok = await confirm({
        title: '¿Descartar los cambios?',
        message: 'Hay cambios sin guardar en este alumno.',
        confirmLabel: 'Descartar',
        danger: true,
      })
      if (!ok) return
    }
    onClose()
  }

  const save = async (e: FormEvent) => {
    e.preventDefault()
    if (!student || !form || changed.length === 0) return
    const clean = cleanForm(form)
    const all = validateStudent(clean)
    // Solo cuentan los errores de lo que se editó: un dato viejo raro no bloquea lo demás.
    const errs: FormErrors = {}
    for (const k of changed) if (all[k]) errs[k] = all[k]
    setErrors(errs)
    if (Object.keys(errs).length > 0) {
      requestAnimationFrame(() => formRef.current?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus())
      return
    }
    const patch: Record<string, unknown> = {}
    for (const k of changed) patch[k] = clean[k]
    setSaving(true)
    try {
      const res = await api.admin.updateStudent(student.id, patch as StudentInput)
      setLatest(res.student)
      // Se descarta lo que ya se guardó; si se siguió escribiendo mientras tanto, eso se queda.
      setEdits((cur) => {
        const next = { ...cur }
        for (const k of changed) if (next[k] === form[k]) delete next[k]
        return next
      })
      done('Cambios guardados.', { auditId: res.auditId }, () => {
        refresh()
        onChanged()
      })
      onChanged()
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setSaving(false)
    }
  }

  const remove = async () => {
    if (!student) return
    const ok = await confirm({
      title: `¿Eliminar a ${student.name}?`,
      message: 'También se borran sus compras. Puedes deshacerlo desde el aviso o en Actividad.',
      confirmLabel: 'Eliminar',
      danger: true,
    })
    if (!ok) return
    setDeleting(true)
    try {
      const res = await api.admin.deleteStudent(student.id)
      done(`Se eliminó a ${student.name}.`, { auditId: res.auditId }, onChanged)
      onChanged()
      onClose()
    } catch (e) {
      toast.error(errorMessage(e))
      setDeleting(false)
    }
  }

  const resetPassword = async (typed?: string) => {
    if (!student) return
    const ok = await confirm({
      title: 'Restablecer contraseña',
      message: `${student.name} ya no podrá entrar con su contraseña actual.`,
      confirmLabel: typed ? 'Guardar contraseña' : 'Generar contraseña',
    })
    if (!ok) return
    setPwBusy(true)
    try {
      const res = await api.admin.setStudentPassword(student.id, typed)
      setNewPassword(res.password || typed || null)
      setPwOpen(false)
      setPwText('')
      done('Contraseña restablecida.', { auditId: res.auditId }, () => setNewPassword(null))
    } catch (e) {
      toast.error(errorMessage(e))
    } finally {
      setPwBusy(false)
    }
  }

  const avatars = avatarsQ.data?.avatars ?? []
  const avatarUrl = (level: number) => avatars.find((a) => a.level === level)?.url ?? ''

  const changeAvatar = async (level: number, url: string) => {
    if (!student) return
    setAvatarBusy(level)
    try {
      if (url) {
        const avatar = await avatarActions.save(student.code, level, url)
        if (avatar) {
          avatarsQ.setData((d) => d && { avatars: [...(d.avatars ?? []).filter((a) => a.level !== level), avatar] })
          onChanged()
        }
      } else if (await avatarActions.remove(student.code, level)) {
        avatarsQ.setData((d) => d && { avatars: (d.avatars ?? []).filter((a) => a.level !== level) })
        onChanged()
      }
    } finally {
      setAvatarBusy(null)
    }
  }

  const togglePurchase = async (p: AdminPurchase) => {
    setPurchaseBusy(p.id)
    try {
      const res = p.revertedAt ? await api.admin.restorePurchase(p.id) : await api.admin.revertPurchase(p.id)
      purchasesQ.setData((d) => d && { purchases: (d.purchases ?? []).map((x) => (x.id === p.id ? res.purchase : x)) })
      refresh()
      onChanged()
      done(
        p.revertedAt
          ? `Se rehízo la compra de ${p.badgeName}.`
          : `Se deshizo la compra de ${p.badgeName}; se devolvieron ${formatNumber(p.totalCost)} monedas.`,
        { auditId: res.auditId },
        () => {
          purchasesQ.reload()
          refresh()
          onChanged()
        },
      )
    } catch (e) {
      toast.error(errorMessage(e))
    } finally {
      setPurchaseBusy(null)
    }
  }

  const courses = sortCourses(coursesQ.data?.courses ?? [])
  const courseOptions =
    courses.length > 0
      ? courses.map((c) => ({ value: c.id, label: c.active ? c.name : `${c.name} (oculto)` }))
      : student
        ? [{ value: student.courseId, label: student.courseName }]
        : []
  const levels = [...(levelsQ.data?.levels ?? [])].sort((a, b) => a.level - b.level)
  const purchases = purchasesQ.data?.purchases ?? []
  const busy = saving || deleting

  return (
    <Drawer
      open
      onClose={() => void requestClose()}
      title={student?.name ?? 'Alumno'}
      description={student ? `${student.code} · ${student.courseName}` : undefined}
      footer={
        student && (
          <>
            <Button variant="danger" icon={Trash2} className="mr-auto" onClick={remove} loading={deleting} disabled={saving}>
              Eliminar
            </Button>
            <Button variant="ghost" onClick={() => void requestClose()} disabled={busy}>
              {changed.length > 0 ? 'Cancelar' : 'Cerrar'}
            </Button>
            <Button type="submit" form={formId} loading={saving} disabled={changed.length === 0 || deleting}>
              Guardar cambios
            </Button>
          </>
        )
      }
    >
      {!student || !form ? (
        fetched.error ? (
          <ErrorBanner message={errorMessage(fetched.error)} onRetry={fetched.reload} />
        ) : fetched.loading ? (
          <LoadingBlock label="Cargando alumno…" />
        ) : (
          <ErrorBanner message="Este alumno ya no existe; tal vez se eliminó." />
        )
      ) : (
        <div className="space-y-8">
          <div className="flex items-center gap-4">
            <StudentAvatar name={student.name} url={avatarUrl(student.level) || undefined} size="lg" />
            <div className="min-w-0 space-y-1.5">
              <div className="flex flex-wrap gap-1.5">
                <Pill tone="purple">Nivel {student.level}</Pill>
                <Pill tone="orange">
                  {formatNumber(student.monedas)} {pl(student.monedas, 'moneda', 'monedas')}
                </Pill>
                <Pill tone="blue">
                  {formatNumber(student.badges)} {pl(student.badges, 'insignia', 'insignias')}
                </Pill>
                {student.active ? <Pill tone="green">Activo</Pill> : <Pill tone="gray">Inactivo</Pill>}
              </div>
              <p className="text-xs text-gray-500">Último acceso: {formatDateTime(student.lastLoginAt)}</p>
            </div>
          </div>

          <form id={formId} ref={formRef} onSubmit={save} noValidate className="space-y-8">
            <Section title="Datos">
              <div className="grid gap-4 sm:grid-cols-2">
                <Select
                  label="Curso"
                  value={form.courseId}
                  onChange={(v) => set('courseId', Number(v))}
                  options={courseOptions}
                  error={errors.courseId}
                />
                <TextField
                  label="Matrícula (ID)"
                  value={form.code}
                  onChange={(e) => set('code', e.target.value)}
                  error={errors.code}
                  autoComplete="off"
                  spellCheck={false}
                />
                <TextField
                  label="Nombre completo"
                  value={form.name}
                  onChange={(e) => set('name', e.target.value)}
                  error={errors.name}
                  autoComplete="off"
                  className="sm:col-span-2"
                />
                {/* La key lo reinicia cuando el servidor devuelve otra versión del alumno. */}
                <SexoField
                  key={student.updatedAt}
                  value={form.sexo}
                  onChange={(v) => set('sexo', v)}
                  error={errors.sexo}
                />
                <TextField
                  label="Correo"
                  type="email"
                  value={form.email}
                  onChange={(e) => set('email', e.target.value)}
                  error={errors.email}
                  autoComplete="off"
                />
              </div>
              <Toggle
                className="mt-5"
                label="Activo"
                description="Si lo desactivas, ya no puede iniciar sesión."
                checked={form.active}
                onChange={(v) => set('active', v)}
              />
            </Section>

            <Section title="Contadores">
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
                {COUNTERS.map((c) => (
                  <NumberField
                    key={c.key}
                    label={c.label}
                    value={form[c.key]}
                    onChange={(v) => set(c.key, v)}
                    min={0}
                    step={1}
                    error={errors[c.key]}
                  />
                ))}
              </div>
              {levelsQ.error ? (
                <p className="mt-3 text-xs text-gray-500">No se pudieron cargar los niveles para la vista previa.</p>
              ) : (
                <LevelPreview exp={form.exp} levels={levels} savedLevel={student.level} className="mt-3" />
              )}
            </Section>

            <Section title="Comentarios">
              <TextArea
                label="Comentarios"
                hideLabel
                hint="El alumno los ve en su perfil."
                rows={4}
                maxLength={MAX_COMMENTS}
                value={form.comments}
                onChange={(e) => set('comments', e.target.value)}
              />
            </Section>

            {changed.length > 0 && (
              <p role="status" className="rounded-xl bg-orange-50 px-4 py-2 text-sm text-orange-800">
                Tienes {changed.length} {pl(changed.length, 'cambio', 'cambios')} sin guardar.
              </p>
            )}
          </form>

          <Section
            title="Avatares"
            description={`Uno por nivel. Se guardan por matrícula (${student.code}) y se aplican al momento.`}
          >
            {levelsQ.error || avatarsQ.error ? (
              <ErrorBanner
                message={errorMessage(levelsQ.error ?? avatarsQ.error)}
                onRetry={() => {
                  levelsQ.reload()
                  avatarsQ.reload()
                }}
              />
            ) : levelsQ.loading && levels.length === 0 ? (
              <LoadingBlock label="Cargando avatares…" />
            ) : (
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
                {levels.map((l) => (
                  <ImageUpload
                    key={l.level}
                    label={l.level === student.level ? `Nivel ${l.level} (actual)` : `Nivel ${l.level}`}
                    hint=""
                    value={avatarUrl(l.level)}
                    onChange={(url) => void changeAvatar(l.level, url)}
                    disabled={avatarBusy !== null || avatarsQ.loading}
                    previewClassName="h-28 w-full"
                  />
                ))}
              </div>
            )}
          </Section>

          <Section title="Restablecer contraseña" description="La contraseña nueva se muestra una sola vez.">
            {newPassword && (
              <div className="mb-3">
                <PasswordReveal password={newPassword} title={`Contraseña nueva de ${student.name}`} />
              </div>
            )}
            <div className="flex flex-wrap gap-2">
              <Button
                variant="secondary"
                size="sm"
                icon={KeyRound}
                loading={pwBusy && !pwOpen}
                disabled={pwBusy}
                onClick={() => void resetPassword()}
              >
                Generar una nueva
              </Button>
              <Button
                variant="ghost"
                size="sm"
                icon={PencilLine}
                aria-expanded={pwOpen}
                disabled={pwBusy}
                onClick={() => setPwOpen((o) => !o)}
              >
                Escribir una
              </Button>
            </div>
            {pwOpen && (
              <form
                className="mt-3 flex items-end gap-2"
                onSubmit={(e) => {
                  e.preventDefault()
                  if (pwText.trim()) void resetPassword(pwText.trim())
                }}
              >
                <TextField
                  label="Contraseña nueva"
                  value={pwText}
                  onChange={(e) => setPwText(e.target.value)}
                  autoComplete="off"
                  spellCheck={false}
                  className="flex-1"
                />
                <Button type="submit" size="sm" className="mb-0.5" loading={pwBusy} disabled={!pwText.trim()}>
                  Guardar
                </Button>
              </form>
            )}
          </Section>

          <Section title="Compras recientes" description="Las últimas 20. Deshacer devuelve las monedas y el inventario.">
            {purchasesQ.error ? (
              <ErrorBanner message={errorMessage(purchasesQ.error)} onRetry={purchasesQ.reload} />
            ) : purchasesQ.loading && !purchasesQ.data ? (
              <LoadingBlock label="Cargando compras…" />
            ) : purchases.length === 0 ? (
              <p className="rounded-xl bg-gray-50 px-4 py-3 text-sm text-gray-500">Todavía no ha comprado nada.</p>
            ) : (
              <ul className="divide-y divide-gray-100 rounded-xl border border-gray-100">
                {purchases.map((p) => (
                  <li key={p.id} className="flex flex-wrap items-center gap-3 px-3 py-2.5">
                    <div className="min-w-0 flex-1">
                      <p
                        className={cx(
                          'text-sm font-medium',
                          p.revertedAt ? 'text-gray-400 line-through' : 'text-gray-800',
                        )}
                      >
                        {p.quantity} × {p.badgeName}
                      </p>
                      <p className="text-xs text-gray-500">
                        {formatNumber(p.totalCost)} monedas · {formatDateTime(p.createdAt)}
                      </p>
                    </div>
                    {p.revertedAt && <Pill tone="gray">Deshecha</Pill>}
                    <Button
                      size="sm"
                      variant="ghost"
                      icon={p.revertedAt ? Redo2 : Undo2}
                      loading={purchaseBusy === p.id}
                      disabled={purchaseBusy !== null}
                      onClick={() => void togglePurchase(p)}
                      aria-label={`${p.revertedAt ? 'Rehacer' : 'Deshacer'} la compra de ${p.badgeName} del ${formatDateTime(p.createdAt)}`}
                    >
                      {p.revertedAt ? 'Rehacer' : 'Deshacer'}
                    </Button>
                  </li>
                ))}
              </ul>
            )}
          </Section>
        </div>
      )}
    </Drawer>
  )
}
