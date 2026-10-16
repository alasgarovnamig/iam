import { Route, Routes } from 'react-router-dom';
import { Layout } from './components/Layout';
import { RequireAnonymous, RequireAuth, RequirePermission } from './components/guards';
import { P } from './auth/permissions';
import { LoginPage } from './pages/LoginPage';
import { ForcePasswordChangePage } from './pages/ForcePasswordChangePage';
import { DashboardPage } from './pages/DashboardPage';
import { TenantsPage } from './pages/TenantsPage';
import { TenantDetailsPage } from './pages/TenantDetailsPage';
import { UsersPage } from './pages/UsersPage';
import { UserCreatePage } from './pages/UserCreatePage';
import { UserDetailsPage } from './pages/UserDetailsPage';
import { UserEditPage } from './pages/UserEditPage';
import { UserPermissionsPage } from './pages/UserPermissionsPage';
import { RolesPage } from './pages/RolesPage';
import { RoleCreatePage } from './pages/RoleCreatePage';
import { RoleEditPage } from './pages/RoleEditPage';
import { ApiClientsPage } from './pages/ApiClientsPage';
import { ApiClientCreatePage } from './pages/ApiClientCreatePage';
import { ApiClientDetailsPage } from './pages/ApiClientDetailsPage';
import { AuditLogsPage } from './pages/AuditLogsPage';
import { SessionsPage } from './pages/SessionsPage';
import { ProfilePage } from './pages/ProfilePage';
import { ForbiddenPage } from './pages/ForbiddenPage';
import { NotFoundPage } from './pages/NotFoundPage';

export default function App() {
  return (
    <Routes>
      <Route element={<RequireAnonymous />}>
        <Route path="/login" element={<LoginPage />} />
      </Route>

      <Route element={<RequireAuth />}>
        {/* Outside the layout: the user must not reach the app before changing it. */}
        <Route path="/force-password-change" element={<ForcePasswordChangePage />} />

        <Route element={<Layout />}>
          <Route index element={<DashboardPage />} />

          <Route
            path="tenants"
            element={
              <RequirePermission anyOf={[P.tenants.list, P.tenants.read]}>
                <TenantsPage />
              </RequirePermission>
            }
          />
          <Route
            path="tenants/:tenantId"
            element={
              <RequirePermission anyOf={[P.tenants.read]}>
                <TenantDetailsPage />
              </RequirePermission>
            }
          />

          <Route
            path="users"
            element={
              <RequirePermission anyOf={[P.users.list, P.users.read]}>
                <UsersPage />
              </RequirePermission>
            }
          />
          <Route
            path="users/new"
            element={
              <RequirePermission anyOf={[P.users.create]}>
                <UserCreatePage />
              </RequirePermission>
            }
          />
          <Route
            path="users/:userId"
            element={
              <RequirePermission anyOf={[P.users.read]}>
                <UserDetailsPage />
              </RequirePermission>
            }
          />
          <Route
            path="users/:userId/edit"
            element={
              <RequirePermission anyOf={[P.users.update]}>
                <UserEditPage />
              </RequirePermission>
            }
          />
          <Route
            path="users/:userId/permissions"
            element={
              <RequirePermission anyOf={[P.permissions.read, P.permissions.list]}>
                <UserPermissionsPage />
              </RequirePermission>
            }
          />

          <Route
            path="roles"
            element={
              <RequirePermission anyOf={[P.roles.list, P.roles.read]}>
                <RolesPage />
              </RequirePermission>
            }
          />
          <Route
            path="roles/new"
            element={
              <RequirePermission anyOf={[P.roles.create]}>
                <RoleCreatePage />
              </RequirePermission>
            }
          />
          <Route
            path="roles/:roleId"
            element={
              <RequirePermission anyOf={[P.roles.read]}>
                <RoleEditPage />
              </RequirePermission>
            }
          />

          <Route
            path="api-clients"
            element={
              <RequirePermission anyOf={[P.apiClients.list, P.apiClients.read]}>
                <ApiClientsPage />
              </RequirePermission>
            }
          />
          <Route
            path="api-clients/new"
            element={
              <RequirePermission anyOf={[P.apiClients.create]}>
                <ApiClientCreatePage />
              </RequirePermission>
            }
          />
          <Route
            path="api-clients/:clientId"
            element={
              <RequirePermission anyOf={[P.apiClients.read]}>
                <ApiClientDetailsPage />
              </RequirePermission>
            }
          />

          <Route
            path="audit-logs"
            element={
              <RequirePermission anyOf={[P.auditLogs.list, P.auditLogs.read]}>
                <AuditLogsPage />
              </RequirePermission>
            }
          />

          <Route path="sessions" element={<SessionsPage />} />
          <Route path="profile" element={<ProfilePage />} />
          <Route path="forbidden" element={<ForbiddenPage />} />
          <Route path="*" element={<NotFoundPage />} />
        </Route>
      </Route>

      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  );
}
