import { defineConfig } from '@playwright/test'

const PORT = process.env.FUNC_APP_PORT ?? '8090'

export default defineConfig({
  testDir: './tests',
  timeout: 30_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: process.env.CI
    ? [['junit', { outputFile: '../../.functest/e2e-junit.xml' }], ['html', { open: 'never' }]]
    : [['list']],
  use: {
    baseURL: process.env.E2E_BASE_URL ?? `http://localhost:${PORT}`,
    // Match the app's default settings timezone so the date picker, slot
    // labels, and API-side bookable-hours validation all agree.
    timezoneId: 'America/Denver',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { browserName: 'chromium' } }],
  globalSetup: './global-setup.ts',
  outputDir: '../../.functest/e2e-artifacts',
})