import { Card, PageHeader } from '../ui'

// Provisional: la página real llega en otra tarea.
export default function Dashboard() {
  return (
    <>
      <PageHeader title="Resumen" />
      <Card className="text-sm text-gray-500">Pendiente</Card>
    </>
  )
}
