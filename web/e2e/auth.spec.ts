import { expect, test } from '@playwright/test';
import { ADMIN_EMAIL, ADMIN_PASSWORD } from './fixtures';

/**
 * Authentication specs run in a signed-out browser (the default context), so
 * they own their sessions and never disturb the shared admin session used by
 * the rest of the suite.
 */

test('rejects wrong credentials without revealing whether the account exists', async ({ page }) => {
  await page.goto('/login');
  await page.getByTestId('login-email').fill('nobody@example.com');
  await page.getByTestId('login-password').fill('WrongPassword1!');
  await page.getByTestId('login-submit').click();

  const error = page.getByTestId('login-error');
  await expect(error).toBeVisible();
  // The same message a wrong password produces: no user enumeration.
  await expect(error).toContainText(/incorrect/i);
});

test('protected routes bounce an anonymous visitor to the login page', async ({ page }) => {
  await page.goto('/users');
  await page.waitForURL(/\/login/);
  await expect(page.getByTestId('login-submit')).toBeVisible();
});

/**
 * Signing out revokes the entire refresh-token family, so this spec signs in
 * with its own session rather than touching the shared one.
 */
test('signs in, restores the session on reload, then signs out', async ({ page }) => {
  await page.goto('/login');
  await page.getByTestId('login-email').fill(ADMIN_EMAIL);
  await page.getByTestId('login-password').fill(ADMIN_PASSWORD);
  await page.getByTestId('login-submit').click();
  await page.waitForURL('/');
  await expect(page.getByTestId('current-user')).toBeVisible();

  // Session restore comes from the HttpOnly refresh cookie, not from storage.
  await page.reload();
  await expect(page.getByTestId('current-user')).toBeVisible();

  const storage = await page.evaluate(() => ({
    local: JSON.stringify(window.localStorage),
    session: JSON.stringify(window.sessionStorage),
    cookies: document.cookie,
  }));
  expect(storage.local).not.toMatch(/token/i);
  expect(storage.session).not.toMatch(/token/i);
  expect(storage.cookies).not.toContain('wg_refresh_token');

  await page.getByLabel('account menu').click();
  await page.getByTestId('menu-logout').click();
  await page.waitForURL('/login');

  await page.goto('/users');
  await page.waitForURL(/\/login/);
});
