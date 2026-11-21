import { Drawer } from '../ui'

export interface StudentDrawerProps {
  studentId: number | null
  onClose: () => void
  onChanged: () => void
}

// Provisional: el detalle real del alumno llega en otra tarea.
export default function StudentDrawer({ studentId, onClose }: StudentDrawerProps) {
  return (
    <Drawer open={studentId !== null} onClose={onClose} title="Alumno">
      <p className="text-sm text-gray-500">Pendiente</p>
    </Drawer>
  )
}
