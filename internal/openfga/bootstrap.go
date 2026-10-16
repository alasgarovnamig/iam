package openfga

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	language "github.com/openfga/language/pkg/go/transformer"
)

// modelDSL is the single source of truth for the relationship model.
//
//go:embed model.fga
var modelDSL string

// StoreName is the OpenFGA store this service owns.
const StoreName = "wallet-gateway"

type store struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type listStoresResponse struct {
	Stores            []store `json:"stores"`
	ContinuationToken string  `json:"continuation_token"`
}

// EnsureStore finds the store named StoreName, creating it when absent.
func EnsureStore(ctx context.Context, baseURL string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	token := ""
	for {
		url := baseURL + "/stores?page_size=100"
		if token != "" {
			url += "&continuation_token=" + token
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return "", err
		}
		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("list stores: %w", err)
		}
		var listed listStoresResponse
		err = json.NewDecoder(resp.Body).Decode(&listed)
		resp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("decode stores: %w", err)
		}
		for _, s := range listed.Stores {
			if s.Name == StoreName {
				return s.ID, nil
			}
		}
		if listed.ContinuationToken == "" {
			break
		}
		token = listed.ContinuationToken
	}

	body, err := json.Marshal(map[string]string{"name": StoreName})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/stores", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("create store: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("create store: status %d", resp.StatusCode)
	}
	var created store
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return "", err
	}
	return created.ID, nil
}

// EnsureModel transforms the embedded DSL and writes it as the store's
// authorization model, returning the model id. Writing the same model twice
// is harmless (OpenFGA versions models), but we reuse the latest model when
// it already matches to avoid churn on every boot.
func EnsureModel(ctx context.Context, baseURL, storeID string) (string, error) {
	modelJSON, err := language.TransformDSLToJSON(modelDSL)
	if err != nil {
		return "", fmt.Errorf("transform authorization model: %w", err)
	}
	payload := []byte(modelJSON)

	client := &http.Client{Timeout: 10 * time.Second}
	if id, ok := latestMatchingModel(ctx, client, baseURL, storeID, payload); ok {
		return id, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/stores/%s/authorization-models", baseURL, storeID), bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("write authorization model: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return "", fmt.Errorf("write authorization model: status %d: %v", resp.StatusCode, e)
	}
	var out struct {
		AuthorizationModelID string `json:"authorization_model_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.AuthorizationModelID, nil
}

// latestMatchingModel reports the newest model id when it is semantically the
// same as the embedded model, so restarts do not pile up identical versions.
//
// A byte comparison does not work: OpenFGA stores a normalized model (empty
// `relations` maps, explicit `metadata: null`, an `id`). Both sides are
// therefore rendered back to DSL — the canonical form — and compared there.
func latestMatchingModel(ctx context.Context, client *http.Client, baseURL, storeID string, want []byte) (string, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/stores/%s/authorization-models?page_size=1", baseURL, storeID), nil)
	if err != nil {
		return "", false
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	var listed struct {
		AuthorizationModels []json.RawMessage `json:"authorization_models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil || len(listed.AuthorizationModels) == 0 {
		return "", false
	}
	latest := listed.AuthorizationModels[0]
	var meta struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(latest, &meta); err != nil || meta.ID == "" {
		return "", false
	}
	storedDSL, err := canonicalDSL(string(latest))
	if err != nil {
		return "", false
	}
	wantDSL, err := canonicalDSL(string(want))
	if err != nil {
		return "", false
	}
	if storedDSL != wantDSL {
		return "", false
	}
	return meta.ID, true
}

// canonicalDSL renders a JSON authorization model back to its DSL form.
func canonicalDSL(modelJSON string) (string, error) {
	dsl, err := language.TransformJSONStringToDSL(modelJSON)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(*dsl), nil
}

// Bootstrap resolves the store and model ids, honouring explicit overrides.
// Passing both ids skips discovery entirely (deterministic production setup).
func Bootstrap(ctx context.Context, baseURL, storeID, modelID string) (string, string, error) {
	var err error
	if storeID == "" {
		storeID, err = EnsureStore(ctx, baseURL)
		if err != nil {
			return "", "", err
		}
	}
	if modelID == "" {
		modelID, err = EnsureModel(ctx, baseURL, storeID)
		if err != nil {
			return "", "", err
		}
	}
	return storeID, modelID, nil
}

// WaitReady blocks until OpenFGA answers its health endpoint or ctx expires.
func WaitReady(ctx context.Context, baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 3 * time.Second}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/healthz", nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("openfga not ready within %s", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}
