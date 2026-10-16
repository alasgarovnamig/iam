import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import {
  Card,
  CardContent,
  Chip,
  Collapse,
  Grid,
  IconButton,
  MenuItem,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
  TextField,
  Typography,
} from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import { filterParams, getList } from '../api/client';
import type { AuditLog } from '../api/types';
import { EmptyState, ErrorAlert, PageHeader, formatDate } from '../components/common';

const RESULTS = ['success', 'failure', 'denied'];

export function AuditLogsPage() {
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(20);
  const [filters, setFilters] = useState<Record<string, string>>({});
  const [expanded, setExpanded] = useState<string | null>(null);

  const query = useQuery({
    queryKey: ['audit-logs', page, pageSize, filters],
    queryFn: () =>
      getList<AuditLog>('/api/v1/audit-logs', {
        page: page + 1,
        page_size: pageSize,
        ...filterParams(filters),
      }),
  });

  const setFilter = (field: string, value: string) => {
    setFilters((prev) => {
      const next = { ...prev };
      if (value) next[field] = value;
      else delete next[field];
      return next;
    });
    setPage(0);
  };

  return (
    <>
      <PageHeader
        title="Audit logs"
        subtitle="Immutable record of security-sensitive operations. Secrets are never stored here."
      />

      <ErrorAlert error={query.error} />

      <Card sx={{ mb: 2 }}>
        <CardContent>
          <Grid container spacing={2}>
            <Grid item xs={12} sm={4}>
              <TextField
                fullWidth
                size="small"
                label="Action"
                placeholder="e.g. user.created"
                value={filters.action ?? ''}
                onChange={(e) => setFilter('action', e.target.value)}
                inputProps={{ 'data-testid': 'filter-action' }}
              />
            </Grid>
            <Grid item xs={12} sm={4}>
              <TextField
                fullWidth
                size="small"
                label="Resource"
                placeholder="e.g. users"
                value={filters.resource ?? ''}
                onChange={(e) => setFilter('resource', e.target.value)}
                inputProps={{ 'data-testid': 'filter-resource' }}
              />
            </Grid>
            <Grid item xs={12} sm={4}>
              <TextField
                select
                fullWidth
                size="small"
                label="Result"
                value={filters.result ?? ''}
                onChange={(e) => setFilter('result', e.target.value)}
                inputProps={{ 'data-testid': 'filter-result' }}
              >
                <MenuItem value="">Any</MenuItem>
                {RESULTS.map((r) => (
                  <MenuItem key={r} value={r}>
                    {r}
                  </MenuItem>
                ))}
              </TextField>
            </Grid>
          </Grid>
        </CardContent>
      </Card>

      <Card>
        <TableContainer>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell />
                <TableCell>When</TableCell>
                <TableCell>Action</TableCell>
                <TableCell>Resource</TableCell>
                <TableCell>Actor</TableCell>
                <TableCell>Result</TableCell>
                <TableCell>IP</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {(query.data?.items ?? []).map((entry) => (
                <>
                  <TableRow key={entry.id} hover data-testid="audit-row">
                    <TableCell>
                      <IconButton
                        size="small"
                        onClick={() => setExpanded(expanded === entry.id ? null : entry.id)}
                      >
                        <ExpandMoreIcon fontSize="small" />
                      </IconButton>
                    </TableCell>
                    <TableCell>{formatDate(entry.occurred_at)}</TableCell>
                    <TableCell>{entry.action}</TableCell>
                    <TableCell>{entry.resource}</TableCell>
                    <TableCell>{entry.actor_type}</TableCell>
                    <TableCell>
                      <Chip
                        size="small"
                        label={entry.result}
                        color={
                          entry.result === 'success'
                            ? 'success'
                            : entry.result === 'denied'
                              ? 'warning'
                              : 'error'
                        }
                        variant="outlined"
                      />
                    </TableCell>
                    <TableCell>{entry.ip ?? '—'}</TableCell>
                  </TableRow>
                  <TableRow key={`${entry.id}-detail`}>
                    <TableCell colSpan={7} sx={{ py: 0, border: 0 }}>
                      <Collapse in={expanded === entry.id} unmountOnExit>
                        <Card variant="outlined" sx={{ my: 1 }}>
                          <CardContent>
                            <Typography variant="caption" color="text.secondary">
                              Request {entry.request_id ?? '—'} · Correlation {entry.correlation_id ?? '—'}
                            </Typography>
                            {entry.denial_reason && (
                              <Typography variant="body2" color="warning.main" sx={{ mt: 1 }}>
                                Denial reason: {entry.denial_reason}
                              </Typography>
                            )}
                            {entry.changed_fields && entry.changed_fields.length > 0 && (
                              <Typography variant="body2" sx={{ mt: 1 }}>
                                Changed: {entry.changed_fields.join(', ')}
                              </Typography>
                            )}
                            <Grid container spacing={2} sx={{ mt: 0.5 }}>
                              {entry.before_state && (
                                <Grid item xs={12} md={6}>
                                  <Typography variant="caption" color="text.secondary">
                                    Before
                                  </Typography>
                                  <pre style={preStyle}>{JSON.stringify(entry.before_state, null, 2)}</pre>
                                </Grid>
                              )}
                              {entry.after_state && (
                                <Grid item xs={12} md={6}>
                                  <Typography variant="caption" color="text.secondary">
                                    After
                                  </Typography>
                                  <pre style={preStyle}>{JSON.stringify(entry.after_state, null, 2)}</pre>
                                </Grid>
                              )}
                            </Grid>
                          </CardContent>
                        </Card>
                      </Collapse>
                    </TableCell>
                  </TableRow>
                </>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
        {query.data?.items.length === 0 && <EmptyState message="No audit entries match these filters." />}
        <TablePagination
          component="div"
          count={query.data?.pagination.total ?? 0}
          page={page}
          onPageChange={(_, next) => setPage(next)}
          rowsPerPage={pageSize}
          onRowsPerPageChange={(e) => {
            setPageSize(parseInt(e.target.value, 10));
            setPage(0);
          }}
          rowsPerPageOptions={[20, 50, 100]}
        />
      </Card>
    </>
  );
}

const preStyle: React.CSSProperties = {
  margin: 0,
  fontSize: 12,
  background: '#f5f5f5',
  padding: 8,
  borderRadius: 4,
  overflowX: 'auto',
};
