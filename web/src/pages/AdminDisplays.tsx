import { useEffect, useState } from 'react'
import { formatDistanceToNow } from 'date-fns'
import { AdminPageHeader } from '../components/admin/AdminPageHeader'
import {
  adminDeleteDisplay,
  adminDisplayPreviewURL,
  adminGetSettings,
  adminListDisplays,
  adminUpdateDisplay,
  adminUpdateSettings,
  ApiError,
} from '../lib/api'
import type { Display, DisplayStatus } from '../lib/types'

const statusStyles: Record<DisplayStatus, string> = {
  pending: 'bg-amber-50 text-amber-700 border-amber-200',
  approved: 'bg-green-50 text-green-700 border-green-200',
  revoked: 'bg-gray-100 text-gray-500 border-gray-200',
}

function StatusBadge({ status }: { status: DisplayStatus }) {
  return (
    <span className={`inline-block rounded-full border px-2 py-0.5 text-xs font-medium capitalize ${statusStyles[status]}`}>
      {status}
    </span>
  )
}

function signalLabel(rssi: number | null) {
  if (rssi === null) return '—'
  const quality = rssi >= -60 ? 'strong' : rssi >= -72 ? 'ok' : 'weak'
  return `${rssi} dBm (${quality})`
}

function batteryLabel(d: Display) {
  if (d.battery_voltage === null || d.battery_voltage <= 0) return '—'
  return `${d.battery_voltage.toFixed(2)} V${d.battery_low ? ' · low' : ''}`
}

function lastSeenLabel(iso: string | null) {
  if (!iso) return 'never'
  return formatDistanceToNow(new Date(iso), { addSuffix: true })
}

function DisplayCard({
  display,
  onChange,
  onDelete,
}: {
  display: Display
  onChange: (d: Display) => void
  onDelete: (id: number) => void
}) {
  const [name, setName] = useState(display.name)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const run = async (fn: () => Promise<void>) => {
    setBusy(true)
    setError(null)
    try {
      await fn()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Something went wrong')
    } finally {
      setBusy(false)
    }
  }

  const setStatus = (status: DisplayStatus) =>
    run(async () => onChange(await adminUpdateDisplay(display.id, { status })))

  const saveName = () => {
    if (name.trim() === display.name) return
    return run(async () => {
      const updated = await adminUpdateDisplay(display.id, { name })
      setName(updated.name)
      onChange(updated)
    })
  }

  const remove = () => {
    if (!confirm('Delete this display? If it is still powered on, it will reappear as pending on its next check-in.')) return
    return run(async () => {
      await adminDeleteDisplay(display.id)
      onDelete(display.id)
    })
  }

  return (
    <div className="bg-white rounded-xl border border-gray-200 p-5 space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0 space-y-1">
          <div className="flex items-center gap-2">
            <span className="font-mono text-lg font-bold tracking-wider text-gray-900">{display.friendly_id}</span>
            <StatusBadge status={display.status} />
          </div>
          <p className="text-xs text-gray-500 font-mono">{display.mac_address}</p>
        </div>
        <div className="flex flex-wrap gap-2 shrink-0">
          {display.status !== 'approved' && (
            <button
              onClick={() => setStatus('approved')}
              disabled={busy}
              className="bg-blue-600 text-white rounded-lg px-3 py-1.5 text-sm font-medium hover:bg-blue-700 disabled:opacity-50 transition-colors"
            >
              Approve
            </button>
          )}
          {display.status !== 'revoked' && (
            <button
              onClick={() => setStatus('revoked')}
              disabled={busy}
              className="bg-gray-100 text-gray-700 rounded-lg px-3 py-1.5 text-sm hover:bg-gray-200 disabled:opacity-50 transition-colors"
            >
              Revoke
            </button>
          )}
          <button
            onClick={remove}
            disabled={busy}
            className="text-sm text-red-500 hover:underline px-2 disabled:opacity-50"
          >
            Delete
          </button>
        </div>
      </div>

      {display.status === 'pending' && (
        <p className="text-xs text-amber-700 bg-amber-50 border border-amber-200 rounded-lg px-3 py-2">
          Check that the code above matches the one on the display before approving it.
        </p>
      )}

      <div>
        <label className="block text-xs font-medium text-gray-600 mb-1">Name</label>
        <input
          type="text"
          value={name}
          maxLength={100}
          placeholder="e.g. Darkroom door"
          onChange={(e) => setName(e.target.value)}
          onBlur={saveName}
          onKeyDown={(e) => e.key === 'Enter' && (e.target as HTMLInputElement).blur()}
          className="w-full sm:max-w-xs rounded-lg border border-gray-300 px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
        />
      </div>

      <dl className="grid grid-cols-2 sm:grid-cols-4 gap-3 text-sm">
        <div>
          <dt className="text-xs text-gray-500">Last check-in</dt>
          <dd className="text-gray-900">{lastSeenLabel(display.last_seen_at)}</dd>
        </div>
        <div>
          <dt className="text-xs text-gray-500">Battery</dt>
          <dd className={display.battery_low ? 'text-red-600 font-medium' : 'text-gray-900'}>{batteryLabel(display)}</dd>
        </div>
        <div>
          <dt className="text-xs text-gray-500">Wi-Fi</dt>
          <dd className="text-gray-900">{signalLabel(display.rssi)}</dd>
        </div>
        <div>
          <dt className="text-xs text-gray-500">Firmware</dt>
          <dd className="text-gray-900">{display.firmware_version || '—'}</dd>
        </div>
      </dl>

      {error && (
        <div className="rounded-lg bg-red-50 border border-red-200 text-red-700 text-xs px-3 py-2">{error}</div>
      )}
    </div>
  )
}

