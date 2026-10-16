import { useState } from 'react';
import { Link as RouterLink, useNavigate } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import {
  Button,
  Card,
  Chip,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import { getList } from '../api/client';
import type { Role } from '../api/types';
import { usePermissions } from '../auth/AuthContext';
import { P } from '../auth/permissions';
import { Can } from '../components/guards';
import { EmptyState, ErrorAlert, PageHeader, formatDate } from '../components/common';

export function RolesPage() {
  const permissions = usePermissions();
  const navigate = useNavigate();
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(20);

  const query = useQuery({
    queryKey: ['roles', page, pageSize],
    queryFn: () => getList<Role>('/api/v1/roles', { page: page + 1, page_size: pageSize }),
  });

  return (
    <>
      <PageHeader
        title="Roles"
        subtitle="System roles are read-only; custom roles carry the permissions you delegate."
        actions={
          <Can permission={P.roles.create}>
            <Button
              variant="contained"
              startIcon={<AddIcon />}
              onClick={() => navigate('/roles/new')}
              data-testid="create-role"
            >
              New role
            </Button>
          </Can>
        }
      />

      <ErrorAlert error={query.error} />

      <Card>
        <TableContainer>
          <Table>
            <TableHead>
              <TableRow>
                <TableCell>Name</TableCell>
                <TableCell>Description</TableCell>
                <TableCell>Type</TableCell>
                <TableCell>Origin</TableCell>
                <TableCell>Status</TableCell>
                <TableCell>Created</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {(query.data?.items ?? []).map((role) => (
                <TableRow key={role.id} hover data-testid="role-row">
                  <TableCell>
                    {permissions.has(P.roles.read) ? (
                      <RouterLink to={`/roles/${role.id}`}>{role.name}</RouterLink>
                    ) : (
                      role.name
                    )}
                  </TableCell>
                  <TableCell>{role.description || '—'}</TableCell>
                  <TableCell>{role.role_type}</TableCell>
                  <TableCell>
                    <Chip
                      size="small"
                      label={role.is_system ? 'system' : 'custom'}
                      color={role.is_system ? 'default' : 'primary'}
                      variant="outlined"
                    />
                  </TableCell>
                  <TableCell>{role.status}</TableCell>
                  <TableCell>{formatDate(role.created_at)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
        {query.data?.items.length === 0 && <EmptyState message="No roles yet." />}
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
