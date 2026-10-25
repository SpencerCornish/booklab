import { test, expect } from '@playwright/test'
import { dateButtonLabel, futureDate } from '../helpers/time'
import { waitForMessage, messageHtml } from '../helpers/mailpit'
import { uniqueEmail } from '../helpers/api'

// Full booking flow including the real Stripe.js card confirmation.
//
// SAFETY: this spec only ever runs against Stripe TEST MODE keys. It skips when
// they are absent and aborts loudly on any non-test-mode key — it cannot touch
// live Stripe by construction. In test mode no funds move; the 4242… test card
// is only valid in test mode anyway.
const secretKey = process.env.STRIPE_TEST_SECRET_KEY
const publishableKey = process.env.STRIPE_TEST_PUBLISHABLE_KEY

if (secretKey && !secretKey.startsWith('sk_test_')) {
  throw new Error('STRIPE_TEST_SECRET_KEY must be a TEST MODE key (sk_test_…); refusing to run against live Stripe.')
}
if (publishableKey && !publishableKey.startsWith('pk_test_')) {
  throw new Error('STRIPE_TEST_PUBLISHABLE_KEY must be a TEST MODE key (pk_test_…).')
}

test.skip(!secretKey || !publishableKey, 'STRIPE_TEST_SECRET_KEY and STRIPE_TEST_PUBLISHABLE_KEY are required for the real-payment flow (they are optional; every other E2E spec runs hermetically).')

test('book end-to-end with a test card, then see the confirmation email', async ({ page }) => {
  const email = uniqueEmail('pay')
  await page.goto('/')

  const date = futureDate(25)
  await page.getByRole('button', { name: dateButtonLabel(date), exact: true }).click()
  await page.getByRole('button', { name: '10:00 AM' }).click()
  await page.getByRole('button', { name: '11:00 AM' }).click()

  await page.getByPlaceholder('Your full name').fill('Paying Booker')
  await page.getByPlaceholder('you@example.com').fill(email)
  await page.getByPlaceholder('(406) 555-0100').fill('555-555-0100')
  await page.getByRole('checkbox').check()

  // Stripe.js CardElement lives inside its own iframe.
  const card = page.frameLocator('.__PrivateStripeElement iframe')
  await card.locator('input[name="cardnumber"]').fill('4242424242424242')
  await card.locator('input[name="exp-date"]').fill('12/35')
  await card.locator('input[name="cvc"]').fill('123')

  await page.getByRole('button', { name: 'Confirm Booking' }).click()
  await expect(page.getByRole('heading', { name: "You're booked!" })).toBeVisible({ timeout: 20_000 })
  await expect(page.getByText(email)).toBeVisible()

  // The confirmation email lands in Mailpit.
  const msg = await waitForMessage(`to:${email}`)
  expect(msg.Subject).toContain('Booking Confirmed')
  const html = await messageHtml(msg.ID)
  expect(html).toContain('Paying Booker')
})