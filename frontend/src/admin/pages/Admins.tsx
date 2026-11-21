import { useId, useState, type FormEvent } from 'react'
import { KeyRound, Plus, ShieldCheck, Trash } from 'lucide-react'
import { api, errorMessage } from '../../lib/api'
import type { AdminAccount } from '../../lib/types'
import {
  Button,
  DataTable,
  EmptyState,
  IconButton,
  Modal,
  PageHeader,
  Pill,
  TextField,
  Toggle,
  useAdmin,
  useAsync,
  useConfirm,
  useToast,
  useUndo,
  type Column,
} from '../ui'
import { formatDateTime, formatRelative } from './system/format'
import { LoadError } from './system/PageState'
import { useNow } from './system/useNow'

const MIN_PASSWORD = 8
// bcrypt no acepta contraseñas de más de 72 bytes.
const MAX_PASSWORD_BYTES = 72

function usernameError(username: string): string | undefined {
  const u = username.trim()
  if (!u) return 'Escribe un nombre de usuario.'
  if (/\s/.test(u)) return 'El usuario no puede llevar espacios.'
  if (u.length > 64) return 'Máximo 64 caracteres.'
  return undefined
}

function passwordError(password: string): string | undefined {
  if (password.length < MIN_PASSWORD) return `Usa al menos ${MIN_PASSWORD} caracteres.`
  if (new TextEncoder().encode(password).length > MAX_PASSWORD_BYTES) return 'Máximo 72 caracteres.'
  return undefined
}

type Dialog = { mode: 'create' } | { mode: 'password'; account: AdminAccount }

interface FormModalProps {
  dialog: Dialog
  onClose: () => void
  onSaved: (admin: AdminAccount, auditId: number) => void
}

// Alta de administrador o cambio de contraseña; las dos piden confirmar la contraseña.
function AdminFormModal({ dialog, onClose, onSaved }: FormModalProps) {
  const formId = useId()
  const creating = dialog.mode === 'create'
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [repeat, setRepeat] = useState('')
  // Los errores aparecen hasta el primer intento de guardar.
  const [tried, setTried] = useState(false)
  const [busy, setBusy] = useState(false)
  const [serverError, setServerError] = useState('')

  const errors = {
    username: creating ? usernameError(username) : undefined,
    password: passwordError(password),
    repeat: repeat !== password ? 'Las contraseñas no coinciden.' : undefined,
  }
  const valid = !errors.username && !errors.password && !errors.repeat

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setTried(true)
    if (busy) return
    if (!valid) {
      // Lleva el foco al primer campo con error para que se lea su mensaje.
      const first = errors.username ? 'user' : errors.password ? 'pass' : 'repeat'
      document.getElementById(`${formId}-${first}`)?.focus()
      return
    }
    setBusy(true)
    setServerError('')
    try {
      const res =
        dialog.mode === 'create'
          ? await api.admin.createAdmin({ username: username.trim(), password })
          : await api.admin.updateAdmin(dialog.account.id, { password })
      onSaved(res.admin, res.auditId)
    } catch (err) {
      setServerError(errorMessage(err))
      setBusy(false)
    }
  }

  return (
    <Modal
      open
      onClose={busy ? () => {} : onClose}
      title={creating ? 'Nuevo administrador' : 'Cambiar contraseña'}
      description={
        creating
          ? 'Podrá entrar al panel con este usuario y contraseña.'
          : `Nueva contraseña para ${dialog.account.username}.`
      }
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            Cancelar
          </Button>
          <Button type="submit" form={formId} loading={busy}>
            {creating ? 'Crear administrador' : 'Guardar contraseña'}
          </Button>
        </>
      }
    >
      <form id={formId} noValidate onSubmit={submit} className="space-y-4">
        {creating && (
          <TextField
            id={`${formId}-user`}
            label="Usuario"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoComplete="off"
            autoCapitalize="none"
            spellCheck={false}
            maxLength={64}
            required
            data-autofocus
            error={tried ? errors.username : undefined}
          />
        )}
        <TextField
          id={`${formId}-pass`}
          label="Contraseña"
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          autoComplete="new-password"
          required
          data-autofocus={creating ? undefined : true}
          hint={`Mínimo ${MIN_PASSWORD} caracteres.`}
          error={tried ? errors.password : undefined}
        />
        <TextField
          id={`${formId}-repeat`}
          label="Confirmar contraseña"
          type="password"
          value={repeat}
          onChange={(e) => setRepeat(e.target.value)}
          autoComplete="new-password"
          required
          error={tried ? errors.repeat : undefined}
        />
        {serverError && (
          <p role="alert" className="rounded-xl bg-red-50 px-4 py-3 text-sm text-red-700">
            {serverError}
          </p>
        )}
      </form>
    </Modal>
  )
}

