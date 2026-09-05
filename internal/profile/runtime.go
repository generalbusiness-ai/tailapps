package profile

import (
	"fmt"
	"io/fs"

	"github.com/generalbusiness-ai/tailapps/jsonataddl"
)

// The composed runtime identity is the current runtime of this binary: the
// core's own components, the Tailapp dialect (bound at full strength through
// its canonical digest), and the host components Tailapp alone can name.
// It replaces the repository-global RuntimeID string for new revisions and
// projections at migration stage 5; RuntimeID remains as the legacy
// identity whose projections stay recognizable and queryable through the
// retained legacy resolver until the explicit continue-or-reset upgrade.
//
// Component values are load-bearing: changing any value (or the dialect)
// changes CurrentRuntimeID, which reseeds revision digests and makes every
// active projection upgrade-pending. That is deliberate - implementation
// identity participates in deterministic interpretation - but it must
// always be a conscious, reviewed change; the pinned descriptor test exists
// so no component moves silently.
func hostComponents() []jsonataddl.Component {
	return []jsonataddl.Component{
		// The OTLP-to-canonical-record conversion and envelope meaning
		// (docs/reference/otel-records.md), carried forward from the legacy
		// runtime string's otlp-1.8/json-v1 pins. The parser uses
		// go.opentelemetry.io/proto/otlp v1.11.0; its profiling-only
		// string-table references are explicitly rejected at ingestion.
		{Key: "host.canonicalization", Value: "otlp-1.8-json-v1"},
		// One delivery, one transaction: normalizer, its writes and
		// emissions, folds over each emission, then frontier and stats.
		{Key: "host.orchestration", Value: "two-stage-txn/3"},
		// Externally observable projection and query value conversion.
		{Key: "host.projection", Value: "query-values/1"},
	}
}

var composedRuntime = mustComposeRuntime()

func mustComposeRuntime() jsonataddl.RuntimeIdentity {
	components := append(jsonataddl.CoreComponents(), jsonataddl.DialectComponent(jsonataddl.Tailapp()))
	components = append(components, hostComponents()...)
	identity, err := jsonataddl.ComposeIdentity(components...)
	if err != nil {
		// The component set is static; a failure here is a programming
		// error that must stop the binary before it writes any identity.
		panic(fmt.Sprintf("compose runtime identity: %v", err))
	}
	return identity
}

// ComposedRuntime is the current composed runtime identity.
func ComposedRuntime() jsonataddl.RuntimeIdentity { return composedRuntime }

// CurrentRuntimeID is the runtime identity string recorded with new
// revisions and projections: the composed identity's canonical digest.
func CurrentRuntimeID() string { return composedRuntime.Digest() }

// LoadCurrent is the live compilation path since migration stage 5: the
// extracted core under the Tailapp dialect and the composed runtime
// identity. Load remains the legacy resolver for projections recorded
// under RuntimeID.
func LoadCurrent(files fs.FS, root, name string) (*Profile, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	application, err := jsonataddl.LoadApplication(files, root, name, jsonataddl.Tailapp(), CurrentRuntimeID())
	if err != nil {
		return nil, err
	}
	return profileFromCore(application), nil
}
