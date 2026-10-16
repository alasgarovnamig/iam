// Package openfga is a thin HTTP adapter for the OpenFGA API implementing
// the RelationshipStore port. Kept SDK-free intentionally: the port surface
// is small and the domain stays vendor-neutral.
package openfga

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/sr-alasgarov/wallet-gateway/internal/ports"
)

type Client struct {
	baseURL string
	storeID string
	modelID string
	http    *http.Client
}

func NewClient(baseURL, storeID, modelID string) *Client {
	return &Client{
		baseURL: baseURL,
		storeID: storeID,
		modelID: modelID,
		http:    &http.Client{Timeout: 3 * time.Second},
	}
}

type checkRequest struct {
	TupleKey             tupleKey `json:"tuple_key"`
	AuthorizationModelID string   `json:"authorization_model_id,omitempty"`
}

type tupleKey struct {
	User     string `json:"user"`
	Relation string `json:"relation"`
	Object   string `json:"object"`
}

type checkResponse struct {
	Allowed bool `json:"allowed"`
}

func (c *Client) Check(ctx context.Context, user, relation, object string) (bool, error) {
	body, err := json.Marshal(checkRequest{
		TupleKey:             tupleKey{User: user, Relation: relation, Object: object},
		AuthorizationModelID: c.modelID,
	})
	if err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/stores/%s/check", c.baseURL, c.storeID), bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("openfga check: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("openfga check: status %d", resp.StatusCode)
	}
	var out checkResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false, err
	}
	return out.Allowed, nil
}

type writeRequest struct {
	Writes               *writeTuples `json:"writes,omitempty"`
	Deletes              *writeTuples `json:"deletes,omitempty"`
	AuthorizationModelID string       `json:"authorization_model_id,omitempty"`
}

type writeTuples struct {
	TupleKeys []tupleKey `json:"tuple_keys"`
}

// Write applies relationship changes.
//
// OpenFGA applies a write batch atomically, so one already-existing tuple
// rejects the whole batch. Treating that rejection as success would silently
// drop the tuples that were new — which fails closed in the worst way: the
// caller believes the relationships exist, and every later authorization check
// denies. So on a duplicate rejection we retry tuple by tuple, and only then is
// "already exists" genuinely a no-op.
func (c *Client) Write(ctx context.Context, writes []ports.RelationTuple, deletes []ports.RelationTuple) error {
	err := c.writeBatch(ctx, writes, deletes)
	if !errors.Is(err, errTupleConflict) {
		return err
	}
	if len(writes)+len(deletes) <= 1 {
		// A single-tuple batch that conflicts really is already applied.
		return nil
	}
	for _, t := range writes {
		if e := c.writeBatch(ctx, []ports.RelationTuple{t}, nil); e != nil && !errors.Is(e, errTupleConflict) {
			return e
		}
	}
	for _, t := range deletes {
		if e := c.writeBatch(ctx, nil, []ports.RelationTuple{t}); e != nil && !errors.Is(e, errTupleConflict) {
			return e
		}
	}
	return nil
}

// errTupleConflict marks "this tuple is already in the state you asked for".
var errTupleConflict = errors.New("openfga: tuple already applied")

func (c *Client) writeBatch(ctx context.Context, writes []ports.RelationTuple, deletes []ports.RelationTuple) error {
	req := writeRequest{AuthorizationModelID: c.modelID}
	if len(writes) > 0 {
		wt := &writeTuples{}
		for _, t := range writes {
			wt.TupleKeys = append(wt.TupleKeys, tupleKey{User: t.User, Relation: t.Relation, Object: t.Object})
		}
		req.Writes = wt
	}
	if len(deletes) > 0 {
		dt := &writeTuples{}
		for _, t := range deletes {
			dt.TupleKeys = append(dt.TupleKeys, tupleKey{User: t.User, Relation: t.Relation, Object: t.Object})
		}
		req.Deletes = dt
	}
	if req.Writes == nil && req.Deletes == nil {
		return nil
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/stores/%s/write", c.baseURL, c.storeID), bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("openfga write: %w", err)
	}
	defer resp.Body.Close()
	// A duplicate write (or a delete of something already gone) surfaces as a
	// 400 with this code. Report it as errTupleConflict rather than success —
	// Write decides what it means, because for a multi-tuple batch it means
	// nothing was applied at all.
	if resp.StatusCode >= 400 {
		var e struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		if e.Code == "write_failed_due_to_invalid_input" {
			return fmt.Errorf("%w: %s", errTupleConflict, e.Message)
		}
		return fmt.Errorf("openfga write: status %d code %s: %s", resp.StatusCode, e.Code, e.Message)
	}
	return nil
}

func (c *Client) Healthy(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("openfga unhealthy: status %d", resp.StatusCode)
	}
	return nil
}
