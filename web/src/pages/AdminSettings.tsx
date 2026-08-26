import { useEffect, useState } from 'react'
import { adminGetSettings, adminUpdateSettings, ApiError } from '../lib/api'
import type { BookingScreening, BookingScreeningOutcome, Settings } from '../lib/types'
import { RichTextEditor } from '../components/RichTextEditor'

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
            <Field
              label="Minimum booking lead time (minutes)"
              hint="Minimum time before a slot starts that it can be booked. 0 to allow booking up to the last minute."
            >
              <input type="number" min="0" required className={inputClass} {...numberField('min_booking_lead_minutes')} />
            </Field>
          </div>
        </section>

        <section>
          <h2 className="text-sm font-semibold text-gray-500 uppercase tracking-wider mb-3">Booking screening</h2>
          <div className="bg-white rounded-xl border border-gray-200 p-5 space-y-4">
            <label className="flex items-center gap-2 text-sm text-gray-700">
              <input
                type="checkbox"
                checked={Boolean(form.booking_screening?.enabled)}
                onChange={(e) => {
                  if (e.target.checked) {
                    setForm((prev) => ({
                      ...prev,
                      booking_screening: { ...(prev.booking_screening ?? defaultBookingScreening()), enabled: true },
                    }))
                  } else {
                    setForm((prev) => ({
                      ...prev,
                      booking_screening: prev.booking_screening
                        ? { ...prev.booking_screening, enabled: false }
                        : { ...defaultBookingScreening(), enabled: false },
                    }))
                  }
                }}
                className="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
              />
              Enable pre-booking screening question
            </label>

            {form.booking_screening?.enabled && (
              <>
                <Field label="Title">
                  <input
                    type="text"
                    className={inputClass}
                    value={form.booking_screening.title}
                    onChange={(e) =>
                      setForm((prev) => ({
                        ...prev,
                        booking_screening: { ...prev.booking_screening!, title: e.target.value },
                      }))
                    }
                  />
                </Field>
                <Field label="Description">
                  <input
                    type="text"
                    className={inputClass}
                    value={form.booking_screening.description}
                    onChange={(e) =>
                      setForm((prev) => ({
                        ...prev,
                        booking_screening: { ...prev.booking_screening!, description: e.target.value },
                      }))
                    }
                  />
                </Field>
                <Field label="Question">
                  <input
                    type="text"
                    className={inputClass}
                    value={form.booking_screening.question}
                    onChange={(e) =>
                      setForm((prev) => ({
                        ...prev,
                        booking_screening: { ...prev.booking_screening!, question: e.target.value },
                      }))
                    }
                  />
                </Field>
                <Field label="Answer options" hint="Each option maps to either proceeding to booking or collecting contact info.">
                  <ScreeningOptionsEditor
                    options={form.booking_screening.options}
                    onChange={(options) =>
                      setForm((prev) => ({
                        ...prev,
                        booking_screening: { ...prev.booking_screening!, options },
                      }))
                    }
                  />
                </Field>
                <Field label="Collect-info heading">
                  <input
                    type="text"
                    className={inputClass}
                    value={form.booking_screening.collect_info_heading}
                    onChange={(e) =>
                      setForm((prev) => ({
                        ...prev,
                        booking_screening: { ...prev.booking_screening!, collect_info_heading: e.target.value },
                      }))
                    }
                  />
                </Field>
                <Field label="Collect-info description">
                  <textarea
                    rows={2}
                    className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 resize-none"
                    value={form.booking_screening.collect_info_description}
                    onChange={(e) =>
                      setForm((prev) => ({
                        ...prev,
                        booking_screening: { ...prev.booking_screening!, collect_info_description: e.target.value },
                      }))
                    }
                  />
                </Field>
              </>
            )}
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
              <RichTextEditor
                value={(form.terms_content ?? '') as string}
                onChange={(terms_content) => setForm((prev) => ({ ...prev, terms_content }))}
                placeholder="Enter your terms and conditions…"
              />
            </Field>
            <Field
              label="Privacy Policy content"
              hint="Shown on the public Privacy Policy page and linked from the booking form."
            >
              <RichTextEditor
                value={(form.privacy_content ?? '') as string}
                onChange={(privacy_content) => setForm((prev) => ({ ...prev, privacy_content }))}
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

