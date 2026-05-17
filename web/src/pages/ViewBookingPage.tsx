import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router'
import { format } from 'date-fns'
import { getBookingByToken } from '../lib/api'
import type { BookingPublic } from '../lib/types'

const statusBadge: Record<string, string> = {
  confirmed: 'bg-green-100 text-green-700',
  cancelled: 'bg-gray-100 text-gray-500',
  completed: 'bg-blue-100 text-blue-700',
  charged: 'bg-purple-100 text-purple-700',
}

export default function ViewBookingPage() {
  const { token } = useParams<{ token: string }>()
  const [booking, setBooking] = useState<BookingPublic | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!token) return
    getBookingByToken(token)
      .then(setBooking)
      .catch(() => setError('Booking not found.'))
  }, [token])

  if (error) {
    return (
      <div className="min-h-screen flex items-center justify-center px-4">
        <div className="text-center">
          <p className="text-gray-500">{error}</p>
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

  return (
    <div className="min-h-screen bg-gray-50 flex items-center justify-center px-4">
      <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-8 max-w-md w-full">
        <h1 className="text-xl font-bold text-gray-900 mb-1">Your Booking</h1>
        <span
          className={`inline-block px-2 py-0.5 rounded-full text-xs font-medium capitalize mb-5 ${statusBadge[booking.status] ?? 'bg-gray-100'}`}
        >
          {booking.status}
        </span>

        <dl className="space-y-3 text-sm">
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
          <div>
            <dt className="text-gray-400">Booked on</dt>
            <dd className="text-gray-600">{format(new Date(booking.created_at), 'MMM d, yyyy')}</dd>
          </div>
        </dl>

        {booking.status === 'confirmed' && (
          <Link
            to={`/cancel/${booking.cancel_token}`}
            className="mt-6 block text-center text-red-600 text-sm hover:underline"
          >
            Cancel this booking
          </Link>
        )}
        <Link to="/" className="mt-3 block text-center text-blue-600 text-sm hover:underline">
          Make another booking
        </Link>
      </div>
    </div>
  )
}