function RefreshSetting() {
  const [minutes, setMinutes] = useState<string | null>(null)
  const [saved, setSaved] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    adminGetSettings()
      .then((s) => {
        setMinutes(String(s.display_refresh_minutes))
        setSaved(s.display_refresh_minutes)
      })
      .catch(console.error)
  }, [])

  if (minutes === null) return null

  const valid = minutes.trim() !== '' && Number.isFinite(Number(minutes))

  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!valid) return
    setError(null)
    try {
      const s = await adminUpdateSettings({ display_refresh_minutes: Number(minutes) })
      setSaved(s.display_refresh_minutes)
      setMinutes(String(s.display_refresh_minutes))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Save failed')
    }
  }

  return (
    <form onSubmit={save} className="bg-white rounded-xl border border-gray-200 p-5 space-y-2">
      <label htmlFor="refresh-minutes" className="block text-sm font-medium text-gray-800">
        Refresh during open hours
      </label>
      <div className="flex items-center gap-2">
        <input
          id="refresh-minutes"
          type="number"
          min={1}
          max={60}
          required
          value={minutes}
          onChange={(e) => setMinutes(e.target.value)}
          className="w-20 rounded-lg border border-gray-300 px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
        />
        <span className="text-sm text-gray-600">minutes</span>
        <button
          type="submit"
          disabled={!valid || Number(minutes) === saved}
          className="bg-blue-600 text-white rounded-lg px-3 py-1.5 text-sm font-medium hover:bg-blue-700 disabled:opacity-50 transition-colors"
        >
          Save
        </button>
      </div>
      <p className="text-xs text-gray-500">
        Displays also wake at the start of each hour and at booking start and end times. Outside opening hours they
        check in hourly. Shorter intervals show changes sooner but use more battery.
      </p>
      {error && <p className="text-xs text-red-600">{error}</p>}
    </form>
  )
}

export default function AdminDisplays() {
  const [displays, setDisplays] = useState<Display[]>([])
  const [loading, setLoading] = useState(true)
  const [previewURL, setPreviewURL] = useState(adminDisplayPreviewURL)

  const load = () =>
    adminListDisplays()
      .then(setDisplays)
      .catch(console.error)
      .finally(() => setLoading(false))

  useEffect(() => {
    load()
    // Pending devices show up within a minute of setup; keep the list fresh.
    const id = setInterval(load, 30_000)
    return () => clearInterval(id)
  }, [])

  return (
    <div className="p-4 sm:p-8 space-y-6">
      <AdminPageHeader title="Displays" />

      <section className="space-y-2">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-semibold text-gray-800">Preview</h2>
          <button onClick={() => setPreviewURL(adminDisplayPreviewURL())} className="text-xs text-blue-600 hover:underline">
            Refresh preview
          </button>
        </div>
        <div className="bg-white rounded-xl border border-gray-200 p-3">
          <img
            src={previewURL}
            alt="What approved displays are showing now"
            width={800}
            height={480}
            className="w-full max-w-[800px] h-auto border border-gray-300 [image-rendering:pixelated]"
          />
        </div>
      </section>

      <RefreshSetting />

      <section className="space-y-3">
        <h2 className="text-sm font-semibold text-gray-800">Devices</h2>
        {loading ? (
          <div className="flex items-center justify-center h-32">
            <div className="animate-spin rounded-full h-7 w-7 border-b-2 border-gray-400" />
          </div>
        ) : displays.length === 0 ? (
          <div className="bg-white rounded-xl border border-dashed border-gray-300 p-6 text-sm text-gray-600 space-y-2">
            <p className="font-medium text-gray-800">No displays yet.</p>
            <p>
              During the display's Wi-Fi setup, choose <span className="font-medium">Custom Server</span> and enter this
              site's address (<span className="font-mono">{window.location.origin}</span>). It will appear here as
              pending within a minute.
            </p>
          </div>
        ) : (
          displays.map((d) => (
            <DisplayCard
              key={d.id}
              display={d}
              onChange={(u) => setDisplays((prev) => prev.map((x) => (x.id === u.id ? u : x)))}
              onDelete={(id) => setDisplays((prev) => prev.filter((x) => x.id !== id))}
            />
          ))
        )}
      </section>
    </div>
  )
}
