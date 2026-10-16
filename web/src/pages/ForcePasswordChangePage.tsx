import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { useNavigate } from 'react-router-dom';
import { Alert, Box, Button, Card, CardContent, Stack, TextField, Typography } from '@mui/material';
import { ApiError, postData } from '../api/client';
import { useAuth } from '../auth/AuthContext';

/** Mirrors the backend password policy so users get feedback before submitting. */
export const passwordPolicy = z
  .string()
  .min(10, 'Use at least 10 characters')
  .max(128, 'Use at most 128 characters')
  .regex(/[A-Z]/, 'Add an upper-case letter')
  .regex(/[a-z]/, 'Add a lower-case letter')
  .regex(/[0-9]/, 'Add a digit')
  .regex(/[^A-Za-z0-9]/, 'Add a special character');

const schema = z
  .object({
    currentPassword: z.string().min(1, 'Current password is required'),
    newPassword: passwordPolicy,
    confirmPassword: z.string().min(1, 'Confirm your new password'),
  })
  .refine((v) => v.newPassword === v.confirmPassword, {
    path: ['confirmPassword'],
    message: 'Passwords do not match',
  })
  .refine((v) => v.newPassword !== v.currentPassword, {
    path: ['newPassword'],
    message: 'The new password must differ from the current one',
  });

type FormValues = z.infer<typeof schema>;

export function ForcePasswordChangePage() {
  const navigate = useNavigate();
  const { clearMustChangePassword, refreshMe } = useAuth();
  const [formError, setFormError] = useState<string | null>(null);

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({ resolver: zodResolver(schema) });

  const onSubmit = async (values: FormValues) => {
    setFormError(null);
    try {
      await postData('/api/v1/auth/change-password', {
        current_password: values.currentPassword,
        new_password: values.newPassword,
      });
      clearMustChangePassword();
      await refreshMe();
      navigate('/', { replace: true });
    } catch (error) {
      setFormError(error instanceof ApiError ? error.detail : 'Could not change the password.');
    }
  };

  return (
    <Box
      sx={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        bgcolor: 'grey.100',
        p: 2,
      }}
    >
      <Card sx={{ width: 460, maxWidth: '100%' }} elevation={3}>
        <CardContent sx={{ p: 4 }}>
          <Typography variant="h5" fontWeight={600} gutterBottom>
            Change your password
          </Typography>
          <Alert severity="info" sx={{ mb: 2 }}>
            Your account still uses its initial password. Choose a new one to continue.
          </Alert>

          {formError && (
            <Alert severity="error" sx={{ mb: 2 }} data-testid="password-error">
              {formError}
            </Alert>
          )}

          <form onSubmit={handleSubmit(onSubmit)} noValidate>
            <Stack spacing={2}>
              <TextField
                label="Current password"
                type="password"
                autoComplete="current-password"
                fullWidth
                error={Boolean(errors.currentPassword)}
                helperText={errors.currentPassword?.message}
                inputProps={{ 'data-testid': 'current-password' }}
                {...register('currentPassword')}
              />
              <TextField
                label="New password"
                type="password"
                autoComplete="new-password"
                fullWidth
                error={Boolean(errors.newPassword)}
                helperText={
                  errors.newPassword?.message ?? 'At least 10 characters with upper, lower, digit and symbol.'
                }
                inputProps={{ 'data-testid': 'new-password' }}
                {...register('newPassword')}
              />
              <TextField
                label="Confirm new password"
                type="password"
                autoComplete="new-password"
                fullWidth
                error={Boolean(errors.confirmPassword)}
                helperText={errors.confirmPassword?.message}
                inputProps={{ 'data-testid': 'confirm-password' }}
                {...register('confirmPassword')}
              />
              <Button
                type="submit"
                variant="contained"
                size="large"
                disabled={isSubmitting}
                data-testid="change-password-submit"
              >
                {isSubmitting ? 'Saving…' : 'Change password'}
              </Button>
            </Stack>
          </form>
        </CardContent>
      </Card>
    </Box>
  );
}
