import { test, expect } from '@playwright/test'
import { futureDate } from '../helpers/time'
import { seedBooking, uniqueEmail } from '../helpers/api'

test('cancelling via the email link frees the slot', async ({ page }) => {
  const date = futureDate(31)
  const booking = await seedBooking(page, {
    email: uniqueEmail('cancel'),
    name: 'Cancel Me',
    date,
    startHour: 10,
    endHour: 12,
  })

  await page.goto(`/cancel/${booking.token}`)
  await expect(page.getByRole('heading', { name: 'Cancel Booking' })).toBeVisible()
  await expect(page.getByText('Cancel Me')).toBeVisible()

  await page.getByRole('button', { name: 'Yes, cancel it' }).click()
  await expect(page.getByRole('heading', { name: 'Booking Cancelled' })).toBeVisible()
  await expect(page.getByText('No charge will be made.')).toBeVisible()

  // The cancelled slot is bookable again.
  const avail = await page.request.get(`/api/availability?date=${date}`)
  expect(avail.ok()).toBeTruthy()
  const data = (await avail.json()) as { slots: Array<{ start: string; available: boolean }> }
  const slot = data.slots.find((s) => s.start.startsWith(`${date}T10:00`))
  expect(slot?.available).toBe(true)

  // The cancellation view is now terminal.
  await page.goto(`/cancel/${booking.token}`)
  await expect(page.getByRole('heading', { name: 'Booking Cancelled' })).toBeVisible()
})