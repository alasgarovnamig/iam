import { useNavigate } from 'react-router-dom';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Button, Card, CardContent, Stack, TextField, Typography } from '@mui/material';
import { postData } from '../api/client';
import type { Role } from '../api/types';
import { ErrorAlert, PageHeader } from '../components/common';

const schema = z.object({
  name: z.string().min(1, 'Name is required').max(100),
  description: z.string().max(500).optional(),
});

type FormValues = z.infer<typeof schema>;

export function RoleCreatePage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({ resolver: zodResolver(schema) });

  const createRole = useMutation({
    mutationFn: (values: FormValues) =>
      postData<Role>('/api/v1/roles', values, { 'Idempotency-Key': crypto.randomUUID() }),
    onSuccess: (role) => {
      queryClient.invalidateQueries({ queryKey: ['roles'] });
      // Permissions are attached on the edit page, where the tree lives.
      navigate(`/roles/${role.id}`);
    },
  });

  return (
    <>
      <PageHeader title="New role" subtitle="Create the role first, then attach permissions to it." />
      <ErrorAlert error={createRole.error} />

      <Card sx={{ maxWidth: 640 }}>
        <CardContent>
          <form onSubmit={handleSubmit((values) => createRole.mutateAsync(values))} noValidate>
            <Stack spacing={2}>
              <TextField
                label="Name"
                fullWidth
                error={Boolean(errors.name)}
                helperText={errors.name?.message}
                inputProps={{ 'data-testid': 'role-name' }}
                {...register('name')}
              />
              <TextField
                label="Description"
                fullWidth
                multiline
                rows={3}
                error={Boolean(errors.description)}
                helperText={errors.description?.message}
                inputProps={{ 'data-testid': 'role-description' }}
                {...register('description')}
              />
              <Typography variant="caption" color="text.secondary">
                The role is created inside your tenant. You will only be able to attach permissions you hold
                yourself.
              </Typography>
              <Stack direction="row" spacing={1} justifyContent="flex-end">
                <Button onClick={() => navigate('/roles')}>Cancel</Button>
                <Button type="submit" variant="contained" disabled={isSubmitting} data-testid="role-submit">
                  Create role
                </Button>
              </Stack>
            </Stack>
          </form>
        </CardContent>
      </Card>
    </>
  );
}
