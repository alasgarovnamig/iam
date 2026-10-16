import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Button, Card, CardContent, MenuItem, Stack, TextField } from '@mui/material';
import { getList, postData } from '../api/client';
import type { CreatedApiClient, Tenant } from '../api/types';
import { useAuth } from '../auth/AuthContext';
import { P } from '../auth/permissions';
import { SecretDisplay } from '../components/SecretDisplay';
import { ErrorAlert, PageHeader } from '../components/common';

const schema = z.object({
  tenant_id: z.string().optional(),
  name: z.string().min(1, 'Name is required').max(200),
  description: z.string().max(500).optional(),
  allowed_ips: z.string().optional(),
  scopes: z.string().optional(),
});

type FormValues = z.infer<typeof schema>;

export function ApiClientCreatePage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { user, permissions } = useAuth();
  const isPlatform = user?.user_type === 'platform_user';
  const [created, setCreated] = useState<CreatedApiClient | null>(null);

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({ resolver: zodResolver(schema) });

  const tenants = useQuery({
    queryKey: ['tenants', 'all'],
    queryFn: async () => (await getList<Tenant>('/api/v1/tenants', { page_size: 100 })).items,
    enabled: isPlatform && permissions.has(P.tenants.list),
  });

  const create = useMutation({
    mutationFn: (values: FormValues) =>
      postData<CreatedApiClient>(
        '/api/v1/api-clients',
        {
          ...(values.tenant_id ? { tenant_id: values.tenant_id } : {}),
          name: values.name,
          description: values.description ?? '',
          allowed_ips: splitList(values.allowed_ips),
          scopes: splitList(values.scopes),
        },
        { 'Idempotency-Key': crypto.randomUUID() },
      ),
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ['api-clients'] });
      // The secret exists only in this response — show it before navigating.
      setCreated(result);
    },
  });

  return (
    <>
      <PageHeader title="New API client" subtitle="The client secret is displayed once, right after creation." />
      <ErrorAlert error={create.error} />

      <Card sx={{ maxWidth: 720 }}>
        <CardContent>
          <form onSubmit={handleSubmit((values) => create.mutateAsync(values))} noValidate>
            <Stack spacing={2}>
              {isPlatform && (
                <TextField
                  select
                  label="Tenant"
                  fullWidth
                  defaultValue=""
                  helperText="API clients always belong to a tenant."
                  inputProps={{ 'data-testid': 'client-tenant' }}
                  {...register('tenant_id')}
                >
                  {(tenants.data ?? []).map((t) => (
                    <MenuItem key={t.id} value={t.id}>
                      {t.name}
                    </MenuItem>
                  ))}
                </TextField>
              )}
              <TextField
                label="Name"
                fullWidth
                error={Boolean(errors.name)}
                helperText={errors.name?.message}
                inputProps={{ 'data-testid': 'client-name' }}
                {...register('name')}
              />
              <TextField label="Description" fullWidth multiline rows={2} {...register('description')} />
              <TextField
                label="Allowed IPs"
                fullWidth
                helperText="Comma-separated. Leave empty to allow any source IP."
                inputProps={{ 'data-testid': 'client-ips' }}
                {...register('allowed_ips')}
              />
              <TextField
                label="Scopes"
                fullWidth
                helperText="Comma-separated, e.g. wallet.read, payments.write"
                inputProps={{ 'data-testid': 'client-scopes' }}
                {...register('scopes')}
              />
              <Stack direction="row" spacing={1} justifyContent="flex-end">
                <Button onClick={() => navigate('/api-clients')}>Cancel</Button>
                <Button type="submit" variant="contained" disabled={isSubmitting} data-testid="client-submit">
                  Create client
                </Button>
              </Stack>
            </Stack>
          </form>
        </CardContent>
      </Card>

      {created && (
        <SecretDisplay
          open
          clientId={created.client.client_id}
          secret={created.client_secret}
          onClose={() => navigate(`/api-clients/${created.client.id}`)}
        />
      )}
    </>
  );
}

function splitList(value?: string): string[] {
  if (!value) return [];
  return value
    .split(',')
    .map((v) => v.trim())
    .filter(Boolean);
}
