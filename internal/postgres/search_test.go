package postgres

import (
	"strings"
	"testing"

	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
)

// These tests exercise the security-critical WHERE builder WITHOUT a database:
// they assert that a dynamic filter tree becomes correct, fully-parameterised
// SQL — no field name or value is ever concatenated into the statement.

func TestBuildWhereParameterisesEverything(t *testing.T) {
	// (first_name = 'Elam') AND ( last_name ILIKE 'swn%' OR status IN [active,pending] )
	filter := domain.FilterNode{
		Op: domain.LogicAnd,
		Conditions: []domain.FilterNode{
			{Field: "first_name", Compare: domain.OpEq, Value: "Elam"},
			{Op: domain.LogicOr, Conditions: []domain.FilterNode{
				{Field: "last_name", Compare: domain.OpILike, Value: "swn%"},
				{Field: "status", Compare: domain.OpIn, Value: []any{"active", "pending"}},
			}},
		},
	}
	q := domain.SearchQuery{Filter: &filter, Page: 1, PageSize: 20}
	base := []string{"deleted_at IS NULL", "tenant_id = $1"}
	baseArgs := []any{"tenant-1"}

	where, args, next, err := buildWhere(domain.UsersSearchSchema, q, base, baseArgs, len(baseArgs)+1)
	if err != nil {
		t.Fatalf("buildWhere: %v", err)
	}

	// The base predicates survive and the tree is appended with correct logic.
	wantFragments := []string{
		"deleted_at IS NULL",
		"tenant_id = $1",
		"first_name = $2",
		"last_name ILIKE $3",
		"status = ANY($4)",
		" OR ",
		" AND ",
	}
	for _, f := range wantFragments {
		if !strings.Contains(where, f) {
			t.Errorf("WHERE %q is missing fragment %q", where, f)
		}
	}

	// Every value is a bound parameter, in order, with no literal leaking in.
	if len(args) != 4 {
		t.Fatalf("args = %v, want 4 (tenant, Elam, swn%%, [active pending])", args)
	}
	if args[0] != "tenant-1" || args[1] != "Elam" || args[2] != "swn%" {
		t.Errorf("scalar args = %v", args[:3])
	}
	list, ok := args[3].([]string)
	if !ok || len(list) != 2 || list[0] != "active" || list[1] != "pending" {
		t.Errorf("IN arg = %v, want []string{active pending}", args[3])
	}
	if next != 5 {
		t.Errorf("next placeholder index = %d, want 5", next)
	}

	// A value that looks like an injection attempt is data, never SQL.
	if strings.Contains(where, "Elam") || strings.Contains(where, "swn") || strings.Contains(where, "active") {
		t.Errorf("a value leaked into the SQL text: %q", where)
	}
}

func TestBuildWhereInjectionValueStaysData(t *testing.T) {
	filter := domain.FilterNode{Field: "first_name", Compare: domain.OpEq, Value: "x'; DROP TABLE users; --"}
	q := domain.SearchQuery{Filter: &filter}
	where, args, _, err := buildWhere(domain.UsersSearchSchema, q, []string{"deleted_at IS NULL"}, nil, 1)
	if err != nil {
		t.Fatalf("buildWhere: %v", err)
	}
	if strings.Contains(where, "DROP TABLE") {
		t.Fatalf("injection payload reached the SQL text: %q", where)
	}
	if where != "deleted_at IS NULL AND first_name = $1" {
		t.Errorf("WHERE = %q", where)
	}
	if len(args) != 1 || args[0] != "x'; DROP TABLE users; --" {
		t.Errorf("payload must be a bound parameter, got %v", args)
	}
}

func TestBuildWhereTimestampGetsCast(t *testing.T) {
	filter := domain.FilterNode{Field: "created_at", Compare: domain.OpGt, Value: "2026-01-01T00:00:00Z"}
	q := domain.SearchQuery{Filter: &filter}
	where, _, _, err := buildWhere(domain.UsersSearchSchema, q, nil, nil, 1)
	if err != nil {
		t.Fatalf("buildWhere: %v", err)
	}
	if !strings.Contains(where, "created_at > $1::timestamptz") {
		t.Errorf("timestamp comparison should cast the parameter: %q", where)
	}
}

func TestOrderClauseUsesAllowListAndStableTieBreaker(t *testing.T) {
	q := domain.SearchQuery{Sort: []domain.SortField{
		{Field: "last_name", Desc: false},
		{Field: "not_a_column", Desc: true}, // ignored: not in the schema
	}}
	got := orderClause(domain.UsersSearchSchema, q)
	if got != "last_name ASC, id ASC" {
		t.Errorf("orderClause = %q, want %q", got, "last_name ASC, id ASC")
	}
	// Unknown sort field must not appear as a column.
	if strings.Contains(got, "not_a_column") {
		t.Errorf("unknown sort field leaked into ORDER BY: %q", got)
	}
	// Default when no valid sort is given.
	if def := orderClause(domain.UsersSearchSchema, domain.SearchQuery{}); def != "created_at DESC, id ASC" {
		t.Errorf("default order = %q", def)
	}
}
