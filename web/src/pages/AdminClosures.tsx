import { useEffect, useState } from 'react'
import { adminListClosures, adminCreateClosure, adminUpdateClosure, adminDeleteClosure, ApiError } from '../lib/api'
import type { Closure } from '../lib/types'

interface ClosureForm {
  start_date: string
  end_date: string
  reason: string
}

const emptyForm: ClosureForm = { start_date: '', end_date: '', reason: '' }

function ClosureFormPanel({
  initial,
  onSave,
  onCancel,
}: {
  initial?: Closure
  onSave: (data: ClosureForm) => Promise<void>
  onCancel: () => void
}) {
  const [form, setForm] = useState<ClosureForm>(
    initial
      ? { start_date: initial.start_date, end_date: initial.end_date, reason: initial.reason ?? '' }
      : emptyForm,
  )
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)
    setError(null)
    try {
      await onSave(form)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Save failed')
    } finally {
      setSaving(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="bg-gray-50 rounded-xl border border-gray-200 p-5 space-y-3">
      <h3 className="font-semibold text-gray-800 text-sm">{initial ? 'Edit Closure' : 'New Closure'}</h3>
      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="block text-xs font-medium text-gray-600 mb-1">Start date</label>
          <input
            type="date"
            required
            value={form.start_date}
            onChange={(e) => setForm({ ...form, start_date: e.target.value })}
            className="w-full rounded-lg border border-gray-300 px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </div>
        <div>
          <label className="block text-xs font-medium text-gray-600 mb-1">End date</label>
          <input
            type="date"
            required
            value={form.end_date}
            onChange={(e) => setForm({ ...form, end_date: e.target.value })}
            className="w-full rounded-lg border border-gray-300 px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </div>
      </div>
      <div>
        <label className="block text-xs font-medium text-gray-600 mb-1">Reason (optional)</label>
        <input
          type="text"
          value={form.reason}
          onChange={(e) => setForm({ ...form, reason: e.target.value })}
          className="w-full rounded-lg border border-gray-300 px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          placeholder="e.g. Holiday, Maintenance"
        />
      </div>
      {error && (
        <div className="rounded-lg bg-red-50 border border-red-200 text-red-700 text-xs px-3 py-2">{error}</div>
      )}
      <div className="flex gap-2">
        <button
          type="submit"
          disabled={saving}
          className="bg-blue-600 text-white rounded-lg px-4 py-1.5 text-sm font-medium hover:bg-blue-700 disabled:opacity-50 transition-colors"
        >
          {saving ? 'Saving…' : 'Save'}
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="bg-gray-100 text-gray-700 rounded-lg px-4 py-1.5 text-sm hover:bg-gray-200 transition-colors"
        >
          Cancel
        </button>
      </div>
    </form>
  )
}

export default function AdminClosures() {
  const [closures, setClosures] = useState<Closure[]>([])
  const [loading, setLoading] = useState(true)
  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState<Closure | null>(null)

  const load = () => {
    setLoading(true)
    adminListClosures()
      .then(setClosures)
      .catch(console.error)
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [])

  const handleCreate = async (form: ClosureForm) => {
    const closure = await adminCreateClosure({
      start_date: form.start_date,
      end_date: form.end_date,
      reason: form.reason || undefined,
    })
    setClosures((prev) => [...prev, closure].sort((a, b) => a.start_date.localeCompare(b.start_date)))
    setCreating(false)
  }

  const handleUpdate = async (form: ClosureForm) => {
    if (!editing) return
    const updated = await adminUpdateClosure(editing.id, {
      start_date: form.start_date,
      end_date: form.end_date,
      reason: form.reason || undefined,
    })
    setClosures((prev) => prev.map((c) => (c.id === updated.id ? updated : c)))
    setEditing(null)
  }

  const handleDelete = async (id: number) => {
    if (!confirm('Delete this closure?')) return
    await adminDeleteClosure(id)
    setClosures((prev) => prev.filter((c) => c.id !== id))
  }

  return (
    <div className="p-8">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold text-gray-900">Closures</h1>
        <button
          onClick={() => { setCreating(true); setEditing(null) }}
          className="bg-blue-600 text-white rounded-lg px-4 py-2 text-sm font-medium hover:bg-blue-700 transition-colors"
        >
          + Add closure
        </button>
      </div>

      {creating && (
        <div className="mb-6">
          <ClosureFormPanel onSave={handleCreate} onCancel={() => setCreating(false)} />
        </div>
      )}

      {loading ? (
        <div className="flex items-center justify-center h-32">
          <div className="animate-spin rounded-full h-7 w-7 border-b-2 border-gray-400" />
        </div>
      ) : closures.length === 0 ? (
        <p className="text-sm text-gray-400 py-8 text-center">No closures defined.</p>
      ) : (
        <div className="space-y-3">
          {closures.map((closure) => (
            <div key={closure.id}>
              {editing?.id === closure.id ? (
                <ClosureFormPanel
                  initial={closure}
                  onSave={handleUpdate}
                  onCancel={() => setEditing(null)}
                />
              ) : (
                <div className="bg-white rounded-xl border border-gray-200 px-5 py-4 flex items-center justify-between">
                  <div>
                    <p className="text-sm font-medium text-gray-900">
                      {closure.start_date === closure.end_date
                        ? closure.start_date
                        : `${closure.start_date} – ${closure.end_date}`}
                    </p>
                    {closure.reason && (
                      <p className="text-xs text-gray-500 mt-0.5">{closure.reason}</p>
                    )}
                  </div>
                  <div className="flex gap-2">
                    <button
                      onClick={() => { setEditing(closure); setCreating(false) }}
                      className="text-xs text-blue-600 hover:underline"
                    >
                      Edit
                    </button>
                    <button
                      onClick={() => handleDelete(closure.id)}
                      className="text-xs text-red-500 hover:underline"
                    >
                      Delete
                    </button>
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
