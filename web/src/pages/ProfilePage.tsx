import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { useMutation } from '@tanstack/react-query';
import { Alert, Button, Card, CardContent, Grid, Snackbar, Stack, TextField, Typography } from '@mui/material';
import { postData } from '../api/client';
import { displayName } from '../api/types';
import { useAuth } from '../auth/AuthContext';
import { passwordPolicy } from './ForcePasswordChangePage';
import { ErrorAlert, PageHeader } from '../components/common';

const schema = z
  .object({
    currentPassword: z.string().min(1, 'Current password is required'),
    newPassword: passwordPolicy,
    confirmPassword: z.string().min(1, 'Confirm your new password'),
  })
  .refine((v) => v.newPassword === v.confirmPassword, {
    path: ['confirmPassword'],
    message: 'Passwords do not match',
  });

type FormValues = z.infer<typeof schema>;

export function ProfilePage() {
  const { user, permissions } = useAuth();
  const [toast, setToast] = useState<string | null>(null);

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({ resolver: zodResolver(schema) });

  const changePassword = useMutation({
    mutationFn: (values: FormValues) =>
      postData('/api/v1/auth/change-password', {
        current_password: values.currentPassword,
        new_password: values.newPassword,
      }),
    onSuccess: () => {
      reset();
      setToast('Password changed. Your other sessions were signed out.');
    },
  });

  if (!user) return null;

  return (
    <>
      <PageHeader title="Profile" subtitle={displayName(user)} />

      <Grid container spacing={3}>
        <Grid item xs={12} md={6}>
          <Card>
            <CardContent>
              <Typography variant="h6" gutterBottom>
                Account
              </Typography>
              <Stack spacing={1}>
                <Field label="Email" value={user.email ?? '—'} />
                <Field label="Type" value={user.user_type.replace('_', ' ')} />
                <Field label="Status" value={user.status ?? '—'} />
                <Field label="Permissions held" value={String(permissions.size)} />
              </Stack>
            </CardContent>
          </Card>
        </Grid>

        <Grid item xs={12} md={6}>
          <Card>
            <CardContent>
              <Typography variant="h6" gutterBottom>
                Change password
              </Typography>
              <ErrorAlert error={changePassword.error} />
              <Alert severity="info" sx={{ mb: 2 }}>
                Changing your password signs out every other device.
              </Alert>
              <form onSubmit={handleSubmit((values) => changePassword.mutateAsync(values))} noValidate>
                <Stack spacing={2}>
                  <TextField
                    label="Current password"
                    type="password"
                    autoComplete="current-password"
                    fullWidth
                    error={Boolean(errors.currentPassword)}
                    helperText={errors.currentPassword?.message}
                    inputProps={{ 'data-testid': 'profile-current-password' }}
                    {...register('currentPassword')}
                  />
                  <TextField
                    label="New password"
                    type="password"
                    autoComplete="new-password"
                    fullWidth
                    error={Boolean(errors.newPassword)}
                    helperText={errors.newPassword?.message}
                    inputProps={{ 'data-testid': 'profile-new-password' }}
                    {...register('newPassword')}
                  />
                  <TextField
                    label="Confirm new password"
                    type="password"
                    autoComplete="new-password"
                    fullWidth
                    error={Boolean(errors.confirmPassword)}
                    helperText={errors.confirmPassword?.message}
                    inputProps={{ 'data-testid': 'profile-confirm-password' }}
                    {...register('confirmPassword')}
                  />
                  <Button type="submit" variant="contained" disabled={isSubmitting} data-testid="profile-submit">
                    Change password
                  </Button>
                </Stack>
              </form>
            </CardContent>
          </Card>
        </Grid>
      </Grid>

      <Snackbar open={Boolean(toast)} autoHideDuration={4000} onClose={() => setToast(null)} message={toast ?? ''} />
    </>
  );
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <Stack direction="row" justifyContent="space-between">
      <Typography variant="body2" color="text.secondary">
        {label}
      </Typography>
      <Typography variant="body2">{value}</Typography>
    </Stack>
  );
}
