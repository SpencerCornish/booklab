import { useEffect, useState } from 'react'
import { format } from 'date-fns'
import { ResponsiveDataView } from '../components/admin/ResponsiveDataView'
import { adminGetInsights } from '../lib/api'
import type { CustomerInsight, InsightsData } from '../lib/types'

const statusLabels: Record<string, string> = {
  confirmed: 'Confirmed',
  cancelled: 'Cancelled',
  completed: 'Completed',
  charging: 'Charging',
  charged: 'Charged',
}

function formatCurrency(cents: number) {
  return `$${(cents / 100).toFixed(2)}`
}

function CustomerCard({ customer }: { customer: CustomerInsight }) {
  return (
    <div className="bg-white rounded-xl border border-gray-200 p-4">
      <div className="mb-3">
        <p className="text-sm font-semibold text-gray-900 truncate">{customer.name}</p>
        <p className="text-xs text-gray-500 truncate">{customer.email}</p>
      </div>
      <div className="grid grid-cols-2 gap-3 text-sm">
        <div>
          <p className="text-xs text-gray-400">Bookings</p>
          <p className="font-medium text-gray-800">{customer.booking_count}</p>
        </div>
        <div>
          <p className="text-xs text-gray-400">Cancelled</p>
          <p className="font-medium text-gray-600">
            {customer.cancelled_count > 0 ? customer.cancelled_count : '—'}
          </p>
        </div>
        <div>
          <p className="text-xs text-gray-400">Revenue</p>
          <p className="font-medium text-gray-800">{formatCurrency(customer.revenue_cents)}</p>
        </div>
        <div>
          <p className="text-xs text-gray-400">Last visit</p>
          <p className="font-medium text-gray-600">
            {customer.last_booking_at
              ? format(new Date(customer.last_booking_at), 'MMM d, yyyy')
              : '—'}
          </p>
        </div>
      </div>
    </div>
  )
}

