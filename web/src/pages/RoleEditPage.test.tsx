import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Role, RoleWithPermissions } from '../api/types';
import { PermissionSet } from '../auth/permissions';

/**
 * Cover for role editing.
 *
 * What actually made editing feel broken: almost every role in a fresh install
 * is a system role, and the page disabled the form with only a one-line note —
 * no save button, no explanation of why. These tests pin the explanation down,
 * along with the smaller edges: save stays disabled until something changes, a
 * rejected save refetches so the retry is not doomed, and clearing a field does
 * not silently refill itself.
 */

const { getData, patchData, deleteData, postData } = vi.hoisted(() => ({
  getData: vi.fn(),
  patchData: vi.fn(),
  deleteData: vi.fn(),
  postData: vi.fn(),
}));

// Partial mock: ErrorAlert needs the real ApiError class to narrow on.
vi.mock('../api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/client')>()),
  getData,
  patchData,
  deleteData,
  postData,
}));

vi.mock('react-router-dom', () => ({
  useParams: () => ({ roleId: 'role-1' }),
  useNavigate: () => vi.fn(),
}));

let actorPermissions = PermissionSet.fromRaw([]);
vi.mock('../auth/AuthContext', () => ({
  usePermissions: () => actorPermissions,
}));

const { RoleEditPage } = await import('./RoleEditPage');

const ADMIN = PermissionSet.fromRaw([
  'user-management.roles.update:tenant',
  'user-management.roles.read:tenant',
]);

function makeRole(overrides: Partial<Role> = {}): Role {
  return {
    id: 'role-1',
    name: 'Ops Role',
    description: 'first description',
    role_type: 'tenant',
    is_system: false,
    status: 'active',
    created_at: '2026-07-25T06:00:00Z',
    version: 1,
    ...overrides,
  };
}

function payload(role: Role): RoleWithPermissions {
  return { role, permissions: [] };
}

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <RoleEditPage />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  actorPermissions = ADMIN;
});

describe('RoleEditPage', () => {
  // Characterisation, not a regression: ['roles'] is a key prefix so the detail
  // query already refetched. Pinned so a future "optimisation" that narrows the
  // invalidation cannot silently reintroduce a stale-version conflict.
  it('sends the refreshed version on a second save, not the one it loaded with', async () => {
    const user = userEvent.setup();

    // The server copy advances to version 2 once the first save lands.
    let serverRole = makeRole();
    getData.mockImplementation(async () => payload(serverRole));
    patchData.mockImplementation(async () => {
      serverRole = makeRole({ name: 'Renamed once', version: 2 });
      return serverRole;
    });

    renderPage();
    const nameField = await screen.findByTestId('role-name');

    await user.clear(nameField);
    await user.type(nameField, 'Renamed once');
    await user.click(screen.getByTestId('save-role'));

    await waitFor(() => expect(patchData).toHaveBeenCalledTimes(1));
    expect(patchData.mock.calls[0][1]).toMatchObject({ name: 'Renamed once', version: 1 });

    // The page must now be holding version 2, not the version it loaded with.
    await waitFor(() => expect(screen.getByTestId('role-name')).toHaveValue('Renamed once'));

    await user.clear(screen.getByTestId('role-name'));
    await user.type(screen.getByTestId('role-name'), 'Renamed twice');
    await user.click(screen.getByTestId('save-role'));

    await waitFor(() => expect(patchData).toHaveBeenCalledTimes(2));
    // The regression: this used to be 1, which the API rejected as a conflict.
    expect(patchData.mock.calls[1][1]).toMatchObject({ name: 'Renamed twice', version: 2 });
  });

  it('refetches the role after a rejected save so a retry is not doomed', async () => {
    const user = userEvent.setup();
    getData.mockResolvedValue(payload(makeRole()));
    patchData.mockRejectedValue(new Error('conflict'));

    renderPage();
    const nameField = await screen.findByTestId('role-name');
    await user.clear(nameField);
    await user.type(nameField, 'Attempted');

    const getCallsBefore = getData.mock.calls.length;
    await user.click(screen.getByTestId('save-role'));

    await waitFor(() => expect(patchData).toHaveBeenCalled());
    await waitFor(() => expect(getData.mock.calls.length).toBeGreaterThan(getCallsBefore));
  });

  it('keeps save disabled until something actually changes', async () => {
    const user = userEvent.setup();
    getData.mockResolvedValue(payload(makeRole()));

    renderPage();
    const nameField = await screen.findByTestId('role-name');
    await waitFor(() => expect(screen.getByTestId('save-role')).toBeDisabled());

    await user.type(nameField, '!');
    await waitFor(() => expect(screen.getByTestId('save-role')).toBeEnabled());
  });

  it('does not silently refill a cleared name when the description is empty', async () => {
    const user = userEvent.setup();
    // The empty description is the trigger: the old init re-seeded whenever
    // BOTH fields were empty, so clearing the name of a role with no
    // description put the old name straight back.
    getData.mockResolvedValue(payload(makeRole({ description: '' })));

    renderPage();
    const nameField = await screen.findByTestId('role-name');
    await user.clear(nameField);

    await waitFor(() => expect(nameField).toHaveValue(''));
  });

  it('explains that a system role is read-only rather than just disabling it', async () => {
    getData.mockResolvedValue(payload(makeRole({ is_system: true })));

    renderPage();
    expect(await screen.findByText(/read-only by design/i)).toBeInTheDocument();
    expect(screen.getByTestId('role-name')).toBeDisabled();
    expect(screen.queryByTestId('save-role')).not.toBeInTheDocument();
  });

  it('tells a viewer without roles.update why they cannot edit', async () => {
    actorPermissions = PermissionSet.fromRaw(['user-management.roles.read:tenant']);
    getData.mockResolvedValue(payload(makeRole()));

    renderPage();
    expect(await screen.findByText(/editing needs the/i)).toBeInTheDocument();
    expect(screen.queryByTestId('save-role')).not.toBeInTheDocument();
  });
});
