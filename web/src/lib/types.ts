export type BookingStatus = 'confirmed' | 'cancelled' | 'completed' | 'charged'

export interface PublicSettings {
  resource_name: string
  hourly_rate_cents: number
  currency: string
  bookable_start: string
  bookable_end: string
  min_hours: number
  max_hours: number
  timezone: string
}

export interface Settings extends PublicSettings {
  reminder_hours_before: number
  notification_emails: string
}

export interface TimeSlot {
  start: string
  end: string
  available: boolean
}

export interface AvailabilityResponse {
  date: string
  is_closed: boolean
  closure_reason?: string
  slots: TimeSlot[]
}

export interface BookingPublic {
  id: number
  name: string
  email: string
  start_time: string
  end_time: string
  status: BookingStatus
  cancel_token: string
  created_at: string
}

export interface BookingAdmin extends BookingPublic {
  stripe_setup_intent_id?: string
  stripe_payment_method_id?: string
  stripe_payment_intent_id?: string
  stripe_receipt_url?: string
  amount_cents?: number
  completed_at?: string
}

export interface CreateBookingResponse {
  booking: BookingPublic
  setup_intent_client_secret: string
}

export interface Closure {
  id: number
  start_date: string
  end_date: string
  reason?: string
  created_at: string
}
