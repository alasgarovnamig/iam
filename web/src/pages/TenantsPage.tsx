import { useMemo, useState } from 'react';
import { Link as RouterLink } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Button,
  Card,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
  TextField,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { getList, postData, postList } from '../api/client';
import type { Tenant } from '../api/types';
import type { SearchRequest } from '../api/search';
import { usePermissions } from '../auth/AuthContext';
import { P } from '../auth/permissions';
import { TENANT_SEARCH_FIELDS, searchableFields } from '../auth/searchFields';
import { Can } from '../components/guards';
import { SearchBuilder } from '../components/SearchBuilder';
import { EmptyState, ErrorAlert, PageHeader, StatusChip, formatDate } from '../components/common';

const createSchema = z.object({
  name: z.string().min(1, 'Name is required').max(200),
  slug: z
    .string()
    .min(2, 'Slug is required')
    .max(63)
    .regex(/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/, 'Lowercase letters, digits and hyphens only'),
});

type CreateForm = z.infer<typeof createSchema>;

export function TenantsPage() {
  const permissions = usePermissions();
  const queryClient = useQueryClient();
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(20);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [search, setSearch] = useState<SearchRequest | null>(null);

  const fields = useMemo(
    () => searchableFields(permissions, 'tenants', TENANT_SEARCH_FIELDS),
    [permissions],
  );
  const canSearch = fields.length > 0;

  const query = useQuery({
    queryKey: ['tenants', canSearch ? 'search' : 'list', search, page, pageSize],
    queryFn: () => {
      if (canSearch) {
        return postList<Tenant>('/api/v1/tenants/search', {
          ...(search ?? {}),
          page: page + 1,
          page_size: pageSize,
        });
      }
      return getList<Tenant>('/api/v1/tenants', { page: page + 1, page_size: pageSize });
    },
  });

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<CreateForm>({ resolver: zodResolver(createSchema) });

  const createTenant = useMutation({
    mutationFn: (values: CreateForm) =>
      postData<Tenant>('/api/v1/tenants', values, {
        // Retrying a create must not produce a second tenant.
        'Idempotency-Key': crypto.randomUUID(),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tenants'] });
      setDialogOpen(false);
      reset();
    },
  });

  return (
    <>
      <PageHeader
        title="Tenants"
        subtitle="Customer companies integrated with the gateway"
        actions={
          <Can permission={P.tenants.create}>
            <Button
              variant="contained"
              startIcon={<AddIcon />}
              onClick={() => setDialogOpen(true)}
              data-testid="create-tenant"
            >
              New tenant
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
                <TableCell>Slug</TableCell>
                <TableCell>Status</TableCell>
                <TableCell>Created</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {(query.data?.items ?? []).map((tenant) => (
                <TableRow key={tenant.id} hover data-testid="tenant-row">
                  <TableCell>
                    {permissions.has(P.tenants.read) ? (
                      <RouterLink to={`/tenants/${tenant.id}`}>{tenant.name}</RouterLink>
                    ) : (
                      tenant.name
                    )}
                  </TableCell>
                  <TableCell>{tenant.slug}</TableCell>
                  <TableCell>
                    <StatusChip status={tenant.status} kind="tenant" />
                  </TableCell>
                  <TableCell>{formatDate(tenant.created_at)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
        {query.data?.items.length === 0 && <EmptyState message="No tenants yet." />}
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

      <Dialog open={dialogOpen} onClose={() => setDialogOpen(false)} fullWidth maxWidth="sm">
        <form onSubmit={handleSubmit((values) => createTenant.mutateAsync(values))}>
          <DialogTitle>New tenant</DialogTitle>
          <DialogContent>
            <ErrorAlert error={createTenant.error} />
            <Stack spacing={2} sx={{ mt: 1 }}>
              <TextField
                label="Name"
                fullWidth
                error={Boolean(errors.name)}
                helperText={errors.name?.message}
                inputProps={{ 'data-testid': 'tenant-name' }}
                {...register('name')}
              />
              <TextField
                label="Slug"
                fullWidth
                error={Boolean(errors.slug)}
                helperText={errors.slug?.message ?? 'Used at sign-in to disambiguate users, e.g. acme'}
                inputProps={{ 'data-testid': 'tenant-slug' }}
                {...register('slug')}
              />
            </Stack>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setDialogOpen(false)}>Cancel</Button>
            <Button type="submit" variant="contained" disabled={isSubmitting} data-testid="tenant-submit">
              Create
            </Button>
          </DialogActions>
        </form>
      </Dialog>
    </>
  );
}
