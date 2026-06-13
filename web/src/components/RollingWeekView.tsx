import { Component, type ComponentProps, type ComponentType } from 'react'
import { addDays, startOfDay } from 'date-fns'
import TimeGrid from 'react-big-calendar/lib/TimeGrid'
import { Navigate } from 'react-big-calendar'

export const ROLLING_DAYS = 7

export function rollingWeekRange(date: Date): Date[] {
  const start = startOfDay(date)
  return Array.from({ length: ROLLING_DAYS }, (_, index) => addDays(start, index))
}

type CalendarLocalizer = {
  startOf: (date: Date, unit: string) => Date
  endOf: (date: Date, unit: string) => Date
  add: (date: Date, amount: number, unit: string) => Date
  format: (range: { start: Date; end: Date }, format: string) => string
}

type TimeGridProps = ComponentProps<typeof TimeGrid> & {
  date: Date
  localizer: CalendarLocalizer
}

class RollingWeek extends Component<TimeGridProps> {
  static range = rollingWeekRange

  static navigate(date: Date, action: string, { localizer }: { localizer: { add: (date: Date, amount: number, unit: string) => Date } }) {
    switch (action) {
      case Navigate.PREVIOUS:
        return localizer.add(date, -ROLLING_DAYS, 'day')
      case Navigate.NEXT:
        return localizer.add(date, ROLLING_DAYS, 'day')
      default:
        return date
    }
  }

  static title(date: Date, { localizer }: { localizer: { format: (range: { start: Date; end: Date }, format: string) => string } }) {
    const range = rollingWeekRange(date)
    return localizer.format(
      { start: range[0], end: range[range.length - 1] },
      'dayRangeHeaderFormat',
    )
  }

  render() {
    const {
      date,
      localizer,
      min = localizer.startOf(new Date(), 'day'),
      max = localizer.endOf(new Date(), 'day'),
      scrollToTime = localizer.startOf(new Date(), 'day'),
      enableAutoScroll = true,
      ...props
    } = this.props

    return (
      <TimeGrid
        {...props}
        date={date}
        range={rollingWeekRange(date)}
        eventOffset={15}
        localizer={localizer}
        min={min}
        max={max}
        scrollToTime={scrollToTime}
        enableAutoScroll={enableAutoScroll}
      />
    )
  }
}

export default RollingWeek as ComponentType<TimeGridProps> & {
  range: typeof rollingWeekRange
  navigate: typeof RollingWeek.navigate
  title: typeof RollingWeek.title
}
