import { useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Alert,
  Button,
  Card,
  CardContent,
  Chip,
  IconButton,
  MenuItem,
  Snackbar,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from '@mui/material';
import DeleteIcon from '@mui/icons-material/Delete';
import LockIcon from '@mui/icons-material/Lock';
import { deleteData, getData, patchData, postData } from '../api/client';
import type {
  PermissionGrant,
  PermissionTreeService,
  RoleWithPermissions,
  Scope,
} from '../api/types';
import { SCOPES, SCOPE_LABELS } from '../api/types';
import { usePermissions } from '../auth/AuthContext';
import { P } from '../auth/permissions';
import { PermissionTree, type GrantSelection } from '../components/PermissionTree';
import { PermissionInfo } from '../components/PermissionInfo';
import { ErrorAlert, FullPageSpinnerFallback, PageHeader } from '../components/common';

export function RoleEditPage() {
  const { roleId = '' } = useParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const actorPermissions = usePermissions();
  const [toast, setToast] = useState<string | null>(null);

  const roleQuery = useQuery({
    queryKey: ['roles', roleId],
    queryFn: () => getData<RoleWithPermissions>(`/api/v1/roles/${roleId}`),
  });

  const treeQuery = useQuery({
    queryKey: ['permissions', 'tree'],
    queryFn: () => getData<PermissionTreeService[]>('/api/v1/permissions/tree'),
    enabled: actorPermissions.hasAny(P.permissions.list, P.permissions.read),
  });

  const role = roleQuery.data?.role;
  const grants = useMemo(() => roleQuery.data?.permissions ?? [], [roleQuery.data]);
  const isSystem = role?.is_system ?? false;

  const existingCodes = useMemo(() => new Set(grants.map((g) => g.code)), [grants]);

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');

  // Seed the form from the server copy, and re-seed whenever the server copy
  // changes underneath us (a save bumps the version). Keying on id+version
  // rather than on "the fields are still empty" means clearing a field does not
  // silently refill itself, and a fresh version does not leave a stale form.
  const loadedStamp = role ? `${role.id}:${role.version}` : '';
  const [seededStamp, setSeededStamp] = useState('');
  useEffect(() => {
    if (role && loadedStamp !== seededStamp) {
      setName(role.name);
      setDescription(role.description);
      setSeededStamp(loadedStamp);
    }
  }, [role, loadedStamp, seededStamp]);

  const updateRole = useMutation({
    mutationFn: () =>
      patchData(`/api/v1/roles/${roleId}`, { name, description, version: role?.version }),
    // ['roles'] is a key prefix, so this also invalidates ['roles', roleId] and
    // the detail query refetches with the bumped version.
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['roles'] });
      setToast('Role updated');
    },
    // A rejected save usually means someone else moved the role on. Pull the
    // fresh copy so the retry carries the current version instead of failing
    // with the same conflict forever.
    onError: () => {
      queryClient.invalidateQueries({ queryKey: ['roles', roleId] });
    },
  });

  const addPermission = useMutation({
    mutationFn: (selection: GrantSelection) =>
      postData(`/api/v1/roles/${roleId}/permissions`, {
        permission_id: selection.permissionId,
        scope: selection.scope,
        effect: 'allow',
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['roles', roleId] });
      setToast('Permission granted');
    },
  });

  // There is no "update grant" endpoint, by design: a grant is revoked and
  // re-issued so the delegation rule is re-checked against the new scope.
  const changeScope = useMutation({
    mutationFn: async ({ grant, scope }: { grant: PermissionGrant; scope: Scope }) => {
      if (grant.id) {
        await deleteData(`/api/v1/roles/${roleId}/permissions/${grant.id}`);
      }
      return postData(`/api/v1/roles/${roleId}/permissions`, {
        permission_id: grant.permission_id,
        scope,
        effect: grant.effect,
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['roles', roleId] });
      setToast('Scope updated');
    },
    onError: () => {
      queryClient.invalidateQueries({ queryKey: ['roles', roleId] });
    },
  });

  const removePermission = useMutation({
    mutationFn: (grantId: string) => deleteData(`/api/v1/roles/${roleId}/permissions/${grantId}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['roles', roleId] });
      setToast('Permission revoked');
    },
  });

  const deleteRole = useMutation({
    mutationFn: () => deleteData(`/api/v1/roles/${roleId}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['roles'] });
      navigate('/roles');
    },
  });

  if (roleQuery.isLoading) return <FullPageSpinnerFallback />;
  if (roleQuery.error) return <ErrorAlert error={roleQuery.error} />;
  if (!role) return null;

  const canEdit = actorPermissions.has(P.roles.update) && !isSystem;
  const dirty = name !== role.name || description !== role.description;
  const canAssign = actorPermissions.has(P.permissions.assign) && !isSystem;
  const canRevoke = actorPermissions.has(P.permissions.revoke) && !isSystem;

  return (
    <>
      <PageHeader
        title={role.name}
        subtitle={role.is_system ? 'System role — read only' : `Custom role · ${role.role_type}`}
        actions={
          canEdit && (
            <Button
              color="error"
              onClick={() => deleteRole.mutate()}
              disabled={deleteRole.isPending}
              data-testid="delete-role"
            >
              Delete role
            </Button>
          )
        }
      />

      {isSystem ? (
        <Alert severity="info" sx={{ mb: 2 }} icon={<LockIcon fontSize="inherit" />}>
          <strong>This is a system role, so it is read-only by design.</strong> The platform defines
          it, every tenant sees the same one, and nobody — not even a platform administrator — can
          rename it or change what it grants. That is what makes it safe to rely on. To get
          something you can edit, create a custom role and grant it the permissions you need.
        </Alert>
      ) : (
        !actorPermissions.has(P.roles.update) && (
          <Alert severity="warning" sx={{ mb: 2 }}>
            You can view this role but not change it — editing needs the{' '}
            <code>roles.update</code> permission.
          </Alert>
        )
      )}

      <ErrorAlert error={updateRole.error ?? addPermission.error ?? removePermission.error ?? deleteRole.error} />

      <Card sx={{ mb: 3, maxWidth: 720 }}>
        <CardContent>
          <Stack spacing={2}>
            <TextField
              label="Name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={!canEdit}
              fullWidth
              inputProps={{ 'data-testid': 'role-name' }}
            />
            <TextField
              label="Description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              disabled={!canEdit}
              fullWidth
              multiline
              rows={2}
            />
            {canEdit && (
              <Stack direction="row" alignItems="center" spacing={2}>
                <Button
                  variant="contained"
                  onClick={() => updateRole.mutate()}
                  disabled={updateRole.isPending || !dirty}
                  data-testid="save-role"
                >
                  {updateRole.isPending ? 'Saving…' : 'Save changes'}
                </Button>
                {dirty && (
                  <Typography variant="body2" color="text.secondary">
                    Unsaved changes
                  </Typography>
                )}
              </Stack>
            )}
          </Stack>
        </CardContent>
      </Card>

      <Card sx={{ mb: 3 }}>
        <CardContent>
          <Typography variant="h6" gutterBottom>
            Attached permissions ({grants.length})
          </Typography>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Permission</TableCell>
                <TableCell>Scope</TableCell>
                <TableCell>Effect</TableCell>
                {canRevoke && <TableCell align="right">Actions</TableCell>}
              </TableRow>
            </TableHead>
            <TableBody>
              {grants.map((grant) => (
                <TableRow key={grant.id} data-testid="grant-row">
                  <TableCell>
                    <Stack direction="row" alignItems="center" spacing={0.75}>
                      <code>{grant.code}</code>
                      <PermissionInfo code={grant.code} scope={grant.scope} effect={grant.effect} />
                    </Stack>
                  </TableCell>
                  <TableCell>
                    {canAssign && canRevoke && grant.permission_id ? (
                      <TextField
                        select
                        size="small"
                        value={grant.scope}
                        onChange={(e) =>
                          changeScope.mutate({ grant, scope: e.target.value as Scope })
                        }
                        disabled={changeScope.isPending}
                        sx={{ minWidth: 170 }}
                        inputProps={{ 'data-testid': `scope-of-${grant.code}` }}
                      >
                        {SCOPES.map((s) => {
                          const allowed = actorPermissions
                            .delegatableScopes(grant.code, SCOPES)
                            .includes(s);
                          return (
                            <MenuItem key={s} value={s} disabled={!allowed && s !== grant.scope}>
                              {SCOPE_LABELS[s]}
                              {!allowed && s !== grant.scope && ' — wider than yours'}
                            </MenuItem>
                          );
                        })}
                      </TextField>
                    ) : (
                      <Chip size="small" label={grant.scope} />
                    )}
                  </TableCell>
                  <TableCell>
                    <Chip
                      size="small"
                      label={grant.effect}
                      color={grant.effect === 'deny' ? 'error' : 'success'}
                      variant="outlined"
                    />
                  </TableCell>
                  {canRevoke && (
                    <TableCell align="right">
                      <IconButton
                        size="small"
                        onClick={() => grant.id && removePermission.mutate(grant.id)}
                        data-testid={`revoke-${grant.code}`}
                      >
                        <DeleteIcon fontSize="small" />
                      </IconButton>
                    </TableCell>
                  )}
                </TableRow>
              ))}
              {grants.length === 0 && (
                <TableRow>
                  <TableCell colSpan={4}>
                    <Typography variant="body2" color="text.secondary">
                      No permissions attached yet.
                    </Typography>
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {canAssign && treeQuery.data && (
        <Card>
          <CardContent>
            <Typography variant="h6" gutterBottom>
              Grant a permission
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              You can only grant what you hold yourself, at your own scope or narrower. Locked entries show why.
            </Typography>
            <PermissionTree
              tree={treeQuery.data}
              actorPermissions={actorPermissions}
              existingCodes={existingCodes}
              onGrant={(selection) => addPermission.mutate(selection)}
              disabled={addPermission.isPending}
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
