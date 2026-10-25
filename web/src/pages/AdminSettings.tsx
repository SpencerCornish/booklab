import { useEffect, useState } from 'react'
import { Navigate, NavLink, useParams } from 'react-router'
import { adminGetSettings, adminUpdateSettings, ApiError } from '../lib/api'
import type { NewCustomerRestriction, NewUserGateAdmin, NewUserWindow, Settings } from '../lib/types'
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

  const sectionParam = useParams().section
  const page: SectionKey = sectionParam && isSectionKey(sectionParam) ? sectionParam : 'general'

  // Unsaved fields are those whose form value differs from the last saved settings.
  const dirtyFields = new Set<keyof Settings>(
    settings
      ? (Object.keys(form) as (keyof Settings)[]).filter(
          (key) => JSON.stringify(form[key]) !== JSON.stringify(settings[key]),
        )
      : [],
  )
  const dirty = dirtyFields.size > 0

  // Warn before a full page unload (tab close/reload) with pending edits.
  useEffect(() => {
    if (!dirty) return
    const onBeforeUnload = (e: BeforeUnloadEvent) => e.preventDefault()
    window.addEventListener('beforeunload', onBeforeUnload)
    return () => window.removeEventListener('beforeunload', onBeforeUnload)
  }, [dirty])

  if (sectionParam && !isSectionKey(sectionParam)) {
    return <Navigate to="/admin/settings/general" replace />
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    setSaved(false)

    // Blank dates mean "no restriction"; one date without the other is incomplete.
    const r = form.new_customer_restriction
    let payload = form
    if (r && !r.start_date && !r.end_date) {
      payload = { ...form, new_customer_restriction: null }
    } else if (r && (!r.start_date || !r.end_date)) {
      setError('Set both new customer restriction dates, or clear the restriction.')
      return
    }

    setSaving(true)
    try {
      const updated = await adminUpdateSettings(payload)
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

  const restriction = form.new_customer_restriction ?? null

  const updateRestriction = (patch: Partial<NewCustomerRestriction>) =>
    setForm((prev) => ({
      ...prev,
      new_customer_restriction: { start_date: '', end_date: '', message: '', ...prev.new_customer_restriction, ...patch },
    }))

  const gate: NewUserGateAdmin = form.new_user_gate ?? {
    enabled: false,
    question: DEFAULT_NEW_USER_QUESTION,
    schedule: [],
  }

  const updateGate = (patch: Partial<NewUserGateAdmin>) =>
    setForm((prev) => {
      const current: NewUserGateAdmin = prev.new_user_gate ?? {
        enabled: false,
        question: DEFAULT_NEW_USER_QUESTION,
        schedule: [],
      }
      return { ...prev, new_user_gate: { ...current, ...patch } }
    })

  if (loading || !settings) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-7 w-7 border-b-2 border-gray-400" />
      </div>
    )
  }

  return (
    <div className="p-4 sm:p-8">
      <div className="max-w-3xl">
        <div className="flex flex-wrap items-center gap-3 mb-4">
          <h1 className="text-2xl font-bold text-gray-900">Settings</h1>
          {dirty && (
            <span className="inline-flex items-center gap-1.5 rounded-full bg-amber-100 px-2.5 py-1 text-xs font-medium text-amber-800">
              <span className="h-1.5 w-1.5 rounded-full bg-amber-500" />
              Unsaved changes
            </span>
          )}
        </div>
        <nav aria-label="Settings sections" className="flex flex-wrap gap-1 border-b border-gray-200 mb-6">
          {SECTIONS.map((s) => (
            <NavLink
              key={s.key}
              to={`/admin/settings/${s.key}`}
              className={({ isActive }) =>
                `-mb-px flex items-center gap-1.5 border-b-2 px-3 py-2 text-sm font-medium transition-colors ${
                  isActive
                    ? 'border-gray-900 text-gray-900'
                    : 'border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-800'
                }`
              }
            >
              {s.label}
              {s.fields.some((field) => dirtyFields.has(field)) && (
                <span
                  className="h-1.5 w-1.5 rounded-full bg-amber-500"
                  title="Unsaved changes in this section"
                />
              )}
            </NavLink>
          ))}
        </nav>
        <form onSubmit={handleSubmit} className="space-y-6 max-w-2xl">
        {page === 'general' && (
          <>
          <section id="resource">
          <h2 className="text-lg font-semibold text-gray-900 mb-3">Resource</h2>
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

        <section id="booking-hours" className="scroll-mt-16 md:scroll-mt-6">
          <h2 className="text-lg font-semibold text-gray-900 mb-3">Booking hours</h2>
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
            <Field label="Minimum notice (hours)" hint="Bookings must start at least this many hours from now. 0 allows booking right up to the start time.">
              <input type="number" min="0" required className={inputClass} {...numberField('min_lead_hours')} />
            </Field>
          </div>
        </section>
          </>
        )}

        {page === 'booking-rules' && (
          <>
        <section id="new-customer-restriction" className="scroll-mt-16 md:scroll-mt-6">
          <h2 className="text-lg font-semibold text-gray-900 mb-3">New customer restriction</h2>
          <div className="bg-white rounded-xl border border-gray-200 p-5 space-y-4">
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <Field label="Start date">
                <input
                  type="date"
                  className={inputClass}
                  value={restriction?.start_date ?? ''}
                  onChange={(e) => updateRestriction({ start_date: e.target.value })}
                />
              </Field>
              <Field label="End date">
                <input
                  type="date"
                  className={inputClass}
                  value={restriction?.end_date ?? ''}
                  onChange={(e) => updateRestriction({ end_date: e.target.value })}
                />
              </Field>
            </div>
            <Field
              label="Message"
              hint="Shown to new customers in a popup on the booking page, and as the error when they try to book these dates. Leave empty for a default message."
            >
              <textarea
                rows={3}
                value={restriction?.message ?? ''}
                onChange={(e) => updateRestriction({ message: e.target.value })}
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 resize-none"
                placeholder="We're only accepting returning customers during these dates."
              />
            </Field>
            <div className="flex items-start justify-between gap-3">
              <p className="text-xs text-gray-400">
                Returning customers (with a past completed booking) can still book these dates.
              </p>
              <button
                type="button"
                onClick={() => setForm((prev) => ({ ...prev, new_customer_restriction: null }))}
                disabled={!restriction}
                className="shrink-0 text-sm text-red-600 hover:text-red-800 disabled:opacity-40"
              >
                Clear
              </button>
            </div>
          </div>
        </section>

        <section id="new-user-gate" className="scroll-mt-16 md:scroll-mt-6">
          <h2 className="text-lg font-semibold text-gray-900 mb-3">New user gate</h2>
          <div className="bg-white rounded-xl border border-gray-200 p-5 space-y-4">
            <label className="flex items-start gap-2 text-sm text-gray-700">
              <input
                type="checkbox"
                checked={gate.enabled}
                onChange={(e) => updateGate({ enabled: e.target.checked })}
                className="mt-0.5 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
              />
              <span>
                Ask a question before booking and send self-reported new customers to trainer hours.
              </span>
            </label>
            <Field
              label="Question"
              hint="Shown before the date picker. Phrase it so “Yes” means the customer has used the space before."
            >
              <input
                type="text"
                className={inputClass}
                value={gate.question}
                onChange={(e) => updateGate({ question: e.target.value })}
                placeholder={DEFAULT_NEW_USER_QUESTION}
              />
            </Field>
            <Field
              label="Trainer hours"
              hint="New customers can only book during these weekly hours. Normal booking hours do not apply to them."
            >
              <NewUserScheduleEditor
                windows={gate.schedule}
                onChange={(schedule) => updateGate({ schedule })}
              />
            </Field>
            {gate.enabled && gate.schedule.length === 0 && (
              <p className="text-xs text-amber-600">
                No trainer hours configured: new customers will see no available slots.
              </p>
            )}
          </div>
        </section>

          </>
        )}

        {page === 'operations' && (
          <>
        <section id="billing" className="scroll-mt-16 md:scroll-mt-6">
          <h2 className="text-lg font-semibold text-gray-900 mb-3">Billing</h2>
          <div className="bg-white rounded-xl border border-gray-200 p-5 space-y-4">
            <Field
              label="Auto-charge delay (minutes)"
              hint="How long after a session is marked complete before the card is automatically charged. Default 1440 (24 hours). Set lower (e.g. 30) for testing."
            >
              <input type="number" min="1" required className={inputClass} {...numberField('auto_charge_delay_minutes')} />
            </Field>
          </div>
        </section>

        <section id="marketing" className="scroll-mt-16 md:scroll-mt-6">
          <h2 className="text-lg font-semibold text-gray-900 mb-3">Marketing</h2>
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

        <section id="emails" className="scroll-mt-16 md:scroll-mt-6">
          <h2 className="text-lg font-semibold text-gray-900 mb-3">Emails</h2>
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
          </>
        )}

        {page === 'content' && (
          <>
        <section id="legal" className="scroll-mt-16 md:scroll-mt-6">
          <h2 className="text-lg font-semibold text-gray-900 mb-3">Legal</h2>
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
          </>
        )}

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
    </div>
  )
}

