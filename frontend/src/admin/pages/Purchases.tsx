import { Card, PageHeader } from '../ui'

// Provisional: la página real llega en otra tarea.
export default function Purchases() {
  return (
    <>
      <PageHeader title="Compras" />
      <Card className="text-sm text-gray-500">Pendiente</Card>
    </>
  )
}
