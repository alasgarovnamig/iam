import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Button,
  Card,
  Chip,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
} from '@mui/material';
import { deleteData, getData, postData } from '../api/client';
import type { Session } from '../api/types';
import { EmptyState, ErrorAlert, PageHeader, formatDate } from '../components/common';

export function SessionsPage() {
  const queryClient = useQueryClient();

  const query = useQuery({
    queryKey: ['sessions'],
    queryFn: () => getData<Session[]>('/api/v1/auth/sessions'),
  });

  const revoke = useMutation({
    mutationFn: (sessionId: string) => deleteData(`/api/v1/auth/sessions/${sessionId}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['sessions'] }),
  });

  const revokeAll = useMutation({
    mutationFn: () => postData('/api/v1/auth/logout-all'),
    onSuccess: () => {
      // Every session including this one is gone; the next API call triggers
      // the unauthenticated handler and drops us back to the login page.
      queryClient.invalidateQueries();
    },
  });

  const sessions = query.data ?? [];

  return (
    <>
      <PageHeader
        title="Active sessions"
        subtitle="Devices currently signed in with your account"
        actions={
          <Button
            color="error"
            variant="outlined"
            onClick={() => revokeAll.mutate()}
            disabled={revokeAll.isPending}
            data-testid="revoke-all-sessions"
          >
            Sign out everywhere
          </Button>
        }
      />

      <ErrorAlert error={query.error ?? revoke.error ?? revokeAll.error} />

      <Card>
        <TableContainer>
          <Table>
            <TableHead>
              <TableRow>
                <TableCell>Device</TableCell>
                <TableCell>IP</TableCell>
                <TableCell>Started</TableCell>
                <TableCell>Last used</TableCell>
                <TableCell align="right">Actions</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {sessions.map((session) => (
                <TableRow key={session.id} hover data-testid="session-row">
                  <TableCell>
                    {session.user_agent || 'Unknown device'}
                    {session.current && <Chip size="small" label="this device" color="primary" sx={{ ml: 1 }} />}
                  </TableCell>
                  <TableCell>{session.ip || '—'}</TableCell>
                  <TableCell>{formatDate(session.created_at)}</TableCell>
                  <TableCell>{formatDate(session.last_used_at)}</TableCell>
                  <TableCell align="right">
                    <Button
                      size="small"
                      color="error"
                      onClick={() => revoke.mutate(session.id)}
                      disabled={revoke.isPending}
                      data-testid="revoke-session"
                    >
                      Revoke
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
        {sessions.length === 0 && <EmptyState message="No active sessions." />}
      </Card>
    </>
  );
}
