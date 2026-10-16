package domain

import (
	"fmt"
	"sort"
)

// Dynamic search model.
//
// A search request is a tree of conditions: inner nodes combine children with
// AND / OR, leaf nodes compare a field with an operator and a value. The tree
// is framework-free and validated here so both the permission layer (which
// fields are touched?) and the SQL adapter (is every field/operator allowed?)
// work from the same source of truth.
//
// Security note: field names are matched against a per-resource allow-list
// (ResourceSchema) and operators against a per-type allow-list. Nothing from
// the request ever becomes a column name or an operator token by string
// concatenation — the adapter only ever emits parameterised SQL.

// LogicOp joins the children of a group node.
type LogicOp string

const (
	LogicAnd LogicOp = "and"
	LogicOr  LogicOp = "or"
)

// CompareOp is a leaf comparison operator.
type CompareOp string

const (
	OpEq    CompareOp = "eq"
	OpNeq   CompareOp = "neq"
	OpLike  CompareOp = "like"  // case-sensitive substring/pattern
	OpILike CompareOp = "ilike" // case-insensitive
	OpIn    CompareOp = "in"
	OpGt    CompareOp = "gt"
	OpGte   CompareOp = "gte"
	OpLt    CompareOp = "lt"
	OpLte   CompareOp = "lte"
)

// FieldType decides which operators a field accepts.
type FieldType int

const (
	FieldText FieldType = iota
	FieldEnum
	FieldTimestamp
)

// allowedOps lists the comparison operators valid for each field type.
var allowedOps = map[FieldType]map[CompareOp]bool{
	FieldText:      {OpEq: true, OpNeq: true, OpLike: true, OpILike: true, OpIn: true},
	FieldEnum:      {OpEq: true, OpNeq: true, OpIn: true},
	FieldTimestamp: {OpEq: true, OpGt: true, OpGte: true, OpLt: true, OpLte: true},
}

// FieldSpec describes one searchable field.
type FieldSpec struct {
	Column string    // physical column name (trusted, from code)
	Type   FieldType // decides the allowed operators
	Enum   []string  // for FieldEnum: the permitted values
}

// ResourceSchema is the allow-list of searchable fields for one resource.
type ResourceSchema struct {
	Service  string
	Resource string
	Fields   map[string]FieldSpec
}

func (s ResourceSchema) spec(field string) (FieldSpec, bool) {
	fs, ok := s.Fields[field]
	return fs, ok
}

// FilterNode is a group (Op + Conditions) or a leaf (Field + Compare + Value).
type FilterNode struct {
	// Group node:
	Op         LogicOp
	Conditions []FilterNode
	// Leaf node:
	Field   string
	Compare CompareOp
	Value   any
}

func (n FilterNode) isLeaf() bool { return n.Field != "" }

// MaxFilterDepth caps nesting so a malicious payload cannot explode the query.
const MaxFilterDepth = 6

// SortField is one ordering term.
type SortField struct {
	Field string
	Desc  bool
}

// SearchQuery is a validated search request.
type SearchQuery struct {
	Filter   *FilterNode // nil means "match all in scope"
	Sort     []SortField
	Page     int
	PageSize int
}

// Offset is the SQL offset for the current page.
func (q SearchQuery) Offset() int {
	if q.Page < 1 {
		return 0
	}
	return (q.Page - 1) * q.PageSize
}

