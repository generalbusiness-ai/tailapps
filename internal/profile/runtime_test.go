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
const pinnedRuntimeDescriptor = "core.grammar=ddl/1; " +
	"core.interface=jsonata-ddl-application-interface/2026-08-26; " +
	"core.jsonata=jsonata-go-v206/bounded-2; " +
	"core.sqlite=sqlite-3.53.4/read-authorizer-1; " +
	"core.value-codec=logical-values/1; " +
	"dialect=tailapp-otlp/1+sha256:e5607de7863520bb9859bacdddf6a537a9fb2b7db3e1e0389f1dbe9c0c5243ce; " +
	"host.canonicalization=otlp-1.8-json-v1; " +
	"host.orchestration=two-stage-txn/2; " +
	"host.projection=query-values/1"

const pinnedRuntimeDigest = "jsonata-ddl-runtime:sha256:5032bcfe6634db4462fcfe39775e55d1c7e11fcb7663c961f6ebe8860192b8b1"

// The module-owned v1 corpus uses this literal to preserve its independence
// from the Tailapps host package. Pinning the host constant here ensures that
// its legacy resolver and those corpus goldens cannot drift apart silently.
const pinnedLegacyRuntimeID = "tailapp-otlp-1.8-json-v1-ddl-jsonata-v206-sqlite-3.53.4@5"

func TestComposedRuntimeIsPinned(t *testing.T) {
	if RuntimeID != pinnedLegacyRuntimeID {
		t.Fatalf("legacy RuntimeID changed; update it and the module corpus goldens together:\n got %s\nwant %s", RuntimeID, pinnedLegacyRuntimeID)
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
		Meta:  map[string]any{"position": 1},
		Event: map[string]any{"record": map[string]any{"attributes": map[string]any{"key": "alpha", "count": 1, "ratio": nil, "flag": nil, "payload": nil, "extra": nil}}},
		Rows:  map[string]any{},
	})
	if err != nil {
		t.Fatalf("evaluation through the current path: %v", err)
	}
	if result.Decision != "effective" {
		t.Fatalf("decision = %q", result.Decision)
	}
}
