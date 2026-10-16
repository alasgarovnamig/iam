import { useMemo, useState } from 'react';
import {
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  IconButton,
  MenuItem,
  Select,
  Stack,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import CloseIcon from '@mui/icons-material/Close';
import SearchIcon from '@mui/icons-material/Search';
import {
  OPERATORS,
  buildSearchRequest,
  type Condition,
  type Logic,
  type SearchField,
  type SearchRequest,
} from '../api/search';

let counter = 0;
const newId = () => `c${++counter}`;

/**
 * Permission-aware dynamic search builder.
 *
 * The user assembles a flat list of conditions (field · operator · value)
 * joined by one AND/OR combinator — enough for "(name AND slug)",
 * "(name OR slug contains …)" and so on. Only fields the actor may search are
 * offered (`fields` is already filtered by the caller via searchableFields),
 * so an unsearchable field cannot even be selected. The backend re-enforces
 * everything.
 */
export function SearchBuilder({
  fields,
  onSearch,
  onClear,
}: {
  fields: SearchField[];
  onSearch: (req: SearchRequest) => void;
  onClear: () => void;
}) {
  const [logic, setLogic] = useState<Logic>('and');
  const [conditions, setConditions] = useState<Condition[]>([]);

  const firstField = fields[0];

  const makeCondition = (): Condition => ({
    id: newId(),
    field: firstField?.name ?? '',
    operator: firstField ? OPERATORS[firstField.type][0].key : 'equals',
    value: '',
    values: [],
  });

  const add = () => setConditions((cs) => [...cs, makeCondition()]);
  const remove = (id: string) => setConditions((cs) => cs.filter((c) => c.id !== id));
  const patch = (id: string, next: Partial<Condition>) =>
    setConditions((cs) => cs.map((c) => (c.id === id ? { ...c, ...next } : c)));

  const fieldByName = useMemo(() => new Map(fields.map((f) => [f.name, f])), [fields]);

  const apply = () => onSearch(buildSearchRequest(fields, logic, conditions, 1, 20));
  const clear = () => {
    setConditions([]);
    onClear();
  };

  if (fields.length === 0) return null;

  return (
    <Card sx={{ mb: 2 }} data-testid="search-builder">
      <CardContent>
        <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 1.5 }}>
          <SearchIcon fontSize="small" color="action" />
          <Typography variant="subtitle2">Search</Typography>
          {conditions.length > 1 && (
            <ToggleButtonGroup
              size="small"
              exclusive
              value={logic}
              onChange={(_, v) => v && setLogic(v)}
              sx={{ ml: 1 }}
            >
              <ToggleButton value="and" data-testid="logic-and">
                Match ALL
              </ToggleButton>
              <ToggleButton value="or" data-testid="logic-or">
                Match ANY
              </ToggleButton>
            </ToggleButtonGroup>
          )}
        </Stack>

        <Stack spacing={1.5}>
          {conditions.map((c) => {
            const field = fieldByName.get(c.field);
            if (!field) return null;
            const ops = OPERATORS[field.type];
            return (
              <Stack key={c.id} direction="row" spacing={1} alignItems="center" data-testid="condition-row">
                <Select
                  size="small"
                  value={c.field}
                  onChange={(e) => {
                    const nf = fieldByName.get(e.target.value)!;
                    patch(c.id, {
                      field: nf.name,
                      operator: OPERATORS[nf.type][0].key,
                      value: '',
                      values: [],
                    });
                  }}
                  sx={{ minWidth: 150 }}
                  inputProps={{ 'data-testid': 'condition-field' }}
                >
                  {fields.map((f) => (
                    <MenuItem key={f.name} value={f.name}>
                      {f.label}
                    </MenuItem>
                  ))}
                </Select>

                <Select
                  size="small"
                  value={c.operator}
                  onChange={(e) => patch(c.id, { operator: e.target.value })}
                  sx={{ minWidth: 140 }}
                  inputProps={{ 'data-testid': 'condition-op' }}
                >
                  {ops.map((o) => (
                    <MenuItem key={o.key} value={o.key}>
                      {o.label}
                    </MenuItem>
                  ))}
                </Select>

                <ConditionValue field={field} condition={c} onChange={(v) => patch(c.id, v)} />

                <IconButton size="small" onClick={() => remove(c.id)} data-testid="condition-remove">
                  <CloseIcon fontSize="small" />
                </IconButton>
              </Stack>
            );
          })}
        </Stack>

        <Stack direction="row" spacing={1} sx={{ mt: 2 }}>
          <Button size="small" startIcon={<AddIcon />} onClick={add} data-testid="add-condition">
            Add condition
          </Button>
          <Box sx={{ flexGrow: 1 }} />
          <Button size="small" onClick={clear} data-testid="search-clear">
            Clear
          </Button>
          <Button
            size="small"
            variant="contained"
            startIcon={<SearchIcon />}
            onClick={apply}
            disabled={conditions.length === 0}
            data-testid="search-apply"
          >
            Search
          </Button>
        </Stack>
      </CardContent>
    </Card>
  );
}

function ConditionValue({
  field,
  condition,
  onChange,
}: {
  field: SearchField;
  condition: Condition;
  onChange: (patch: Partial<Condition>) => void;
}) {
  const isIn = condition.operator === 'in';

  if (isIn && field.type === 'enum') {
    return (
      <Select
        multiple
        size="small"
        value={condition.values}
        onChange={(e) => onChange({ values: e.target.value as string[] })}
        sx={{ minWidth: 220 }}
        renderValue={(vals) => (
          <Stack direction="row" spacing={0.5} flexWrap="wrap">
            {(vals as string[]).map((v) => (
              <Chip key={v} label={v} size="small" />
            ))}
          </Stack>
        )}
        inputProps={{ 'data-testid': 'condition-values' }}
      >
        {(field.options ?? []).map((opt) => (
          <MenuItem key={opt} value={opt}>
            {opt}
          </MenuItem>
        ))}
      </Select>
    );
  }

  if (field.type === 'enum') {
    return (
      <Select
        size="small"
        value={condition.value}
        onChange={(e) => onChange({ value: e.target.value })}
        sx={{ minWidth: 180 }}
        inputProps={{ 'data-testid': 'condition-value' }}
      >
        {(field.options ?? []).map((opt) => (
          <MenuItem key={opt} value={opt}>
            {opt}
          </MenuItem>
        ))}
      </Select>
    );
  }

  if (field.type === 'timestamp') {
    return (
      <TextField
        size="small"
        type="datetime-local"
        value={condition.value}
        onChange={(e) => onChange({ value: toRFC3339(e.target.value) })}
        sx={{ minWidth: 220 }}
        inputProps={{ 'data-testid': 'condition-value' }}
      />
    );
  }

  // text
  return (
    <TextField
      size="small"
      placeholder={isIn ? 'value1, value2, …' : 'value'}
      value={condition.value}
      onChange={(e) => onChange({ value: e.target.value })}
      sx={{ minWidth: 220 }}
      inputProps={{ 'data-testid': 'condition-value' }}
    />
  );
}

// datetime-local yields "2026-01-02T15:04"; the API wants RFC 3339.
function toRFC3339(local: string): string {
  if (!local) return '';
  const d = new Date(local);
  return Number.isNaN(d.getTime()) ? local : d.toISOString();
}
