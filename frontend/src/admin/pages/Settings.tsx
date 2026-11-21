import { useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { useLocation } from 'react-router'
import {
  ExternalLink,
  ImagePlay,
  KeyRound,
  Lock,
  Mail,
  RotateCcw,
  Save,
  Sheet,
  Timer,
  ToggleRight,
  Trash,
  type LucideIcon,
} from 'lucide-react'
import { api, errorMessage } from '../../lib/api'
import type { AdminSettings, SecretStatus, SettingsPatch } from '../../lib/types'
import {
  Button,
  Card,
  ImageUpload,
  NumberField,
  PageHeader,
  TextField,
  Toggle,
  cx,
  focusRing,
  useAsync,
  useConfirm,
  useToast,
  useUndo,
} from '../ui'
import { ChipsInput } from './system/ChipsInput'
import { parseSpreadsheetId, sheetUrl, spreadsheetIdError } from './system/google'
import { LoadError, PageLoading } from './system/PageState'
import { SecretBadge } from './system/SecretBadge'
import { ServiceAccountPicker } from './system/ServiceAccountPicker'
import { useDraft } from './system/useDraft'

// Guarda un parche y avisa; resuelve false si falló (el error ya se mostró).
type Save = (patch: SettingsPatch, message: string) => Promise<boolean>

interface SectionProps {
  settings: AdminSettings
  save: Save
}

function pick<K extends keyof AdminSettings>(s: AdminSettings, keys: readonly K[]): Pick<AdminSettings, K> {
  return Object.fromEntries(keys.map((k) => [k, s[k]])) as Pick<AdminSettings, K>
}

const EMAIL_RE = /^[^\s@<>(),;:"]+@[^\s@<>(),;:"]+\.[^\s@<>(),;:"]{2,}$/

function emailError(v: string): string | undefined {
  return EMAIL_RE.test(v) ? undefined : `«${v}» no parece un correo válido.`
}

// Resend acepta "correo@dominio" o "Nombre <correo@dominio>".
function senderError(v: string): string | undefined {
  const t = v.trim()
  if (!t) return 'Escribe el correo del remitente.'
  const named = /^[^<>]*<([^<>]+)>$/.exec(t)
  return EMAIL_RE.test(named ? named[1].trim() : t)
    ? undefined
    : 'Usa un correo (avisos@dominio.mx) o Nombre <avisos@dominio.mx>.'
}

function domainError(v: string): string | undefined {
  if (!v) return 'Escribe el dominio.'
  return /^(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,}$/.test(v)
    ? undefined
    : 'Escribe sólo el dominio, por ejemplo up.edu.mx.'
}

// El servidor revisa el patrón con RE2 (Go): sin lookahead, lookbehind ni referencias.
function compilePattern(p: string): { regex?: RegExp; error?: string } {
  if (!p.trim()) return { error: 'Escribe el patrón.' }
  if (/\(\?<?[=!]|\\[1-9]/.test(p)) {
    return { error: 'Usa una expresión sencilla: el servidor no acepta lookahead, lookbehind ni referencias como \\1.' }
  }
  try {
    return { regex: new RegExp(p) }
  } catch {
    return { error: 'La expresión regular no es válida.' }
  }
}

function textError(v: string): string | undefined {
  return v.trim() ? undefined : 'Este texto no puede quedar vacío.'
}

// Formas que pueden salir del generador de contraseñas (2 letras y 2 números, revueltos).
const GENERATED_SAMPLES = ['AB23', 'A2B3', 'A23B', '2AB3', '2A3B', '23AB']
const DEFAULT_GIF = '/media/legacy/gifs/levelup.gif'

interface SettingsSectionProps {
  id: string
  icon: LucideIcon
  title: string
  description: string
  children: ReactNode
}

function SettingsSection({ id, icon: Icon, title, description, children }: SettingsSectionProps) {
  return (
    <section id={id} aria-labelledby={`${id}-title`} className="scroll-mt-24">
      <Card>
        <div className="mb-5 flex items-start gap-3">
          <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-orange-100 text-orange-600">
            <Icon aria-hidden="true" className="h-5 w-5" />
          </span>
          <div className="min-w-0">
            <h2 id={`${id}-title`} className="text-lg font-semibold text-gray-800">
              {title}
            </h2>
            <p className="mt-0.5 text-sm text-gray-500">{description}</p>
          </div>
        </div>
        {children}
      </Card>
    </section>
  )
}

interface SectionFormProps {
  dirty: boolean
  invalid?: boolean
  onSubmit: () => Promise<unknown>
  onReset: () => void
  children: ReactNode
}

// Formulario de una sección: guarda sólo lo que cambió y permite descartar.
function SectionForm({ dirty, invalid = false, onSubmit, onReset, children }: SectionFormProps) {
  const [saving, setSaving] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (!dirty || invalid || saving) return
    setSaving(true)
    try {
      await onSubmit()
    } finally {
      setSaving(false)
    }
  }

  return (
    <form noValidate onSubmit={submit}>
      <div className="space-y-4">{children}</div>
      <div className="mt-6 flex flex-wrap items-center justify-end gap-2 border-t border-gray-100 pt-4">
        {dirty && <span className="mr-auto text-xs font-medium text-orange-600">Hay cambios sin guardar</span>}
        <Button variant="ghost" onClick={onReset} disabled={!dirty || saving}>
          Descartar
        </Button>
        <Button type="submit" icon={Save} loading={saving} disabled={!dirty || invalid}>
          Guardar cambios
        </Button>
      </div>
    </form>
  )
}

const FEATURES = [
  ['registrationEnabled', 'Registro de alumnos', 'Los alumnos pueden crear su cuenta desde el portal.'],
  [
    'passwordResetEnabled',
    'Recuperación de contraseña',
    'Los alumnos pueden pedir un código por correo para cambiar su contraseña.',
  ],
  ['storeEnabled', 'Tienda de insignias', 'Los alumnos pueden comprar insignias con sus monedas.'],
  ['leaderboardEnabled', 'Leaderboard', 'Muestra la tabla de posiciones de cada curso.'],
  ['levelUpAnimationEnabled', 'Animación de nivel', 'Muestra el GIF cuando un alumno sube de nivel.'],
  ['purchaseEmailEnabled', 'Correo de compras', 'Manda un correo de confirmación después de cada compra.'],
] as const

const FEATURE_KEYS = FEATURES.map(([key]) => key)

function FeaturesSection({ settings, save }: SectionProps) {
  const d = useDraft(pick(settings, FEATURE_KEYS))
  return (
    <SettingsSection
      id="funciones"
      icon={ToggleRight}
      title="Funciones del portal"
      description="Prende o apaga partes del portal de alumnos."
    >
      <SectionForm dirty={d.dirty} onReset={d.reset} onSubmit={() => save(d.patch, 'Funciones guardadas.')}>
        <div className="grid gap-3 sm:grid-cols-2">
          {FEATURES.map(([key, label, description]) => (
            <div key={key} className="rounded-xl border border-gray-100 p-3">
              <Toggle checked={d.draft[key]} onChange={(v) => d.set(key, v)} label={label} description={description} />
            </div>
          ))}
        </div>
      </SectionForm>
    </SettingsSection>
  )
}

function ResendKey({ status, save }: { status: SecretStatus; save: Save }) {
  const confirm = useConfirm()
  const [editing, setEditing] = useState(false)
  const [value, setValue] = useState('')
  const [busy, setBusy] = useState(false)
  // Al cerrar el formulario el foco regresa al botón que lo abrió.
  const [refocus, setRefocus] = useState(false)
  const key = value.trim()
  const error = /\s/.test(key) ? 'La clave no lleva espacios.' : undefined

  const close = () => {
    setEditing(false)
    setValue('')
    setRefocus(true)
  }

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (!key || error || busy) return
    setBusy(true)
    const ok = await save({ resendApiKey: key }, 'API key de Resend guardada.')
    setBusy(false)
    if (ok) close()
  }

  const clear = async () => {
    const ok = await confirm({
      title: '¿Quitar la API key de Resend?',
      message:
        'Se borra la clave guardada en Ajustes. Si el servidor tiene RESEND_API_KEY se usará esa; si no, los correos dejarán de enviarse.',
      confirmLabel: 'Quitar clave',
      danger: true,
    })
    if (!ok) return
    setBusy(true)
    await save({ resendApiKey: null }, 'Se quitó la API key de Resend.')
    setBusy(false)
  }

  return (
    <div className="rounded-xl border border-gray-100 bg-gray-50 p-4">
      <div className="flex flex-wrap items-center gap-2">
        <KeyRound aria-hidden="true" className="h-4 w-4 text-gray-500" />
        <h3 className="text-sm font-semibold text-gray-700">API key de Resend</h3>
        <SecretBadge status={status} />
      </div>
      <p className="mt-1 text-xs text-gray-500">
        Por seguridad la clave nunca se muestra; sólo puedes reemplazarla o quitarla.
      </p>
      {editing ? (
        <form noValidate onSubmit={submit} className="mt-3 flex flex-col gap-2 sm:flex-row sm:items-start">
          <TextField
            label="Nueva API key de Resend"
            hideLabel
            type="password"
            autoComplete="new-password"
            spellCheck={false}
            placeholder="re_…"
            autoFocus
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Escape' && !busy) {
                e.stopPropagation()
                close()
              }
            }}
            error={error}
            className="min-w-0 flex-1"
          />
          <div className="flex gap-2">
            <Button type="submit" loading={busy} disabled={!key || Boolean(error)}>
              Guardar clave
            </Button>
            <Button variant="ghost" onClick={close} disabled={busy}>
              Cancelar
            </Button>
          </div>
        </form>
      ) : (
        <div className="mt-3 flex flex-wrap gap-2">
          <Button
            variant="secondary"
            size="sm"
            icon={KeyRound}
            autoFocus={refocus}
            disabled={busy}
            onClick={() => setEditing(true)}
          >
            {status.configured ? 'Reemplazar clave' : 'Agregar clave'}
          </Button>
          {status.source === 'db' && (
            <Button variant="ghost" size="sm" icon={Trash} loading={busy} onClick={() => void clear()}>
              Quitar clave
            </Button>
          )}
        </div>
      )}
    </div>
  )
}

