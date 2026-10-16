import { useState } from 'react';
import { useParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Button,
  Card,
  CardContent,
  Grid,
  MenuItem,
  Snackbar,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { getData, patchData } from '../api/client';
import type { Tenant, TenantStatus } from '../api/types';
import { useAuth } from '../auth/AuthContext';
import { P } from '../auth/permissions';
import {
  ErrorAlert,
  FullPageSpinnerFallback,
  PageHeader,
  StatusChip,
  formatDate,
} from '../components/common';

const STATUSES: TenantStatus[] = ['pending', 'active', 'suspended', 'terminated'];

export function TenantDetailsPage() {
  const { tenantId = '' } = useParams();
  const queryClient = useQueryClient();
  const { user, permissions } = useAuth();
  const [toast, setToast] = useState<string | null>(null);
  const [name, setName] = useState<string | null>(null);
  const [status, setStatus] = useState<string | null>(null);

  const query = useQuery({
    queryKey: ['tenants', tenantId],
    queryFn: () => getData<Tenant>(`/api/v1/tenants/${tenantId}`),
  });

  const update = useMutation({
    mutationFn: () =>
      patchData<Tenant>(`/api/v1/tenants/${tenantId}`, {
        ...(name !== null ? { name } : {}),
        ...(status !== null ? { status } : {}),
        version: query.data?.version,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tenants'] });
      setToast('Tenant updated');
      setName(null);
      setStatus(null);
    },
  });

  if (query.isLoading) return <FullPageSpinnerFallback />;
  if (query.error) return <ErrorAlert error={query.error} />;
  const tenant = query.data;
  if (!tenant) return null;

  const canEdit = permissions.has(P.tenants.update);
  // Only the platform may move a tenant through its lifecycle.
  const canChangeStatus = canEdit && user?.user_type === 'platform_user';

  return (
    <>
      <PageHeader title={tenant.name} subtitle={`Slug: ${tenant.slug}`} />
      <ErrorAlert error={update.error} />

      <Card sx={{ maxWidth: 720 }}>
        <CardContent>
          <Grid container spacing={2} sx={{ mb: 3 }}>
            <Grid item xs={6}>
              <Typography variant="caption" color="text.secondary">
                Status
              </Typography>
              <Typography variant="body1">
                <StatusChip status={tenant.status} kind="tenant" />
              </Typography>
            </Grid>
            <Grid item xs={6}>
              <Typography variant="caption" color="text.secondary">
                Created
              </Typography>
              <Typography variant="body1">{formatDate(tenant.created_at)}</Typography>
            </Grid>
          </Grid>

          <Stack spacing={2}>
            <TextField
              label="Name"
              value={name ?? tenant.name}
              onChange={(e) => setName(e.target.value)}
              disabled={!canEdit}
              fullWidth
              inputProps={{ 'data-testid': 'tenant-name' }}
            />
            <TextField
              select
              label="Status"
              value={status ?? tenant.status}
              onChange={(e) => setStatus(e.target.value)}
              disabled={!canChangeStatus}
              helperText={canChangeStatus ? undefined : 'Only platform administrators can change tenant status.'}
              fullWidth
              inputProps={{ 'data-testid': 'tenant-status' }}
            >
              {STATUSES.map((s) => (
                <MenuItem key={s} value={s}>
                  {s}
                </MenuItem>
              ))}
            </TextField>
            {canEdit && (
              <Stack direction="row" justifyContent="flex-end">
                <Button
                  variant="contained"
                  onClick={() => update.mutate()}
                  disabled={update.isPending || (name === null && status === null)}
                  data-testid="save-tenant"
                >
                  Save changes
                </Button>
              </Stack>
            )}
          </Stack>
        </CardContent>
      </Card>

      <Snackbar open={Boolean(toast)} autoHideDuration={3000} onClose={() => setToast(null)} message={toast ?? ''} />
    </>
  );
}
