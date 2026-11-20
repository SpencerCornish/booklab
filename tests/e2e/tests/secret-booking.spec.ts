import { test, expect } from '@playwright/test'
import { dateButtonLabel, futureDate } from '../helpers/time'
import {
  adminLogin,
  enableSecretLink,
  seedSecretBooking,
  uniqueEmail,
} from '../helpers/api'

// The secret studio link: a token-holder books free sessions through the same
// BookingPage, with the payment machinery absent rather than merely hidden.

test('an invalid secret link renders the expired-link error', async ({ page }) => {
  await page.goto('/book/not-a-real-token')
  await expect(page.getByRole('heading', { name: 'Link no longer valid' })).toBeVisible()
  await expect(page.getByText('This booking link is invalid or has expired.')).toBeVisible()
})

test('a valid secret link books a free session with no payment UI', async ({ page }) => {
  const { token } = await enableSecretLink(page)

  await page.goto(`/book/${token}`)
  await expect(page.getByRole('heading', { level: 1 })).toContainText('Book The Space (Studio Artist)')
  await expect(page.getByText('Studio booking · No payment required')).toBeVisible()

  // No pricing disclosure, no lead-time banner, no card, no estimate.
  const date = futureDate(12)
  await page.getByRole('button', { name: dateButtonLabel(date), exact: true }).click()
  await page.getByRole('button', { name: '10:00 AM' }).click()

  await expect(page.getByText(/Estimated total/)).toHaveCount(0)
  await expect(page.getByText('Bookings must be made at least', { exact: false })).toHaveCount(0)
  await expect(page.locator('label:text-is("Card")')).toHaveCount(0)

  await page.getByPlaceholder('Your full name').fill('Studio Artist')
  await page.getByPlaceholder('you@example.com').fill(uniqueEmail('secret'))
  await page.getByPlaceholder('(406) 555-0100').fill('555-555-0100')

  await page.getByRole('checkbox').check()
  await page.getByRole('button', { name: 'Confirm Studio Booking' }).click()

  await expect(page.getByText("You're booked!")).toBeVisible()
  await expect(page.getByText('No payment required.')).toBeVisible()
  await expect(page.getByText('Payment will be collected after your session.')).toHaveCount(0)
})

test('the admin list badges free bookings and offers no charge action', async ({ page }) => {
  const { token } = await enableSecretLink(page)
  const email = uniqueEmail('badge')
  await seedSecretBooking(page, {
    token,
    email,
    name: 'Badge Artist',
    date: futureDate(13),
    startHour: 13,
    endHour: 14,
  })

  await adminLogin(page)
  await page.goto('/admin/bookings')
  const row = page.locator('tr', { hasText: email }).first()
  await expect(row.getByText('Free · Studio booking')).toBeVisible()
  await expect(row.getByText('Free', { exact: true })).toBeVisible()
  // No Charge action on a free row (the API 400 is the backstop, not the UX).
  await expect(row.getByRole('button', { name: 'Charge' })).toHaveCount(0)
  await expect(row.getByRole('button', { name: 'Mark done' })).toBeVisible()

  // Leave the feature off so later specs see the default settings.
  await enableSecretLink(page, { enabled: false })
})
