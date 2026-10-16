import { expect, test } from './fixtures';
import { ADMIN_EMAIL } from './fixtures';

/**
 * End-to-end coverage against a running stack (`docker compose up --build`).
 *
 * These specs run inside one shared, already-signed-in admin context (see
 * fixtures.ts). Authentication itself is covered by auth.spec.ts, which uses
 * signed-out browsers so it cannot disturb this session.
 *
 * Records are created with unique names so repeated runs stay independent.
 */

const API_URL = process.env.API_URL ?? 'http://localhost:8090';
const unique = () => Date.now().toString(36) + Math.random().toString(36).slice(2, 6);

test.describe('tenant and user management', () => {
  test('creates a tenant, then a user inside it', async ({ page }) => {
    const slug = `acme-${unique()}`;

    await page.goto('/tenants');
    await page.getByTestId('create-tenant').click();
    await page.getByTestId('tenant-name').fill(`Acme ${slug}`);
    await page.getByTestId('tenant-slug').fill(slug);
    await page.getByTestId('tenant-submit').click();

    await expect(page.getByText(`Acme ${slug}`)).toBeVisible({ timeout: 10_000 });

    await page.goto('/users/new');
    const email = `user-${unique()}@acme.test`;
    await page.getByTestId('user-email').fill(email);
    await page.getByTestId('user-password').fill('Tenant@Str0ng1');
    await page.getByTestId('user-first-name').fill('Test');
    await page.getByTestId('user-last-name').fill('User');
    await page.getByTestId('user-submit').click();

    // Landing on the details page means the API accepted the create.
    await expect(page).toHaveURL(/\/users\/[0-9a-f-]{36}$/, { timeout: 10_000 });
    await expect(page.getByTestId('field-email')).toContainText(email);
  });

  test('rejects a duplicate email with a conflict, not a second user', async ({ page }) => {
    const email = `dupe-${unique()}@acme.test`;

    for (const attempt of [1, 2]) {
      await page.goto('/users/new');
      await page.getByTestId('user-email').fill(email);
      await page.getByTestId('user-password').fill('Tenant@Str0ng1');
      await page.getByTestId('user-first-name').fill('Dupe');
      await page.getByTestId('user-submit').click();

      if (attempt === 1) {
        await expect(page).toHaveURL(/\/users\/[0-9a-f-]{36}$/, { timeout: 10_000 });
      } else {
        await expect(page.getByTestId('error-alert')).toContainText(/already exists/i);
      }
    }
  });

  test('shows the effective permissions of a user', async ({ page }) => {
    await page.goto('/users');

    // Target the bootstrap admin explicitly: users created by other specs have
    // no roles, so "the first row" would legitimately show an empty set.
    await page.getByTestId('filter-email').fill(ADMIN_EMAIL);
    await page.getByTestId('user-row').first().getByRole('link').click();
    await page.getByTestId('view-permissions').click();

    await expect(page).toHaveURL(/\/permissions$/);
    // The bootstrap admin is Super Admin, so it holds the whole catalog.
    await expect(page.getByTestId('effective-row').first()).toBeVisible({ timeout: 10_000 });
  });
});

test.describe('roles and permission delegation', () => {
  test('creates a custom role and grants it a permission the actor holds', async ({ page }) => {
    await page.goto('/roles/new');
    await page.getByTestId('role-name').fill(`Support ${unique()}`);
    await page.getByTestId('role-description').fill('Created by the e2e suite');
    await page.getByTestId('role-submit').click();

    await expect(page).toHaveURL(/\/roles\/[0-9a-f-]{36}$/, { timeout: 10_000 });

    // Grant users.read — the super admin holds it, so the tree offers it.
    await page.getByTestId('permission-tree').scrollIntoViewIfNeeded();
    await page.getByTestId('tree-resource-users').click();
    await page.getByTestId('grant-user-management.users.read').click();

    // Assert inside the attached-permissions table: the same code also appears
    // in the tree below, so an unscoped match would be ambiguous.
    const grantRow = page.getByTestId('grant-row').first();
    await expect(grantRow).toBeVisible({ timeout: 10_000 });
    await expect(grantRow).toContainText('user-management.users.read');
  });

  test('offers grantable permissions to an actor that holds them', async ({ page }) => {
    await page.goto('/roles/new');
    await page.getByTestId('role-name').fill(`Grantable ${unique()}`);
    await page.getByTestId('role-submit').click();
    await expect(page).toHaveURL(/\/roles\/[0-9a-f-]{36}$/, { timeout: 10_000 });

    // The bootstrap super admin holds everything, so entries are enabled and
    // the widest scope is on offer. The inverse (a limited actor sees locked
    // entries and narrower scopes) is covered by the unit tests.
    await page.getByTestId('tree-resource-users').click();
    await expect(page.getByTestId('grant-user-management.users.read')).toBeEnabled({ timeout: 10_000 });
    await expect(page.getByTestId('perm-locked-user-management.users.read')).toHaveCount(0);
  });

  test('system roles cannot be edited', async ({ page }) => {
    await page.goto('/roles');
    await page.getByRole('link', { name: 'Super Admin' }).click();

    await expect(page.getByText(/System roles are defined by the platform/i)).toBeVisible();
    await expect(page.getByTestId('role-name')).toBeDisabled();
    await expect(page.getByTestId('delete-role')).toHaveCount(0);
  });
});

