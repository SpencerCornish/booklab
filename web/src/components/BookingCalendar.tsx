import { useMemo, useState } from 'react'
import { Calendar, dateFnsLocalizer, type CalendarProps } from 'react-big-calendar'
import { format, parse, startOfWeek, getDay, startOfDay } from 'date-fns'
import { enUS } from 'date-fns/locale/en-US'
import RollingWeek, { ROLLING_DAYS } from './RollingWeekView'
import type { BookingAdmin, BookingStatus } from '../lib/types'
import 'react-big-calendar/lib/css/react-big-calendar.css'
import './booking-calendar.css'

const localizer = dateFnsLocalizer({
  format,
  parse,
  startOfWeek,
  getDay,
  locales: { 'en-US': enUS },
})

type CalendarView = 'rolling' | 'month'

export interface BookingCalendarEvent {
  id: number
  title: string
  start: Date
  end: Date
  resource: BookingAdmin
}

const eventColors: Record<BookingStatus, { background: string; border: string }> = {
  confirmed: { background: '#16a34a', border: '#15803d' },
  cancelled: { background: '#9ca3af', border: '#6b7280' },
  completed: { background: '#2563eb', border: '#1d4ed8' },
  charging: { background: '#d97706', border: '#b45309' },
  charged: { background: '#9333ea', border: '#7e22ce' },
}

function toCalendarEvent(booking: BookingAdmin): BookingCalendarEvent {
  return {
    id: booking.id,
    title: booking.name,
    start: new Date(booking.start_time),
    end: new Date(booking.end_time),
    resource: booking,
  }
}

interface Props {
  bookings: BookingAdmin[]
  onSelectBooking?: (booking: BookingAdmin) => void
}

export function BookingCalendar({ bookings, onSelectBooking }: Props) {
  const [view, setView] = useState<CalendarView>('rolling')
  const [date, setDate] = useState(() => startOfDay(new Date()))

  const events = useMemo(
    () => bookings.filter((b) => b.status !== 'cancelled').map(toCalendarEvent),
    [bookings],
  )

  return (
    <div className="booking-calendar rounded-xl border border-gray-200 bg-white p-3 sm:p-4">
      <Calendar<BookingCalendarEvent>
        localizer={localizer}
        events={events}
        view={view}
        date={date}
        onView={(nextView) => setView(nextView as CalendarView)}
        onNavigate={(newDate) => setDate(startOfDay(newDate))}
        views={{ rolling: RollingWeek, month: true } as CalendarProps<BookingCalendarEvent>['views']}
        defaultView="rolling"
        messages={{
          rolling: `${ROLLING_DAYS} days`,
          month: 'Month',
        }}
        step={60}
        timeslots={1}
        scrollToTime={new Date(1970, 0, 1, 8, 0)}
        popup
        showMultiDayTimes={false}
        toolbar
        style={{ height: 560 }}
        onSelectEvent={(event: BookingCalendarEvent) => onSelectBooking?.(event.resource)}
        eventPropGetter={(event: BookingCalendarEvent) => {
          const colors = eventColors[event.resource.status]
          return {
            style: {
              backgroundColor: colors.background,
              borderColor: colors.border,
              color: '#fff',
              fontSize: '0.8rem',
            },
          }
        }}
        tooltipAccessor={(event: BookingCalendarEvent) => {
          const { resource } = event
          return `${resource.name} · ${format(event.start, 'h:mm a')} – ${format(event.end, 'h:mm a')}`
        }}
      />
    </div>
  )
}
