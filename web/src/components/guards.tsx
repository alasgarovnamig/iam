import type { ReactNode } from 'react';
import { Navigate, Outlet, useLocation } from 'react-router-dom';
import { Box, CircularProgress, Tooltip } from '@mui/material';
import { useAuth } from '../auth/AuthContext';

/**
 * Route guards. These keep the UI coherent — they are not the security
 * boundary; every API call is authorized again server-side.
 */

export function RequireAuth() {
  const { status, mustChangePassword } = useAuth();
  const location = useLocation();

  if (status === 'loading') return <FullPageSpinner />;
  if (status === 'anonymous') return <Navigate to="/login" state={{ from: location }} replace />;
  // A forced password change blocks everything except the change page itself.
  if (mustChangePassword && location.pathname !== '/force-password-change') {
    return <Navigate to="/force-password-change" replace />;
  }
  return <Outlet />;
}

export function RequireAnonymous() {
  const { status } = useAuth();
  if (status === 'loading') return <FullPageSpinner />;
  if (status === 'authenticated') return <Navigate to="/" replace />;
  return <Outlet />;
}

/** Blocks a route unless the actor holds at least one of `anyOf`. */
export function RequirePermission({ anyOf, children }: { anyOf: string[]; children: ReactNode }) {
  const { permissions } = useAuth();
  if (!permissions.hasAny(...anyOf)) return <Navigate to="/forbidden" replace />;
  return <>{children}</>;
}

export function FullPageSpinner() {
  return (
    <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '60vh' }}>
      <CircularProgress />
    </Box>
  );
}

/**
 * Renders children only when the actor holds the permission.
 * `fallback` shows something else (e.g. a disabled control) instead.
 */
export function Can({
  permission,
  anyOf,
  children,
  fallback = null,
}: {
  permission?: string;
  anyOf?: string[];
  children: ReactNode;
  fallback?: ReactNode;
}) {
  const { permissions } = useAuth();
  const codes = anyOf ?? (permission ? [permission] : []);
  const allowed = codes.length > 0 && permissions.hasAny(...codes);
  return <>{allowed ? children : fallback}</>;
}

/** Wraps a disabled control with the reason it is disabled. */
export function DisabledHint({ reason, children }: { reason: string | null; children: ReactNode }) {
  if (!reason) return <>{children}</>;
  return (
    <Tooltip title={reason} arrow>
      <span>{children}</span>
    </Tooltip>
  );
}
