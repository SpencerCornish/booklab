import { test, expect } from '@playwright/test'
import { futureDate } from '../helpers/time'
import { ADMIN_PASS, ADMIN_USER, adminLogin, seedBooking, uniqueEmail } from '../helpers/api'

const RUN = String(Date.now())

test('login rejects bad credentials, then accepts good ones', async ({ page }) => {
  await page.goto('/admin/login')
  await page.locator('input[autocomplete="username"]').fill(ADMIN_USER)
  await page.locator('input[autocomplete="current-password"]').fill('wrong-password')
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByText('invalid credentials')).toBeVisible()

  await page.locator('input[autocomplete="current-password"]').fill(ADMIN_PASS)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page).toHaveURL(/\/admin$/)
  await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible()
})

test('bookings list shows a seeded booking with its details', async ({ page }) => {
  await adminLogin(page)
  const email = uniqueEmail('adminbook')
  await seedBooking(page, { email, name: 'Admin Visible', date: futureDate(32), startHour: 13, endHour: 14 })

  await page.goto('/admin/bookings')
  await expect(page.getByRole('heading', { name: 'Bookings', level: 1 })).toBeVisible()
  // The booking renders in both card and table layouts; only one is visible.
  await expect(page.getByText(email).filter({ visible: true }).first()).toBeVisible()
  await expect(page.getByText('Admin Visible').filter({ visible: true }).first()).toBeVisible()
})

test('closures can be created and deleted through the UI', async ({ page }) => {
  await adminLogin(page)
  const date = futureDate(24)

  await page.goto('/admin/closures')
  await page.getByRole('button', { name: '+ Add closure' }).click()
  await page.locator('input[type="date"]').first().fill(date)
  await page.locator('input[type="date"]').last().fill(date)
  await page.getByPlaceholder('e.g. Holiday, Maintenance').fill('UI Closure Test')
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(page.getByText('UI Closure Test')).toBeVisible()

  // Delete it again (window.confirm dialog must be accepted).
  page.once('dialog', (dialog) => dialog.accept())
  await page.getByRole('button', { name: 'Delete' }).first().click()
  await expect(page.getByText('UI Closure Test')).toHaveCount(0)
})

test('settings editor round-trips a value', async ({ page }) => {
  await adminLogin(page)
  await page.goto('/admin/settings/general')
  await expect(page.getByRole('heading', { name: 'Settings', level: 1 })).toBeVisible()

  const resourceName = page.locator('label:text-is("Resource name") + input')
  await expect(resourceName).toHaveValue('The Space')

  await resourceName.fill('E2E Test Space')
  await page.getByRole('button', { name: 'Save settings' }).click()
  await expect(page.getByText('Saved!')).toBeVisible()
  await expect(page.locator('label:text-is("Resource name") + input')).toHaveValue('E2E Test Space')

  // The public booking page header reflects the new name.
  await page.goto('/')
  await expect(page.getByRole('heading', { level: 1 })).toContainText('Book E2E Test Space')

  // Restore.
  await adminLogin(page)
  await page.goto('/admin/settings/general')
  await page.locator('label:text-is("Resource name") + input').fill('The Space')
  await page.getByRole('button', { name: 'Save settings' }).click()
  await expect(page.getByText('Saved!')).toBeVisible()
})

test('users can be created and deleted through the UI', async ({ page }) => {
  await adminLogin(page)
  const username = `ui-admin-${RUN}`

  await page.goto('/admin/users')
  await expect(page.getByRole('heading', { name: 'Admin Users' })).toBeVisible()

  const form = page.locator('form', { has: page.getByRole('button', { name: 'Add user' }) })
  await form.locator('input[type="text"]').fill(username)
  await form.locator('input[type="password"]').fill('ui-admin-pass-123')
  await page.getByRole('button', { name: 'Add user' }).click()
  await expect(page.getByText(username)).toBeVisible()

  page.once('dialog', (dialog) => dialog.accept())
  await page.locator('tr', { hasText: username }).getByRole('button', { name: 'Delete' }).click()
  await expect(page.getByText(username)).toHaveCount(0)
})

test('sign out ends the admin session', async ({ page }) => {
  await adminLogin(page)
  await page.goto('/admin')
  await page.getByRole('button', { name: 'Sign out' }).click()
  await expect(page).toHaveURL(/\/admin\/login$/)

  await page.goto('/admin/bookings')
  await expect(page).toHaveURL(/\/admin\/login$/)
})