import type { ReactNode } from 'react';
import { Alert, Box, Chip, CircularProgress, Stack, Typography } from '@mui/material';
import InboxOutlinedIcon from '@mui/icons-material/InboxOutlined';
import { ApiError } from '../api/client';
import type { ApiClientStatus, TenantStatus, UserStatus } from '../api/types';

/** Page header with an optional action area. */
export function PageHeader({
  title,
  subtitle,
  actions,
}: {
  title: string;
  subtitle?: string;
  actions?: ReactNode;
}) {
  return (
    <Stack
      direction={{ xs: 'column', sm: 'row' }}
      alignItems={{ xs: 'stretch', sm: 'flex-start' }}
      justifyContent="space-between"
      spacing={2}
      sx={{ mb: 3 }}
    >
      <Box sx={{ minWidth: 0 }}>
        <Typography variant="h4" sx={{ fontSize: { xs: 24, md: 28 } }}>
          {title}
        </Typography>
        {subtitle && (
          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, maxWidth: '72ch' }}>
            {subtitle}
          </Typography>
        )}
      </Box>
      <Stack direction="row" spacing={1} sx={{ flexShrink: 0 }}>
        {actions}
      </Stack>
    </Stack>
  );
}

/**
 * Renders an API failure. Authorization denials get a plain explanation
 * instead of a scary error, since they are an expected outcome.
 */
export function ErrorAlert({ error }: { error: unknown }) {
  if (!error) return null;
  if (error instanceof ApiError) {
    const severity = error.status === 403 || error.status === 404 ? 'warning' : 'error';
    return (
      <Alert severity={severity} sx={{ mb: 2 }} data-testid="error-alert">
        {error.detail}
        {error.fieldErrors.length > 0 && (
          <ul style={{ margin: '8px 0 0', paddingLeft: 18 }}>
            {error.fieldErrors.map((f) => (
              <li key={f.field}>
                <strong>{f.field}</strong>: {f.message}
              </li>
            ))}
          </ul>
        )}
      </Alert>
    );
  }
  return (
    <Alert severity="error" sx={{ mb: 2 }} data-testid="error-alert">
      Something went wrong.
    </Alert>
  );
}

const userStatusColor: Record<UserStatus, 'success' | 'warning' | 'error' | 'default'> = {
  active: 'success',
  pending: 'warning',
  suspended: 'warning',
  locked: 'error',
  disabled: 'default',
};

const tenantStatusColor: Record<TenantStatus, 'success' | 'warning' | 'error' | 'default'> = {
  active: 'success',
  pending: 'warning',
  suspended: 'warning',
  terminated: 'error',
};

const clientStatusColor: Record<ApiClientStatus, 'success' | 'warning' | 'error' | 'default'> = {
  active: 'success',
  suspended: 'warning',
  expired: 'default',
  revoked: 'error',
};

export function StatusChip({ status, kind }: { status: string; kind: 'user' | 'tenant' | 'api-client' }) {
  const palette =
    kind === 'user' ? userStatusColor : kind === 'tenant' ? tenantStatusColor : clientStatusColor;
  const color = (palette as Record<string, 'success' | 'warning' | 'error' | 'default'>)[status] ?? 'default';
  return <Chip size="small" label={status} color={color} data-testid={`status-${status}`} />;
}

/** Placeholder for a field the caller may not read. */
export function Hidden() {
  return (
    <Typography component="span" variant="body2" color="text.disabled" sx={{ fontStyle: 'italic' }}>
      hidden
    </Typography>
  );
}

/** Inline spinner used while a page's primary query loads. */
export function FullPageSpinnerFallback() {
  return (
    <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
      <CircularProgress />
    </Box>
  );
}

export function EmptyState({ message, action }: { message: string; action?: ReactNode }) {
  return (
    <Stack alignItems="center" spacing={1.5} sx={{ py: 7, px: 2, textAlign: 'center' }}>
      <Box
        aria-hidden
        sx={{
          width: 44,
          height: 44,
          borderRadius: '50%',
          display: 'grid',
          placeItems: 'center',
          bgcolor: 'action.hover',
          color: 'text.disabled',
        }}
      >
        <InboxOutlinedIcon fontSize="small" />
      </Box>
      <Typography color="text.secondary" sx={{ maxWidth: '48ch' }}>
        {message}
      </Typography>
      {action}
    </Stack>
  );
}

export function formatDate(value?: string): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return date.toLocaleString();
}
