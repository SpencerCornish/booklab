import type {
  AdminUser,
  AdminUsersResponse,
  AvailabilityResponse,
  BookingAdmin,
  BookingPublic,
  Closure,
  FinalizeBookingResponse,
  InsightsData,
  PrepareBookingResponse,
  PublicSettings,
  Settings,
} from './types'

function getCsrfToken(): string | undefined {
  const match = document.cookie.match(/booklab_csrf=([^;]+)/)
  if (!match?.[1]) return undefined
  return decodeURIComponent(match[1].trim())
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const method = (options?.method ?? 'GET').toUpperCase()
  const headers = new Headers(options?.headers)
  if (!headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  if (['POST', 'PUT', 'PATCH', 'DELETE'].includes(method)) {
    const csrf = getCsrfToken()
    if (csrf) headers.set('X-CSRF-Token', csrf)
  }
  const res = await fetch(`/api${path}`, {
    ...options,
    credentials: 'include',
    headers,
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
export const getTerms = () => request<{ content: string }>('/terms')
export const getPrivacy = () => request<{ content: string }>('/privacy')
export const getAvailability = (date: string) =>
  request<AvailabilityResponse>(`/availability?date=${date}`)

export const prepareBooking = (body: {
  email: string
  start_time: string
  end_time: string
}) =>
  request<PrepareBookingResponse>('/bookings/prepare', {
    method: 'POST',
    body: JSON.stringify(body),
  })

export const createBooking = (body: {
  setup_intent_id: string
  name: string
  email: string
  metadata?: Record<string, string>
  start_time: string
  end_time: string
}) =>
  request<FinalizeBookingResponse>('/bookings', {
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

export const adminGetInsights = () => request<InsightsData>('/admin/insights')

export const adminListClosures = () => request<Closure[]>('/admin/closures')
export const adminCreateClosure = (body: {
  start_date: string
  end_date: string
  all_day?: boolean
  start_time?: string
  end_time?: string
  reason?: string
}) =>
  request<Closure>('/admin/closures', {
    method: 'POST',
    body: JSON.stringify(body),
  })
export const adminUpdateClosure = (
  id: number,
  body: {
    start_date: string
    end_date: string
    all_day?: boolean
    start_time?: string
    end_time?: string
    reason?: string
  },
) =>
  request<Closure>(`/admin/closures/${id}`, {
    method: 'PUT',
    body: JSON.stringify(body),
  })
export const adminDeleteClosure = (id: number) =>
  request<void>(`/admin/closures/${id}`, { method: 'DELETE' })

export const adminListUsers = () => request<AdminUsersResponse>('/admin/users')

export const adminCreateUser = (username: string, password: string) =>
  request<AdminUser>('/admin/users', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })

export const adminDeleteUser = (username: string) =>
  request<void>(`/admin/users/${encodeURIComponent(username)}`, { method: 'DELETE' })

export const adminChangePassword = (currentPassword: string, newPassword: string) =>
  request<void>('/admin/users/me/password', {
    method: 'PUT',
    body: JSON.stringify({
      current_password: currentPassword,
      new_password: newPassword,
    }),
  })
