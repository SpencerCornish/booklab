// Helpers for driving the app through its API (with the browser's cookie jar),
// so specs can seed state fast and reserve the UI for what's under test.
import type { Page } from '@playwright/test'
import { denverTime, futureDate } from './time'

export const APP_BASE_URL =
  process.env.E2E_BASE_URL ?? `http://localhost:${process.env.FUNC_APP_PORT ?? '8090'}`

export const ADMIN_USER = process.env.FUNC_ADMIN_USER ?? 'admin'
export const ADMIN_PASS = process.env.FUNC_ADMIN_PASS ?? 'functest-admin-pass'

export interface SeededBooking {
  id: number
  token: string
  email: string
  start: string
  end: string
}

/** Log the browser context in as an admin (cookies are shared with the page). */
export async function adminLogin(page: Page): Promise<void> {
  const resp = await page.context().request.post(`${APP_BASE_URL}/api/admin/login`, {
    data: { username: ADMIN_USER, password: ADMIN_PASS },
  })
  if (resp.status() !== 200) throw new Error(`admin login failed: HTTP ${resp.status()}`)
}

/**
 * The admin mutating endpoints require double-submit CSRF: the X-CSRF-Token
 * header must equal the booklab_csrf cookie. GET requests bootstrap the cookie.
 */
async function adminCsrfToken(page: Page): Promise<string> {
  const cookies = await page.context().cookies(APP_BASE_URL)
  const csrf = cookies.find((c) => c.name === 'booklab_csrf')
  if (!csrf) {
    await page.context().request.get(`${APP_BASE_URL}/api/admin/bookings`)
    const retry = await page.context().cookies(APP_BASE_URL)
    const token = retry.find((c) => c.name === 'booklab_csrf')
    if (!token) throw new Error('booklab_csrf cookie never appeared')
    return token.value
  }
  return csrf.value
}

/** Seeds a confirmed booking through the public API (works in fake or real-Stripe mode). */
export async function seedBooking(
  page: Page,
  opts: { email: string; name: string; date: string; startHour: number; endHour: number },
): Promise<SeededBooking> {
  const start = denverTime(opts.date, opts.startHour)
  const end = denverTime(opts.date, opts.endHour)

  const prep = await page.context().request.post(`${APP_BASE_URL}/api/bookings/prepare`, {
    data: { email: opts.email, start_time: start, end_time: end },
  })
  if (prep.status() !== 200) throw new Error(`prepare failed: HTTP ${prep.status()}: ${await prep.text()}`)
  const { setup_intent_id } = (await prep.json()) as { setup_intent_id: string }

  const create = await page.context().request.post(`${APP_BASE_URL}/api/bookings`, {
    data: {
      setup_intent_id,
      name: opts.name,
      email: opts.email,
      start_time: start,
      end_time: end,
    },
  })
  if (create.status() !== 201) throw new Error(`create failed: HTTP ${create.status()}: ${await create.text()}`)
  const { booking } = (await create.json()) as { booking: { id: number; cancel_token: string } }
  return { id: booking.id, token: booking.cancel_token, email: opts.email, start, end }
}

/** Marks a booking completed and charges it (fake or real). */
export async function completeAndCharge(
  page: Page,
  bookingId: number,
  amountCents: number,
): Promise<void> {
  await adminLogin(page)
  const csrf = await adminCsrfToken(page)
  const patch = await page.context().request.patch(`${APP_BASE_URL}/api/admin/bookings/${bookingId}`, {
    headers: { 'X-CSRF-Token': csrf },
    data: { status: 'completed' },
  })
  if (patch.status() !== 200) throw new Error(`complete failed: HTTP ${patch.status()}`)
  const charge = await page.context().request.post(`${APP_BASE_URL}/api/admin/bookings/${bookingId}/charge`, {
    headers: { 'X-CSRF-Token': csrf },
    data: { amount_cents: amountCents },
  })
  if (charge.status() !== 200) throw new Error(`charge failed: HTTP ${charge.status()}: ${await charge.text()}`)
}

/** Adds an admin closure through the API. */
export async function createClosure(
  page: Page,
  opts: { startDate: string; endDate: string; reason?: string },
): Promise<void> {
  await adminLogin(page)
  const csrf = await adminCsrfToken(page)
  const resp = await page.context().request.post(`${APP_BASE_URL}/api/admin/closures`, {
    headers: { 'X-CSRF-Token': csrf },
    data: {
      start_date: opts.startDate,
      end_date: opts.endDate,
      all_day: true,
      reason: opts.reason ?? 'E2E closure',
    },
  })
  if (resp.status() !== 201) throw new Error(`closure create failed: HTTP ${resp.status()}`)
}

/** Convenience: a unique-ish email per spec run. */
export function uniqueEmail(prefix: string): string {
  return `${prefix}.${Date.now()}.e2e@example.com`
}

export { denverTime, futureDate }