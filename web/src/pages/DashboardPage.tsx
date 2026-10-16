import { Link as RouterLink } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import {
  Box,
  Card,
  CardActionArea,
  CardContent,
  Chip,
  Grid,
  Stack,
  Typography,
  alpha,
} from '@mui/material';
import ApartmentIcon from '@mui/icons-material/Apartment';
import PeopleIcon from '@mui/icons-material/People';
import SecurityIcon from '@mui/icons-material/Security';
import VpnKeyIcon from '@mui/icons-material/VpnKey';
import { getList } from '../api/client';
import type { ApiClient, AuditLog, Role, Tenant, User } from '../api/types';
import { useAuth } from '../auth/AuthContext';
import { P } from '../auth/permissions';
import { PageHeader, formatDate } from '../components/common';

/**
 * Each tile is fetched only when the actor may list that resource, so the
 * dashboard never triggers a request that is certain to be denied.
 */
function useCount(key: string, url: string, enabled: boolean) {
  return useQuery({
    queryKey: [key, 'count'],
    queryFn: async () => (await getList<unknown>(url, { page: 1, page_size: 1 })).pagination.total,
    enabled,
  });
}

export function DashboardPage() {
  const { user, permissions } = useAuth();

  const canTenants = permissions.has(P.tenants.list);
  const canUsers = permissions.has(P.users.list);
  const canRoles = permissions.has(P.roles.list);
  const canClients = permissions.has(P.apiClients.list);
  const canAudit = permissions.has(P.auditLogs.list);

  const tenants = useCount('tenants', '/api/v1/tenants', canTenants);
  const users = useCount('users', '/api/v1/users', canUsers);
  const roles = useCount('roles', '/api/v1/roles', canRoles);
  const clients = useCount('api-clients', '/api/v1/api-clients', canClients);

  const recentAudit = useQuery({
    queryKey: ['audit-logs', 'recent'],
    queryFn: async () =>
      (await getList<AuditLog>('/api/v1/audit-logs', { page: 1, page_size: 5, sort_dir: 'desc' })).items,
    enabled: canAudit,
  });

  const tiles = [
    { label: 'Tenants', value: tenants.data, to: '/tenants', show: canTenants, icon: <ApartmentIcon /> },
    { label: 'Users', value: users.data, to: '/users', show: canUsers, icon: <PeopleIcon /> },
    { label: 'Roles', value: roles.data, to: '/roles', show: canRoles, icon: <SecurityIcon /> },
    {
      label: 'API clients',
      value: clients.data,
      to: '/api-clients',
      show: canClients,
      icon: <VpnKeyIcon />,
    },
  ].filter((t) => t.show);

  return (
    <Box>
      <PageHeader
        title={`Welcome${user?.first_name ? `, ${user.first_name}` : ''}`}
        subtitle={
          user?.user_type === 'platform_user'
            ? 'Platform administration'
            : 'Tenant administration'
        }
      />

      <Grid container spacing={2} sx={{ mb: 3 }}>
        {tiles.map((tile) => (
          <Grid item xs={12} sm={6} md={3} key={tile.label}>
            <Card
              sx={{
                height: '100%',
                transition: 'border-color .15s ease, transform .15s ease',
                '&:hover': { borderColor: 'primary.main', transform: 'translateY(-2px)' },
              }}
            >
              <CardActionArea component={RouterLink} to={tile.to} sx={{ height: '100%' }}>
                <CardContent>
                  <Stack direction="row" alignItems="center" justifyContent="space-between">
                    <Typography variant="body2" color="text.secondary" fontWeight={600}>
                      {tile.label}
                    </Typography>
                    <Box
                      aria-hidden
                      sx={{
                        width: 32,
                        height: 32,
                        borderRadius: 2,
                        display: 'grid',
                        placeItems: 'center',
                        color: 'primary.main',
                        bgcolor: (t) => alpha(t.palette.primary.main, 0.12),
                        '& svg': { fontSize: 18 },
                      }}
                    >
                      {tile.icon}
                    </Box>
                  </Stack>
                  <Typography
                    variant="h4"
                    sx={{ mt: 1.5, fontVariantNumeric: 'tabular-nums' }}
                    data-testid={`tile-${tile.label.toLowerCase()}`}
                  >
                    {tile.value ?? '—'}
                  </Typography>
                </CardContent>
              </CardActionArea>
            </Card>
          </Grid>
        ))}
      </Grid>

      {canAudit && (
        <Card>
          <CardContent>
            <Typography variant="h6" gutterBottom>
              Recent activity
            </Typography>
            <Stack divider={<Box sx={{ borderTop: 1, borderColor: 'divider' }} />}>
              {(recentAudit.data ?? []).map((entry) => (
                <Stack
                  key={entry.id}
                  direction="row"
                  spacing={2}
                  alignItems="center"
                  sx={{ py: 1.25 }}
                >
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ minWidth: 168, fontVariantNumeric: 'tabular-nums' }}
                  >
                    {formatDate(entry.occurred_at)}
                  </Typography>
                  <Typography variant="body2" sx={{ flexGrow: 1, minWidth: 0 }} noWrap>
                    <Box component="span" fontWeight={600}>
                      {entry.action}
                    </Box>{' '}
                    <Box component="span" color="text.secondary">
                      · {entry.resource}
                    </Box>
                  </Typography>
                  <Chip
                    size="small"
                    label={entry.result}
                    color={entry.result === 'success' ? 'success' : 'error'}
                  />
                </Stack>
              ))}
              {recentAudit.data?.length === 0 && (
                <Typography variant="body2" color="text.secondary">
                  No audit entries yet.
                </Typography>
              )}
            </Stack>
          </CardContent>
        </Card>
      )}

      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 3 }}>
        You hold {permissions.size} distinct permissions. Menus and actions you cannot use are hidden.
      </Typography>
    </Box>
  );
}

// Keeps the unused-type checker honest about the API shapes used above.
export type _DashboardTypes = [Tenant, User, Role, ApiClient, AuditLog];
