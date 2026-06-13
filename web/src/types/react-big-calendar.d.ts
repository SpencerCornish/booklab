declare module 'react-big-calendar' {
  import type { ComponentType, CSSProperties } from 'react'

  export type View = 'month' | 'week' | 'work_week' | 'day' | 'agenda' | string

  export const Navigate: {
    PREVIOUS: 'PREV'
    NEXT: 'NEXT'
    TODAY: 'TODAY'
    DATE: 'DATE'
  }

  export interface Event {
    title?: string
    start?: Date
    end?: Date
    allDay?: boolean
    resource?: unknown
  }

  export interface CalendarProps<TEvent extends Event = Event> {
    localizer: object
    events?: TEvent[]
    view?: View
    date?: Date
    defaultView?: View
    views?: View[] | Record<string, boolean | ComponentType<object>>
    messages?: Record<string, string>
    step?: number
    timeslots?: number
    scrollToTime?: Date
    popup?: boolean
    showMultiDayTimes?: boolean
    toolbar?: boolean
    style?: CSSProperties
    onView?: (view: View) => void
    onNavigate?: (date: Date) => void
    onSelectEvent?: (event: TEvent) => void
    eventPropGetter?: (event: TEvent) => { style?: CSSProperties }
    tooltipAccessor?: (event: TEvent) => string | null
  }

  export const Calendar: <TEvent extends Event = Event>(
    props: CalendarProps<TEvent>,
  ) => ReturnType<ComponentType<CalendarProps<TEvent>>>

  export type { CalendarProps }

  export function dateFnsLocalizer(config: {
    format: (...args: never[]) => string
    parse: (...args: never[]) => Date
    startOfWeek: (...args: never[]) => Date
    getDay: (...args: never[]) => number
    locales?: Record<string, object>
  }): object
}

declare module 'react-big-calendar/lib/css/react-big-calendar.css'

declare module 'react-big-calendar/lib/TimeGrid' {
  import type { ComponentType } from 'react'

  const TimeGrid: ComponentType<Record<string, unknown>>
  export default TimeGrid
}