const EMAIL_KEYS = ['emailFrom', 'emailCc', 'studentEmailDomain'] as const

function EmailSection({ settings, save }: SectionProps) {
  const d = useDraft(pick(settings, EMAIL_KEYS))
  const fromError = senderError(d.draft.emailFrom)
  const domError = domainError(d.draft.studentEmailDomain)

  const submit = () => {
    const patch: SettingsPatch = { ...d.patch }
    if (patch.emailFrom !== undefined) patch.emailFrom = patch.emailFrom.trim()
    return save(patch, 'Ajustes de correo guardados.')
  }

  return (
    <SettingsSection
      id="correo"
      icon={Mail}
      title="Correo"
      description="Remitente y copias de los correos del portal: códigos de recuperación y confirmaciones de compra."
    >
      <SectionForm dirty={d.dirty} invalid={Boolean(fromError || domError)} onReset={d.reset} onSubmit={submit}>
        <TextField
          label="Remitente"
          inputMode="email"
          autoComplete="off"
          spellCheck={false}
          value={d.draft.emailFrom}
          onChange={(e) => d.set('emailFrom', e.target.value)}
          onBlur={() => d.set('emailFrom', d.draft.emailFrom.trim())}
          error={fromError}
          hint="Debe ser un remitente verificado en Resend. Puede llevar nombre: Gamificación <avisos@dominio.mx>."
        />
        <ChipsInput
          label="Copias (CC)"
          values={d.draft.emailCc ?? []}
          onChange={(v) => d.set('emailCc', v)}
          validate={emailError}
          placeholder="correo@up.edu.mx"
          hint="Reciben copia de cada código de recuperación y de cada compra. Escribe un correo y presiona Enter."
        />
        <TextField
          label="Dominio de correo de los alumnos"
          autoComplete="off"
          spellCheck={false}
          value={d.draft.studentEmailDomain}
          onChange={(e) =>
            d.set('studentEmailDomain', e.target.value.replace(/^@+/, '').replace(/\s/g, '').toLowerCase())
          }
          error={domError}
          hint={`A los alumnos que se registran se les asigna matrícula@${d.draft.studentEmailDomain || 'dominio'}.`}
        />
      </SectionForm>
      <div className="mt-6">
        <ResendKey status={settings.resendApiKey} save={save} />
      </div>
    </SettingsSection>
  )
}

