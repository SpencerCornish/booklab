export type BookingStatus = 'confirmed' | 'cancelled' | 'completed' | 'charging' | 'charged'

export interface PublicSettings {
  resource_name: string
  hourly_rate_cents: number
  currency: string
  bookable_start: string
  bookable_end: string
  min_hours: number
  max_hours: number
  timezone: string
  referral_sources: string[]
}

export interface Settings extends PublicSettings {
  reminder_hours_before: number
  notification_emails: string
  auto_charge_delay_minutes: number
  terms_content: string
  privacy_content: string
}

export interface ReferralSourceCount {
  source: string
  count: number
}

export interface CustomerInsight {
  email: string
  name: string
  booking_count: number
  cancelled_count: number
  revenue_cents: number
  last_booking_at?: string
}

export interface InsightsData {
  total_bookings: number
  total_revenue_cents: number
  unique_customers: number
  recent_bookings: number
  bookings_by_status: Record<string, number>
  referral_sources: ReferralSourceCount[]
  customers: CustomerInsight[]
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
  metadata?: Record<string, string>
  reminder_sent?: boolean
  updated_at?: string
  charge_attempts?: number
  last_charge_error?: string
}

export interface PrepareBookingResponse {
  setup_intent_id: string
  setup_intent_client_secret: string
}

export interface FinalizeBookingResponse {
  booking: BookingPublic
}

export interface Closure {
  id: number
  start_date: string
  end_date: string
  all_day: boolean
  start_time?: string
  end_time?: string
  reason?: string
  created_at: string
}

export interface AdminUser {
  id: number
  username: string
  created_at: string
}

export interface AdminUsersResponse {
  users: AdminUser[]
  current_username: string
}
