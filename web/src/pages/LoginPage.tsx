import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { useLocation, useNavigate } from 'react-router-dom';
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Stack,
  TextField,
  Typography,
  alpha,
} from '@mui/material';
import { useAuth } from '../auth/AuthContext';
import { ApiError } from '../api/client';

const loginSchema = z.object({
  email: z.string().min(1, 'Email is required').email('Enter a valid email address'),
  password: z.string().min(1, 'Password is required'),
  // Only needed when the same email exists in several tenants.
  tenantSlug: z.string().optional(),
});

type LoginForm = z.infer<typeof loginSchema>;

export function LoginPage() {
  const { login } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const [formError, setFormError] = useState<string | null>(null);
  const [needsTenant, setNeedsTenant] = useState(false);

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<LoginForm>({ resolver: zodResolver(loginSchema) });

  const onSubmit = async (values: LoginForm) => {
    setFormError(null);
    try {
      await login(values.email, values.password, values.tenantSlug || undefined);
      const from = (location.state as { from?: { pathname: string } } | null)?.from?.pathname ?? '/';
      navigate(from, { replace: true });
    } catch (error) {
      if (error instanceof ApiError) {
        // The API asks for a tenant slug when an email is ambiguous.
        if (error.code === 'VALIDATION_ERROR' && error.detail.includes('tenant_slug')) {
          setNeedsTenant(true);
          setFormError('This email exists in several tenants. Enter your tenant slug.');
          return;
        }
        setFormError(error.detail);
        return;
      }
      setFormError('Sign-in failed. Try again.');
    }
  };

  return (
    <Box
      sx={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        p: 2,
        bgcolor: 'background.default',
        // A quiet wash of brand colour instead of a flat grey slab.
        backgroundImage: (t) =>
          `radial-gradient(900px 480px at 15% -10%, ${alpha(t.palette.primary.main, 0.16)}, transparent 60%),
           radial-gradient(760px 420px at 110% 110%, ${alpha(t.palette.secondary.main, 0.14)}, transparent 55%)`,
      }}
    >
      <Card sx={{ width: 428, maxWidth: '100%' }}>
        <CardContent sx={{ p: 4 }}>
          <Stack direction="row" alignItems="center" spacing={1.5} sx={{ mb: 2.5 }}>
            <Box
              aria-hidden
              sx={{
                width: 38,
                height: 38,
                borderRadius: 2.5,
                display: 'grid',
                placeItems: 'center',
                color: '#fff',
                fontWeight: 800,
                background: (t) =>
                  `linear-gradient(135deg, ${t.palette.primary.main}, ${t.palette.secondary.main})`,
              }}
            >
              W
            </Box>
            <Box>
              <Typography variant="h6">Wallet Gateway</Typography>
              <Typography variant="caption" color="text.secondary">
                Administration portal
              </Typography>
            </Box>
          </Stack>

          {formError && (
            <Alert severity="error" sx={{ mb: 2 }} data-testid="login-error">
              {formError}
            </Alert>
          )}

          <form onSubmit={handleSubmit(onSubmit)} noValidate>
            <Stack spacing={2}>
              <TextField
                label="Email"
                type="email"
                autoComplete="username"
                autoFocus
                fullWidth
                error={Boolean(errors.email)}
                helperText={errors.email?.message}
                inputProps={{ 'data-testid': 'login-email' }}
                {...register('email')}
              />
              <TextField
                label="Password"
                type="password"
                autoComplete="current-password"
                fullWidth
                error={Boolean(errors.password)}
                helperText={errors.password?.message}
                inputProps={{ 'data-testid': 'login-password' }}
                {...register('password')}
              />
              {needsTenant && (
                <TextField
                  label="Tenant slug"
                  fullWidth
                  helperText="e.g. acme"
                  inputProps={{ 'data-testid': 'login-tenant' }}
                  {...register('tenantSlug')}
                />
              )}
              <Button
                type="submit"
                variant="contained"
                size="large"
                disabled={isSubmitting}
                data-testid="login-submit"
              >
                {isSubmitting ? 'Signing in…' : 'Sign in'}
              </Button>
            </Stack>
          </form>
        </CardContent>
      </Card>
    </Box>
  );
}
