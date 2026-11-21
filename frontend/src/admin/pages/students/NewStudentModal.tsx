import { useId, useRef, useState, type FormEvent } from 'react'
import { api, errorMessage } from '../../../lib/api'
import type { AdminCourse, AdminStudent } from '../../../lib/types'
import { Button, Modal, NumberField, Select, TextArea, TextField, Toggle, useToast } from '../../ui'
import { PasswordReveal, SexoField } from './parts'
import { useUndoToast } from './useUndoToast'
import { COUNTERS, MAX_COMMENTS, cleanForm, validateStudent, type FormErrors, type StudentForm } from './util'

export interface NewStudentModalProps {
  open: boolean
  courses: AdminCourse[]
  /** Curso que se propone (el del filtro actual). */
  defaultCourseId: number | null
  onClose: () => void
  onCreated: () => void
}

// Se monta al abrir, así cada alta empieza con el formulario limpio.
export default function NewStudentModal(props: NewStudentModalProps) {
  return props.open ? <NewStudentDialog {...props} /> : null
}

function blank(courseId: number | null): StudentForm {
  return {
    courseId: courseId ?? 0,
    code: '',
    name: '',
    sexo: 'Hombre',
    email: '',
    tareas: 0,
    monedas: 0,
    asistencias: 0,
    games: 0,
    exp: 0,
    comments: '',
    active: true,
  }
}

function NewStudentDialog({ courses, defaultCourseId, onClose, onCreated }: NewStudentModalProps) {
  const toast = useToast()
  const done = useUndoToast()
  const formId = useId()
  const formRef = useRef<HTMLFormElement>(null)
  const firstCourse = defaultCourseId ?? (courses.length === 1 ? courses[0].id : null)
  const [form, setForm] = useState(() => blank(firstCourse))
  const [password, setPassword] = useState('')
  const [errors, setErrors] = useState<FormErrors>({})
  const [saving, setSaving] = useState(false)
  const [created, setCreated] = useState<{ student: AdminStudent; password: string } | null>(null)

  const set = <K extends keyof StudentForm>(key: K, value: StudentForm[K]) => {
    setForm((f) => ({ ...f, [key]: value }))
    setErrors((prev) => {
      if (!prev[key]) return prev
      const next = { ...prev }
      delete next[key]
      return next
    })
  }

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    const clean = cleanForm(form)
    const errs = validateStudent(clean)
    setErrors(errs)
    if (Object.keys(errs).length > 0) {
      // Lleva el foco al primer campo con error para que se lea el mensaje.
      requestAnimationFrame(() => formRef.current?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus())
      return
    }
    const pw = password.trim()
    // Sin correo no se manda el campo, para que el servidor ponga el suyo si tiene uno.
    const { email, ...rest } = clean
    setSaving(true)
    try {
      const res = await api.admin.createStudent({ ...rest, ...(email ? { email } : {}), ...(pw ? { password: pw } : {}) })
      setCreated({ student: res.student, password: res.password ?? pw })
      done(`Se creó a ${res.student.name}.`, { auditId: res.auditId }, onCreated)
      onCreated()
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setSaving(false)
    }
  }

  // Cada vista lleva su key: al cambiar se vuelve a montar el diálogo y el foco entra de nuevo.
  if (created) {
    return (
      <Modal
        key="creado"
        open
        onClose={onClose}
        title="Alumno creado"
        footer={
          <>
            <Button
              variant="ghost"
              onClick={() => {
                setForm(blank(created.student.courseId))
                setPassword('')
                setCreated(null)
              }}
            >
              Crear otro
            </Button>
            <Button onClick={onClose} data-autofocus>
              Listo
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <p className="text-sm text-gray-600">
            <span className="font-semibold text-gray-800">{created.student.name}</span> ({created.student.code}) quedó
            registrado en {created.student.courseName}.
          </p>
          {created.password ? (
            <PasswordReveal password={created.password} title="Contraseña para entrar al portal" />
          ) : (
            <p className="text-sm text-gray-500">Usa la contraseña que escribiste.</p>
          )}
        </div>
      </Modal>
    )
  }

  return (
    <Modal
      key="formulario"
      open
      onClose={onClose}
      title="Nuevo alumno"
      size="lg"
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={saving}>
            Cancelar
          </Button>
          <Button type="submit" form={formId} loading={saving}>
            Crear alumno
          </Button>
        </>
      }
    >
      <form id={formId} ref={formRef} onSubmit={submit} noValidate className="space-y-5">
        <div className="grid gap-4 sm:grid-cols-2">
          <Select
            label="Curso"
            value={form.courseId || ''}
            onChange={(v) => set('courseId', Number(v))}
            options={courses.map((c) => ({ value: c.id, label: c.active ? c.name : `${c.name} (oculto)` }))}
            placeholder="Elige un curso"
            error={errors.courseId}
            required
          />
          <TextField
            label="Matrícula (ID)"
            value={form.code}
            onChange={(e) => set('code', e.target.value)}
            error={errors.code}
            autoComplete="off"
            spellCheck={false}
            required
            data-autofocus
          />
          <TextField
            label="Nombre completo"
            value={form.name}
            onChange={(e) => set('name', e.target.value)}
            error={errors.name}
            autoComplete="off"
            required
            className="sm:col-span-2"
          />
          <SexoField value={form.sexo} onChange={(v) => set('sexo', v)} error={errors.sexo} />
          <TextField
            label="Correo"
            type="email"
            value={form.email}
            onChange={(e) => set('email', e.target.value)}
            error={errors.email}
            hint="Opcional."
            autoComplete="off"
          />
          <TextField
            label="Contraseña"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            hint="Déjala vacía para generar una."
            autoComplete="off"
            spellCheck={false}
          />
        </div>

        <fieldset>
          <legend className="mb-2 text-sm font-medium text-gray-700">Contadores iniciales</legend>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-5">
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
        </fieldset>

        <TextArea
          label="Comentarios"
          hint="El alumno los ve en su perfil."
          value={form.comments}
          onChange={(e) => set('comments', e.target.value)}
          maxLength={MAX_COMMENTS}
        />

        <Toggle
          label="Activo"
          description="Puede iniciar sesión en el portal."
          checked={form.active}
          onChange={(v) => set('active', v)}
        />
      </form>
    </Modal>
  )
}
