import type {
  AvailabilityResponse,
  BookingAdmin,
  BookingPublic,
  Closure,
  CreateBookingResponse,
  PublicSettings,
  Settings,
} from './types'

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`/api${path}`, {
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    ...options,
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new ApiError(res.status, body.error ?? 'Unknown error')
  }
  if (res.status === 204) return undefined as T
  return res.json()
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message)
  }
}

// Public
export const getPublicSettings = () => request<PublicSettings>('/settings/public')
export const getAvailability = (date: string) =>
  request<AvailabilityResponse>(`/availability?date=${date}`)

export const createBooking = (body: {
  name: string
  email: string
  start_time: string
  end_time: string
}) =>
  request<CreateBookingResponse>('/bookings', {
    method: 'POST',
    body: JSON.stringify(body),
  })

export const getBookingByToken = (token: string) =>
  request<BookingPublic>(`/bookings/${token}`)

export const cancelBooking = (token: string) =>
  request<BookingPublic>(`/bookings/${token}/cancel`, { method: 'POST' })

// Admin
export const adminLogin = (username: string, password: string) =>
  request<{ status: string }>('/admin/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })

export const adminLogout = () =>
  request<void>('/admin/logout', { method: 'POST' })

export const adminListBookings = (params?: { date?: string; status?: string }) => {
  const qs = new URLSearchParams()
  if (params?.date) qs.set('date', params.date)
  if (params?.status) qs.set('status', params.status)
  const q = qs.toString() ? `?${qs}` : ''
  return request<BookingAdmin[]>(`/admin/bookings${q}`)
}

export const adminUpdateBooking = (
  id: number,
  body: { end_time?: string; status?: string },
) =>
  request<BookingAdmin>(`/admin/bookings/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(body),
  })

export const adminChargeBooking = (id: number, amount_cents?: number) =>
  request<BookingAdmin>(`/admin/bookings/${id}/charge`, {
    method: 'POST',
    body: JSON.stringify(amount_cents !== undefined ? { amount_cents } : {}),
  })

export const adminGetSettings = () => request<Settings>('/admin/settings')
export const adminUpdateSettings = (body: Partial<Settings>) =>
  request<Settings>('/admin/settings', {
    method: 'PUT',
    body: JSON.stringify(body),
  })

export const adminListClosures = () => request<Closure[]>('/admin/closures')
export const adminCreateClosure = (body: {
  start_date: string
  end_date: string
  reason?: string
}) =>
  request<Closure>('/admin/closures', {
    method: 'POST',
    body: JSON.stringify(body),
  })
export const adminUpdateClosure = (
  id: number,
  body: { start_date: string; end_date: string; reason?: string },
) =>
  request<Closure>(`/admin/closures/${id}`, {
    method: 'PUT',
    body: JSON.stringify(body),
  })
export const adminDeleteClosure = (id: number) =>
  request<void>(`/admin/closures/${id}`, { method: 'DELETE' })
