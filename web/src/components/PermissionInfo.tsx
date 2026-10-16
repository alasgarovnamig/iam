import { Box, Chip, Divider, Stack, Tooltip, Typography } from '@mui/material';
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import WarningAmberIcon from '@mui/icons-material/WarningAmber';
import { RISK_LABELS, permissionDoc, type RiskLevel } from '../auth/permissionDocs';
import { SCOPE_LABELS, type Effect, type Scope } from '../api/types';

/**
 * The "i" affordance next to a permission. Hovering (or focusing, or tapping on
 * touch) explains what the permission actually does, what it does not grant,
 * and how sensitive it is — enough to decide whether to hand it over without
 * leaving the page.
 */

const RISK_COLOR: Record<RiskLevel, 'default' | 'warning' | 'error'> = {
  low: 'default',
  medium: 'warning',
  high: 'error',
};

interface PermissionInfoProps {
  /** Permission code, e.g. `users.read.email`. */
  code: string;
  /** When shown against an existing grant, explains that grant's scope too. */
  scope?: Scope;
  /** Effect of the grant, when this documents a concrete grant. */
  effect?: Effect;
}

export function PermissionInfo({ code, scope, effect }: PermissionInfoProps) {
  const doc = permissionDoc(code);

  const body = (
    <Box sx={{ p: 0.5, maxWidth: 360 }}>
      <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 0.75 }}>
        <Typography variant="caption" sx={{ fontFamily: 'monospace', opacity: 0.85 }}>
          {code}
        </Typography>
        <Chip
          size="small"
          label={RISK_LABELS[doc.risk]}
          color={RISK_COLOR[doc.risk]}
          variant={doc.risk === 'low' ? 'outlined' : 'filled'}
          sx={{ height: 18, fontSize: 11 }}
        />
      </Stack>

      <Typography variant="body2" fontWeight={600} sx={{ mb: 0.5 }}>
        {doc.summary}
      </Typography>

      <Typography variant="body2" sx={{ opacity: 0.9 }}>
        {doc.detail}
      </Typography>

      {doc.caveat && (
        <Stack direction="row" spacing={0.75} sx={{ mt: 1 }}>
          <WarningAmberIcon sx={{ fontSize: 16, mt: '2px', flexShrink: 0 }} />
          <Typography variant="body2" sx={{ opacity: 0.9 }}>
            {doc.caveat}
          </Typography>
        </Stack>
      )}

      {(scope || effect) && (
        <>
          <Divider sx={{ my: 1, opacity: 0.3 }} />
          {scope && (
            <Typography variant="body2" sx={{ opacity: 0.9 }}>
              <strong>Granted at:</strong> {SCOPE_LABELS[scope]}
            </Typography>
          )}
          {effect === 'deny' && (
            <Typography variant="body2" sx={{ opacity: 0.9 }}>
              <strong>This is a deny.</strong> A deny always wins over any allow that reaches the
              same target, no matter how narrow the allow is.
            </Typography>
          )}
        </>
      )}
    </Box>
  );

  return (
    <Tooltip
      title={body}
      arrow
      enterTouchDelay={0}
      leaveTouchDelay={6000}
      slotProps={{ tooltip: { sx: { maxWidth: 380 } } }}
    >
      <InfoOutlinedIcon
        role="img"
        aria-label={`What ${code} allows`}
        tabIndex={0}
        sx={{
          fontSize: 16,
          color: 'text.disabled',
          cursor: 'help',
          flexShrink: 0,
          '&:hover, &:focus-visible': { color: 'primary.main' },
        }}
        data-testid={`perm-info-${code}`}
      />
    </Tooltip>
  );
}
