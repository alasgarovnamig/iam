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
import type { ApiClient } from '../api/types';
import type { SearchRequest } from '../api/search';
import { usePermissions } from '../auth/AuthContext';
import { P } from '../auth/permissions';
import { API_CLIENT_SEARCH_FIELDS, searchableFields } from '../auth/searchFields';
import { Can } from '../components/guards';
import { SearchBuilder } from '../components/SearchBuilder';
import { EmptyState, ErrorAlert, PageHeader, StatusChip, formatDate } from '../components/common';

export function ApiClientsPage() {
  const permissions = usePermissions();
  const navigate = useNavigate();
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(20);
  const [search, setSearch] = useState<SearchRequest | null>(null);

  const fields = useMemo(
    () => searchableFields(permissions, 'api-clients', API_CLIENT_SEARCH_FIELDS),
    [permissions],
  );
  const canSearch = fields.length > 0;

  const query = useQuery({
    queryKey: ['api-clients', canSearch ? 'search' : 'list', search, page, pageSize],
    queryFn: () => {
      if (canSearch) {
        return postList<ApiClient>('/api/v1/api-clients/search', {
          ...(search ?? {}),
          page: page + 1,
          page_size: pageSize,
        });
      }
      return getList<ApiClient>('/api/v1/api-clients', { page: page + 1, page_size: pageSize });
    },
  });

  return (
    <>
      <PageHeader
        title="API clients"
        subtitle="Machine-to-machine credentials using the OAuth 2.0 client-credentials grant"
        actions={
          <Can permission={P.apiClients.create}>
            <Button
              variant="contained"
              startIcon={<AddIcon />}
              onClick={() => navigate('/api-clients/new')}
              data-testid="create-api-client"
            >
              New API client
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
                <TableCell>Client ID</TableCell>
                <TableCell>Status</TableCell>
                <TableCell>Last used</TableCell>
                <TableCell>Created</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {(query.data?.items ?? []).map((client) => (
                <TableRow key={client.id} hover data-testid="api-client-row">
                  <TableCell>
                    {permissions.has(P.apiClients.read) ? (
                      <RouterLink to={`/api-clients/${client.id}`}>{client.name}</RouterLink>
                    ) : (
                      client.name
                    )}
                  </TableCell>
                  <TableCell>
                    <code>{client.client_id}</code>
                  </TableCell>
                  <TableCell>
                    <StatusChip status={client.status} kind="api-client" />
                  </TableCell>
                  <TableCell>{formatDate(client.last_used_at)}</TableCell>
                  <TableCell>{formatDate(client.created_at)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
        {query.data?.items.length === 0 && <EmptyState message="No API clients yet." />}
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
          rowsPerPageOptions={[10, 20, 50]}
        />
      </Card>
    </>
  );
}
