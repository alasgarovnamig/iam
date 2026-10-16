package domain

import "testing"

// leaf/group builders keep the table terse.
func leaf(field string, op CompareOp, value any) FilterNode {
	return FilterNode{Field: field, Compare: op, Value: value}
}
func group(op LogicOp, conds ...FilterNode) FilterNode {
	return FilterNode{Op: op, Conditions: conds}
}

func TestSearchQueryValidate(t *testing.T) {
	tests := []struct {
		name    string
		filter  FilterNode
		wantErr string // substring; empty = expect success
	}{
		{
			name:   "simple equality on a text field",
			filter: leaf("first_name", OpEq, "Elam"),
		},
		{
			name: "nested and/or of allowed fields",
			filter: group(LogicAnd,
				leaf("first_name", OpEq, "Elam"),
				group(LogicOr,
					leaf("last_name", OpILike, "swn%"),
					leaf("status", OpIn, []any{"active", "pending"}),
				),
			),
		},
		{
			name:    "unknown field is rejected",
			filter:  leaf("father_name", OpEq, "x"),
			wantErr: "not searchable",
		},
		{
			name:    "operator not allowed for the field type",
			filter:  leaf("status", OpILike, "act%"), // ilike on an enum
			wantErr: "not allowed on field",
		},
		{
			name:    "in needs a list",
			filter:  leaf("status", OpIn, "active"),
			wantErr: "needs a non-empty list",
		},
		{
			name:    "scalar operator rejects a list",
			filter:  leaf("first_name", OpEq, []any{"a", "b"}),
			wantErr: "expects a single value",
		},
		{
			name:    "enum value must be in the allowed set",
			filter:  leaf("status", OpEq, "banana"),
			wantErr: "must be one of",
		},
		{
			name:    "enum in-list validates every element",
			filter:  leaf("status", OpIn, []any{"active", "banana"}),
			wantErr: "must be one of",
		},
		{
			name:    "empty group is rejected",
			filter:  group(LogicAnd),
			wantErr: "at least one condition",
		},
		{
			name:   "timestamp accepts range operators",
			filter: leaf("created_at", OpGt, "2026-01-01T00:00:00Z"),
		},
		{
			name:    "timestamp rejects like",
			filter:  leaf("created_at", OpLike, "2026%"),
			wantErr: "not allowed on field",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := SearchQuery{Filter: &tc.filter}
			err := q.Validate(UsersSearchSchema)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error containing %q", tc.wantErr)
			}
			if !contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestFilterNodeFieldsCollectsEveryReferencedField(t *testing.T) {
	f := group(LogicAnd,
		leaf("first_name", OpEq, "Elam"),
		group(LogicOr,
			leaf("last_name", OpILike, "swn%"),
			leaf("status", OpIn, []any{"active"}),
			leaf("first_name", OpNeq, "X"), // duplicate should collapse
		),
	)
	got := f.Fields()
	want := []string{"first_name", "last_name", "status"}
	if len(got) != len(want) {
		t.Fatalf("Fields() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Fields() = %v, want %v", got, want)
		}
	}
}

func TestSearchDepthLimit(t *testing.T) {
	// Build a chain deeper than MaxFilterDepth.
	node := leaf("first_name", OpEq, "x")
	for i := 0; i < MaxFilterDepth+2; i++ {
		node = group(LogicAnd, node)
	}
	q := SearchQuery{Filter: &node}
	if err := q.Validate(UsersSearchSchema); err == nil {
		t.Fatal("Validate() accepted a filter nested past MaxFilterDepth")
	}
}

func TestSchemasCoverTheirStatusEnums(t *testing.T) {
	// A quick guard that each schema's status field carries an enum, so a bad
	// status value is rejected before it reaches SQL.
	for _, sc := range []ResourceSchema{UsersSearchSchema, TenantsSearchSchema, APIClientsSearchSchema} {
		spec, ok := sc.Fields["status"]
		if !ok {
			t.Errorf("%s has no status field", sc.Resource)
			continue
		}
		if spec.Type != FieldEnum || len(spec.Enum) == 0 {
			t.Errorf("%s.status must be an enum with values", sc.Resource)
		}
	}
}
