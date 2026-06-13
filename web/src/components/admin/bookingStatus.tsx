import type { BookingStatus } from '../../lib/types'

export const statusColors: Record<string, string> = {
  confirmed: 'bg-green-100 text-green-700',
  cancelled: 'bg-gray-100 text-gray-500',
  completed: 'bg-blue-100 text-blue-700',
  charging: 'bg-yellow-100 text-yellow-700',
  charged: 'bg-purple-100 text-purple-700',
}

export function BookingStatusBadge({ status }: { status: BookingStatus | string }) {
  return (
    <span
      className={`inline-block px-2 py-0.5 rounded-full text-xs font-medium capitalize ${
        statusColors[status] ?? 'bg-gray-100 text-gray-500'
      }`}
    >
      {status}
    </span>
  )
}
