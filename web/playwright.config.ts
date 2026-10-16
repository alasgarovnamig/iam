import { defineConfig, devices } from '@playwright/test';

// The E2E suite runs against a fully running stack (docker compose up).
// PORTAL_URL/API_URL let CI point at the compose-published ports.
const portalURL = process.env.PORTAL_URL ?? 'http://localhost:5173';

export default defineConfig({
  testDir: './e2e',
  // The suite drives one shared admin session (see e2e/fixtures.ts), so tests
  // must not run concurrently against it.
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  timeout: 60_000,
  expect: { timeout: 15_000 },
  use: {
    baseURL: portalURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
});
