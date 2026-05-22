import { useEffect, useState } from 'react'
import { format, isToday, isFuture } from 'date-fns'
import { adminListBookings } from '../lib/api'
import type { BookingAdmin } from '../lib/types'

const statusColors: Record<string, string> = {
  confirmed: 'bg-green-100 text-green-700',
  cancelled: 'bg-gray-100 text-gray-500',
  completed: 'bg-blue-100 text-blue-700',
  charged: 'bg-purple-100 text-purple-700',
}

function BookingRow({ booking }: { booking: BookingAdmin }) {
  return (
    <tr className="hover:bg-gray-50">
      <td className="px-4 py-3 text-sm font-medium text-gray-900">{booking.name}</td>
      <td className="px-4 py-3 text-sm text-gray-600">
        {format(new Date(booking.start_time), 'h:mm a')} – {format(new Date(booking.end_time), 'h:mm a')}
      </td>
      <td className="px-4 py-3">
        <span className={`inline-block px-2 py-0.5 rounded-full text-xs font-medium capitalize ${statusColors[booking.status]}`}>
          {booking.status}
        </span>
      </td>
      <td className="px-4 py-3 text-sm text-gray-500">{booking.email}</td>
    </tr>
  )
}

export default function AdminDashboard() {
  const [bookings, setBookings] = useState<BookingAdmin[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    adminListBookings()
      .then(setBookings)
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [])

  const todayBookings = bookings.filter((b) => isToday(new Date(b.start_time)))
  const upcoming = bookings.filter(
    (b) => isFuture(new Date(b.start_time)) && !isToday(new Date(b.start_time)) && b.status === 'confirmed',
  )
  const needsCharge = bookings.filter(
    (b) => b.status === 'completed' || (b.status === 'confirmed' && !isFuture(new Date(b.end_time))),
  )

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-gray-400" />
      </div>
    )
  }

  return (
    <div className="p-4 sm:p-8">
      <h1 className="text-2xl font-bold text-gray-900 mb-6">Dashboard</h1>

      {/* Stats row */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-8">
        {[
          { label: "Today's bookings", value: todayBookings.length },
          { label: 'Upcoming', value: upcoming.length },
          { label: 'Needs charge', value: needsCharge.length, alert: needsCharge.length > 0 },
        ].map((stat) => (
          <div
            key={stat.label}
            className={`bg-white rounded-xl border p-5 ${stat.alert ? 'border-amber-300 bg-amber-50' : 'border-gray-200'}`}
          >
            <p className="text-3xl font-bold text-gray-900">{stat.value}</p>
            <p className={`text-sm mt-1 ${stat.alert ? 'text-amber-700' : 'text-gray-500'}`}>{stat.label}</p>
          </div>
        ))}
      </div>

      {/* Today's bookings */}
      {todayBookings.length > 0 && (
        <section className="mb-8">
          <h2 className="text-sm font-semibold text-gray-500 uppercase tracking-wider mb-3">Today</h2>
          <div className="bg-white rounded-xl border border-gray-200 overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50 border-b border-gray-200">
                <tr>
                  {['Name', 'Time', 'Status', 'Email'].map((h) => (
                    <th key={h} className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {todayBookings.map((b) => (
                  <BookingRow key={b.id} booking={b} />
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {/* Needs charge */}
      {needsCharge.length > 0 && (
        <section>
          <h2 className="text-sm font-semibold text-amber-600 uppercase tracking-wider mb-3">Needs Charge</h2>
          <div className="bg-white rounded-xl border border-amber-200 overflow-x-auto">
            <table className="w-full">
              <thead className="bg-amber-50 border-b border-amber-100">
                <tr>
                  {['Name', 'Date', 'Status', 'Email'].map((h) => (
                    <th key={h} className="px-4 py-2 text-left text-xs font-medium text-amber-700 uppercase">
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {needsCharge.map((b) => (
                  <tr key={b.id} className="hover:bg-gray-50">
                    <td className="px-4 py-3 text-sm font-medium text-gray-900">{b.name}</td>
                    <td className="px-4 py-3 text-sm text-gray-600">
                      {format(new Date(b.start_time), 'MMM d')} · {format(new Date(b.start_time), 'h:mm a')}–{format(new Date(b.end_time), 'h:mm a')}
                    </td>
                    <td className="px-4 py-3">
                      <span className={`inline-block px-2 py-0.5 rounded-full text-xs font-medium capitalize ${statusColors[b.status]}`}>
                        {b.status}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-500">{b.email}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <p className="text-xs text-gray-400 mt-2">Go to <a href="/admin/bookings" className="text-blue-600 hover:underline">Bookings</a> to charge.</p>
        </section>
      )}
    </div>
  )
}
