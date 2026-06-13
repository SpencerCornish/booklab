import { addDays, eachDayOfInterval, format, parseISO } from 'date-fns'
import type { BookingAdmin, Closure } from './types'

export type CalendarResource =
  | { type: 'booking'; booking: BookingAdmin }
  | { type: 'closure'; closure: Closure }

export interface ScheduleCalendarEvent {
  id: string | number
  title: string
  start: Date
  end: Date
  allDay?: boolean
  resource: CalendarResource
}

function parseLocalDate(dateStr: string): Date {
  const [year, month, day] = dateStr.split('-').map(Number)
  return new Date(year, month - 1, day)
}

function parseLocalDateTime(dateStr: string, timeStr: string): Date {
  const date = parseLocalDate(dateStr)
  const [hours, minutes] = timeStr.split(':').map(Number)
  date.setHours(hours, minutes, 0, 0)
  return date
}

export function closureToCalendarEvents(closure: Closure): ScheduleCalendarEvent[] {
  const title = closure.reason?.trim() || 'Closed'
  const rangeStart = parseLocalDate(closure.start_date)
  const rangeEnd = parseLocalDate(closure.end_date)

  if (closure.all_day) {
    return [
      {
        id: `closure-${closure.id}`,
        title,
        start: rangeStart,
        end: addDays(rangeEnd, 1),
        allDay: true,
        resource: { type: 'closure', closure },
      },
    ]
  }

  if (!closure.start_time || !closure.end_time) return []

  return eachDayOfInterval({ start: rangeStart, end: rangeEnd }).map((day) => {
    const localDayStr = format(day, 'yyyy-MM-dd')

    return {
      id: `closure-${closure.id}-${localDayStr}`,
      title,
      start: parseLocalDateTime(localDayStr, closure.start_time!),
      end: parseLocalDateTime(localDayStr, closure.end_time!),
      resource: { type: 'closure', closure },
    }
  })
}

export function bookingToCalendarEvent(booking: BookingAdmin): ScheduleCalendarEvent {
  return {
    id: booking.id,
    title: booking.name,
    start: parseISO(booking.start_time),
    end: parseISO(booking.end_time),
    resource: { type: 'booking', booking },
  }
}
