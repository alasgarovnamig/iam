import { Link as RouterLink, useParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Button,
  Card,
  CardContent,
  Divider,
  Grid,
  Stack,
  Typography,
} from '@mui/material';
import { getData, postData } from '../api/client';
import { displayName, has, type User } from '../api/types';
import { P } from '../auth/permissions';
import { Can } from '../components/guards';
import {
  ErrorAlert,
  FullPageSpinnerFallback,
  Hidden,
  PageHeader,
  StatusChip,
  formatDate,
} from '../components/common';

/** A field the caller cannot read is absent from the payload, not empty. */
function Field({ label, value, readable }: { label: string; value?: string; readable: boolean }) {
  return (
    <Grid item xs={12} sm={6}>
      <Typography variant="caption" color="text.secondary">
        {label}
      </Typography>
      <Typography variant="body1" data-testid={`field-${label.toLowerCase().replace(/\s+/g, '-')}`}>
        {readable ? value || '—' : <Hidden />}
      </Typography>
    </Grid>
  );
}

export function UserDetailsPage() {
  const { userId = '' } = useParams();
  const queryClient = useQueryClient();

  const query = useQuery({
    queryKey: ['users', userId],
    queryFn: () => getData<User>(`/api/v1/users/${userId}`),
  });

  const lifecycle = useMutation({
    mutationFn: (action: 'activate' | 'suspend' | 'unlock') =>
      postData(`/api/v1/users/${userId}/${action}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['users'] }),
  });

  if (query.isLoading) return <FullPageSpinnerFallback />;
  if (query.error) return <ErrorAlert error={query.error} />;
  const user = query.data;
  if (!user) return null;

  return (
    <>
      <PageHeader
        title={displayName(user)}
        subtitle={user.user_type.replace('_', ' ')}
        actions={
          <Stack direction="row" spacing={1}>
            <Can anyOf={[P.permissions.read, P.permissions.list]}>
              <Button component={RouterLink} to={`/users/${userId}/permissions`} data-testid="view-permissions">
                Effective permissions
              </Button>
            </Can>
            <Can permission={P.users.update}>
              <Button
                component={RouterLink}
                to={`/users/${userId}/edit`}
                variant="contained"
                data-testid="edit-user"
              >
                Edit
              </Button>
            </Can>
          </Stack>
        }
      />

      <ErrorAlert error={lifecycle.error} />

      <Card sx={{ maxWidth: 860 }}>
        <CardContent>
          <Grid container spacing={2}>
            <Field label="Email" value={user.email} readable={has(user, 'email')} />
            <Field label="Phone" value={user.phone} readable={has(user, 'phone')} />
            <Field label="First name" value={user.first_name} readable={has(user, 'first_name')} />
            <Field label="Last name" value={user.last_name} readable={has(user, 'last_name')} />
            <Grid item xs={12} sm={6}>
              <Typography variant="caption" color="text.secondary">
                Status
              </Typography>
              <Typography variant="body1">
                {has(user, 'status') ? <StatusChip status={user.status} kind="user" /> : <Hidden />}
              </Typography>
            </Grid>
            <Field label="Created" value={formatDate(user.created_at)} readable />
            {has(user, 'internal_notes') && (
              <Grid item xs={12}>
                <Typography variant="caption" color="text.secondary">
                  Internal notes
                </Typography>
                <Typography variant="body2">{user.internal_notes || '—'}</Typography>
              </Grid>
            )}
          </Grid>

          <Can permission={P.users.update}>
            <>
              <Divider sx={{ my: 3 }} />
              <Typography variant="subtitle2" gutterBottom>
                Lifecycle
              </Typography>
              <Stack direction="row" spacing={1}>
                <Button
                  size="small"
                  variant="outlined"
                  onClick={() => lifecycle.mutate('activate')}
                  disabled={lifecycle.isPending || user.status === 'active'}
                  data-testid="activate-user"
                >
                  Activate
                </Button>
                <Button
                  size="small"
                  variant="outlined"
                  color="warning"
                  onClick={() => lifecycle.mutate('suspend')}
                  disabled={lifecycle.isPending || user.status === 'suspended'}
                  data-testid="suspend-user"
                >
                  Suspend
                </Button>
                <Button
                  size="small"
                  variant="outlined"
                  onClick={() => lifecycle.mutate('unlock')}
                  disabled={lifecycle.isPending || user.status !== 'locked'}
                  data-testid="unlock-user"
                >
                  Unlock
                </Button>
              </Stack>
            </>
          </Can>
        </CardContent>
      </Card>
    </>
  );
}