const PASSWORD_KEYS = ['passwordPattern', 'passwordHintRegister', 'passwordHintReset', 'passwordError'] as const

function PasswordSection({ settings, save }: SectionProps) {
  const d = useDraft(pick(settings, PASSWORD_KEYS))
  const [sample, setSample] = useState('')
  const { regex, error: patternError } = compilePattern(d.draft.passwordPattern)
  const anchored = d.draft.passwordPattern.startsWith('^') && d.draft.passwordPattern.endsWith('$')
  const generatorOk = !regex || GENERATED_SAMPLES.some((s) => regex.test(s))
  const errors = {
    hintRegister: textError(d.draft.passwordHintRegister),
    hintReset: textError(d.draft.passwordHintReset),
    error: textError(d.draft.passwordError),
  }
  const invalid = Boolean(patternError || errors.hintRegister || errors.hintReset || errors.error)
  const tested = sample.trim()

  return (
    <SettingsSection
      id="contrasenas"
      icon={Lock}
      title="Contraseñas de alumnos"
      description="La regla que deben cumplir las contraseñas al registrarse o recuperarlas, y los textos que la explican."
    >
      <SectionForm
        dirty={d.dirty}
        invalid={invalid}
        onReset={d.reset}
        onSubmit={() => save(d.patch, 'Regla de contraseñas guardada.')}
      >
        <div className="grid gap-4 lg:grid-cols-2">
          <div className="space-y-2">
            <TextField
              label="Patrón (expresión regular)"
              autoComplete="off"
              spellCheck={false}
              autoCapitalize="none"
              style={{ fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' }}
              value={d.draft.passwordPattern}
              onChange={(e) => d.set('passwordPattern', e.target.value)}
              error={patternError}
              hint="Ejemplo: ^[A-Z0-9]{4}$ (4 letras mayúsculas o números)."
            />
            {regex && !anchored && (
              <p className="text-xs text-orange-700">
                Sugerencia: empieza con ^ y termina con $ para revisar la contraseña completa.
              </p>
            )}
            {!generatorOk && (
              <p className="text-xs text-orange-700">
                Ojo: las contraseñas que genera el panel (2 letras y 2 números, como AB23) no cumplen este patrón; al
                crear alumnos tendrás que escribir la contraseña de cada uno.
              </p>
            )}
          </div>
          <div className="rounded-xl border border-gray-100 bg-gray-50 p-4">
            <TextField
              label="Probar una contraseña"
              autoComplete="off"
              spellCheck={false}
              autoCapitalize="none"
              placeholder="AB12"
              value={sample}
              onChange={(e) => setSample(e.target.value)}
              onKeyDown={(e) => {
                // Probar no es guardar: Enter aquí no manda la sección.
                if (e.key === 'Enter') e.preventDefault()
              }}
            />
            <p aria-live="polite" className="mt-2 min-h-[1.25rem] text-sm">
              {!regex ? (
                <span className="text-gray-500">Corrige el patrón para probarlo.</span>
              ) : !tested ? (
                <span className="text-gray-500">Escribe una contraseña para ver si cumple.</span>
              ) : regex.test(tested) ? (
                <span className="font-medium text-green-700">✓ Cumple el patrón</span>
              ) : (
                <span className="font-medium text-red-700">✗ No cumple el patrón</span>
              )}
            </p>
          </div>
        </div>
        <TextField
          label="Texto de ayuda al registrarse"
          maxLength={200}
          value={d.draft.passwordHintRegister}
          onChange={(e) => d.set('passwordHintRegister', e.target.value)}
          error={errors.hintRegister}
          hint="Aparece en el campo de contraseña del registro. El portal de alumnos está en inglés."
        />
        <TextField
          label="Texto de ayuda al recuperar la contraseña"
          maxLength={200}
          value={d.draft.passwordHintReset}
          onChange={(e) => d.set('passwordHintReset', e.target.value)}
          error={errors.hintReset}
        />
        <TextField
          label="Mensaje cuando la contraseña no cumple"
          maxLength={200}
          value={d.draft.passwordError}
          onChange={(e) => d.set('passwordError', e.target.value)}
          error={errors.error}
        />
      </SectionForm>
    </SettingsSection>
  )
}

function ResetCodeSection({ settings, save }: SectionProps) {
  const d = useDraft(pick(settings, ['resetCodeTtlMinutes'] as const))
  const ttl = d.draft.resetCodeTtlMinutes
  const ttlError =
    Number.isInteger(ttl) && ttl >= 1 && ttl <= 120 ? undefined : 'Escribe un número entero entre 1 y 120.'
  return (
    <SettingsSection
      id="recuperacion"
      icon={Timer}
      title="Códigos de recuperación"
      description="Cuánto tiempo sirve el código de 6 dígitos que se manda por correo para cambiar la contraseña."
    >
      <SectionForm
        dirty={d.dirty}
        invalid={Boolean(ttlError)}
        onReset={d.reset}
        onSubmit={() => save(d.patch, 'Vigencia de los códigos guardada.')}
      >
        <NumberField
          label="Vigencia (minutos)"
          min={1}
          max={120}
          step={1}
          value={ttl}
          onChange={(v) => d.set('resetCodeTtlMinutes', v)}
          error={ttlError}
          hint="Entre 1 y 120 minutos."
          className="max-w-xs"
        />
      </SectionForm>
    </SettingsSection>
  )
}

function LevelUpSection({ settings, save }: SectionProps) {
  const d = useDraft(pick(settings, ['levelUpGifUrl'] as const))
  const url = d.draft.levelUpGifUrl
  const urlError = url && !url.startsWith('/media/') ? 'La imagen debe estar subida al portal.' : undefined
  return (
    <SettingsSection
      id="animacion"
      icon={ImagePlay}
      title="Animación de nivel"
      description="El GIF que ve un alumno cuando sube de nivel. Se prende o apaga en Funciones del portal."
    >
      <SectionForm
        dirty={d.dirty}
        invalid={Boolean(urlError)}
        onReset={d.reset}
        onSubmit={() => save(d.patch, 'Animación de nivel guardada.')}
      >
        <ImageUpload
          label="GIF de subida de nivel"
          value={url}
          onChange={(u) => d.set('levelUpGifUrl', u)}
          previewClassName="h-48 w-full sm:w-80"
          hint="GIF, PNG, JPG o WEBP de máximo 20 MB."
        />
        {urlError && <p className="text-xs text-red-600">{urlError}</p>}
        {!url && <p className="text-xs text-orange-700">Sin imagen no se muestra ninguna animación.</p>}
        {url !== DEFAULT_GIF && (
          <Button variant="ghost" size="sm" icon={RotateCcw} onClick={() => d.set('levelUpGifUrl', DEFAULT_GIF)}>
            Usar el GIF original
          </Button>
        )}
      </SectionForm>
    </SettingsSection>
  )
}

function GoogleSection({ settings, save }: SectionProps) {
  const confirm = useConfirm()
  const d = useDraft(pick(settings, ['spreadsheetId'] as const))
  const [clearing, setClearing] = useState(false)
  const id = parseSpreadsheetId(d.draft.spreadsheetId)
  const idError = spreadsheetIdError(id)
  const creds = settings.googleServiceAccount

  const clear = async () => {
    const ok = await confirm({
      title: '¿Quitar las credenciales de Google?',
      message:
        'Se borran las guardadas en Ajustes. Si el servidor tiene GOOGLE_SERVICE_ACCOUNT se usarán esas; si no, tendrás que subir el JSON cada vez que importes.',
      confirmLabel: 'Quitar credenciales',
      danger: true,
    })
    if (!ok) return
    setClearing(true)
    await save({ googleServiceAccount: null }, 'Se quitaron las credenciales de Google.')
    setClearing(false)
  }

  return (
    <SettingsSection
      id="google"
      icon={Sheet}
      title="Google Sheets"
      description="La hoja de la versión anterior y la cuenta de servicio con la que se lee al importar."
    >
      <SectionForm
        dirty={d.dirty}
        invalid={Boolean(idError)}
        onReset={d.reset}
        onSubmit={() => save({ spreadsheetId: id }, 'ID de la hoja guardado.')}
      >
        <TextField
          label="ID de la hoja"
          autoComplete="off"
          spellCheck={false}
          autoCapitalize="none"
          value={d.draft.spreadsheetId}
          onChange={(e) => d.set('spreadsheetId', e.target.value)}
          onBlur={() => d.set('spreadsheetId', parseSpreadsheetId(d.draft.spreadsheetId))}
          error={idError}
          hint="También puedes pegar el enlace completo de la hoja; se queda sólo el ID."
        />
        {!idError && (
          <a
            href={sheetUrl(id)}
            target="_blank"
            rel="noreferrer"
            className={cx(
              'inline-flex items-center gap-1.5 rounded-full text-sm font-medium text-purple-600 hover:underline',
              focusRing,
            )}
          >
            <ExternalLink aria-hidden="true" className="h-4 w-4" />
            Abrir la hoja<span className="sr-only"> (se abre en otra pestaña)</span>
          </a>
        )}
      </SectionForm>

      <div className="mt-6 rounded-xl border border-gray-100 bg-gray-50 p-4">
        <div className="flex flex-wrap items-center gap-2">
          <KeyRound aria-hidden="true" className="h-4 w-4 text-gray-500" />
          <h3 className="text-sm font-semibold text-gray-700">Credenciales (cuenta de servicio)</h3>
          <SecretBadge status={creds} word="Configuradas" />
        </div>
        {creds.clientEmail && (
          <p className="mt-2 text-sm text-gray-700">
            Cuenta: <span className="break-all font-mono text-xs">{creds.clientEmail}</span>
          </p>
        )}
        <p className="mt-1 text-xs text-gray-500">
          Comparte la hoja con {creds.clientEmail ? 'ese correo' : 'el correo de la cuenta de servicio (client_email)'}{' '}
          como lector para que se pueda importar. El JSON nunca se muestra.
        </p>
        <div className="mt-3 flex flex-wrap items-start gap-2">
          <ServiceAccountPicker
            label={creds.configured ? 'Reemplazar con otro JSON' : 'Subir JSON de la cuenta de servicio'}
            onLoad={async (sa) => {
              await save({ googleServiceAccount: sa.json }, `Credenciales de ${sa.clientEmail} guardadas.`)
            }}
          />
          {creds.source === 'db' && (
            <Button variant="ghost" size="sm" icon={Trash} loading={clearing} onClick={() => void clear()}>
              Quitar credenciales
            </Button>
          )}
        </div>
      </div>
    </SettingsSection>
  )
}

const SECTIONS = [
  ['funciones', 'Funciones'],
  ['correo', 'Correo'],
  ['contrasenas', 'Contraseñas'],
  ['recuperacion', 'Recuperación'],
  ['animacion', 'Animación'],
  ['google', 'Google Sheets'],
] as const

export default function Settings() {
  const toast = useToast()
  const undo = useUndo()
  const { hash } = useLocation()
  const {
    data: settings,
    error,
    loading,
    reload,
    setData,
  } = useAsync(() => api.admin.settings().then((r) => r.settings), [])
  const ready = settings !== undefined

  // Al llegar con #seccion (p. ej. desde Importar) se baja a esa sección cuando ya cargó.
  useEffect(() => {
    if (ready && hash) document.getElementById(hash.slice(1))?.scrollIntoView({ block: 'start' })
  }, [ready, hash])

  const save: Save = async (patch, message) => {
    try {
      const res = await api.admin.updateSettings(patch)
      setData(() => res.settings)
      // Si sólo cambió un secreto, la bitácora lo registra pero no se puede deshacer.
      const secretOnly = Object.keys(patch).every((k) => k === 'resendApiKey' || k === 'googleServiceAccount')
      if (secretOnly) {
        toast.success(`${message} Por seguridad este cambio no se puede deshacer.`)
      } else {
        toast.success(message, {
          undo: async () => {
            if (await undo({ auditId: res.auditId })) reload()
          },
        })
      }
      return true
    } catch (e) {
      toast.error(errorMessage(e))
      return false
    }
  }

  return (
    <>
      <PageHeader
        title="Ajustes"
        subtitle="Cada sección se guarda por separado y puedes deshacer el cambio desde el aviso."
      />
      {!settings ? (
        error ? (
          <LoadError message={errorMessage(error)} onRetry={reload} retrying={loading} />
        ) : (
          <PageLoading label="Cargando ajustes…" />
        )
      ) : (
        <>
          <nav aria-label="Secciones de ajustes" className="mb-4 flex flex-wrap gap-2">
            {SECTIONS.map(([id, label]) => (
              <a
                key={id}
                href={`#${id}`}
                className={cx(
                  'rounded-full border border-gray-200 bg-white px-3 py-1 text-sm font-medium text-gray-600 hover:border-orange-300 hover:text-orange-600',
                  focusRing,
                )}
              >
                {label}
              </a>
            ))}
          </nav>
          <div className="space-y-6">
            <FeaturesSection settings={settings} save={save} />
            <EmailSection settings={settings} save={save} />
            <PasswordSection settings={settings} save={save} />
            <ResetCodeSection settings={settings} save={save} />
            <LevelUpSection settings={settings} save={save} />
            <GoogleSection settings={settings} save={save} />
          </div>
        </>
      )}
    </>
  )
}
