// Helpers for deterministic booking dates/times.
//
// The app's default settings timezone is America/Denver and Playwright runs the
// browser in that zone too (see playwright.config.ts), so "Denver calendar
// date" is what the API validates and what the UI renders.

const APP_TZ = 'America/Denver'

function tzOffsetMinutes(instant: Date, timeZone: string): number {
  const dtf = new Intl.DateTimeFormat('en-US', {
    timeZone,
    hour12: false,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
  const parts = Object.fromEntries(dtf.formatToParts(instant).map((p) => [p.type, p.value]))
  const asUTC = Date.UTC(
    Number(parts.year),
    Number(parts.month) - 1,
    Number(parts.day),
    Number(parts.hour) % 24,
    Number(parts.minute),
    Number(parts.second),
  )
  return (asUTC - instant.getTime()) / 60000
}

/** A YYYY-MM-DD calendar date `daysAhead` days from now, in the app timezone. */
export function futureDate(daysAhead: number): string {
  const fmt = new Intl.DateTimeFormat('en-CA', {
    timeZone: APP_TZ,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  })
  return fmt.format(new Date(Date.now() + daysAhead * 86_400_000))
}

/** The RFC3339 instant where it is `hour` o'clock (app timezone) on `dateStr`. */
export function denverTime(dateStr: string, hour: number): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  const guess = new Date(`${dateStr}T${pad(hour)}:00:00Z`)
  // Offset can differ around DST transitions; resolve twice and settle.
  let instant = new Date(guess.getTime() - tzOffsetMinutes(guess, APP_TZ) * 60_000)
  instant = new Date(guess.getTime() - tzOffsetMinutes(instant, APP_TZ) * 60_000)
  return instant.toISOString()
}

/** The DatePicker button label for a date, e.g. "Mon 23 Nov" (browser TZ = app TZ). */
export function dateButtonLabel(dateStr: string): string {
  const [y, m, d] = dateStr.split('-').map(Number)
  const fmt = new Intl.DateTimeFormat('en-US', {
    timeZone: 'UTC',
    weekday: 'short',
    month: 'short',
  })
  const parts = fmt.formatToParts(new Date(Date.UTC(y, m - 1, d, 12)))
  const weekday = parts.find((p) => p.type === 'weekday')!.value
  const month = parts.find((p) => p.type === 'month')!.value
  return `${weekday} ${d} ${month}`
}