import { useMemo, useState } from 'react';
import {
  Box,
  Button,
  Checkbox,
  Chip,
  Collapse,
  IconButton,
  MenuItem,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import LockIcon from '@mui/icons-material/Lock';
import { SCOPES, SCOPE_LABELS, type PermissionTreeService, type Scope } from '../api/types';
import type { PermissionSet } from '../auth/permissions';
import { PermissionInfo } from './PermissionInfo';

/**
 * Permission assignment tree: Service → Resource → Action → Field.
 *
 * Delegation rule in the UI: an entry the actor does not hold is rendered
 * disabled with the reason, and the scope picker only offers scopes the actor
 * can actually delegate. This is guidance, not enforcement — the API applies
 * the same rule again and is the authority.
 */

export interface GrantSelection {
  permissionId: string;
  code: string;
  scope: Scope;
}

interface PermissionTreeProps {
  tree: PermissionTreeService[];
  /** Permissions the actor holds — determines what can be granted. */
  actorPermissions: PermissionSet;
  /** Codes already granted to the target (rendered as checked + disabled). */
  existingCodes: Set<string>;
  onGrant: (selection: GrantSelection) => void;
  disabled?: boolean;
}

export function PermissionTree({
  tree,
  actorPermissions,
  existingCodes,
  onGrant,
  disabled = false,
}: PermissionTreeProps) {
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const toggle = (key: string) => setExpanded((prev) => ({ ...prev, [key]: !prev[key] }));

  // Every resource key in the tree, so "expand all" can flip them in one go.
  const resourceKeys = useMemo(
    () =>
      tree.flatMap((s) => [s.service, ...s.resources.map((r) => `${s.service}.${r.resource}`)]),
    [tree],
  );
  const setAll = (open: boolean) =>
    setExpanded(Object.fromEntries(resourceKeys.map((k) => [k, open])));

  return (
    <Box data-testid="permission-tree">
      <Stack direction="row" spacing={1} sx={{ mb: 1 }}>
        <Button size="small" onClick={() => setAll(true)} data-testid="expand-all">
          Expand all
        </Button>
        <Button size="small" onClick={() => setAll(false)} data-testid="collapse-all">
          Collapse all
        </Button>
      </Stack>
      {tree.map((service) => (
        <Box key={service.service} sx={{ mb: 1 }}>
          {/* The whole header toggles, not just the chevron. */}
          <Stack
            direction="row"
            alignItems="center"
            spacing={1}
            onClick={() => toggle(service.service)}
            sx={{ cursor: 'pointer', userSelect: 'none' }}
            data-testid={`tree-service-${service.service}`}
          >
            <IconButton size="small" tabIndex={-1}>
              {expanded[service.service] ? <ExpandMoreIcon /> : <ChevronRightIcon />}
            </IconButton>
            <Typography variant="subtitle1" fontWeight={600}>
              {humanize(service.service)}
            </Typography>
          </Stack>
          <Collapse in={expanded[service.service] ?? true} unmountOnExit>
            <Box sx={{ pl: 4 }}>
              {service.resources.map((resource) => {
                const resourceKey = `${service.service}.${resource.resource}`;
                const isOpen = expanded[resourceKey] ?? false;
                const codes = resourceCodes(resource);
                const grantedHere = codes.filter((c) => existingCodes.has(c)).length;
                return (
                  <Box key={resourceKey} sx={{ mb: 1 }}>
                    {/* A real, focusable control: collapsed groups used to look
                        like inert labels, so the tree read as empty. */}
                    <Stack
                      direction="row"
                      alignItems="center"
                      spacing={1}
                      role="button"
                      tabIndex={0}
                      aria-expanded={isOpen}
                      onClick={() => toggle(resourceKey)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter' || e.key === ' ') {
                          e.preventDefault();
                          toggle(resourceKey);
                        }
                      }}
                      sx={{
                        cursor: 'pointer',
                        userSelect: 'none',
                        borderRadius: 1,
                        pr: 1,
                        '&:hover': { bgcolor: 'action.hover' },
                        '&:focus-visible': { outline: '2px solid', outlineColor: 'primary.main' },
                      }}
                      data-testid={`tree-resource-${resource.resource}`}
                    >
                      <IconButton size="small" tabIndex={-1}>
                        {isOpen ? <ExpandMoreIcon /> : <ChevronRightIcon />}
                      </IconButton>
                      <Typography variant="subtitle2">{humanize(resource.resource)}</Typography>
                      <Chip
                        size="small"
                        variant="outlined"
                        label={`${codes.length} permission${codes.length === 1 ? '' : 's'}`}
                      />
                      {grantedHere > 0 && (
                        <Chip
                          size="small"
                          color="success"
                          variant="outlined"
                          label={`${grantedHere} granted`}
                        />
                      )}
                    </Stack>
                    <Collapse in={isOpen} unmountOnExit>
                      <Box sx={{ pl: 4 }}>
                        {resource.actions.map((action) => (
                          <Box key={action.code || `${resourceKey}.${action.action}`} sx={{ mb: 0.5 }}>
                            {action.id && (
                              <PermissionRow
                                permissionId={action.id}
                                code={action.code}
                                label={humanize(action.action)}
                                actorPermissions={actorPermissions}
                                existingCodes={existingCodes}
                                onGrant={onGrant}
                                disabled={disabled}
                              />
                            )}
                            {(action.fields ?? []).length > 0 && (
                              <Box sx={{ pl: 4 }}>
                                {(action.fields ?? []).map((field) => (
                                  <PermissionRow
                                    key={field.id}
                                    permissionId={field.id}
                                    code={field.code}
                                    label={humanize(field.field ?? '')}
                                    isField
                                    actorPermissions={actorPermissions}
                                    existingCodes={existingCodes}
                                    onGrant={onGrant}
                                    disabled={disabled}
                                  />
                                ))}
                              </Box>
                            )}
                          </Box>
                        ))}
                      </Box>
                    </Collapse>
                  </Box>
                );
              })}
            </Box>
          </Collapse>
        </Box>
      ))}
    </Box>
  );
}

