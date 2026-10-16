import { Link as RouterLink } from 'react-router-dom';
import { Box, Button, Typography } from '@mui/material';
import LockIcon from '@mui/icons-material/Lock';

export function ForbiddenPage() {
  return (
    <Box sx={{ textAlign: 'center', py: 10 }} data-testid="forbidden-page">
      <LockIcon sx={{ fontSize: 64, color: 'text.disabled' }} />
      <Typography variant="h5" fontWeight={600} sx={{ mt: 2 }}>
        You do not have access to this page
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mt: 1, mb: 3 }}>
        Your account lacks the permissions this page requires. Ask an administrator if you need them.
      </Typography>
      <Button component={RouterLink} to="/" variant="contained">
        Back to dashboard
      </Button>
    </Box>
  );
}