const inputClass =
  'w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500'

// Settings subsections, each rendered at /admin/settings/<key>. The keys list
// which fields belong to the subsection so the tabs can flag unsaved edits.
type SectionKey = 'general' | 'booking-rules' | 'operations' | 'content'

const SECTIONS: { key: SectionKey; label: string; fields: (keyof Settings)[] }[] = [
  {
    key: 'general',
    label: 'General',
    fields: [
      'resource_name',
      'hourly_rate_cents',
      'currency',
      'timezone',
      'bookable_start',
      'bookable_end',
      'min_hours',
      'max_hours',
      'min_lead_hours',
    ],
  },
  {
    key: 'booking-rules',
    label: 'Booking rules',
    fields: ['new_customer_restriction', 'new_user_gate'],
  },
  {
    key: 'operations',
    label: 'Operations',
    fields: ['auto_charge_delay_minutes', 'referral_sources', 'reminder_hours_before', 'notification_emails'],
  },
  {
    key: 'content',
    label: 'Content',
    fields: ['terms_content', 'privacy_content'],
  },
]

const isSectionKey = (value: string): value is SectionKey =>
  SECTIONS.some((section) => section.key === value)


const DEFAULT_NEW_USER_QUESTION = 'Have you used this space before?'

const WEEKDAYS = [
  { value: 1, label: 'Monday' },
  { value: 2, label: 'Tuesday' },
  { value: 3, label: 'Wednesday' },
  { value: 4, label: 'Thursday' },
  { value: 5, label: 'Friday' },
  { value: 6, label: 'Saturday' },
  { value: 0, label: 'Sunday' },
]

