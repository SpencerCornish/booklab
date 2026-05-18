import { useEffect, useState } from 'react'
import { format } from 'date-fns'
import { adminListBookings, adminChargeBooking, adminUpdateBooking, adminGetSettings, ApiError } from '../lib/api'
import type { BookingAdmin, BookingStatus, Settings } from '../lib/types'

function CancelModal({
  booking,
  onClose,
  onCancelled,
}: {
  booking: BookingAdmin
  onClose: () => void
  onCancelled: (b: BookingAdmin) => void
}) {
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  const handleCancel = async () => {
    setLoading(true)
    setError(null)
    try {
      const updated = await adminUpdateBooking(booking.id, { status: 'cancelled' })
      onCancelled(updated)
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Cancel failed')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 px-4">
      <div className="bg-white rounded-2xl shadow-xl p-6 max-w-sm w-full">
        <h2 className="text-lg font-bold text-gray-900 mb-1">Cancel Booking</h2>
        <p className="text-sm text-gray-500 mb-4">
          Cancel booking for <strong>{booking.name}</strong> on{' '}
          {format(new Date(booking.start_time), 'MMM d')} at{' '}
          {format(new Date(booking.start_time), 'h:mm a')}? This cannot be undone.
        </p>
        {error && (
          <div className="mb-3 rounded-lg bg-red-50 border border-red-200 text-red-700 text-sm px-3 py-2">
            {error}
          </div>
        )}
        <div className="flex gap-3">
          <button
            onClick={handleCancel}
            disabled={loading}
            className="flex-1 bg-red-600 text-white rounded-lg py-2 font-medium text-sm hover:bg-red-700 disabled:opacity-50 transition-colors"
          >
            {loading ? 'Cancelling…' : 'Yes, cancel it'}
          </button>
          <button
            onClick={onClose}
            className="flex-1 bg-gray-100 text-gray-700 rounded-lg py-2 font-medium text-sm hover:bg-gray-200 transition-colors"
          >
            Keep booking
          </button>
        </div>
      </div>
    </div>
  )
}

const statusColors: Record<string, string> = {
  confirmed: 'bg-green-100 text-green-700',
  cancelled: 'bg-gray-100 text-gray-500',
  completed: 'bg-blue-100 text-blue-700',
  charged: 'bg-purple-100 text-purple-700',
}

function ChargeModal({
  booking,
  hourlyRateCents,
  onClose,
  onCharged,
}: {
  booking: BookingAdmin
  hourlyRateCents: number
  onClose: () => void
  onCharged: (b: BookingAdmin) => void
}) {
  const hours = Math.max(
    1,
    Math.round((new Date(booking.end_time).getTime() - new Date(booking.start_time).getTime()) / 3600000),
  )
  const defaultAmount = (hours * hourlyRateCents) / 100
  const [customAmount, setCustomAmount] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  const chargeAmount = customAmount ? parseFloat(customAmount) : defaultAmount
  const isCustom = customAmount !== ''

  const handleCharge = async () => {
    setLoading(true)
    setError(null)
    try {
      const amount = isCustom ? Math.round(parseFloat(customAmount) * 100) : undefined
      const updated = await adminChargeBooking(booking.id, amount)
      onCharged(updated)
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Charge failed')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 px-4">
      <div className="bg-white rounded-2xl shadow-xl p-6 max-w-sm w-full">
        <h2 className="text-lg font-bold text-gray-900 mb-1">Charge Booking</h2>
        <p className="text-sm text-gray-500 mb-4">
          {booking.name} · {format(new Date(booking.start_time), 'MMM d, h:mm a')} – {format(new Date(booking.end_time), 'h:mm a')} ({hours} hr)
        </p>

        <div className="rounded-lg bg-blue-50 border border-blue-100 px-3 py-2 mb-4 flex justify-between items-center">
          <span className="text-sm text-blue-700">
            {isCustom ? 'Custom amount' : `Auto · ${hours} hr × $${(hourlyRateCents / 100).toFixed(0)}/hr`}
          </span>
          <span className="text-lg font-bold text-blue-900">${chargeAmount.toFixed(2)}</span>
        </div>

        <div className="mb-4">
          <label className="block text-sm font-medium text-gray-700 mb-1">
            Override amount <span className="font-normal text-gray-400">(optional)</span>
          </label>
          <div className="relative">
            <span className="absolute left-3 top-2 text-gray-400 text-sm">$</span>
            <input
              type="number"
              step="0.01"
              min="0"
              value={customAmount}
              onChange={(e) => setCustomAmount(e.target.value)}
              className="w-full rounded-lg border border-gray-300 pl-7 pr-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
              placeholder={`${defaultAmount.toFixed(2)} (auto)`}
            />
          </div>
        </div>
        {error && (
          <div className="mb-3 rounded-lg bg-red-50 border border-red-200 text-red-700 text-sm px-3 py-2">
            {error}
          </div>
        )}
        <div className="flex gap-3">
          <button
            onClick={handleCharge}
            disabled={loading}
            className="flex-1 bg-blue-600 text-white rounded-lg py-2 font-medium text-sm hover:bg-blue-700 disabled:opacity-50 transition-colors"
          >
            {loading ? 'Charging…' : `Charge $${chargeAmount.toFixed(2)}`}
          </button>
          <button
            onClick={onClose}
            className="flex-1 bg-gray-100 text-gray-700 rounded-lg py-2 font-medium text-sm hover:bg-gray-200 transition-colors"
          >
            Cancel
          </button>
        </div>
      </div>
    </div>
  )
}

export default function AdminBookings() {
  const [bookings, setBookings] = useState<BookingAdmin[]>([])
  const [settings, setSettings] = useState<Settings | null>(null)
  const [loading, setLoading] = useState(true)
  const [dateFilter, setDateFilter] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [chargeTarget, setChargeTarget] = useState<BookingAdmin | null>(null)
  const [cancelTarget, setCancelTarget] = useState<BookingAdmin | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)

  useEffect(() => {
    adminGetSettings().then(setSettings).catch(console.error)
  }, [])

  const load = () => {
    setLoading(true)
    adminListBookings({
      date: dateFilter || undefined,
      status: statusFilter || undefined,
    })
      .then(setBookings)
      .catch(console.error)
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [dateFilter, statusFilter]) // eslint-disable-line react-hooks/exhaustive-deps

  const handleStatusChange = async (booking: BookingAdmin, status: BookingStatus) => {
    setActionError(null)
    try {
      const updated = await adminUpdateBooking(booking.id, { status })
      setBookings((prev) => prev.map((b) => (b.id === updated.id ? updated : b)))
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : 'Update failed')
    }
  }

  const handleCharged = (updated: BookingAdmin) => {
    setBookings((prev) => prev.map((b) => (b.id === updated.id ? updated : b)))
  }

  const handleCancelled = (updated: BookingAdmin) => {
    setBookings((prev) => prev.map((b) => (b.id === updated.id ? updated : b)))
  }

  return (
    <div className="p-8">
      <h1 className="text-2xl font-bold text-gray-900 mb-6">Bookings</h1>

      {/* Filters */}
      <div className="flex gap-3 mb-6">
        <input
          type="date"
          value={dateFilter}
          onChange={(e) => setDateFilter(e.target.value)}
          className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
        />
        <select
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
          className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
        >
          <option value="">All statuses</option>
          <option value="confirmed">Confirmed</option>
          <option value="completed">Completed</option>
          <option value="charged">Charged</option>
          <option value="cancelled">Cancelled</option>
        </select>
        {(dateFilter || statusFilter) && (
          <button
            onClick={() => { setDateFilter(''); setStatusFilter('') }}
            className="text-sm text-gray-500 hover:text-gray-700"
          >
            Clear filters
          </button>
        )}
      </div>

      {actionError && (
        <div className="mb-4 rounded-lg bg-red-50 border border-red-200 text-red-700 text-sm px-3 py-2">
          {actionError}
        </div>
      )}

      {loading ? (
        <div className="flex items-center justify-center h-32">
          <div className="animate-spin rounded-full h-7 w-7 border-b-2 border-gray-400" />
        </div>
      ) : bookings.length === 0 ? (
        <p className="text-sm text-gray-400 py-8 text-center">No bookings found.</p>
      ) : (
        <div className="bg-white rounded-xl border border-gray-200 overflow-hidden">
          <table className="w-full">
            <thead className="bg-gray-50 border-b border-gray-200">
              <tr>
                {['Name', 'Date / Time', 'Status', 'Payment', 'Actions'].map((h) => (
                  <th key={h} className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {bookings.map((booking) => (
                <tr key={booking.id} className="hover:bg-gray-50">
                  <td className="px-4 py-3">
                    <p className="text-sm font-medium text-gray-900">{booking.name}</p>
                    <p className="text-xs text-gray-400">{booking.email}</p>
                  </td>
                  <td className="px-4 py-3 text-sm text-gray-600">
                    <p>{format(new Date(booking.start_time), 'MMM d, yyyy')}</p>
                    <p className="text-xs text-gray-400">
                      {format(new Date(booking.start_time), 'h:mm a')} – {format(new Date(booking.end_time), 'h:mm a')}
                    </p>
                  </td>
                  <td className="px-4 py-3">
                    <span className={`inline-block px-2 py-0.5 rounded-full text-xs font-medium capitalize ${statusColors[booking.status]}`}>
                      {booking.status}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-xs text-gray-500">
                    {booking.amount_cents ? (
                      <span>
                        ${(booking.amount_cents / 100).toFixed(2)}
                        {booking.stripe_receipt_url && (
                          <a
                            href={booking.stripe_receipt_url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="ml-2 text-blue-500 hover:underline"
                          >
                            Receipt ↗
                          </a>
                        )}
                      </span>
                    ) : booking.stripe_payment_method_id ? (
                      'Card on file'
                    ) : (
                      '-'
                    )}
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex gap-2">
                      {(booking.status === 'confirmed' || booking.status === 'completed') &&
                        booking.stripe_payment_method_id && (
                          <button
                            onClick={() => setChargeTarget(booking)}
                            className="text-xs bg-blue-600 text-white rounded px-2 py-1 hover:bg-blue-700 transition-colors"
                          >
                            Charge
                          </button>
                        )}
                      {booking.status === 'confirmed' && (
                        <button
                          onClick={() => handleStatusChange(booking, 'completed')}
                          className="text-xs bg-gray-100 text-gray-700 rounded px-2 py-1 hover:bg-gray-200 transition-colors"
                        >
                          Mark done
                        </button>
                      )}
                      {(booking.status === 'confirmed' || booking.status === 'completed') && (
                        <button
                          onClick={() => setCancelTarget(booking)}
                          className="text-xs bg-red-50 text-red-600 rounded px-2 py-1 hover:bg-red-100 transition-colors"
                        >
                          Cancel
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {chargeTarget && (
        <ChargeModal
          booking={chargeTarget}
          hourlyRateCents={settings?.hourly_rate_cents ?? 0}
          onClose={() => setChargeTarget(null)}
          onCharged={handleCharged}
        />
      )}
      {cancelTarget && (
        <CancelModal
          booking={cancelTarget}
          onClose={() => setCancelTarget(null)}
          onCancelled={handleCancelled}
        />
      )}
    </div>
  )
}
