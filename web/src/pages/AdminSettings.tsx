import { useEffect, useState } from 'react'
import { adminGetSettings, adminUpdateSettings, ApiError } from '../lib/api'
import type { Settings } from '../lib/types'

export default function AdminSettings() {
  const [settings, setSettings] = useState<Settings | null>(null)
  const [form, setForm] = useState<Partial<Settings>>({})
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    adminGetSettings()
      .then((s) => {
        setSettings(s)
        setForm(s)
      })
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)
    setError(null)
    setSaved(false)
    try {
      const updated = await adminUpdateSettings(form)
      setSettings(updated)
      setForm(updated)
      setSaved(true)
      setTimeout(() => setSaved(false), 3000)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Save failed')
    } finally {
      setSaving(false)
    }
  }

  const field = (key: keyof Settings) => ({
    value: (form[key] ?? '') as string,
    onChange: (e: React.ChangeEvent<HTMLInputElement>) =>
      setForm((prev) => ({ ...prev, [key]: e.target.value })),
  })

  const numberField = (key: keyof Settings) => ({
    value: (form[key] ?? '') as number,
    onChange: (e: React.ChangeEvent<HTMLInputElement>) =>
      setForm((prev) => ({ ...prev, [key]: parseInt(e.target.value) })),
  })

  if (loading || !settings) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-7 w-7 border-b-2 border-gray-400" />
      </div>
    )
  }

  return (
    <div className="p-4 sm:p-8 max-w-2xl">
      <h1 className="text-2xl font-bold text-gray-900 mb-6">Settings</h1>
      <form onSubmit={handleSubmit} className="space-y-6">
        <section>
          <h2 className="text-sm font-semibold text-gray-500 uppercase tracking-wider mb-3">Resource</h2>
          <div className="bg-white rounded-xl border border-gray-200 p-5 space-y-4">
            <Field label="Resource name" hint="Used in emails and the booking page">
              <input type="text" required className={inputClass} {...field('resource_name')} />
            </Field>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <Field label="Hourly rate (cents)">
                <input type="number" min="0" required className={inputClass} {...numberField('hourly_rate_cents')} />
              </Field>
              <Field label="Currency">
                <input type="text" maxLength={3} className={inputClass} {...field('currency')} placeholder="usd" />
              </Field>
            </div>
            <Field label="Timezone">
              <input type="text" className={inputClass} {...field('timezone')} placeholder="America/Denver" />
            </Field>
          </div>
        </section>

        <section>
          <h2 className="text-sm font-semibold text-gray-500 uppercase tracking-wider mb-3">Booking hours</h2>
          <div className="bg-white rounded-xl border border-gray-200 p-5 space-y-4">
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <Field label="Open time (HH:MM)">
                <input type="time" required className={inputClass} {...field('bookable_start')} />
              </Field>
              <Field label="Close time (HH:MM)">
                <input type="time" required className={inputClass} {...field('bookable_end')} />
              </Field>
            </div>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <Field label="Min hours">
                <input type="number" min="1" required className={inputClass} {...numberField('min_hours')} />
              </Field>
              <Field label="Max hours">
                <input type="number" min="1" required className={inputClass} {...numberField('max_hours')} />
              </Field>
            </div>
          </div>
        </section>

        <section>
          <h2 className="text-sm font-semibold text-gray-500 uppercase tracking-wider mb-3">Billing</h2>
          <div className="bg-white rounded-xl border border-gray-200 p-5 space-y-4">
            <Field
              label="Auto-charge delay (minutes)"
              hint="How long after a session is marked complete before the card is automatically charged. Default 1440 (24 hours). Set lower (e.g. 30) for testing."
            >
              <input type="number" min="1" required className={inputClass} {...numberField('auto_charge_delay_minutes')} />
            </Field>
          </div>
        </section>

        <section>
          <h2 className="text-sm font-semibold text-gray-500 uppercase tracking-wider mb-3">Marketing</h2>
          <div className="bg-white rounded-xl border border-gray-200 p-5 space-y-4">
            <Field
              label="Referral sources"
              hint="Shown as an optional question on the booking page. Leave empty to hide the question. Include “Other” to allow a free-text response."
            >
              <ReferralSourcesEditor
                sources={(form.referral_sources ?? []) as string[]}
                onChange={(referral_sources) => setForm((prev) => ({ ...prev, referral_sources }))}
              />
            </Field>
          </div>
        </section>

        <section>
          <h2 className="text-sm font-semibold text-gray-500 uppercase tracking-wider mb-3">Emails</h2>
          <div className="bg-white rounded-xl border border-gray-200 p-5 space-y-4">
            <Field label="Send reminder N hours before" hint="0 to disable">
              <input type="number" min="0" required className={inputClass} {...numberField('reminder_hours_before')} />
            </Field>
            <Field
              label="Staff notification emails"
              hint="Comma-separated list of emails to notify on new bookings and completed sessions."
            >
              <textarea
                rows={3}
                value={(form.notification_emails ?? '') as string}
                onChange={(e) => setForm((prev) => ({ ...prev, notification_emails: e.target.value }))}
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 resize-none"
                placeholder="staff@example.com, owner@example.com"
              />
            </Field>
          </div>
        </section>

        <section>
          <h2 className="text-sm font-semibold text-gray-500 uppercase tracking-wider mb-3">Legal</h2>
          <div className="bg-white rounded-xl border border-gray-200 p-5 space-y-4">
            <Field
              label="Terms & Conditions content"
              hint="Shown on the public Terms page and linked from the booking form."
            >
              <textarea
                rows={10}
                value={(form.terms_content ?? '') as string}
                onChange={(e) => setForm((prev) => ({ ...prev, terms_content: e.target.value }))}
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 resize-y font-mono"
                placeholder="Enter your terms and conditions…"
              />
            </Field>
            <Field
              label="Privacy Policy content"
              hint="Shown on the public Privacy Policy page and linked from the booking form."
            >
              <textarea
                rows={10}
                value={(form.privacy_content ?? '') as string}
                onChange={(e) => setForm((prev) => ({ ...prev, privacy_content: e.target.value }))}
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 resize-y font-mono"
                placeholder="Enter your privacy policy…"
              />
            </Field>
          </div>
        </section>

        {error && (
          <div className="rounded-lg bg-red-50 border border-red-200 text-red-700 text-sm px-3 py-2">{error}</div>
        )}

        <div className="flex items-center gap-3">
          <button
            type="submit"
            disabled={saving}
            className="bg-blue-600 text-white rounded-lg px-5 py-2 font-medium text-sm hover:bg-blue-700 disabled:opacity-50 transition-colors"
          >
            {saving ? 'Saving…' : 'Save settings'}
          </button>
          {saved && <span className="text-sm text-green-600 font-medium">Saved!</span>}
        </div>
      </form>
    </div>
  )
}

