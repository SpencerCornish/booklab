import { useEffect, useMemo, useState } from 'react'
import { Calendar, dateFnsLocalizer, type View } from 'react-big-calendar'
import { format, parse, startOfWeek, getDay } from 'date-fns'
import { enUS } from 'date-fns/locale/en-US'
import { useMediaQuery } from '../hooks/useMediaQuery'
import { bookingToCalendarEvent, closureToCalendarEvents, type ScheduleCalendarEvent } from '../lib/scheduleCalendar'
import type { BookingAdmin, BookingStatus, Closure } from '../lib/types'
import 'react-big-calendar/lib/css/react-big-calendar.css'
import './booking-calendar.css'

const localizer = dateFnsLocalizer({
  format,
  parse,
  startOfWeek,
  getDay,
  locales: { 'en-US': enUS },
})

const bookingColors: Record<BookingStatus, { background: string; border: string }> = {
  confirmed: { background: '#16a34a', border: '#15803d' },
  cancelled: { background: '#9ca3af', border: '#6b7280' },
  completed: { background: '#2563eb', border: '#1d4ed8' },
  charging: { background: '#d97706', border: '#b45309' },
  charged: { background: '#9333ea', border: '#7e22ce' },
}

const closureColors = { background: '#e5e7eb', border: '#9ca3af', color: '#374151' }

interface Props {
  bookings: BookingAdmin[]
  closures: Closure[]
  onSelectBooking?: (booking: BookingAdmin) => void
  onSelectClosure?: (closure: Closure) => void
}

export function BookingCalendar({ bookings, closures, onSelectBooking, onSelectClosure }: Props) {
  const isDesktop = useMediaQuery('(min-width: 768px)')
  const [view, setView] = useState<View>(() => (isDesktop ? 'week' : 'day'))
  const [date, setDate] = useState(new Date())

  useEffect(() => {
    setView(isDesktop ? 'week' : 'day')
  }, [isDesktop])

  const events = useMemo(() => {
    const bookingEvents = bookings
      .filter((b) => b.status !== 'cancelled')
      .map(bookingToCalendarEvent)
    const closureEvents = closures.flatMap(closureToCalendarEvents)
    return [...closureEvents, ...bookingEvents]
  }, [bookings, closures])

  return (
    <div className="overflow-x-auto">
      <div className="booking-calendar rounded-xl border border-gray-200 bg-white p-3 sm:p-4 min-w-0">
        <Calendar<ScheduleCalendarEvent>
          localizer={localizer}
          events={events}
          view={view}
          date={date}
          onView={setView}
          onNavigate={setDate}
          views={isDesktop ? ['week', 'month', 'day'] : ['day', 'week', 'month']}
          defaultView={isDesktop ? 'week' : 'day'}
          step={60}
          timeslots={1}
          scrollToTime={new Date(1970, 0, 1, 8, 0)}
          popup
          showMultiDayTimes={false}
          toolbar
          style={{ height: isDesktop ? 560 : 420 }}
        onSelectEvent={(event: ScheduleCalendarEvent) => {
          if (event.resource.type === 'booking') {
            onSelectBooking?.(event.resource.booking)
          } else {
            onSelectClosure?.(event.resource.closure)
          }
        }}
        eventPropGetter={(event: ScheduleCalendarEvent) => {
          if (event.resource.type === 'closure') {
            return {
              style: {
                backgroundColor: closureColors.background,
                borderColor: closureColors.border,
                color: closureColors.color,
                fontSize: '0.8rem',
                backgroundImage:
                  'repeating-linear-gradient(-45deg, transparent, transparent 4px, rgba(156,163,175,0.35) 4px, rgba(156,163,175,0.35) 8px)',
              },
            }
          }
          const colors = bookingColors[event.resource.booking.status]
          return {
            style: {
              backgroundColor: colors.background,
              borderColor: colors.border,
              color: '#fff',
              fontSize: '0.8rem',
            },
          }
        }}
        tooltipAccessor={(event: ScheduleCalendarEvent) => {
          if (event.resource.type === 'closure') {
            const { closure } = event.resource
            if (event.allDay) {
              const range =
                closure.start_date === closure.end_date
                  ? closure.start_date
                  : `${closure.start_date} – ${closure.end_date}`
              return `Closed · ${range}${closure.reason ? ` · ${closure.reason}` : ''}`
            }
            return `Closed · ${format(event.start, 'h:mm a')} – ${format(event.end, 'h:mm a')}${closure.reason ? ` · ${closure.reason}` : ''}`
          }
          const { booking } = event.resource
          return `${booking.name} · ${format(event.start, 'h:mm a')} – ${format(event.end, 'h:mm a')}`
        }}
      />
      </div>
    </div>
  )
}
