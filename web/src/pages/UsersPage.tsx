import { useMemo, useState } from 'react';
import { Link as RouterLink, useNavigate } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import {
  Button,
  Card,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import { getList, postList } from '../api/client';
import type { SearchRequest } from '../api/search';
import { displayName, has, type User } from '../api/types';
import { usePermissions } from '../auth/AuthContext';
import { P } from '../auth/permissions';
import { USER_SEARCH_FIELDS, searchableFields } from '../auth/searchFields';
import { Can } from '../components/guards';
import { SearchBuilder } from '../components/SearchBuilder';
import { EmptyState, ErrorAlert, Hidden, PageHeader, StatusChip, formatDate } from '../components/common';

export function UsersPage() {
  const permissions = usePermissions();
  const navigate = useNavigate();
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(20);
  const [search, setSearch] = useState<SearchRequest | null>(null);

  // Only fields the actor may search are offered; the builder is hidden when
  // the actor lacks the action-level users.search permission entirely.
  const fields = useMemo(
    () => searchableFields(permissions, 'users', USER_SEARCH_FIELDS),
    [permissions],
  );
  const canSearch = fields.length > 0;

  // Columns follow read field grants: a field we cannot read is not requested
  // and is rendered as "hidden" if the API omits it.
  const canReadEmail = permissions.has(P.users.readField('email'));
  const canReadPhone = permissions.has(P.users.readField('phone'));
  const canReadStatus = permissions.has(P.users.readField('status'));

  const query = useQuery({
    queryKey: ['users', canSearch ? 'search' : 'list', search, page, pageSize],
    queryFn: () => {
      if (canSearch) {
        return postList<User>('/api/v1/users/search', {
          ...(search ?? {}),
          page: page + 1,
          page_size: pageSize,
        });
      }
      return getList<User>('/api/v1/users', { page: page + 1, page_size: pageSize });
    },
  });

  return (
    <>
      <PageHeader
        title="Users"
        subtitle="People with access to the gateway"
        actions={
          <Can permission={P.users.create}>
            <Button
              variant="contained"
              startIcon={<AddIcon />}
              onClick={() => navigate('/users/new')}
              data-testid="create-user"
            >
              New user
            </Button>
          </Can>
        }
      />

      <ErrorAlert error={query.error} />

      {canSearch && (
        <SearchBuilder
          fields={fields}
          onSearch={(req) => {
            setSearch(req);
            setPage(0);
          }}
          onClear={() => {
            setSearch(null);
            setPage(0);
          }}
        />
      )}

      <Card>
        <TableContainer>
          <Table>
            <TableHead>
              <TableRow>
                <TableCell>Name</TableCell>
                {canReadEmail && <TableCell>Email</TableCell>}
                {canReadPhone && <TableCell>Phone</TableCell>}
                {canReadStatus && <TableCell>Status</TableCell>}
                <TableCell>Type</TableCell>
                <TableCell>Created</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {(query.data?.items ?? []).map((user) => (
                <TableRow key={user.id} hover data-testid="user-row">
                  <TableCell>
                    {permissions.has(P.users.read) ? (
                      <RouterLink to={`/users/${user.id}`}>{displayName(user)}</RouterLink>
                    ) : (
                      displayName(user)
                    )}
                  </TableCell>
                  {canReadEmail && <TableCell>{has(user, 'email') ? user.email : <Hidden />}</TableCell>}
                  {canReadPhone && <TableCell>{has(user, 'phone') ? user.phone || '—' : <Hidden />}</TableCell>}
                  {canReadStatus && (
                    <TableCell>
                      {has(user, 'status') ? <StatusChip status={user.status} kind="user" /> : <Hidden />}
                    </TableCell>
                  )}
                  <TableCell>{user.user_type.replace('_', ' ')}</TableCell>
                  <TableCell>{formatDate(user.created_at)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
        {query.data?.items.length === 0 && <EmptyState message="No users match this search." />}
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
          rowsPerPageOptions={[10, 20, 50, 100]}
        />
      </Card>
    </>
  );
}
