import { Fragment, useEffect, useState } from 'react'
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
  charging: 'bg-yellow-100 text-yellow-700',
  charged: 'bg-purple-100 text-purple-700',
}

function CopyButton({ value }: { value: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <button
      onClick={() => { navigator.clipboard.writeText(value); setCopied(true); setTimeout(() => setCopied(false), 1500) }}
      className="ml-1 text-gray-400 hover:text-gray-600 transition-colors"
      title="Copy"
    >
      {copied ? '✓' : '⎘'}
    </button>
  )
}

function StripeID({ value }: { value: string | undefined | null }) {
  if (!value) return <span className="text-gray-300 text-xs">—</span>
  const short = value.slice(0, 8) + '…' + value.slice(-4)
  return (
    <span className="font-mono text-xs text-gray-600" title={value}>
      {short}
      <CopyButton value={value} />
    </span>
  )
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

function getAutoChargeInfo(booking: BookingAdmin, settings: Settings | null) {
  const delayMs = (settings?.auto_charge_delay_minutes ?? 1440) * 60 * 1000
  const autoChargeAt = booking.completed_at
    ? new Date(new Date(booking.completed_at).getTime() + delayMs)
    : null
  const autoChargeInFuture = autoChargeAt ? autoChargeAt > new Date() : false
  return { autoChargeAt, autoChargeInFuture }
}

function BookingStatusBadges({ booking }: { booking: BookingAdmin }) {
  const failures = booking.charge_attempts ?? 0
  return (
    <div className="flex flex-wrap gap-1 items-start">
      <span className={`inline-block px-2 py-0.5 rounded-full text-xs font-medium capitalize ${statusColors[booking.status] ?? 'bg-gray-100 text-gray-500'}`}>
        {booking.status}
      </span>
      {failures > 0 && booking.status !== 'charged' && (
        <span
          className="inline-block px-2 py-0.5 rounded-full text-xs font-medium bg-red-100 text-red-700 cursor-help"
          title={booking.last_charge_error ?? undefined}
        >
          {failures} charge failure{failures > 1 ? 's' : ''}
        </span>
      )}
      {booking.reminder_sent && (
        <span className="inline-block px-2 py-0.5 rounded-full text-xs font-medium bg-teal-50 text-teal-600">
          reminder sent
        </span>
      )}
    </div>
  )
}

function BookingChargeTiming({
  booking,
  autoChargeAt,
  autoChargeInFuture,
  compact,
}: {
  booking: BookingAdmin
  autoChargeAt: Date | null
  autoChargeInFuture: boolean
  compact?: boolean
}) {
  if (!booking.completed_at) {
    return compact ? null : <span className="text-gray-300">—</span>
  }

  const doneLabel = compact
    ? `Done ${format(new Date(booking.completed_at), 'MMM d, h:mm a')}`
    : `Done: ${format(new Date(booking.completed_at), 'MMM d, h:mm a')}`

  return (
    <div className={compact ? 'text-xs' : 'text-xs whitespace-nowrap'}>
      <p className="text-gray-500">{doneLabel}</p>
      {autoChargeAt && (
        <p className={autoChargeInFuture ? 'text-amber-600' : booking.status === 'charged' ? 'text-gray-400' : 'text-red-500'}>
          {booking.status === 'charged'
            ? 'Charged'
            : autoChargeInFuture
            ? `Auto-charge: ${format(autoChargeAt, 'MMM d, h:mm a')}`
            : `Overdue: ${format(autoChargeAt, 'MMM d, h:mm a')}`}
        </p>
      )}
    </div>
  )
}

function BookingPaymentInfo({ booking, onLinkClick }: { booking: BookingAdmin; onLinkClick?: (e: React.MouseEvent) => void }) {
  if (booking.amount_cents) {
    return (
      <span className="text-xs text-gray-500">
        ${(booking.amount_cents / 100).toFixed(2)}
        {booking.stripe_receipt_url && (
          <a
            href={booking.stripe_receipt_url}
            target="_blank"
            rel="noopener noreferrer"
            onClick={onLinkClick}
            className="ml-2 text-blue-500 hover:underline"
          >
            Receipt ↗
          </a>
        )}
      </span>
    )
  }
  if (booking.stripe_payment_method_id) {
    return <span className="text-xs text-gray-500">Card on file</span>
  }
  return <span className="text-xs text-gray-300">—</span>
}

function BookingActions({
  booking,
  onCharge,
  onMarkDone,
  onCancel,
  size = 'sm',
}: {
  booking: BookingAdmin
  onCharge: () => void
  onMarkDone: () => void
  onCancel: () => void
  size?: 'sm' | 'md'
}) {
  const btn =
    size === 'md'
      ? 'text-sm rounded-lg px-3 py-1.5 font-medium transition-colors'
      : 'text-xs rounded px-2 py-1 transition-colors'

  return (
    <div className="flex gap-2 flex-wrap">
      {(booking.status === 'confirmed' || booking.status === 'completed') &&
        booking.stripe_payment_method_id && (
          <button
            onClick={onCharge}
            className={`${btn} bg-blue-600 text-white hover:bg-blue-700`}
          >
            Charge
          </button>
        )}
      {booking.status === 'confirmed' && (
        <button
          onClick={onMarkDone}
          className={`${btn} bg-gray-100 text-gray-700 hover:bg-gray-200`}
        >
          Mark done
        </button>
      )}
      {(booking.status === 'confirmed' || booking.status === 'completed') && (
        <button
          onClick={onCancel}
          className={`${btn} bg-red-50 text-red-600 hover:bg-red-100`}
        >
          Cancel
        </button>
      )}
    </div>
  )
}

function BookingDetailPanel({
  booking,
  autoChargeAt,
  autoChargeInFuture,
}: {
  booking: BookingAdmin
  autoChargeAt: Date | null
  autoChargeInFuture: boolean
}) {
  const metadataEntries = Object.entries(booking.metadata ?? {})

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-x-8 gap-y-3 text-xs">
      <div>
        <p className="font-medium text-gray-500 uppercase tracking-wide mb-1">Stripe IDs</p>
        <div className="space-y-1">
          <div className="flex items-center gap-1 flex-wrap">
            <span className="text-gray-400 w-24 shrink-0">Setup intent</span>
            <StripeID value={booking.stripe_setup_intent_id} />
          </div>
          <div className="flex items-center gap-1 flex-wrap">
            <span className="text-gray-400 w-24 shrink-0">Payment method</span>
            <StripeID value={booking.stripe_payment_method_id} />
          </div>
          <div className="flex items-center gap-1 flex-wrap">
            <span className="text-gray-400 w-24 shrink-0">Payment intent</span>
            <StripeID value={booking.stripe_payment_intent_id} />
          </div>
        </div>
      </div>
      <div>
        <p className="font-medium text-gray-500 uppercase tracking-wide mb-1">Timestamps</p>
        <div className="space-y-1 text-gray-600">
          <p><span className="text-gray-400 mr-1">Created</span>{format(new Date(booking.created_at), 'MMM d, yyyy h:mm a')}</p>
          {booking.updated_at && (
            <p><span className="text-gray-400 mr-1">Updated</span>{format(new Date(booking.updated_at), 'MMM d, yyyy h:mm a')}</p>
          )}
          {booking.completed_at && (
            <p><span className="text-gray-400 mr-1">Completed</span>{format(new Date(booking.completed_at), 'MMM d, yyyy h:mm a')}</p>
          )}
          {autoChargeAt && booking.status !== 'charged' && (
            <p>
              <span className="text-gray-400 mr-1">Auto-charge at</span>
              <span className={autoChargeInFuture ? 'text-amber-600' : 'text-red-500'}>
                {format(autoChargeAt, 'MMM d, yyyy h:mm a')}
                {!autoChargeInFuture && ' (overdue)'}
              </span>
            </p>
          )}
        </div>
      </div>
      {(booking.charge_attempts ?? 0) > 0 && (
        <div>
          <p className="font-medium text-gray-500 uppercase tracking-wide mb-1">Charge failures</p>
          <div className="space-y-1">
            <p className="text-red-600">{booking.charge_attempts} failed attempt{(booking.charge_attempts ?? 0) > 1 ? 's' : ''}</p>
            {booking.last_charge_error && (
              <p className="text-gray-600 font-mono text-xs break-all">{booking.last_charge_error}</p>
            )}
          </div>
        </div>
      )}
      {metadataEntries.length > 0 && (
        <div>
          <p className="font-medium text-gray-500 uppercase tracking-wide mb-1">Metadata</p>
          <div className="space-y-1 text-gray-600">
            {metadataEntries.map(([k, v]) => (
              <p key={k}><span className="text-gray-400 mr-1">{k}</span>{v}</p>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

function BookingCard({
  booking,
  settings,
  isExpanded,
  onToggleExpand,
  onCharge,
  onMarkDone,
  onCancel,
}: {
  booking: BookingAdmin
  settings: Settings | null
  isExpanded: boolean
  onToggleExpand: () => void
  onCharge: () => void
  onMarkDone: () => void
  onCancel: () => void
}) {
  const { autoChargeAt, autoChargeInFuture } = getAutoChargeInfo(booking, settings)
  const metadataEntries = Object.entries(booking.metadata ?? {})
  const isOverdue =
    autoChargeAt && !autoChargeInFuture && booking.status !== 'charged' && booking.completed_at

  return (
    <div
      className={`bg-white rounded-xl border overflow-hidden ${
        isOverdue ? 'border-amber-300' : 'border-gray-200'
      }`}
    >
      <div className="p-4">
        <div className="flex items-start justify-between gap-3 mb-2">
          <div className="min-w-0 flex-1">
            <p className="text-sm font-semibold text-gray-900 truncate">{booking.name}</p>
            <p className="text-xs text-gray-500 truncate">{booking.email}</p>
          </div>
          <BookingStatusBadges booking={booking} />
        </div>

        <p className="text-sm text-gray-700">
          {format(new Date(booking.start_time), 'MMM d, yyyy')} ·{' '}
          {format(new Date(booking.start_time), 'h:mm a')} – {format(new Date(booking.end_time), 'h:mm a')}
        </p>

        {metadataEntries.length > 0 && (
          <p className="text-xs text-gray-400 mt-1 truncate" title={metadataEntries.map(([k, v]) => `${k}: ${v}`).join(', ')}>
            {metadataEntries.map(([k, v]) => `${k}: ${v}`).join(', ')}
          </p>
        )}

        <div className="mt-2 space-y-1">
          <BookingChargeTiming
            booking={booking}
            autoChargeAt={autoChargeAt}
            autoChargeInFuture={autoChargeInFuture}
            compact
          />
          <div>
            <span className="text-xs text-gray-400 mr-1">Payment</span>
            <BookingPaymentInfo booking={booking} onLinkClick={(e) => e.stopPropagation()} />
          </div>
        </div>

        <div className="mt-3">
          <BookingActions
            booking={booking}
            onCharge={onCharge}
            onMarkDone={onMarkDone}
            onCancel={onCancel}
            size="md"
          />
        </div>

        <button
          type="button"
          onClick={onToggleExpand}
          className="mt-3 w-full text-left text-xs font-medium text-gray-500 hover:text-gray-700 flex items-center gap-1"
        >
          {isExpanded ? '▲ Hide details' : '▼ Details'}
        </button>
      </div>

      {isExpanded && (
        <div className="px-4 pb-4 pt-0 border-t border-gray-100 bg-gray-50">
          <p className="text-xs text-gray-400 font-mono mb-3 pt-3">#{booking.id}</p>
          <BookingDetailPanel
            booking={booking}
            autoChargeAt={autoChargeAt}
            autoChargeInFuture={autoChargeInFuture}
          />
        </div>
      )}
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
  const [expandedId, setExpandedId] = useState<number | null>(null)

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

  const toggleExpanded = (id: number) => {
    setExpandedId((prev) => (prev === id ? null : id))
  }

  return (
    <div className="p-4 sm:p-8">
      <h1 className="text-2xl font-bold text-gray-900 mb-6">Bookings</h1>

      {/* Filters */}
      <div className="flex flex-wrap gap-3 mb-6">
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
          <option value="charging">Charging</option>
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
        <>
          {/* Mobile: card list */}
          <div className="md:hidden space-y-3">
            {bookings.map((booking) => (
              <BookingCard
                key={booking.id}
                booking={booking}
                settings={settings}
                isExpanded={expandedId === booking.id}
                onToggleExpand={() => toggleExpanded(booking.id)}
                onCharge={() => setChargeTarget(booking)}
                onMarkDone={() => handleStatusChange(booking, 'completed')}
                onCancel={() => setCancelTarget(booking)}
              />
            ))}
          </div>

          {/* Desktop: table */}
          <div className="hidden md:block bg-white rounded-xl border border-gray-200 overflow-x-auto">
            <table className="w-full min-w-[900px]">
              <thead className="bg-gray-50 border-b border-gray-200">
                <tr>
                  {['#', 'Name', 'Date / Time', 'Status', 'Charge timing', 'Payment', 'Actions', ''].map((h) => (
                    <th key={h} className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase whitespace-nowrap">
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {bookings.map((booking) => {
                  const isExpanded = expandedId === booking.id
                  const { autoChargeAt, autoChargeInFuture } = getAutoChargeInfo(booking, settings)
                  const metadataEntries = Object.entries(booking.metadata ?? {})
                  return (
                    <Fragment key={booking.id}>
                      <tr
                        className={`hover:bg-gray-50 cursor-pointer ${isExpanded ? 'bg-gray-50' : ''}`}
                        onClick={() => toggleExpanded(booking.id)}
                      >
                        <td className="px-4 py-3 text-xs text-gray-400 font-mono">{booking.id}</td>
                        <td className="px-4 py-3">
                          <p className="text-sm font-medium text-gray-900">{booking.name}</p>
                          <p className="text-xs text-gray-400">{booking.email}</p>
                          {metadataEntries.length > 0 && (
                            <p className="text-xs text-gray-400 mt-0.5 truncate max-w-[160px]" title={metadataEntries.map(([k, v]) => `${k}: ${v}`).join(', ')}>
                              {metadataEntries.map(([k, v]) => `${k}: ${v}`).join(', ')}
                            </p>
                          )}
                        </td>
                        <td className="px-4 py-3 text-sm text-gray-600 whitespace-nowrap">
                          <p>{format(new Date(booking.start_time), 'MMM d, yyyy')}</p>
                          <p className="text-xs text-gray-400">
                            {format(new Date(booking.start_time), 'h:mm a')} – {format(new Date(booking.end_time), 'h:mm a')}
                          </p>
                        </td>
                        <td className="px-4 py-3">
                          <BookingStatusBadges booking={booking} />
                        </td>
                        <td className="px-4 py-3">
                          <BookingChargeTiming
                            booking={booking}
                            autoChargeAt={autoChargeAt}
                            autoChargeInFuture={autoChargeInFuture}
                          />
                        </td>
                        <td className="px-4 py-3 whitespace-nowrap">
                          <BookingPaymentInfo
                            booking={booking}
                            onLinkClick={(e) => e.stopPropagation()}
                          />
                        </td>
                        <td className="px-4 py-3" onClick={(e) => e.stopPropagation()}>
                          <BookingActions
                            booking={booking}
                            onCharge={() => setChargeTarget(booking)}
                            onMarkDone={() => handleStatusChange(booking, 'completed')}
                            onCancel={() => setCancelTarget(booking)}
                          />
                        </td>
                        <td className="px-3 py-3 text-gray-400 text-sm">
                          {isExpanded ? '▲' : '▼'}
                        </td>
                      </tr>
                      {isExpanded && (
                        <tr className="bg-gray-50 border-b border-gray-200">
                          <td colSpan={8} className="px-6 py-4">
                            <BookingDetailPanel
                              booking={booking}
                              autoChargeAt={autoChargeAt}
                              autoChargeInFuture={autoChargeInFuture}
                            />
                          </td>
                        </tr>
                      )}
                    </Fragment>
                  )
                })}
              </tbody>
            </table>
          </div>
        </>
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
