import { test as base, expect, type BrowserContext, type Page } from '@playwright/test';

/**
 * Test fixtures for the E2E suite.
 *
 * Why a shared browser context instead of Playwright's `storageState`:
 * refresh tokens are single-use and rotate on every use, and replaying a
 * rotated token is treated as theft (the whole family is revoked). A saved
 * storage state hands the *same* token to every test, so the second test
 * would trip reuse detection and revoke the session for the rest of the run.
 * One live context keeps the rotation chain coherent — and signing in once
 * also stays under the login rate limit that protects against brute force.
 *
 * `test`          — authenticated as the platform admin, shared context.
 * `anonymousTest` — a fresh, signed-out context per test.
 */

const BOOTSTRAP_EMAIL = process.env.BOOTSTRAP_ADMIN_EMAIL ?? 'sr.alasgarov@gmail.com';
const BOOTSTRAP_PASSWORD = process.env.BOOTSTRAP_ADMIN_PASSWORD ?? 'Anh0@111118';
/** The password after the mandatory first-login change. */
export const ADMIN_PASSWORD = process.env.E2E_ADMIN_PASSWORD ?? 'E2e@Str0ngPass1';
export const ADMIN_EMAIL = BOOTSTRAP_EMAIL;

/** Signs in, performing the forced password change on a fresh deployment. */
export async function signIn(page: Page): Promise<void> {
  await page.goto('/login');
  await page.getByTestId('login-email').fill(BOOTSTRAP_EMAIL);
  await page.getByTestId('login-password').fill(ADMIN_PASSWORD);
  await page.getByTestId('login-submit').click();

  const signedIn = await Promise.race([
    page.waitForURL('/', { timeout: 8000 }).then(() => true),
    page
      .getByTestId('login-error')
      .waitFor({ timeout: 8000 })
      .then(() => false),
  ]).catch(() => false);

  if (signedIn) return;

  // Fresh deployment: the bootstrap password is still in place.
  await page.getByTestId('login-email').fill(BOOTSTRAP_EMAIL);
  await page.getByTestId('login-password').fill(BOOTSTRAP_PASSWORD);
  await page.getByTestId('login-submit').click();

  await page.waitForURL('/force-password-change', { timeout: 15_000 });
  await page.getByTestId('current-password').fill(BOOTSTRAP_PASSWORD);
  await page.getByTestId('new-password').fill(ADMIN_PASSWORD);
  await page.getByTestId('confirm-password').fill(ADMIN_PASSWORD);
  await page.getByTestId('change-password-submit').click();
  await page.waitForURL('/', { timeout: 15_000 });
}

type WorkerFixtures = {
  /** One signed-in context for the whole worker. */
  adminContext: BrowserContext;
};

export const test = base.extend<Record<string, never>, WorkerFixtures>({
  adminContext: [
    // baseURL is a test-scoped builtin, so a worker fixture reads it from the
    // environment instead (same source the config uses).
    async ({ browser }, use) => {
      const context = await browser.newContext({
        baseURL: process.env.PORTAL_URL ?? 'http://localhost:5173',
      });
      const page = await context.newPage();
      await signIn(page);
      await page.close();
      await use(context);
      await context.close();
    },
    { scope: 'worker' },
  ],

  // Each test gets a fresh page inside the shared, already-signed-in context.
  page: async ({ adminContext }, use) => {
    const page = await adminContext.newPage();
    await use(page);
    await page.close();
  },
});

/** A signed-out browser, for tests about authentication itself. */
export const anonymousTest = base;

export { expect };