// Fields returns every distinct field referenced anywhere in the tree, sorted.
// The application uses this to demand search.<field> permission for each one.
func (n *FilterNode) Fields() []string {
	set := map[string]struct{}{}
	var walk func(node FilterNode)
	walk = func(node FilterNode) {
		if node.isLeaf() {
			set[node.Field] = struct{}{}
			return
		}
		for _, c := range node.Conditions {
			walk(c)
		}
	}
	if n != nil {
		walk(*n)
	}
	out := make([]string, 0, len(set))
	for f := range set {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// Validate checks the tree structure, depth, fields, operators and value
// shapes against the schema. It returns a message safe to show the caller.
func (q *SearchQuery) Validate(schema ResourceSchema) error {
	if q.Filter != nil {
		if err := validateNode(*q.Filter, schema, 1); err != nil {
			return err
		}
	}
	for _, s := range q.Sort {
		if _, ok := schema.spec(s.Field); !ok && s.Field != "created_at" {
			return fmt.Errorf("cannot sort by unknown field %q", s.Field)
		}
	}
	return nil
}

func validateNode(n FilterNode, schema ResourceSchema, depth int) error {
	if depth > MaxFilterDepth {
		return fmt.Errorf("filter is nested too deeply (max %d levels)", MaxFilterDepth)
	}
	if n.isLeaf() {
		return validateLeaf(n, schema)
	}
	// Group node.
	if n.Op != LogicAnd && n.Op != LogicOr {
		return fmt.Errorf("group operator must be \"and\" or \"or\"")
	}
	if len(n.Conditions) == 0 {
		return fmt.Errorf("a %q group must have at least one condition", n.Op)
	}
	for _, c := range n.Conditions {
		if err := validateNode(c, schema, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func validateLeaf(n FilterNode, schema ResourceSchema) error {
	spec, ok := schema.spec(n.Field)
	if !ok {
		return fmt.Errorf("field %q is not searchable on %s", n.Field, schema.Resource)
	}
	if !allowedOps[spec.Type][n.Compare] {
		return fmt.Errorf("operator %q is not allowed on field %q", n.Compare, n.Field)
	}
	if n.Compare == OpIn {
		vals, ok := n.Value.([]any)
		if !ok || len(vals) == 0 {
			return fmt.Errorf("operator \"in\" on %q needs a non-empty list value", n.Field)
		}
		if spec.Type == FieldEnum {
			for _, v := range vals {
				if err := checkEnum(spec, n.Field, v); err != nil {
					return err
				}
			}
		}
		return nil
	}
	// Scalar operators reject lists.
	if _, isList := n.Value.([]any); isList {
		return fmt.Errorf("operator %q on %q expects a single value, not a list", n.Compare, n.Field)
	}
	if spec.Type == FieldEnum {
		return checkEnum(spec, n.Field, n.Value)
	}
	return nil
}

func checkEnum(spec FieldSpec, field string, v any) error {
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("field %q expects one of %v", field, spec.Enum)
	}
	for _, allowed := range spec.Enum {
		if s == allowed {
			return nil
		}
	}
	return fmt.Errorf("field %q value %q must be one of %v", field, s, spec.Enum)
}

// ─── Per-resource searchable schemas (single source of truth) ───────────────

var userStatuses = []string{"pending", "active", "suspended", "locked", "disabled"}
var tenantStatuses = []string{"pending", "active", "suspended", "terminated"}
var apiClientStatuses = []string{"active", "suspended", "expired", "revoked"}

// UsersSearchSchema — searchable fields on the users resource.
var UsersSearchSchema = ResourceSchema{
	Service:  "user-management",
	Resource: "users",
	Fields: map[string]FieldSpec{
		"email":      {Column: "email", Type: FieldText},
		"first_name": {Column: "first_name", Type: FieldText},
		"last_name":  {Column: "last_name", Type: FieldText},
		"phone":      {Column: "phone", Type: FieldText},
		"status":     {Column: "status", Type: FieldEnum, Enum: userStatuses},
		"created_at": {Column: "created_at", Type: FieldTimestamp},
	},
}

// TenantsSearchSchema — searchable fields on the tenants resource.
var TenantsSearchSchema = ResourceSchema{
	Service:  "user-management",
	Resource: "tenants",
	Fields: map[string]FieldSpec{
		"name":       {Column: "name", Type: FieldText},
		"slug":       {Column: "slug", Type: FieldText},
		"status":     {Column: "status", Type: FieldEnum, Enum: tenantStatuses},
		"created_at": {Column: "created_at", Type: FieldTimestamp},
	},
}

// APIClientsSearchSchema — searchable fields on the api-clients resource.
var APIClientsSearchSchema = ResourceSchema{
	Service:  "user-management",
	Resource: "api-clients",
	Fields: map[string]FieldSpec{
		"name":       {Column: "name", Type: FieldText},
		"client_id":  {Column: "client_id", Type: FieldText},
		"status":     {Column: "status", Type: FieldEnum, Enum: apiClientStatuses},
		"created_at": {Column: "created_at", Type: FieldTimestamp},
	},
}
