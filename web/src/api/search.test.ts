import { describe, expect, it } from 'vitest';
import { buildSearchRequest, conditionToLeaf, type Condition } from './search';
import { USER_SEARCH_FIELDS } from '../auth/searchFields';

const field = (name: string) => USER_SEARCH_FIELDS.find((f) => f.name === name)!;
const cond = (over: Partial<Condition>): Condition => ({
  id: 'c1',
  field: 'first_name',
  operator: 'equals',
  value: '',
  values: [],
  ...over,
});

describe('conditionToLeaf value transforms', () => {
  it('maps "contains" to an ilike wrapped in %…%', () => {
    const leaf = conditionToLeaf(field('first_name'), cond({ operator: 'contains', value: 'Ela' }));
    expect(leaf).toEqual({ field: 'first_name', op: 'ilike', value: '%Ela%' });
  });

  it('maps "starts with" to an ilike with a trailing %', () => {
    const leaf = conditionToLeaf(field('last_name'), cond({ field: 'last_name', operator: 'starts_with', value: 'Sw' }));
    expect(leaf).toEqual({ field: 'last_name', op: 'ilike', value: 'Sw%' });
  });

  it('maps "equals" to eq with the exact value', () => {
    const leaf = conditionToLeaf(field('email'), cond({ field: 'email', operator: 'equals', value: 'a@b.c' }));
    expect(leaf).toEqual({ field: 'email', op: 'eq', value: 'a@b.c' });
  });

  it('builds an "in" list from selected enum values', () => {
    const leaf = conditionToLeaf(
      field('status'),
      cond({ field: 'status', operator: 'in', values: ['active', 'pending'] }),
    );
    expect(leaf).toEqual({ field: 'status', op: 'in', value: ['active', 'pending'] });
  });

  it('builds an "in" list from a comma-separated text value', () => {
    const leaf = conditionToLeaf(field('email'), cond({ field: 'email', operator: 'in', value: 'a@x, b@y ,c@z' }));
    expect(leaf).toEqual({ field: 'email', op: 'in', value: ['a@x', 'b@y', 'c@z'] });
  });

  it('drops an empty scalar condition', () => {
    expect(conditionToLeaf(field('first_name'), cond({ operator: 'equals', value: '' }))).toBeNull();
  });

  it('drops an empty "in" condition', () => {
    expect(conditionToLeaf(field('status'), cond({ field: 'status', operator: 'in', values: [] }))).toBeNull();
  });
});

describe('buildSearchRequest', () => {
  it('sends a single leaf directly (no wrapping group)', () => {
    const req = buildSearchRequest(
      USER_SEARCH_FIELDS,
      'and',
      [cond({ operator: 'contains', value: 'Elam' })],
      1,
      20,
    );
    expect(req.filter).toEqual({ field: 'first_name', op: 'ilike', value: '%Elam%' });
  });

  it('wraps multiple leaves in the chosen and/or group', () => {
    const req = buildSearchRequest(
      USER_SEARCH_FIELDS,
      'or',
      [
        cond({ id: 'a', field: 'first_name', operator: 'equals', value: 'Elam' }),
        cond({ id: 'b', field: 'last_name', operator: 'contains', value: 'sw' }),
      ],
      2,
      50,
    );
    expect(req.filter).toEqual({
      op: 'or',
      conditions: [
        { field: 'first_name', op: 'eq', value: 'Elam' },
        { field: 'last_name', op: 'ilike', value: '%sw%' },
      ],
    });
    expect(req.page).toBe(2);
    expect(req.page_size).toBe(50);
  });

  it('omits the filter entirely when every condition is empty', () => {
    const req = buildSearchRequest(USER_SEARCH_FIELDS, 'and', [cond({ value: '' })], 1, 20);
    expect(req.filter).toBeUndefined();
  });
});
