package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/sr-alasgarov/wallet-gateway/internal/app"
	"github.com/sr-alasgarov/wallet-gateway/internal/apperr"
	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
)

// searchRequest is the JSON body of POST /{resource}/search.
//
//	{
//	  "filter": { "op": "and", "conditions": [
//	      { "field": "first_name", "op": "eq",  "value": "Elam" },
//	      { "op": "or", "conditions": [
//	          { "field": "last_name", "op": "ilike", "value": "swn%" },
//	          { "field": "status",    "op": "in",    "value": ["active","pending"] }
//	      ]}
//	  ]},
//	  "sort": [{ "field": "created_at", "dir": "desc" }],
//	  "page": 1, "page_size": 20
//	}
type searchRequest struct {
	Filter   *searchNode `json:"filter"`
	Sort     []sortTerm  `json:"sort"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}

// searchNode is one node of the condition tree. A node with `field` set is a
// leaf (its `op` is a comparison operator); otherwise it is a group (its `op`
// is "and"/"or" and it has `conditions`).
type searchNode struct {
	Op         string          `json:"op"`
	Conditions []searchNode    `json:"conditions,omitempty"`
	Field      string          `json:"field,omitempty"`
	Value      json.RawMessage `json:"value,omitempty"`
}

type sortTerm struct {
	Field string `json:"field"`
	Dir   string `json:"dir"`
}

// toCommand converts the request DTO into a transport-neutral SearchCommand.
// It only shapes data; all validation (fields, operators, permissions) happens
// in the application/domain layers.
func (req searchRequest) toCommand() (app.SearchCommand, error) {
	cmd := app.SearchCommand{Page: req.Page, PageSize: req.PageSize}
	if req.Filter != nil {
		node, err := req.Filter.toDomain(0)
		if err != nil {
			return app.SearchCommand{}, err
		}
		cmd.Filter = &node
	}
	for _, s := range req.Sort {
		cmd.Sort = append(cmd.Sort, domain.SortField{Field: s.Field, Desc: s.Dir == "desc"})
	}
	return cmd, nil
}

func (n searchNode) toDomain(depth int) (domain.FilterNode, error) {
	// Guard here too so a hostile body cannot recurse without bound before the
	// domain validator sees it.
	if depth > domain.MaxFilterDepth+1 {
		return domain.FilterNode{}, apperr.BadRequest("filter is nested too deeply")
	}
	if n.Field != "" {
		var value any
		if len(n.Value) > 0 {
			if err := json.Unmarshal(n.Value, &value); err != nil {
				return domain.FilterNode{}, apperr.BadRequest("invalid value for field " + n.Field)
			}
		}
		return domain.FilterNode{Field: n.Field, Compare: domain.CompareOp(n.Op), Value: value}, nil
	}
	node := domain.FilterNode{Op: domain.LogicOp(n.Op)}
	for _, c := range n.Conditions {
		child, err := c.toDomain(depth + 1)
		if err != nil {
			return domain.FilterNode{}, err
		}
		node.Conditions = append(node.Conditions, child)
	}
	return node, nil
}

func decodeSearch(r *http.Request) (app.SearchCommand, error) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return app.SearchCommand{}, apperr.BadRequest("cannot read request body")
	}
	var req searchRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			return app.SearchCommand{}, apperr.BadRequest("request body must be a valid search object")
		}
	}
	return req.toCommand()
}

func pageOf(cmd app.SearchCommand) (int, int) {
	page, size := cmd.Page, cmd.PageSize
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size
}

// SearchUsers — POST /api/v1/users/search
func (h *ResourceHandlers) SearchUsers(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	cmd, err := decodeSearch(r)
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	views, total, err := h.Users.Search(r.Context(), p, r.URL.Query().Get("tenant_id"), cmd)
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	out := make([]map[string]any, len(views))
	for i, v := range views {
		out[i] = userDTO(v.User, v.AllowedFields, v.User.ID == p.ID)
	}
	page, size := pageOf(cmd)
	respondList(w, r, out, page, size, total)
}

// SearchTenants — POST /api/v1/tenants/search
func (h *ResourceHandlers) SearchTenants(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	cmd, err := decodeSearch(r)
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	tenants, total, err := h.Tenants.Search(r.Context(), p, cmd)
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	out := make([]tenantDTO, len(tenants))
	for i, t := range tenants {
		out[i] = toTenantDTO(t)
	}
	page, size := pageOf(cmd)
	respondList(w, r, out, page, size, total)
}

// SearchAPIClients — POST /api/v1/api-clients/search
func (h *ResourceHandlers) SearchAPIClients(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	cmd, err := decodeSearch(r)
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	clients, total, err := h.APIClients.Search(r.Context(), p, r.URL.Query().Get("tenant_id"), cmd)
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	out := make([]apiClientDTO, len(clients))
	for i, c := range clients {
		out[i] = toAPIClientDTO(c)
	}
	page, size := pageOf(cmd)
	respondList(w, r, out, page, size, total)
}
