package openfga

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sr-alasgarov/wallet-gateway/internal/ports"
)

// OpenFGA applies a write batch atomically. Seeding a second platform admin
// sends a batch where one tuple (the shared Super Admin role edge) already
// exists, so the whole batch is rejected. The adapter used to report that as
// success, leaving the new admin with no relationship tuples at all — every
// authorization check then denied, while the database said the account held
// every permission.

type recordingFGA struct {
	// Tuples the server considers already present.
	existing map[string]bool
	// Every batch the client sent, as user|relation|object joined per tuple.
	batches [][]string
}

func (s *recordingFGA) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Writes *struct {
				TupleKeys []tupleKey `json:"tuple_keys"`
			} `json:"writes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var batch []string
		conflict := false
		if req.Writes != nil {
			for _, t := range req.Writes.TupleKeys {
				key := strings.Join([]string{t.User, t.Relation, t.Object}, "|")
				batch = append(batch, key)
				if s.existing[key] {
					conflict = true
				}
			}
		}
		s.batches = append(s.batches, batch)

		if conflict {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"code":    "write_failed_due_to_invalid_input",
				"message": "cannot write a tuple which already exists",
			})
			return
		}
		for _, k := range batch {
			s.existing[k] = true
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}
}

func TestWriteAppliesNewTuplesWhenOneAlreadyExists(t *testing.T) {
	shared := "platform:platform|platform|role:super"
	srv := &recordingFGA{existing: map[string]bool{shared: true}}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	c := NewClient(ts.URL, "store", "model")

	tuples := []ports.RelationTuple{
		{User: "user:new-admin", Relation: "member", Object: "platform:platform"},
		{User: "user:new-admin", Relation: "super_admin", Object: "platform:platform"},
		{User: "platform:platform", Relation: "platform", Object: "role:super"}, // already exists
		{User: "user:new-admin", Relation: "assignee", Object: "role:super"},
	}

	if err := c.Write(context.Background(), tuples, nil); err != nil {
		t.Fatalf("Write returned %v, want nil", err)
	}

	// The three genuinely new tuples must have landed. Before the fix the
	// batch was rejected wholesale and none of them existed.
	for _, want := range []string{
		"user:new-admin|member|platform:platform",
		"user:new-admin|super_admin|platform:platform",
		"user:new-admin|assignee|role:super",
	} {
		if !srv.existing[want] {
			t.Errorf("tuple %q was never written", want)
		}
	}

	// First a single batch, then one call per tuple as the fallback.
	if len(srv.batches) != 1+len(tuples) {
		t.Errorf("got %d requests, want %d (1 batch + %d singles)",
			len(srv.batches), 1+len(tuples), len(tuples))
	}
}

func TestWriteIsIdempotentForASingleExistingTuple(t *testing.T) {
	key := "user:a|member|platform:platform"
	srv := &recordingFGA{existing: map[string]bool{key: true}}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	c := NewClient(ts.URL, "store", "model")
	err := c.Write(context.Background(), []ports.RelationTuple{
		{User: "user:a", Relation: "member", Object: "platform:platform"},
	}, nil)
	if err != nil {
		t.Fatalf("re-writing an existing tuple should be a no-op, got %v", err)
	}
	if len(srv.batches) != 1 {
		t.Errorf("expected no retry for a single-tuple batch, got %d requests", len(srv.batches))
	}
}

func TestWriteSurfacesRealErrors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":"internal_error","message":"boom"}`))
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "store", "model")
	err := c.Write(context.Background(), []ports.RelationTuple{
		{User: "user:a", Relation: "member", Object: "platform:platform"},
	}, nil)
	if err == nil {
		t.Fatal("a 500 from OpenFGA must not be swallowed")
	}
}