export default function Admins() {
  const { admin: me } = useAdmin()
  const toast = useToast()
  const confirm = useConfirm()
  const undo = useUndo()
  const now = useNow()
  const {
    data: admins,
    error,
    loading,
    reload,
    setData,
  } = useAsync(() => api.admin.admins().then((r) => r.admins ?? []), [])
  const [dialog, setDialog] = useState<Dialog | null>(null)
  const [pending, setPending] = useState<ReadonlySet<number>>(new Set())

  const activeCount = admins?.filter((a) => a.active).length ?? 0

  // Por qué no se puede desactivar ni eliminar esta cuenta ('' si sí se puede).
  const lockReason = (a: AdminAccount) => {
    if (a.id === me.id) return 'No puedes desactivarte ni eliminarte a ti mismo.'
    if (a.active && activeCount <= 1) return 'Es el único administrador activo.'
    return ''
  }

  const busy = (id: number, on: boolean) =>
    setPending((s) => {
      const next = new Set(s)
      if (on) next.add(id)
      else next.delete(id)
      return next
    })

  const undoThenReload = (auditId: number) => async () => {
    if (await undo({ auditId })) reload()
  }

  const toggleActive = async (a: AdminAccount, active: boolean) => {
    busy(a.id, true)
    try {
      const res = await api.admin.updateAdmin(a.id, { active })
      setData((list) => list?.map((x) => (x.id === a.id ? res.admin : x)))
      toast.success(active ? `${a.username} ya puede entrar al panel.` : `${a.username} ya no puede entrar al panel.`, {
        undo: undoThenReload(res.auditId),
      })
    } catch (e) {
      toast.error(errorMessage(e))
    } finally {
      busy(a.id, false)
    }
  }

  const remove = async (a: AdminAccount) => {
    const ok = await confirm({
      title: `¿Eliminar a ${a.username}?`,
      message: 'Ya no podrá entrar al panel. Puedes deshacerlo desde el aviso o desde Actividad.',
      confirmLabel: 'Eliminar',
      danger: true,
    })
    if (!ok) return
    busy(a.id, true)
    try {
      const res = await api.admin.deleteAdmin(a.id)
      setData((list) => list?.filter((x) => x.id !== a.id))
      toast.success(`Se eliminó a ${a.username}.`, { undo: undoThenReload(res.auditId) })
    } catch (e) {
      toast.error(errorMessage(e))
    } finally {
      busy(a.id, false)
    }
  }

  const onSaved = (admin: AdminAccount, auditId: number) => {
    if (dialog?.mode === 'create') {
      setData((list) => [...(list ?? []), admin])
      toast.success(`Se creó el administrador ${admin.username}.`, { undo: undoThenReload(auditId) })
    } else {
      setData((list) => list?.map((x) => (x.id === admin.id ? admin : x)))
      toast.success(`Se cambió la contraseña de ${admin.username}.`, { undo: undoThenReload(auditId) })
    }
    setDialog(null)
  }

  const columns: Column<AdminAccount>[] = [
    {
      key: 'username',
      header: 'Usuario',
      cell: (a) => (
        <span className="flex flex-wrap items-center gap-2">
          <span className="whitespace-nowrap font-medium text-gray-800">{a.username}</span>
          {a.id === me.id && <Pill tone="blue">Tú</Pill>}
        </span>
      ),
    },
    {
      key: 'active',
      header: 'Activo',
      cell: (a) => {
        const reason = lockReason(a)
        return (
          <span title={reason || undefined} className="inline-flex">
            <Toggle
              checked={a.active}
              onChange={(v) => void toggleActive(a, v)}
              label={`${a.username} puede entrar al panel`}
              hideLabel
              description={reason || undefined}
              disabled={Boolean(reason) || pending.has(a.id)}
            />
          </span>
        )
      },
    },
    {
      key: 'lastLogin',
      header: 'Último acceso',
      cell: (a) =>
        a.lastLoginAt ? (
          <span className="block whitespace-nowrap">
            <time dateTime={a.lastLoginAt}>{formatRelative(a.lastLoginAt, now)}</time>
            <span className="block text-xs text-gray-500">{formatDateTime(a.lastLoginAt)}</span>
          </span>
        ) : (
          <span className="text-gray-400">Nunca</span>
        ),
    },
    {
      key: 'created',
      header: 'Creado',
      className: 'hidden whitespace-nowrap md:table-cell',
      headerClassName: 'hidden md:table-cell',
      cell: (a) => formatDateTime(a.createdAt),
    },
    {
      key: 'actions',
      header: <span className="sr-only">Acciones</span>,
      align: 'right',
      cell: (a) => {
        const reason = lockReason(a)
        return (
          <span className="inline-flex gap-1">
            <IconButton
              icon={KeyRound}
              label={`Cambiar contraseña de ${a.username}`}
              size="sm"
              disabled={pending.has(a.id)}
              onClick={() => setDialog({ mode: 'password', account: a })}
            />
            <IconButton
              icon={Trash}
              label={reason ? `No se puede eliminar: ${reason}` : `Eliminar a ${a.username}`}
              size="sm"
              variant="danger"
              disabled={Boolean(reason) || pending.has(a.id)}
              onClick={() => void remove(a)}
            />
          </span>
        )
      },
    },
  ]

  return (
    <>
      <PageHeader
        title="Administradores"
        subtitle="Quién puede entrar a este panel."
        actions={
          <Button icon={Plus} onClick={() => setDialog({ mode: 'create' })}>
            Nuevo administrador
          </Button>
        }
      />

      {error && !admins ? (
        <LoadError message={errorMessage(error)} onRetry={reload} retrying={loading} />
      ) : (
        <>
          <DataTable
            rows={admins ?? []}
            columns={columns}
            rowKey={(a) => a.id}
            loading={loading}
            caption="Administradores del panel"
            empty={<EmptyState icon={ShieldCheck} title="No hay administradores" />}
          />
          <p className="mt-3 text-xs text-gray-500">
            No puedes desactivarte ni eliminarte a ti mismo, y siempre debe quedar al menos un administrador activo.
          </p>
        </>
      )}

      {dialog && <AdminFormModal dialog={dialog} onClose={() => setDialog(null)} onSaved={onSaved} />}
    </>
  )
}
