import { useNavigate } from 'react-router-dom';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Button,
  Card,
  CardContent,
  Checkbox,
  FormControlLabel,
  FormGroup,
  MenuItem,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { getList, postData } from '../api/client';
import type { Role, Tenant, User } from '../api/types';
import { useAuth } from '../auth/AuthContext';
import { P } from '../auth/permissions';
import { passwordPolicy } from './ForcePasswordChangePage';
import { DisabledHint } from '../components/guards';
import { ErrorAlert, PageHeader } from '../components/common';

const schema = z.object({
  tenant_id: z.string().optional(),
  email: z.string().email('Enter a valid email address'),
  password: passwordPolicy,
  first_name: z.string().max(100).optional(),
  last_name: z.string().max(100).optional(),
  phone: z.string().max(40).optional(),
  role_ids: z.array(z.string()).optional(),
});

type FormValues = z.infer<typeof schema>;

export function UserCreatePage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { user: actor, permissions } = useAuth();
  const isPlatform = actor?.user_type === 'platform_user';

  const {
    register,
    handleSubmit,
    watch,
    setValue,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({ resolver: zodResolver(schema), defaultValues: { role_ids: [] } });

  const tenants = useQuery({
    queryKey: ['tenants', 'all'],
    queryFn: async () => (await getList<Tenant>('/api/v1/tenants', { page_size: 100 })).items,
    enabled: isPlatform && permissions.has(P.tenants.list),
  });

  const roles = useQuery({
    queryKey: ['roles', 'assignable'],
    queryFn: async () => (await getList<Role>('/api/v1/roles', { page_size: 100 })).items,
    enabled: permissions.has(P.roles.list),
  });

  const selectedRoles = watch('role_ids') ?? [];

  const createUser = useMutation({
    mutationFn: (values: FormValues) => {
      // Only send fields the actor may write: the API rejects a body that
      // carries a field without the matching update.<field> grant.
      const body: Record<string, unknown> = { email: values.email, password: values.password };
      if (values.tenant_id) body.tenant_id = values.tenant_id;
      if (values.first_name) body.first_name = values.first_name;
      if (values.last_name) body.last_name = values.last_name;
      if (values.phone) body.phone = values.phone;
      if (values.role_ids?.length) body.role_ids = values.role_ids;
      return postData<User>('/api/v1/users', body, { 'Idempotency-Key': crypto.randomUUID() });
    },
    onSuccess: (created) => {
      queryClient.invalidateQueries({ queryKey: ['users'] });
      navigate(`/users/${created.id}`);
    },
  });

  const canWrite = (field: string) => permissions.has(P.users.updateField(field));

  return (
    <>
      <PageHeader title="New user" subtitle="The user must change this password at first sign-in." />
      <ErrorAlert error={createUser.error} />

      <Card sx={{ maxWidth: 720 }}>
        <CardContent>
          <form onSubmit={handleSubmit((values) => createUser.mutateAsync(values))} noValidate>
            <Stack spacing={2}>
              {isPlatform && (
                <TextField
                  select
                  label="Tenant"
                  fullWidth
                  defaultValue=""
                  helperText="Leave empty to create a platform user."
                  inputProps={{ 'data-testid': 'user-tenant' }}
                  {...register('tenant_id')}
                >
                  <MenuItem value="">Platform (no tenant)</MenuItem>
                  {(tenants.data ?? []).map((t) => (
                    <MenuItem key={t.id} value={t.id}>
                      {t.name}
                    </MenuItem>
                  ))}
                </TextField>
              )}

              <DisabledHint reason={canWrite('email') ? null : 'You lack users.update.email'}>
                <TextField
                  label="Email"
                  type="email"
                  fullWidth
                  disabled={!canWrite('email')}
                  error={Boolean(errors.email)}
                  helperText={errors.email?.message}
                  inputProps={{ 'data-testid': 'user-email' }}
                  {...register('email')}
                />
              </DisabledHint>

              <TextField
                label="Initial password"
                type="password"
                fullWidth
                autoComplete="new-password"
                error={Boolean(errors.password)}
                helperText={errors.password?.message ?? 'The user is forced to change it at first sign-in.'}
                inputProps={{ 'data-testid': 'user-password' }}
                {...register('password')}
              />

              <Stack direction="row" spacing={2}>
                <DisabledHint reason={canWrite('first_name') ? null : 'You lack users.update.first_name'}>
                  <TextField
                    label="First name"
                    fullWidth
                    disabled={!canWrite('first_name')}
                    inputProps={{ 'data-testid': 'user-first-name' }}
                    {...register('first_name')}
                  />
                </DisabledHint>
                <DisabledHint reason={canWrite('last_name') ? null : 'You lack users.update.last_name'}>
                  <TextField
                    label="Last name"
                    fullWidth
                    disabled={!canWrite('last_name')}
                    inputProps={{ 'data-testid': 'user-last-name' }}
                    {...register('last_name')}
                  />
                </DisabledHint>
              </Stack>

              {canWrite('phone') && (
                <TextField
                  label="Phone"
                  fullWidth
                  inputProps={{ 'data-testid': 'user-phone' }}
                  {...register('phone')}
                />
              )}

              {permissions.has(P.roles.assign) && (roles.data?.length ?? 0) > 0 && (
                <>
                  <Typography variant="subtitle2">Roles</Typography>
                  <Typography variant="caption" color="text.secondary">
                    You can only assign roles whose permissions you hold yourself; the API rejects the rest.
                  </Typography>
                  <FormGroup>
                    {(roles.data ?? []).map((role) => (
                      <FormControlLabel
                        key={role.id}
                        control={
                          <Checkbox
                            checked={selectedRoles.includes(role.id)}
                            onChange={(e) => {
                              const next = e.target.checked
                                ? [...selectedRoles, role.id]
                                : selectedRoles.filter((id) => id !== role.id);
                              setValue('role_ids', next);
                            }}
                            data-testid={`role-${role.name.replace(/\s+/g, '-').toLowerCase()}`}
                          />
                        }
                        label={`${role.name}${role.is_system ? ' (system)' : ''}`}
                      />
                    ))}
                  </FormGroup>
                </>
              )}

              <Stack direction="row" spacing={1} justifyContent="flex-end">
                <Button onClick={() => navigate('/users')}>Cancel</Button>
                <Button type="submit" variant="contained" disabled={isSubmitting} data-testid="user-submit">
                  Create user
                </Button>
              </Stack>
            </Stack>
          </form>
        </CardContent>
      </Card>
    </>
  );
}
