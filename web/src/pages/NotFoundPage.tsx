import { Link as RouterLink } from 'react-router-dom';
import { Box, Button, Typography } from '@mui/material';

export function NotFoundPage() {
  return (
    <Box sx={{ textAlign: 'center', py: 10 }} data-testid="not-found-page">
      <Typography variant="h2" fontWeight={700} color="text.disabled">
        404
      </Typography>
      <Typography variant="h6" sx={{ mt: 1 }}>
        This page does not exist
      </Typography>
      <Button component={RouterLink} to="/" variant="contained" sx={{ mt: 3 }}>
        Back to dashboard
      </Button>
    </Box>
  );
}
