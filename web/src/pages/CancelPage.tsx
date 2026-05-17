import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router'
import { format } from 'date-fns'
import { getBookingByToken, cancelBooking, ApiError } from '../lib/api'
import type { BookingPublic } from '../lib/types'

export default function CancelPage() {
  const { token } = useParams<{ token: string }>()
  const [booking, setBooking] = useState<BookingPublic | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [cancelling, setCancelling] = useState(false)
  const [cancelled, setCancelled] = useState(false)
  const [cancelError, setCancelError] = useState<string | null>(null)

  useEffect(() => {
    if (!token) return
    getBookingByToken(token)
      .then(setBooking)
      .catch(() => setLoadError('Booking not found or link has expired.'))
  }, [token])

  const handleCancel = async () => {
    if (!token) return
    setCancelling(true)
    setCancelError(null)
    try {
      await cancelBooking(token)
      setCancelled(true)
    } catch (err) {
      setCancelError(err instanceof ApiError ? err.message : 'Cancellation failed')
    } finally {
      setCancelling(false)
    }
  }

  if (loadError) {
    return (
      <div className="min-h-screen flex items-center justify-center px-4">
        <div className="text-center">
          <p className="text-gray-500">{loadError}</p>
          <Link to="/" className="mt-4 inline-block text-blue-600 text-sm hover:underline">
            Make a new booking
          </Link>
        </div>
      </div>
    )
  }

  if (!booking) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600" />
      </div>
    )
  }

  if (cancelled || booking.status === 'cancelled') {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center px-4">
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-8 max-w-md w-full text-center">
          <div className="w-14 h-14 rounded-full bg-gray-100 flex items-center justify-center mx-auto mb-4">
            <svg className="w-7 h-7 text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </div>
          <h2 className="text-xl font-bold text-gray-900 mb-2">Booking Cancelled</h2>
          <p className="text-gray-500 text-sm">Your booking has been cancelled. No charge will be made.</p>
          <Link to="/" className="mt-6 inline-block text-blue-600 text-sm hover:underline">
            Make a new booking
          </Link>
        </div>
      </div>
    )
  }

  if (booking.status !== 'confirmed') {
    return (
      <div className="min-h-screen flex items-center justify-center px-4">
        <div className="text-center">
          <p className="text-gray-500">This booking cannot be cancelled (status: {booking.status}).</p>
          <Link to="/" className="mt-4 inline-block text-blue-600 text-sm hover:underline">Home</Link>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-gray-50 flex items-center justify-center px-4">
      <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-8 max-w-md w-full">
        <h1 className="text-xl font-bold text-gray-900 mb-1">Cancel Booking</h1>
        <p className="text-sm text-gray-500 mb-6">Are you sure you want to cancel?</p>

        <dl className="space-y-3 text-sm mb-6">
          <div>
            <dt className="text-gray-400">Name</dt>
            <dd className="font-medium text-gray-900">{booking.name}</dd>
          </div>
          <div>
            <dt className="text-gray-400">Date</dt>
            <dd className="font-medium text-gray-900">
              {format(new Date(booking.start_time), 'EEEE, MMMM d, yyyy')}
            </dd>
          </div>
          <div>
            <dt className="text-gray-400">Time</dt>
            <dd className="font-medium text-gray-900">
              {format(new Date(booking.start_time), 'h:mm a')} – {format(new Date(booking.end_time), 'h:mm a')}
            </dd>
          </div>
        </dl>

        {cancelError && (
          <div className="mb-4 rounded-lg bg-red-50 border border-red-200 text-red-700 text-sm px-3 py-2">
            {cancelError}
          </div>
        )}

        <div className="flex gap-3">
          <button
            onClick={handleCancel}
            disabled={cancelling}
            className="flex-1 bg-red-600 text-white rounded-lg py-2.5 font-medium text-sm hover:bg-red-700 disabled:opacity-50 transition-colors"
          >
            {cancelling ? 'Cancelling…' : 'Yes, cancel it'}
          </button>
          <Link
            to={`/booking/${booking.cancel_token}`}
            className="flex-1 text-center bg-gray-100 text-gray-700 rounded-lg py-2.5 font-medium text-sm hover:bg-gray-200 transition-colors"
          >
            Keep it
          </Link>
        </div>
      </div>
    </div>
  )
}