function defaultBookingScreening(): BookingScreening {
  return {
    enabled: false,
    title: 'Before you book',
    description: '',
    question: '',
    options: [],
    collect_info_heading: "Let's get you started",
    collect_info_description: '',
  }
}

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

function ScreeningOptionsEditor({
  options,
  onChange,
}: {
  options: BookingScreening['options']
  onChange: (options: BookingScreening['options']) => void
}) {
  const [newLabel, setNewLabel] = useState('')
  const [newOutcome, setNewOutcome] = useState<BookingScreeningOutcome>('proceed')

  const addOption = () => {
    const trimmed = newLabel.trim()
    if (!trimmed) return
    onChange([...options, { label: trimmed, outcome: newOutcome }])
    setNewLabel('')
    setNewOutcome('proceed')
  }

  const removeOption = (index: number) => {
    onChange(options.filter((_, i) => i !== index))
  }

  const moveOption = (index: number, direction: -1 | 1) => {
    const next = index + direction
    if (next < 0 || next >= options.length) return
    const updated = [...options]
    ;[updated[index], updated[next]] = [updated[next], updated[index]]
    onChange(updated)
  }

  const updateOutcome = (index: number, outcome: BookingScreeningOutcome) => {
    onChange(options.map((opt, i) => (i === index ? { ...opt, outcome } : opt)))
  }

  return (
    <div className="space-y-3">
      {options.length === 0 ? (
        <p className="text-sm text-gray-400">No options configured.</p>
      ) : (
        <ul className="space-y-2">
          {options.map((option, index) => (
            <li
              key={`${option.label}-${index}`}
              className="flex flex-col sm:flex-row sm:items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 bg-gray-50"
            >
              <span className="flex-1 text-sm text-gray-800">{option.label}</span>
              <select
                value={option.outcome}
                onChange={(e) => updateOutcome(index, e.target.value as BookingScreeningOutcome)}
                className="rounded-lg border border-gray-300 px-2 py-1 text-sm bg-white"
              >
                <option value="proceed">Proceed to booking</option>
                <option value="collect_info">Collect contact info</option>
              </select>
              <div className="flex items-center gap-1">
                <button
                  type="button"
                  onClick={() => moveOption(index, -1)}
                  disabled={index === 0}
                  className="text-xs text-gray-500 hover:text-gray-800 disabled:opacity-30 px-1"
                  aria-label="Move up"
                >
                  ↑
                </button>
                <button
                  type="button"
                  onClick={() => moveOption(index, 1)}
                  disabled={index === options.length - 1}
                  className="text-xs text-gray-500 hover:text-gray-800 disabled:opacity-30 px-1"
                  aria-label="Move down"
                >
                  ↓
                </button>
                <button
                  type="button"
                  onClick={() => removeOption(index)}
                  className="text-xs text-red-600 hover:text-red-800 px-1"
                  aria-label="Remove"
                >
                  Remove
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
      <div className="flex flex-col sm:flex-row gap-2">
        <input
          type="text"
          value={newLabel}
          onChange={(e) => setNewLabel(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              addOption()
            }
          }}
          className={inputClass}
          placeholder="Option label…"
        />
        <select
          value={newOutcome}
          onChange={(e) => setNewOutcome(e.target.value as BookingScreeningOutcome)}
          className="rounded-lg border border-gray-300 px-3 py-2 text-sm bg-white"
        >
          <option value="proceed">Proceed to booking</option>
          <option value="collect_info">Collect contact info</option>
        </select>
        <button
          type="button"
          onClick={addOption}
          disabled={!newLabel.trim()}
          className="shrink-0 rounded-lg border border-gray-300 px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50"
        >
          Add
        </button>
      </div>
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
