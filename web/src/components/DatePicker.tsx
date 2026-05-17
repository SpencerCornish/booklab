import { addDays, format, isBefore, startOfDay } from 'date-fns'

interface Props {
  selected: string // YYYY-MM-DD
  onChange: (date: string) => void
  daysAhead?: number
}

export function DatePicker({ selected, onChange, daysAhead = 60 }: Props) {
  const today = startOfDay(new Date())
  const days: Date[] = []
  for (let i = 0; i < daysAhead; i++) {
    days.push(addDays(today, i))
  }

  return (
    <div className="overflow-x-auto">
      <div className="flex gap-2 pb-2" style={{ width: 'max-content' }}>
        {days.map((day) => {
          const value = format(day, 'yyyy-MM-dd')
          const isSelected = value === selected
          const isPast = isBefore(day, today)
          return (
            <button
              key={value}
              type="button"
              disabled={isPast}
              onClick={() => onChange(value)}
              className={`flex flex-col items-center justify-center rounded-xl px-3 py-2 min-w-[56px] transition-all ${
                isSelected
                  ? 'bg-blue-600 text-white shadow-md'
                  : isPast
                    ? 'opacity-40 cursor-not-allowed text-gray-400 bg-white'
                    : 'bg-white border border-gray-200 text-gray-700 hover:border-blue-400'
              }`}
            >
              <span className="text-xs font-medium uppercase">{format(day, 'EEE')}</span>
              <span className="text-lg font-bold leading-none mt-0.5">{format(day, 'd')}</span>
              <span className="text-xs mt-0.5">{format(day, 'MMM')}</span>
            </button>
          )
        })}
      </div>
    </div>
  )
}
