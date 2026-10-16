import { useEffect } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { useForm } from 'react-hook-form';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Button, Card, CardContent, Stack, TextField, Typography } from '@mui/material';
import { getData, patchData } from '../api/client';
import { has, type User } from '../api/types';
import { usePermissions } from '../auth/AuthContext';
import { P, USER_EDITABLE_FIELDS } from '../auth/permissions';
import { DisabledHint } from '../components/guards';
import { ErrorAlert, FullPageSpinnerFallback, PageHeader } from '../components/common';

type FormValues = Record<string, string>;

export function UserEditPage() {
  const { userId = '' } = useParams();
  const navigate = useNavigate();
  const permissions = usePermissions();
  const queryClient = useQueryClient();

  const query = useQuery({
    queryKey: ['users', userId],
    queryFn: () => getData<User>(`/api/v1/users/${userId}`),
  });

  const { register, handleSubmit, reset, formState } = useForm<FormValues>();

  useEffect(() => {
    if (query.data) {
      reset({
        email: query.data.email ?? '',
        first_name: query.data.first_name ?? '',
        last_name: query.data.last_name ?? '',
        phone: query.data.phone ?? '',
        internal_notes: query.data.internal_notes ?? '',
      });
    }
  }, [query.data, reset]);

  const update = useMutation({
    mutationFn: (values: FormValues) => {
      /**
       * Send only fields that (a) the actor may write and (b) actually
       * changed. The API rejects the whole request when a body carries a
       * field without the matching users.update.<field> grant, so a blanket
       * PATCH of every field would fail for partially-privileged actors.
       */
      const body: Record<string, unknown> = { version: query.data?.version };
      for (const field of USER_EDITABLE_FIELDS) {
        if (!permissions.has(P.users.updateField(field))) continue;
        const current = (query.data?.[field as keyof User] as string | undefined) ?? '';
        if (values[field] !== current) body[field] = values[field];
      }
      return patchData<User>(`/api/v1/users/${userId}`, body);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] });
      navigate(`/users/${userId}`);
    },
  });

  if (query.isLoading) return <FullPageSpinnerFallback />;
  if (query.error) return <ErrorAlert error={query.error} />;
  const user = query.data;
  if (!user) return null;

  const editable = (field: string) => permissions.has(P.users.updateField(field));
  const anyEditable = USER_EDITABLE_FIELDS.some(editable);

  return (
    <>
      <PageHeader title="Edit user" subtitle="Fields you cannot update are disabled." />
      <ErrorAlert error={update.error} />

      <Card sx={{ maxWidth: 720 }}>
        <CardContent>
          <form onSubmit={handleSubmit((values) => update.mutateAsync(values))} noValidate>
            <Stack spacing={2}>
              <DisabledHint reason={editable('email') ? null : 'You lack users.update.email'}>
                <TextField
                  label="Email"
                  fullWidth
                  disabled={!editable('email') || !has(user, 'email')}
                  helperText={has(user, 'email') ? undefined : 'You cannot read this field.'}
                  inputProps={{ 'data-testid': 'edit-email' }}
                  {...register('email')}
                />
              </DisabledHint>

              <Stack direction="row" spacing={2}>
                <DisabledHint reason={editable('first_name') ? null : 'You lack users.update.first_name'}>
                  <TextField
                    label="First name"
                    fullWidth
                    disabled={!editable('first_name')}
                    inputProps={{ 'data-testid': 'edit-first-name' }}
                    {...register('first_name')}
                  />
                </DisabledHint>
                <DisabledHint reason={editable('last_name') ? null : 'You lack users.update.last_name'}>
                  <TextField
                    label="Last name"
                    fullWidth
                    disabled={!editable('last_name')}
                    inputProps={{ 'data-testid': 'edit-last-name' }}
                    {...register('last_name')}
                  />
                </DisabledHint>
              </Stack>

              <DisabledHint reason={editable('phone') ? null : 'You lack users.update.phone'}>
                <TextField
                  label="Phone"
                  fullWidth
                  disabled={!editable('phone')}
                  inputProps={{ 'data-testid': 'edit-phone' }}
                  {...register('phone')}
                />
              </DisabledHint>

              {editable('internal_notes') && (
                <TextField
                  label="Internal notes"
                  fullWidth
                  multiline
                  rows={3}
                  inputProps={{ 'data-testid': 'edit-notes' }}
                  {...register('internal_notes')}
                />
              )}

              {!anyEditable && (
                <Typography variant="body2" color="warning.main">
                  You hold users.update but no field-level grants, so there is nothing to edit.
                </Typography>
              )}

              <Stack direction="row" spacing={1} justifyContent="flex-end">
                <Button onClick={() => navigate(`/users/${userId}`)}>Cancel</Button>
                <Button
                  type="submit"
                  variant="contained"
                  disabled={formState.isSubmitting || !anyEditable}
                  data-testid="save-user"
                >
                  Save changes
                </Button>
              </Stack>
            </Stack>
          </form>
        </CardContent>
      </Card>
    </>
  );
}