export default function AdminInsights() {
  const [insights, setInsights] = useState<InsightsData | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    adminGetInsights()
      .then(setInsights)
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [])

  if (loading || !insights) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-7 w-7 border-b-2 border-gray-400" />
      </div>
    )
  }

  const totalReferralResponses = insights.referral_sources.reduce((sum, r) => sum + r.count, 0)

  return (
    <div className="p-4 sm:p-8">
      <h1 className="text-2xl font-bold text-gray-900 mb-6">Insights</h1>

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
        {[
          { label: 'Total bookings', value: insights.total_bookings.toString() },
          { label: 'Total revenue', value: formatCurrency(insights.total_revenue_cents) },
          { label: 'Unique customers', value: insights.unique_customers.toString() },
          { label: 'Bookings (30 days)', value: insights.recent_bookings.toString() },
        ].map((stat) => (
          <div key={stat.label} className="bg-white rounded-xl border border-gray-200 p-5">
            <p className="text-3xl font-bold text-gray-900">{stat.value}</p>
            <p className="text-sm text-gray-500 mt-1">{stat.label}</p>
          </div>
        ))}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
        <section>
          <h2 className="text-sm font-semibold text-gray-500 uppercase tracking-wider mb-3">
            Bookings by status
          </h2>
          <div className="bg-white rounded-xl border border-gray-200 overflow-hidden">
            {Object.keys(insights.bookings_by_status).length === 0 ? (
              <p className="px-4 py-6 text-sm text-gray-400">No bookings yet.</p>
            ) : (
              <table className="w-full">
                <thead className="bg-gray-50 border-b border-gray-200">
                  <tr>
                    <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">
                      Status
                    </th>
                    <th className="px-4 py-2 text-right text-xs font-medium text-gray-500 uppercase">
                      Count
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100">
                  {Object.entries(insights.bookings_by_status)
                    .sort(([, a], [, b]) => b - a)
                    .map(([status, count]) => (
                      <tr key={status} className="hover:bg-gray-50">
                        <td className="px-4 py-3 text-sm text-gray-800">
                          {statusLabels[status] ?? status}
                        </td>
                        <td className="px-4 py-3 text-sm text-gray-600 text-right font-medium">
                          {count}
                        </td>
                      </tr>
                    ))}
                </tbody>
              </table>
            )}
          </div>
        </section>

        <section>
          <h2 className="text-sm font-semibold text-gray-500 uppercase tracking-wider mb-3">
            How customers heard about us
          </h2>
          <div className="bg-white rounded-xl border border-gray-200 overflow-hidden">
            {insights.referral_sources.length === 0 ? (
              <p className="px-4 py-6 text-sm text-gray-400">
                No referral responses yet. Configure sources in Settings to start collecting this data.
              </p>
            ) : (
              <table className="w-full">
                <thead className="bg-gray-50 border-b border-gray-200">
                  <tr>
                    <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">
                      Source
                    </th>
                    <th className="px-4 py-2 text-right text-xs font-medium text-gray-500 uppercase">
                      Count
                    </th>
                    <th className="px-4 py-2 text-right text-xs font-medium text-gray-500 uppercase w-28">
                      Share
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100">
                  {insights.referral_sources.map((row) => {
                    const pct =
                      totalReferralResponses > 0
                        ? Math.round((row.count / totalReferralResponses) * 100)
                        : 0
                    return (
                      <tr key={row.source} className="hover:bg-gray-50">
                        <td className="px-4 py-3 text-sm text-gray-800">{row.source}</td>
                        <td className="px-4 py-3 text-sm text-gray-600 text-right font-medium">
                          {row.count}
                        </td>
                        <td className="px-4 py-3">
                          <div className="flex items-center gap-2 justify-end">
                            <div className="w-16 h-2 bg-gray-100 rounded-full overflow-hidden">
                              <div
                                className="h-full bg-blue-500 rounded-full"
                                style={{ width: `${pct}%` }}
                              />
                            </div>
                            <span className="text-xs text-gray-500 w-8 text-right">{pct}%</span>
                          </div>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            )}
          </div>
        </section>
      </div>

      <section className="mt-8">
        <h2 className="text-sm font-semibold text-gray-500 uppercase tracking-wider mb-3">
          Customers
        </h2>
        <ResponsiveDataView
          items={insights.customers}
          keyFn={(c) => c.email}
          emptyMessage="No customers yet."
          renderCard={(customer) => <CustomerCard customer={customer} />}
          columns={[
            {
              header: 'Customer',
              render: (customer) => (
                <>
                  <p className="text-sm font-medium text-gray-900">{customer.name}</p>
                  <p className="text-xs text-gray-500">{customer.email}</p>
                </>
              ),
            },
            {
              header: 'Bookings',
              headerClassName: 'text-right',
              cellClassName: 'text-sm text-gray-600 text-right font-medium',
              render: (customer) => customer.booking_count,
            },
            {
              header: 'Cancelled',
              headerClassName: 'text-right',
              cellClassName: 'text-sm text-gray-500 text-right',
              render: (customer) => (customer.cancelled_count > 0 ? customer.cancelled_count : '—'),
            },
            {
              header: 'Revenue',
              headerClassName: 'text-right',
              cellClassName: 'text-sm text-gray-800 text-right font-medium',
              render: (customer) => formatCurrency(customer.revenue_cents),
            },
            {
              header: 'Last visit',
              headerClassName: 'text-right',
              cellClassName: 'text-sm text-gray-500 text-right',
              render: (customer) =>
                customer.last_booking_at
                  ? format(new Date(customer.last_booking_at), 'MMM d, yyyy')
                  : '—',
            },
          ]}
        />
        <p className="text-xs text-gray-400 mt-2">
          Sorted by collected revenue. Revenue reflects charged bookings only.
        </p>
      </section>
    </div>
  )
}
