/**
 * Client-side model of the dynamic search API (mirrors the backend
 * domain.FilterNode). The UI builds a flat list of conditions joined by one
 * AND/OR combinator — which covers "(name AND slug)", "(name OR slug LIKE …)"
 * etc. The wire format supports arbitrary nesting; a deeper builder can be
 * added later without an API change.
 */

export type Logic = 'and' | 'or';

/** Comparison operators as the API names them. */
export type Compare = 'eq' | 'neq' | 'like' | 'ilike' | 'in' | 'gt' | 'gte' | 'lt' | 'lte';

export interface FilterNode {
  // group
  op?: Logic | Compare;
  conditions?: FilterNode[];
  // leaf
  field?: string;
  value?: unknown;
}

export interface SortTerm {
  field: string;
  dir: 'asc' | 'desc';
}

export interface SearchRequest {
  filter?: FilterNode;
  sort?: SortTerm[];
  page?: number;
  page_size?: number;
}

/** Field types decide which operators and value inputs are offered. */
export type FieldType = 'text' | 'enum' | 'timestamp';

export interface SearchField {
  /** API field name (also the last segment of the permission code). */
  name: string;
  label: string;
  type: FieldType;
  /** For enum fields. */
  options?: string[];
  /**
   * A base field (e.g. created_at) needs only the action-level search
   * permission, not a per-field grant.
   */
  base?: boolean;
}

/** One row the user builds in the UI. */
export interface Condition {
  id: string;
  field: string;
  /** UI operator key (friendly), mapped to a Compare + value transform. */
  operator: string;
  value: string;
  /** For "is one of": selected enum values. */
  values: string[];
}

/** Friendly operator options per field type. */
export const OPERATORS: Record<FieldType, { key: string; label: string; compare: Compare }[]> = {
  text: [
    { key: 'contains', label: 'contains', compare: 'ilike' },
    { key: 'equals', label: 'equals', compare: 'eq' },
    { key: 'not_equals', label: 'does not equal', compare: 'neq' },
    { key: 'starts_with', label: 'starts with', compare: 'ilike' },
    { key: 'in', label: 'is one of', compare: 'in' },
  ],
  enum: [
    { key: 'is', label: 'is', compare: 'eq' },
    { key: 'is_not', label: 'is not', compare: 'neq' },
    { key: 'in', label: 'is one of', compare: 'in' },
  ],
  timestamp: [
    { key: 'after', label: 'after', compare: 'gt' },
    { key: 'before', label: 'before', compare: 'lt' },
    { key: 'on_or_after', label: 'on or after', compare: 'gte' },
    { key: 'on_or_before', label: 'on or before', compare: 'lte' },
  ],
};

/** Turns one UI condition into an API leaf, applying value transforms. */
export function conditionToLeaf(field: SearchField, c: Condition): FilterNode | null {
  const opDef = OPERATORS[field.type].find((o) => o.key === c.operator);
  if (!opDef) return null;

  if (opDef.compare === 'in') {
    const list = field.type === 'enum' ? c.values : splitCsv(c.value);
    if (list.length === 0) return null;
    return { field: field.name, op: 'in', value: list };
  }
  if (c.value.trim() === '') return null;

  let value: string = c.value;
  if (c.operator === 'contains') value = `%${c.value}%`;
  else if (c.operator === 'starts_with') value = `${c.value}%`;
  return { field: field.name, op: opDef.compare, value };
}

function splitCsv(v: string): string[] {
  return v
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean);
}

/** Builds the SearchRequest from the UI state. */
export function buildSearchRequest(
  fields: SearchField[],
  logic: Logic,
  conditions: Condition[],
  page: number,
  pageSize: number,
): SearchRequest {
  const leaves: FilterNode[] = [];
  for (const c of conditions) {
    const field = fields.find((f) => f.name === c.field);
    if (!field) continue;
    const leaf = conditionToLeaf(field, c);
    if (leaf) leaves.push(leaf);
  }
  const req: SearchRequest = { page, page_size: pageSize };
  if (leaves.length === 1) req.filter = leaves[0];
  else if (leaves.length > 1) req.filter = { op: logic, conditions: leaves };
  return req;
}