const inputClass =
  'w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500'

function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: React.ReactNode
}) {
  return (
    <div>
      <label className="block text-sm font-medium text-gray-700 mb-1">{label}</label>
      {children}
      {hint && <p className="text-xs text-gray-400 mt-1">{hint}</p>}
    </div>
  )
}

function ReferralSourcesEditor({
  sources,
  onChange,
}: {
  sources: string[]
  onChange: (sources: string[]) => void
}) {
  const [newSource, setNewSource] = useState('')

  const addSource = () => {
    const trimmed = newSource.trim()
    if (!trimmed || sources.includes(trimmed)) return
    onChange([...sources, trimmed])
    setNewSource('')
  }

  const removeSource = (index: number) => {
    onChange(sources.filter((_, i) => i !== index))
  }

  const moveSource = (index: number, direction: -1 | 1) => {
    const next = index + direction
    if (next < 0 || next >= sources.length) return
    const updated = [...sources]
    ;[updated[index], updated[next]] = [updated[next], updated[index]]
    onChange(updated)
  }

  return (
    <div className="space-y-3">
      {sources.length === 0 ? (
        <p className="text-sm text-gray-400">No referral sources configured.</p>
      ) : (
        <ul className="space-y-2">
          {sources.map((source, index) => (
            <li
              key={`${source}-${index}`}
              className="flex items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 bg-gray-50"
            >
              <span className="flex-1 text-sm text-gray-800">{source}</span>
              <button
                type="button"
                onClick={() => moveSource(index, -1)}
                disabled={index === 0}
                className="text-xs text-gray-500 hover:text-gray-800 disabled:opacity-30 px-1"
                aria-label="Move up"
              >
                ↑
              </button>
              <button
                type="button"
                onClick={() => moveSource(index, 1)}
                disabled={index === sources.length - 1}
                className="text-xs text-gray-500 hover:text-gray-800 disabled:opacity-30 px-1"
                aria-label="Move down"
              >
                ↓
              </button>
              <button
                type="button"
                onClick={() => removeSource(index)}
                className="text-xs text-red-600 hover:text-red-800 px-1"
                aria-label="Remove"
              >
                Remove
              </button>
            </li>
          ))}
        </ul>
      )}
      <div className="flex gap-2">
        <input
          type="text"
          value={newSource}
          onChange={(e) => setNewSource(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              addSource()
            }
          }}
          className={inputClass}
          placeholder="Add a source…"
        />
        <button
          type="button"
          onClick={addSource}
          disabled={!newSource.trim()}
          className="shrink-0 rounded-lg border border-gray-300 px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50"
        >
          Add
        </button>
      </div>
    </div>
  )
}
