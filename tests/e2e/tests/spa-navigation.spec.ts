import { test, expect } from '@playwright/test'
import { futureDate } from '../helpers/time'
import { seedBooking, uniqueEmail } from '../helpers/api'

// The SPA is served by the Go binary from its embedded filesystem; deep links
// must fall back to index.html and the client router must handle them.

test('root renders the booking page from public settings', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { level: 1 })).toContainText('Book The Space')
  await expect(page.getByText('$15/hr · Payment collected after your session')).toBeVisible()
})

test('unauthenticated /admin redirects to the login page', async ({ page }) => {
  await page.goto('/admin')
  await expect(page).toHaveURL(/\/admin\/login$/)
  await expect(page.getByRole('heading', { name: 'Admin Login' })).toBeVisible()
})

test('deep links to unknown booking tokens render error states', async ({ page }) => {
  const uuid = '00000000-0000-0000-0000-000000000000'
  await page.goto(`/booking/${uuid}`)
  await expect(page.getByText('Booking not found.')).toBeVisible()

  await page.goto(`/cancel/${uuid}`)
  await expect(page.getByText('Booking not found or link has expired.')).toBeVisible()
})

test('terms and privacy pages render', async ({ page }) => {
  await page.goto('/terms')
  await expect(page.getByRole('heading', { name: 'Terms & Conditions' })).toBeVisible()

  await page.goto('/privacy')
  await expect(page.getByRole('heading', { name: /Privacy Policy/i })).toBeVisible()
})

test('unknown routes get a server-level 404, not the SPA', async ({ page }) => {
  // The binary intentionally 404s non-SPA paths (vulnerability-scanner probe
  // defense, covered in depth by the Hurl suites). Client-side routing only
  // applies to known SPA routes.
  const resp = await page.goto('/definitely-not-a-route')
  expect(resp?.status()).toBe(404)
})

test('booking view page shows a real booking', async ({ page }) => {
  const booking = await seedBooking(page, {
    email: uniqueEmail('view'),
    name: 'View Page Booker',
    date: futureDate(30),
    startHour: 14,
    endHour: 15,
  })
  await page.goto(`/booking/${booking.token}`)
  await expect(page.getByText('View Page Booker')).toBeVisible()
})