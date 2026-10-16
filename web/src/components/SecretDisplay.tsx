import { useState } from 'react';
import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';

/**
 * One-time secret display.
 *
 * The API returns a client secret exactly once, at creation and at rotation —
 * it is stored only as an Argon2id hash and can never be fetched again. The
 * dialog therefore forces an explicit acknowledgement before closing, and the
 * value is never persisted anywhere by the portal.
 */
export function SecretDisplay({
  open,
  clientId,
  secret,
  onClose,
}: {
  open: boolean;
  clientId: string;
  secret: string;
  onClose: () => void;
}) {
  const [acknowledged, setAcknowledged] = useState(false);
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(secret);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard access can be denied; the value is selectable in the field.
      setCopied(false);
    }
  };

  return (
    <Dialog open={open} maxWidth="sm" fullWidth disableEscapeKeyDown data-testid="secret-dialog">
      <DialogTitle>Save your client secret</DialogTitle>
      <DialogContent>
        <Alert severity="warning" sx={{ mb: 2 }}>
          This secret is shown only once. It is stored hashed and cannot be retrieved later — if you lose it,
          you must rotate the secret.
        </Alert>

        <Stack spacing={2}>
          <TextField label="Client ID" value={clientId} fullWidth InputProps={{ readOnly: true }} />
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <TextField
              label="Client secret"
              value={secret}
              fullWidth
              InputProps={{ readOnly: true }}
              inputProps={{ 'data-testid': 'client-secret-value' }}
            />
            <Tooltip title={copied ? 'Copied' : 'Copy'}>
              <IconButton onClick={copy} data-testid="copy-secret">
                <ContentCopyIcon />
              </IconButton>
            </Tooltip>
          </Box>

          <Typography variant="body2" color="text.secondary">
            Use it with <code>POST /api/v1/oauth/token</code> and{' '}
            <code>grant_type=client_credentials</code>.
          </Typography>

          <label style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <input
              type="checkbox"
              checked={acknowledged}
              onChange={(e) => setAcknowledged(e.target.checked)}
              data-testid="ack-secret"
            />
            <Typography variant="body2">I have stored this secret somewhere safe.</Typography>
          </label>
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button variant="contained" disabled={!acknowledged} onClick={onClose} data-testid="close-secret">
          Done
        </Button>
      </DialogActions>
    </Dialog>
  );
}
