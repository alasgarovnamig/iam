package app

import (
	"github.com/sr-alasgarov/wallet-gateway/internal/apperr"
	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
)

// SearchCommand is the transport-neutral shape the HTTP layer maps a search
// request into. Filter is the raw condition tree; the application validates it
// against the resource schema before it ever reaches SQL.
type SearchCommand struct {
	Filter   *domain.FilterNode
	Sort     []domain.SortField
	Page     int
	PageSize int
}

func (c SearchCommand) toQuery() domain.SearchQuery {
	page, size := c.Page, c.PageSize
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return domain.SearchQuery{Filter: c.Filter, Sort: c.Sort, Page: page, PageSize: size}
}

// authorizeSearch performs the two-stage check the whole feature is built on:
//
//  1. May the actor call search on this resource at all?  (action-level)
//  2. For EVERY field referenced anywhere in the filter or sort, does the
//     actor hold search.<field>?                          (field-level)
//
// A missing field permission is reported by name, so the caller learns exactly
// which field it may not search by. It also validates the tree against the
// resource schema (unknown field / illegal operator / bad value shape).
//
// Returns the validated query the repository can safely run.
func authorizeSearch(dec *Decision, schema domain.ResourceSchema, cmd SearchCommand) (domain.SearchQuery, error) {
	q := cmd.toQuery()
	if err := q.Validate(schema); err != nil {
		return domain.SearchQuery{}, apperr.BadRequest(err.Error())
	}

	// Stage 1: the action-level search permission.
	if !dec.PermSet.Allows(ServiceName, schema.Resource, domain.ActionSearch, dec.Relation) {
		return domain.SearchQuery{}, apperr.PermissionDenied("You do not have search permission on " + schema.Resource + ".")
	}

	// Stage 2: a search.<field> grant for every field the query touches.
	touched := map[string]struct{}{}
	if q.Filter != nil {
		for _, f := range q.Filter.Fields() {
			touched[f] = struct{}{}
		}
	}
	for _, srt := range cmd.Sort {
		touched[srt.Field] = struct{}{}
	}
	for field := range touched {
		// created_at is a base field usable for ordering without a grant.
		if field == "created_at" {
			continue
		}
		if !dec.PermSet.AllowsField(ServiceName, schema.Resource, domain.ActionSearch, field, dec.Relation) {
			return domain.SearchQuery{}, apperr.FieldNotAllowed("search:" + field)
		}
	}
	return q, nil
}
