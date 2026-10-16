package postgres

import (
	"fmt"
	"strings"

	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
)

// buildWhere turns a validated domain.SearchQuery into a parameterised SQL
// WHERE fragment. Column names come only from the resource schema (never from
// the request), operators from a fixed map, and every value is a bound
// parameter — so there is no SQL-injection surface even though the query
// shape is fully dynamic.
//
// baseWhere/baseArgs are the mandatory predicates the caller always applies
// (soft-delete filter, tenant scoping). startIdx is the next $N placeholder.
func buildWhere(schema domain.ResourceSchema, q domain.SearchQuery, baseWhere []string, baseArgs []any, startIdx int) (string, []any, int, error) {
	b := &whereBuilder{schema: schema, args: append([]any{}, baseArgs...), idx: startIdx}
	parts := append([]string{}, baseWhere...)
	if q.Filter != nil {
		frag, err := b.node(*q.Filter)
		if err != nil {
			return "", nil, 0, err
		}
		if frag != "" {
			parts = append(parts, frag)
		}
	}
	if len(parts) == 0 {
		parts = append(parts, "true")
	}
	return strings.Join(parts, " AND "), b.args, b.idx, nil
}

type whereBuilder struct {
	schema domain.ResourceSchema
	args   []any
	idx    int
}

func (b *whereBuilder) placeholder(v any) string {
	b.args = append(b.args, v)
	p := fmt.Sprintf("$%d", b.idx)
	b.idx++
	return p
}

func (b *whereBuilder) node(n domain.FilterNode) (string, error) {
	if n.Field != "" {
		return b.leaf(n)
	}
	joiner := " AND "
	if n.Op == domain.LogicOr {
		joiner = " OR "
	}
	parts := make([]string, 0, len(n.Conditions))
	for _, c := range n.Conditions {
		frag, err := b.node(c)
		if err != nil {
			return "", err
		}
		parts = append(parts, frag)
	}
	return "(" + strings.Join(parts, joiner) + ")", nil
}

func (b *whereBuilder) leaf(n domain.FilterNode) (string, error) {
	spec, ok := b.schema.Fields[n.Field]
	if !ok {
		// Should never happen: SearchQuery.Validate ran first. Fail closed.
		return "", fmt.Errorf("field %q is not searchable", n.Field)
	}
	col := spec.Column
	cast := ""
	if spec.Type == domain.FieldTimestamp {
		cast = "::timestamptz"
	}

	switch n.Compare {
	case domain.OpEq:
		return fmt.Sprintf("%s = %s%s", col, b.placeholder(scalar(n.Value)), cast), nil
	case domain.OpNeq:
		return fmt.Sprintf("%s <> %s%s", col, b.placeholder(scalar(n.Value)), cast), nil
	case domain.OpLike:
		return fmt.Sprintf("%s LIKE %s", col, b.placeholder(scalar(n.Value))), nil
	case domain.OpILike:
		return fmt.Sprintf("%s ILIKE %s", col, b.placeholder(scalar(n.Value))), nil
	case domain.OpGt:
		return fmt.Sprintf("%s > %s%s", col, b.placeholder(scalar(n.Value)), cast), nil
	case domain.OpGte:
		return fmt.Sprintf("%s >= %s%s", col, b.placeholder(scalar(n.Value)), cast), nil
	case domain.OpLt:
		return fmt.Sprintf("%s < %s%s", col, b.placeholder(scalar(n.Value)), cast), nil
	case domain.OpLte:
		return fmt.Sprintf("%s <= %s%s", col, b.placeholder(scalar(n.Value)), cast), nil
	case domain.OpIn:
		// col = ANY($n) with a text[] parameter — one placeholder, no matter
		// how many values, and still fully parameterised.
		return fmt.Sprintf("%s = ANY(%s)", col, b.placeholder(scalarList(n.Value))), nil
	default:
		return "", fmt.Errorf("unsupported operator %q", n.Compare)
	}
}

// orderClause builds a safe ORDER BY from the query's sort terms. Every column
// is resolved through the schema allow-list; a stable id tie-breaker is always
// appended so pagination is deterministic.
func orderClause(schema domain.ResourceSchema, q domain.SearchQuery) string {
	terms := make([]string, 0, len(q.Sort)+1)
	for _, s := range q.Sort {
		spec, ok := schema.Fields[s.Field]
		if !ok {
			continue
		}
		dir := "ASC"
		if s.Desc {
			dir = "DESC"
		}
		terms = append(terms, spec.Column+" "+dir)
	}
	if len(terms) == 0 {
		terms = append(terms, "created_at DESC")
	}
	terms = append(terms, "id ASC")
	return strings.Join(terms, ", ")
}

// scalar coerces a JSON value to a string parameter (text/enum/timestamp are
// all string-valued in our schema; numbers/bools are stringified defensively).
func scalar(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		return fmt.Sprint(t)
	}
}

func scalarList(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		out = append(out, scalar(item))
	}
	return out
}
