package profile

import (
	"os"
	"strings"
	"testing"
)

// The pinned descriptor is the composed-runtime fixture: every semantic
// component of the current runtime, exactly. Changing any core, dialect,
// or host component - including any Tailapp dialect field, through the
// dialect digest - changes this string, reseeds revision digests, and
// makes every active projection upgrade-pending. That must always be a
// conscious, reviewed diff of this constant, never a drive-by.
const pinnedRuntimeDescriptor = "core.grammar=ddl/2; " +
	"core.interface=jsonata-ddl-application-interface/2026-09-05; " +
	"core.jsonata=jsonata-go-v206/bounded-2; " +
	"core.sqlite=sqlite-3.53.4/read-authorizer-1; " +
	"core.value-codec=logical-values/2; " +
	"dialect=tailapp-otlp/2+sha256:59524300ac2d079e8abccaafc6c161294f853c560f2ced69105ff43f8eabc464; " +
	"host.canonicalization=otlp-1.8-json-v1; " +
	"host.orchestration=two-stage-txn/3; " +
	"host.projection=query-values/1"

const pinnedRuntimeDigest = "jsonata-ddl-runtime:sha256:3e60ce38390fa376d766b5880cb8f79ebd9196f0e4036335beb9285783efbd33"

// Keep the historical identity available for recognizing old projections.
// The current corpus has its own new identity for the input-contract semantics.
const pinnedLegacyRuntimeID = "tailapp-otlp-1.8-json-v1-ddl-jsonata-v206-sqlite-3.53.4@5"

func TestComposedRuntimeIsPinned(t *testing.T) {
	if RuntimeID != pinnedLegacyRuntimeID {
		t.Fatalf("historical RuntimeID changed:\n got %s\nwant %s", RuntimeID, pinnedLegacyRuntimeID)
	}
	if descriptor := ComposedRuntime().Descriptor(); descriptor != pinnedRuntimeDescriptor {
		t.Fatalf("composed runtime descriptor changed - this is a deliberate runtime version change that must be reviewed:\n got %s\nwant %s", descriptor, pinnedRuntimeDescriptor)
	}
	if digest := CurrentRuntimeID(); digest != pinnedRuntimeDigest {
		t.Fatalf("composed runtime digest changed:\n got %s\nwant %s", digest, pinnedRuntimeDigest)
	}
	if CurrentRuntimeID() == RuntimeID {
		t.Fatal("the composed runtime must differ from the legacy RuntimeID")
	}
}

// LoadCurrent must produce a different revision than the legacy resolver
// for identical sources - implementation identity participates in
// deterministic interpretation - while all other inspection data agrees.
func TestLoadCurrentChangesOnlyIdentity(t *testing.T) {
	appDir := "../../jsonataddl/corpus/v1/basic/app"
	legacy, err := Load(os.DirFS(appDir), ".", "corpus-app")
	if err != nil {
		t.Fatal(err)
	}
	current, err := LoadCurrent(os.DirFS(appDir), ".", "corpus-app")
	if err != nil {
		t.Fatal(err)
	}
	if current.RuntimeProfile != pinnedRuntimeDigest {
		t.Fatalf("LoadCurrent runtime = %q", current.RuntimeProfile)
	}
	if current.Revision == legacy.Revision {
		t.Fatal("the composed runtime did not reseed the revision digest")
	}
	if !strings.HasPrefix(current.Revision, "sha256:") {
		t.Fatalf("revision shape changed: %q", current.Revision)
	}
	if current.StorageSchemaDigest != legacy.StorageSchemaDigest || current.ExportContractDigest != legacy.ExportContractDigest {
		t.Fatal("storage and export digests must not depend on the runtime identity")
	}
	result, err := current.Evaluate(legacy.Normalizer.Name, EvaluationInput{
		Meta:  map[string]any{"position": 1, "event_id": "local:1", "event_type": "otlp_record"},
		Event: map[string]any{"id": "local:1", "signal": "logs", "name": "example", "source": "test", "time_unix_nano": nil, "observed_unix_nano": nil, "trace_id": nil, "span_id": nil, "content_digest": "test", "record": map[string]any{"attributes": map[string]any{"key": "alpha", "count": 1, "ratio": nil, "flag": nil, "payload": nil, "extra": nil}}},
		Rows:  map[string]any{},
	})
	if err != nil {
		t.Fatalf("evaluation through the current path: %v", err)
	}
	if result.Decision != "effective" {
		t.Fatalf("decision = %q", result.Decision)
	}
}
