import { useState } from 'react'
import type { BookingScreening, BookingScreeningOutcome } from '../lib/types'

interface BookingScreeningProps {
  config: BookingScreening
  onOutcome: (outcome: BookingScreeningOutcome, selectedLabel: string) => void
}

export function BookingScreening({ config, onOutcome }: BookingScreeningProps) {
  const [selectedIndex, setSelectedIndex] = useState<number | null>(null)

  const handleContinue = () => {
    if (selectedIndex === null) return
    const option = config.options[selectedIndex]
    onOutcome(option.outcome, option.label)
  }

  return (
    <section className="space-y-4">
      <div>
        <h2 className="text-xl font-bold text-gray-900">{config.title}</h2>
        {config.description && (
          <p className="text-sm text-gray-500 mt-1">{config.description}</p>
        )}
      </div>

      <p className="text-sm font-medium text-gray-700">{config.question}</p>

      <div className="space-y-2">
        {config.options.map((option, index) => {
          const selected = selectedIndex === index
          return (
            <button
              key={`${option.label}-${index}`}
              type="button"
              onClick={() => setSelectedIndex(index)}
              className={`w-full text-left rounded-xl border px-4 py-3 transition-colors ${
                selected
                  ? 'border-blue-600 bg-blue-50 ring-2 ring-blue-600'
                  : 'border-gray-200 bg-white hover:border-gray-300'
              }`}
            >
              <div className="flex items-start gap-3">
                <span
                  className={`mt-0.5 h-4 w-4 shrink-0 rounded-full border-2 ${
                    selected ? 'border-blue-600 bg-blue-600' : 'border-gray-300'
                  }`}
                  aria-hidden
                />
                <span className="text-sm text-gray-800">{option.label}</span>
              </div>
            </button>
          )
        })}
      </div>

      <button
        type="button"
        onClick={handleContinue}
        disabled={selectedIndex === null}
        className="w-full bg-blue-600 text-white rounded-lg py-2.5 font-medium text-sm hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      >
        Continue
      </button>
    </section>
  )
}
