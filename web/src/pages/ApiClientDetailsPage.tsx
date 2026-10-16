import { useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Button,
  Card,
  CardContent,
  Chip,
  Grid,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import { deleteData, getData, postData } from '../api/client';
import type { ApiClientWithPermissions, CreatedApiClient } from '../api/types';
import { P } from '../auth/permissions';
import { Can } from '../components/guards';
import { SecretDisplay } from '../components/SecretDisplay';
import {
  ErrorAlert,
  FullPageSpinnerFallback,
  PageHeader,
  StatusChip,
  formatDate,
} from '../components/common';
import { PermissionInfo } from '../components/PermissionInfo';

export function ApiClientDetailsPage() {
  const { clientId = '' } = useParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [rotated, setRotated] = useState<CreatedApiClient | null>(null);

  const query = useQuery({
    queryKey: ['api-clients', clientId],
    queryFn: () => getData<ApiClientWithPermissions>(`/api/v1/api-clients/${clientId}`),
  });

  const rotate = useMutation({
    mutationFn: () => postData<CreatedApiClient>(`/api/v1/api-clients/${clientId}/rotate-secret`),
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ['api-clients'] });
      setRotated(result);
    },
  });

  const revoke = useMutation({
    mutationFn: () => deleteData(`/api/v1/api-clients/${clientId}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['api-clients'] });
      navigate('/api-clients');
    },
  });

  if (query.isLoading) return <FullPageSpinnerFallback />;
  if (query.error) return <ErrorAlert error={query.error} />;
  const data = query.data;
  if (!data) return null;
  const client = data.client;

  return (
    <>
      <PageHeader
        title={client.name}
        subtitle={client.description || 'Machine-to-machine client'}
        actions={
          <Stack direction="row" spacing={1}>
            <Can permission={P.apiClients.rotate}>
              <Button
                variant="outlined"
                onClick={() => rotate.mutate()}
                disabled={rotate.isPending || client.status === 'revoked'}
                data-testid="rotate-secret"
              >
                Rotate secret
              </Button>
            </Can>
            <Can permission={P.apiClients.delete}>
              <Button
                color="error"
                onClick={() => revoke.mutate()}
                disabled={revoke.isPending || client.status === 'revoked'}
                data-testid="revoke-client"
              >
                Revoke
              </Button>
            </Can>
          </Stack>
        }
      />

      <ErrorAlert error={rotate.error ?? revoke.error} />

      <Card sx={{ mb: 3, maxWidth: 860 }}>
        <CardContent>
          <Grid container spacing={2}>
            <Grid item xs={12} sm={6}>
              <Typography variant="caption" color="text.secondary">
                Client ID
              </Typography>
              <Typography variant="body1" data-testid="client-id">
                <code>{client.client_id}</code>
              </Typography>
            </Grid>
            <Grid item xs={12} sm={6}>
              <Typography variant="caption" color="text.secondary">
                Status
              </Typography>
              <Typography variant="body1">
                <StatusChip status={client.status} kind="api-client" />
              </Typography>
            </Grid>
            <Grid item xs={12} sm={6}>
              <Typography variant="caption" color="text.secondary">
                Client secret
              </Typography>
              <Typography variant="body2" color="text.disabled">
                Stored hashed — shown only at creation and rotation.
              </Typography>
            </Grid>
            <Grid item xs={12} sm={6}>
              <Typography variant="caption" color="text.secondary">
                Last used
              </Typography>
              <Typography variant="body1">{formatDate(client.last_used_at)}</Typography>
            </Grid>
            <Grid item xs={12} sm={6}>
              <Typography variant="caption" color="text.secondary">
                Allowed IPs
              </Typography>
              <Typography variant="body1">
                {client.allowed_ips.length ? client.allowed_ips.join(', ') : 'Any'}
              </Typography>
            </Grid>
            <Grid item xs={12} sm={6}>
              <Typography variant="caption" color="text.secondary">
                Scopes
              </Typography>
              <Typography variant="body1">
                {client.scopes.length ? client.scopes.join(', ') : '—'}
              </Typography>
            </Grid>
          </Grid>
        </CardContent>
      </Card>

      <Card>
        <CardContent>
          <Typography variant="h6" gutterBottom>
            Permissions ({data.permissions.length})
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            The client can only call endpoints covered by these grants.
          </Typography>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Permission</TableCell>
                <TableCell>Scope</TableCell>
                <TableCell>Effect</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {data.permissions.map((grant) => (
                <TableRow key={grant.id}>
                  <TableCell>
                    <Stack direction="row" alignItems="center" spacing={0.75}>
                      <code>{grant.code}</code>
                      <PermissionInfo code={grant.code} scope={grant.scope} effect={grant.effect} />
                    </Stack>
                  </TableCell>
                  <TableCell>
                    <Chip size="small" label={grant.scope} />
                  </TableCell>
                  <TableCell>{grant.effect}</TableCell>
                </TableRow>
              ))}
              {data.permissions.length === 0 && (
                <TableRow>
                  <TableCell colSpan={3}>
                    <Typography variant="body2" color="text.secondary">
                      No permissions granted — this client cannot call any business endpoint yet.
                    </Typography>
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {rotated && (
        <SecretDisplay
          open
          clientId={rotated.client.client_id}
          secret={rotated.client_secret}
          onClose={() => setRotated(null)}
        />
      )}
    </>
  );
}
