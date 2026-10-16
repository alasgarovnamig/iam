import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { getData, postData, setAccessToken, setUnauthenticatedHandler } from '../api/client';
import type { LoginResponse, MeResponse, User } from '../api/types';
import { PermissionSet } from './permissions';

interface AuthState {
  user: User | null;
  permissions: PermissionSet;
  status: 'loading' | 'authenticated' | 'anonymous';
  mustChangePassword: boolean;
}

interface AuthContextValue extends AuthState {
  login: (email: string, password: string, tenantSlug?: string) => Promise<void>;
  logout: () => Promise<void>;
  refreshMe: () => Promise<void>;
  clearMustChangePassword: () => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

const emptyPermissions = new PermissionSet([]);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>({
    user: null,
    permissions: emptyPermissions,
    status: 'loading',
    mustChangePassword: false,
  });

  const loadMe = useCallback(async () => {
    const me = await getData<MeResponse>('/api/v1/auth/me');
    setState((prev) => ({
      ...prev,
      user: me.user,
      permissions: PermissionSet.fromRaw(me.permissions),
      status: 'authenticated',
      mustChangePassword: me.user.must_change_password,
    }));
  }, []);

  /**
   * Session restore on page load: no token is stored anywhere readable by
   * script, so we ask the API to mint a fresh access token from the HttpOnly
   * refresh cookie. Failure simply means "not signed in".
   */
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const tokens = await postData<{ access_token: string }>('/api/v1/auth/refresh');
        if (cancelled) return;
        setAccessToken(tokens.access_token);
        await loadMe();
      } catch {
        if (!cancelled) {
          setAccessToken(null);
          setState({ user: null, permissions: emptyPermissions, status: 'anonymous', mustChangePassword: false });
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [loadMe]);

  // A failed refresh anywhere in the app drops the session.
  useEffect(() => {
    setUnauthenticatedHandler(() => {
      setAccessToken(null);
      setState({ user: null, permissions: emptyPermissions, status: 'anonymous', mustChangePassword: false });
    });
    return () => setUnauthenticatedHandler(null);
  }, []);

  const login = useCallback(
    async (email: string, password: string, tenantSlug?: string) => {
      const result = await postData<LoginResponse>('/api/v1/auth/login', {
        email,
        password,
        ...(tenantSlug ? { tenant_slug: tenantSlug } : {}),
      });
      setAccessToken(result.access_token);
      // must_change_password gates the whole app until the user changes it.
      setState({
        user: result.user,
        permissions: emptyPermissions,
        status: 'authenticated',
        mustChangePassword: result.must_change_password,
      });
      await loadMe();
      setState((prev) => ({ ...prev, mustChangePassword: result.must_change_password }));
    },
    [loadMe],
  );

  const logout = useCallback(async () => {
    try {
      await postData('/api/v1/auth/logout');
    } finally {
      setAccessToken(null);
      setState({ user: null, permissions: emptyPermissions, status: 'anonymous', mustChangePassword: false });
    }
  }, []);

  const clearMustChangePassword = useCallback(() => {
    setState((prev) => ({ ...prev, mustChangePassword: false }));
  }, []);

  const value = useMemo<AuthContextValue>(
    () => ({ ...state, login, logout, refreshMe: loadMe, clearMustChangePassword }),
    [state, login, logout, loadMe, clearMustChangePassword],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

// eslint-disable-next-line react-refresh/only-export-components
export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used inside <AuthProvider>');
  return ctx;
}

// eslint-disable-next-line react-refresh/only-export-components
export function usePermissions(): PermissionSet {
  return useAuth().permissions;
}
