import type { StoreBadge } from '../lib/types'

interface StoreModalProps {
  monedas: number
  badges: StoreBadge[]
  cart: Record<number, number>
  totalCartCost: number
  addToCart: (badgeId: number, available: number) => void
  removeFromCart: (badgeId: number) => void
  handlePurchase: () => void
  onClose: () => void
}

export default function StoreModal({
  monedas,
  badges,
  cart,
  totalCartCost,
  addToCart,
  removeFromCart,
  handlePurchase,
  onClose,
}: StoreModalProps) {
  return (
    <div className="fixed inset-0 bg-black bg-opacity-60 flex items-center justify-center z-50 p-4 transition-opacity duration-300">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="store-title"
        className="bg-white p-6 rounded-2xl shadow-2xl w-11/12 max-w-2xl mx-auto transform transition-transform duration-300"
      >
        <h3 id="store-title" className="text-2xl font-bold mb-4 text-gray-800">
          Badges Store
        </h3>
        <p className="mb-4 text-lg">
          Available coins:{' '}
          <strong className="text-yellow-500">
            {monedas}
          </strong>
        </p>
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-4 max-h-96 overflow-y-auto p-2">
          {badges.map((b) => (
            <div
              key={b.id}
              className="border p-4 rounded-xl flex flex-col items-center text-center transition-shadow duration-300 hover:shadow-lg"
            >
              <img
                src={b.imageUrl || undefined}
                alt={b.name}
                className="w-20 h-20 object-contain mb-2"
              />
              <strong className="text-sm font-semibold">
                {b.name}
              </strong>
              <span className="text-xs text-gray-500">
                Avail.: {b.quantity}
              </span>
              <span className="text-xs text-gray-500 mb-2">
                Price: {b.cost}
              </span>
              <div className="flex gap-2 items-center mt-2">
                <button
                  aria-label={`Remove one ${b.name}`}
                  className="bg-red-400 text-white w-6 h-6 rounded-full text-lg flex items-center justify-center"
                  onClick={() => removeFromCart(b.id)}
                >
                  -
                </button>
                <span className="text-base font-semibold">
                  {cart[b.id] || 0}
                </span>
                <button
                  aria-label={`Add one ${b.name}`}
                  className="bg-green-400 text-white w-6 h-6 rounded-full text-lg flex items-center justify-center"
                  onClick={() => addToCart(b.id, b.quantity)}
                >
                  +
                </button>
              </div>
            </div>
          ))}
        </div>
        <div className="mt-6 text-center">
          <p className="text-lg">
            Total to pay:{' '}
            <strong className="text-yellow-500">
              {totalCartCost}
            </strong>{' '}
            coins
          </p>
          <button
            className="bg-orange-500 text-white px-6 py-3 rounded-full mt-4 disabled:opacity-50 transition-all duration-300 font-semibold"
            disabled={totalCartCost === 0 || totalCartCost > monedas}
            onClick={handlePurchase}
          >
            Buy
          </button>
          <button
            className="ml-4 bg-gray-400 text-white px-6 py-3 rounded-full mt-4 transition-all duration-300 font-semibold"
            onClick={onClose}
          >
            Close
          </button>
        </div>
      </div>
    </div>
  )
}
