import { useMemo, useState } from 'react';
import { useParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Card,
  CardContent,
  Chip,
  IconButton,
  Snackbar,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import DeleteIcon from '@mui/icons-material/Delete';
import { deleteData, getData, postData } from '../api/client';
import type { PermissionGrant, PermissionTreeService, User } from '../api/types';
import { displayName } from '../api/types';
import { usePermissions } from '../auth/AuthContext';
import { P } from '../auth/permissions';
import { PermissionTree, type GrantSelection } from '../components/PermissionTree';
import { PermissionInfo } from '../components/PermissionInfo';
import { ErrorAlert, FullPageSpinnerFallback, PageHeader } from '../components/common';

export function UserPermissionsPage() {
  const { userId = '' } = useParams();
  const actorPermissions = usePermissions();
  const queryClient = useQueryClient();
  const [toast, setToast] = useState<string | null>(null);

  const userQuery = useQuery({
    queryKey: ['users', userId],
    queryFn: () => getData<User>(`/api/v1/users/${userId}`),
  });

  // Effective = role permissions + direct grants, as computed by the API.
  const effectiveQuery = useQuery({
    queryKey: ['users', userId, 'effective-permissions'],
    queryFn: () => getData<PermissionGrant[]>(`/api/v1/users/${userId}/effective-permissions`),
  });

  const treeQuery = useQuery({
    queryKey: ['permissions', 'tree'],
    queryFn: () => getData<PermissionTreeService[]>('/api/v1/permissions/tree'),
    enabled: actorPermissions.has(P.permissions.assign),
  });

  const effective = useMemo(() => effectiveQuery.data ?? [], [effectiveQuery.data]);
  const existingCodes = useMemo(() => new Set(effective.map((g) => g.code)), [effective]);

  const grant = useMutation({
    mutationFn: (selection: GrantSelection) =>
      postData(`/api/v1/users/${userId}/permissions`, {
        permission_id: selection.permissionId,
        scope: selection.scope,
        effect: 'allow',
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users', userId] });
      setToast('Permission granted');
    },
  });

  const revoke = useMutation({
    mutationFn: (grantId: string) => deleteData(`/api/v1/users/${userId}/permissions/${grantId}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users', userId] });
      setToast('Permission revoked');
    },
  });

  if (userQuery.isLoading || effectiveQuery.isLoading) return <FullPageSpinnerFallback />;

  return (
    <>
      <PageHeader
        title={userQuery.data ? `Permissions — ${displayName(userQuery.data)}` : 'Permissions'}
        subtitle="Everything this user effectively holds, from roles and direct grants."
      />

      <ErrorAlert error={userQuery.error ?? effectiveQuery.error ?? grant.error ?? revoke.error} />

      <Card sx={{ mb: 3 }}>
        <CardContent>
          <Typography variant="h6" gutterBottom>
            Effective permissions ({effective.length})
          </Typography>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Permission</TableCell>
                <TableCell>Scope</TableCell>
                <TableCell>Effect</TableCell>
                {actorPermissions.has(P.permissions.revoke) && <TableCell align="right" />}
              </TableRow>
            </TableHead>
            <TableBody>
              {effective.map((g, index) => (
                <TableRow key={`${g.code}-${g.scope}-${index}`} data-testid="effective-row">
                  <TableCell>
                    <Stack direction="row" alignItems="center" spacing={0.75}>
                      <code>{g.code}</code>
                      <PermissionInfo code={g.code} scope={g.scope} effect={g.effect} />
                    </Stack>
                  </TableCell>
                  <TableCell>
                    <Chip size="small" label={g.scope} />
                  </TableCell>
                  <TableCell>
                    <Chip
                      size="small"
                      label={g.effect}
                      color={g.effect === 'deny' ? 'error' : 'success'}
                      variant="outlined"
                    />
                  </TableCell>
                  {actorPermissions.has(P.permissions.revoke) && (
                    <TableCell align="right">
                      {/* Only direct grants carry an id that can be revoked here;
                          role-derived permissions are removed from the role. */}
                      {g.id ? (
                        <IconButton size="small" onClick={() => revoke.mutate(g.id as string)}>
                          <DeleteIcon fontSize="small" />
                        </IconButton>
                      ) : (
                        <Typography variant="caption" color="text.disabled">
                          via role
                        </Typography>
                      )}
                    </TableCell>
                  )}
                </TableRow>
              ))}
              {effective.length === 0 && (
                <TableRow>
                  <TableCell colSpan={4}>
                    <Typography variant="body2" color="text.secondary">
                      This user holds no permissions.
                    </Typography>
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {actorPermissions.has(P.permissions.assign) && treeQuery.data && (
        <Card>
          <CardContent>
            <Typography variant="h6" gutterBottom>
              Grant a direct permission
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              You can only grant permissions you hold, at your scope or narrower.
            </Typography>
            <PermissionTree
              tree={treeQuery.data}
              actorPermissions={actorPermissions}
              existingCodes={existingCodes}
              onGrant={(selection) => grant.mutate(selection)}
              disabled={grant.isPending}
            />
          </CardContent>
        </Card>
      )}

      <Snackbar
        open={Boolean(toast)}
        autoHideDuration={3000}
        onClose={() => setToast(null)}
        message={toast ?? ''}
      />
    </>
  );
}