test.describe('API clients', () => {
  test('creates a client, shows the secret once, and takes a token with it', async ({ page, request }) => {
    await page.goto('/api-clients/new');
    await page.getByTestId('client-name').fill(`E2E client ${unique()}`);
    await page.getByTestId('client-scopes').fill('wallet.read');

    // Platform users must pick a tenant.
    if (await page.getByTestId('client-tenant').count()) {
      await page.getByLabel('Tenant').click();
      await page.getByRole('option').first().click();
    }
    await page.getByTestId('client-submit').click();

    // The secret dialog appears exactly once, right after creation.
    await expect(page.getByTestId('secret-dialog')).toBeVisible({ timeout: 10_000 });
    const secret = await page.getByTestId('client-secret-value').inputValue();
    expect(secret.length).toBeGreaterThan(20);

    await page.getByTestId('ack-secret').check();
    await page.getByTestId('close-secret').click();
    await expect(page).toHaveURL(/\/api-clients\/[0-9a-f-]{36}$/);

    const clientId = (await page.getByTestId('client-id').innerText()).trim();

    // The secret is never shown again.
    await page.reload();
    await expect(page.getByTestId('secret-dialog')).toHaveCount(0);
    await expect(page.getByText(secret)).toHaveCount(0);

    // And it really works as an OAuth client credential.
    const tokenResponse = await request.post(`${API_URL}/api/v1/oauth/token`, {
      data: { grant_type: 'client_credentials', client_id: clientId, client_secret: secret },
    });
    expect(tokenResponse.status()).toBe(200);
    const token = await tokenResponse.json();
    expect(token.token_type).toBe('Bearer');
    expect(token.access_token).toBeTruthy();

    // A wrong secret is refused.
    const denied = await request.post(`${API_URL}/api/v1/oauth/token`, {
      data: { grant_type: 'client_credentials', client_id: clientId, client_secret: 'wrong-secret' },
    });
    expect(denied.status()).toBe(401);
  });

  test('rotating the secret invalidates the previous one', async ({ page, request }) => {
    await page.goto('/api-clients/new');
    await page.getByTestId('client-name').fill(`Rotate ${unique()}`);
    if (await page.getByTestId('client-tenant').count()) {
      await page.getByLabel('Tenant').click();
      await page.getByRole('option').first().click();
    }
    await page.getByTestId('client-submit').click();

    await expect(page.getByTestId('secret-dialog')).toBeVisible({ timeout: 10_000 });
    const firstSecret = await page.getByTestId('client-secret-value').inputValue();
    await page.getByTestId('ack-secret').check();
    await page.getByTestId('close-secret').click();
    const clientId = (await page.getByTestId('client-id').innerText()).trim();

    await page.getByTestId('rotate-secret').click();
    await expect(page.getByTestId('secret-dialog')).toBeVisible({ timeout: 10_000 });
    const secondSecret = await page.getByTestId('client-secret-value').inputValue();
    expect(secondSecret).not.toBe(firstSecret);
    await page.getByTestId('ack-secret').check();
    await page.getByTestId('close-secret').click();

    const oldSecret = await request.post(`${API_URL}/api/v1/oauth/token`, {
      data: { grant_type: 'client_credentials', client_id: clientId, client_secret: firstSecret },
    });
    expect(oldSecret.status()).toBe(401);

    const newSecret = await request.post(`${API_URL}/api/v1/oauth/token`, {
      data: { grant_type: 'client_credentials', client_id: clientId, client_secret: secondSecret },
    });
    expect(newSecret.status()).toBe(200);
  });
});

test.describe('audit and navigation', () => {
  test('audit logs record the activity of this run', async ({ page }) => {
    await page.goto('/audit-logs');
    await expect(page.getByTestId('audit-row').first()).toBeVisible({ timeout: 10_000 });

    await page.getByTestId('filter-action').fill('auth.login');
    await expect(page.getByTestId('audit-row').first()).toBeVisible({ timeout: 10_000 });
  });

  test('an unknown route renders the not-found page', async ({ page }) => {
    await page.goto('/this-route-does-not-exist');
    await expect(page.getByTestId('not-found-page')).toBeVisible();
  });
});
