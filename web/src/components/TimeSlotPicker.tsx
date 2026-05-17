import { format, parseISO } from 'date-fns'
import type { TimeSlot } from '../lib/types'

interface Props {
  slots: TimeSlot[]
  selectedStart: string | null
  selectedEnd: string | null
  minHours: number
  maxHours: number
  onSelect: (start: string, end: string) => void
  onClear?: () => void
}

export function TimeSlotPicker({ slots, selectedStart, selectedEnd, minHours, maxHours, onSelect, onClear }: Props) {
  if (slots.length === 0) {
    return (
      <p className="text-sm text-gray-500 py-4 text-center">No slots available for this date.</p>
    )
  }

  const handleSlotClick = (slot: TimeSlot) => {
    if (!slot.available) return

    if (!selectedStart) {
      // Start new selection
      onSelect(slot.start, slot.end)
      return
    }

    // Expand or reset selection
    const startIdx = slots.findIndex((s) => s.start === selectedStart)
    const clickIdx = slots.findIndex((s) => s.start === slot.start)

    if (clickIdx < startIdx) {
      // Clicked before start — reset
      onSelect(slot.start, slot.end)
      return
    }

    // Check all slots between start and click are available
    const range = slots.slice(startIdx, clickIdx + 1)
    const hours = range.length
    if (hours < minHours || hours > maxHours) return
    if (!range.every((s) => s.available)) return

    onSelect(selectedStart, slot.end)
  }

  const getSlotState = (slot: TimeSlot) => {
    if (!slot.available) return 'unavailable'
    if (!selectedStart) return 'available'

    const startIdx = slots.findIndex((s) => s.start === selectedStart)
    const slotIdx = slots.findIndex((s) => s.start === slot.start)
    const endIdx = selectedEnd ? slots.findIndex((s) => s.end === selectedEnd) : startIdx

    if (slotIdx >= startIdx && slotIdx <= endIdx) return 'selected'
    return 'available'
  }

  const stateClasses: Record<string, string> = {
    unavailable: 'bg-gray-100 text-gray-400 cursor-not-allowed line-through',
    available: 'bg-white border border-gray-200 text-gray-700 hover:border-blue-400 hover:bg-blue-50 cursor-pointer',
    selected: 'bg-blue-600 text-white border border-blue-600 cursor-pointer',
  }

  return (
    <div>
      <div className="grid grid-cols-3 sm:grid-cols-4 gap-2">
        {slots.map((slot) => {
          const state = getSlotState(slot)
          return (
            <button
              key={slot.start}
              type="button"
              onClick={() => handleSlotClick(slot)}
              disabled={state === 'unavailable'}
              className={`rounded-lg px-3 py-2 text-sm font-medium transition-all ${stateClasses[state]}`}
            >
              {format(parseISO(slot.start), 'h:mm a')}
            </button>
          )
        })}
      </div>
      {selectedStart && selectedEnd && (
        <div className="mt-3 flex items-center gap-3">
          <p className="text-sm text-blue-700 font-medium">
            Selected: {format(parseISO(selectedStart), 'h:mm a')} – {format(parseISO(selectedEnd), 'h:mm a')}
            {' '}({Math.round((new Date(selectedEnd).getTime() - new Date(selectedStart).getTime()) / 3600000)} hr)
          </p>
          {onClear && (
            <button
              type="button"
              onClick={onClear}
              className="text-xs text-gray-400 hover:text-gray-600 underline transition-colors"
            >
              Clear
            </button>
          )}
        </div>
      )}
      <p className="mt-2 text-xs text-gray-400">
        Tap a slot to start. Tap another to extend (min {minHours} hrs, max {maxHours} hrs).
      </p>
    </div>
  )
}