function PermissionRow({
  permissionId,
  code,
  label,
  isField = false,
  actorPermissions,
  existingCodes,
  onGrant,
  disabled,
}: {
  permissionId: string;
  code: string;
  label: string;
  isField?: boolean;
  actorPermissions: PermissionSet;
  existingCodes: Set<string>;
  onGrant: (selection: GrantSelection) => void;
  disabled: boolean;
}) {
  const alreadyGranted = existingCodes.has(code);
  const holds = actorPermissions.has(code);
  const blockReason = actorPermissions.delegationBlockReason(code, 'own');

  // Only scopes the actor may delegate are offered; wider ones are omitted.
  const allowedScopes = useMemo(
    () => actorPermissions.delegatableScopes(code, SCOPES),
    [actorPermissions, code],
  );
  const [scope, setScope] = useState<Scope>(() => allowedScopes[allowedScopes.length - 1] ?? 'own');

  const cannotDelegate = !holds || Boolean(blockReason);

  return (
    <Stack direction="row" alignItems="center" spacing={1} sx={{ py: 0.25 }}>
      <Tooltip title={cannotDelegate ? (blockReason ?? 'You do not hold this permission.') : ''} arrow>
        <span>
          <Checkbox
            size="small"
            checked={alreadyGranted}
            disabled={disabled || alreadyGranted || cannotDelegate}
            onChange={() => onGrant({ permissionId, code, scope })}
            data-testid={`perm-${code}`}
          />
        </span>
      </Tooltip>

      <Typography
        variant="body2"
        color={cannotDelegate ? 'text.disabled' : 'text.primary'}
        sx={{ minWidth: 140, fontStyle: isField ? 'italic' : 'normal' }}
      >
        {label}
      </Typography>

      <PermissionInfo code={code} />

      {cannotDelegate && (
        <Tooltip title={blockReason ?? 'You do not hold this permission.'} arrow>
          <LockIcon fontSize="inherit" color="disabled" data-testid={`perm-locked-${code}`} />
        </Tooltip>
      )}

      {alreadyGranted && <Chip size="small" label="granted" color="success" variant="outlined" />}

      {!alreadyGranted && !cannotDelegate && (
        <>
          <TextField
            select
            size="small"
            value={scope}
            onChange={(e) => setScope(e.target.value as Scope)}
            sx={{ width: 190 }}
            disabled={disabled}
            inputProps={{ 'data-testid': `scope-${code}` }}
          >
            {SCOPES.map((s) => {
              const allowed = allowedScopes.includes(s);
              return (
                <MenuItem key={s} value={s} disabled={!allowed}>
                  {SCOPE_LABELS[s]}
                  {!allowed && ' — wider than yours'}
                </MenuItem>
              );
            })}
          </TextField>
          <Button
            size="small"
            variant="outlined"
            disabled={disabled}
            onClick={() => onGrant({ permissionId, code, scope })}
            data-testid={`grant-${code}`}
          >
            Grant
          </Button>
        </>
      )}

      <Typography variant="caption" color="text.disabled" sx={{ ml: 'auto' }}>
        {code}
      </Typography>
    </Stack>
  );
}

/** Every permission code under a resource, actions plus their fields. */
function resourceCodes(resource: PermissionTreeService['resources'][number]): string[] {
  return resource.actions.flatMap((a) => [
    ...(a.id ? [a.code] : []),
    ...(a.fields ?? []).map((f) => f.code),
  ]);
}

function humanize(value: string): string {
  return value
    .split(/[-_.]/)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ');
}
