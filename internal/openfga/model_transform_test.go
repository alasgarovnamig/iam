package openfga

import (
	"encoding/json"
	"testing"

	language "github.com/openfga/language/pkg/go/transformer"
)

// The embedded DSL is applied verbatim at start-up, so a syntax error would
// only surface as a boot failure. Catch it here instead.
func TestEmbeddedModelTransforms(t *testing.T) {
	modelJSON, err := language.TransformDSLToJSON(modelDSL)
	if err != nil {
		t.Fatalf("transform embedded model: %v", err)
	}
	var decoded struct {
		SchemaVersion   string `json:"schema_version"`
		TypeDefinitions []struct {
			Type      string         `json:"type"`
			Relations map[string]any `json:"relations"`
		} `json:"type_definitions"`
	}
	if err := json.Unmarshal([]byte(modelJSON), &decoded); err != nil {
		t.Fatalf("decode model: %v", err)
	}
	if decoded.SchemaVersion != "1.1" {
		t.Errorf("schema version = %q, want 1.1", decoded.SchemaVersion)
	}

	types := map[string]map[string]any{}
	for _, td := range decoded.TypeDefinitions {
		types[td.Type] = td.Relations
	}

	// Every object type the service writes tuples against must exist.
	for _, typeName := range []string{"platform", "tenant", "user", "role", "api_client", "resource", "organization_unit"} {
		if _, ok := types[typeName]; !ok {
			t.Errorf("authorization model is missing type %q", typeName)
		}
	}

	// Relations the authorizer and seeder depend on by name.
	for typeName, relation := range map[string]string{
		"platform": "member",
		"tenant":   "member",
		"role":     "assignee",
	} {
		if _, ok := types[typeName][relation]; !ok {
			t.Errorf("type %q is missing relation %q used by the service", typeName, relation)
		}
	}
	if _, ok := types["platform"]["super_admin"]; !ok {
		t.Error(`type "platform" is missing relation "super_admin" written by the bootstrap seeder`)
	}
}
