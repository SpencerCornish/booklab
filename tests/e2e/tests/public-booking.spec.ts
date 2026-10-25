import { test, expect } from '@playwright/test'
import { dateButtonLabel, futureDate } from '../helpers/time'
import { uniqueEmail, createClosure } from '../helpers/api'

// The booking flow in hermetic mode (no Stripe publishable key baked in):
// the UI is fully drivable up to the card confirmation, which cannot complete
// without real Stripe. The full payment path lives in payment.spec.ts.

test('date picker and slot grid reflect availability', async ({ page }) => {
  await page.goto('/')

  const date = futureDate(21)
  await page.getByRole('button', { name: dateButtonLabel(date), exact: true }).click()

  // 12 hourly slots for the default 09:00–21:00 bookable window.
  await expect(page.getByRole('button', { name: '9:00 AM' })).toBeVisible()
  await expect(page.getByRole('button', { name: '8:00 PM' })).toBeVisible()
})

test('closed days show the closure banner instead of slots', async ({ page }) => {
  const date = futureDate(22)
  await createClosure(page, { startDate: date, endDate: date, reason: 'E2E Painting' })

  await page.goto('/')
  await page.getByRole('button', { name: dateButtonLabel(date), exact: true }).click()
  await expect(page.getByText(/This date is closed: E2E Painting/)).toBeVisible()
})

test('slot selection, totals, and form guardrails before payment', async ({ page }) => {
  const email = uniqueEmail('flow')
  await page.goto('/')

  const date = futureDate(23)
  await page.getByRole('button', { name: dateButtonLabel(date), exact: true }).click()

  await page.getByRole('button', { name: '10:00 AM' }).click()
  await page.getByRole('button', { name: '11:00 AM' }).click()
  // Clicking 11:00 AM extends the selection through that slot (ends 12:00 PM).
  await expect(page.getByText(/Selected: 10:00 AM – 12:00 PM/)).toBeVisible()
  await expect(page.getByText('Estimated total · 2 hrs')).toBeVisible()
  await expect(page.getByText('$30.00')).toBeVisible()

  await page.getByPlaceholder('Your full name').fill('Flow Booker')
  await page.getByPlaceholder('you@example.com').fill(email)
  await page.getByPlaceholder('(406) 555-0100').fill('555-555-0100')

  // Terms must be accepted before submit is possible...
  const submit = page.getByRole('button', { name: 'Confirm Booking' })
  await expect(submit).toBeDisabled()
  await page.getByRole('checkbox').check()
  // ...and in this hermetic build the button stays disabled: Stripe.js cannot
  // initialize without a publishable key, so the booking cannot be submitted
  // without payment. The real-payment path is covered by payment.spec.ts.
  await expect(submit).toBeDisabled()
})