function NewUserScheduleEditor({
  windows,
  onChange,
}: {
  windows: NewUserWindow[]
  onChange: (windows: NewUserWindow[]) => void
}) {
  const byWeekday = new Map(windows.map((w) => [w.weekday, w]))

  const setWindow = (weekday: number, patch: Partial<NewUserWindow>) => {
    const current = byWeekday.get(weekday) ?? { weekday, start: '09:00', end: '17:00' }
    const next: NewUserWindow = { ...current, ...patch, weekday }
    onChange([...windows.filter((w) => w.weekday !== weekday), next].sort((a, b) => a.weekday - b.weekday))
  }

  const toggle = (weekday: number, enabled: boolean) => {
    if (enabled) {
      if (!byWeekday.has(weekday)) {
        onChange([...windows, { weekday, start: '09:00', end: '17:00' }].sort((a, b) => a.weekday - b.weekday))
      }
    } else {
      onChange(windows.filter((w) => w.weekday !== weekday))
    }
  }

  return (
    <div className="space-y-2">
      {WEEKDAYS.map(({ value, label }) => {
        const dayWindow = byWeekday.get(value)
        return (
          <div key={value} className="flex flex-wrap items-center gap-2">
            <label className="flex items-center gap-2 w-28 shrink-0 text-sm text-gray-700">
              <input
                type="checkbox"
                checked={!!dayWindow}
                onChange={(e) => toggle(value, e.target.checked)}
                className="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
              />
              {label}
            </label>
            {dayWindow ? (
              <>
                <input
                  type="time"
                  value={dayWindow.start}
                  onChange={(e) => setWindow(value, { start: e.target.value })}
                  className="rounded-lg border border-gray-300 px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
                <span className="text-sm text-gray-400">to</span>
                <input
                  type="time"
                  value={dayWindow.end}
                  onChange={(e) => setWindow(value, { end: e.target.value })}
                  className="rounded-lg border border-gray-300 px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
              </>
            ) : (
              <span className="text-xs text-gray-400">Unavailable</span>
            )}
          </div>
        )
      })}
    </div>
  )
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
