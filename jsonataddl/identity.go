package jsonataddl

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// The composed runtime identity replaces a repository-global runtime string
// with independently versioned components. Every semantic participant in
// deterministic interpretation names itself; changing any component changes
// the digest, and the digest (not merely a core version) enters the
// application revision and every compatible persisted projection.
//
// The component set is closed: exactly these keys, each exactly once. A
// closed set is what makes the canonical form canonical - an unknown or
// missing participant is an error, never silently part of or absent from
// the digest.
var requiredComponents = []string{
	"core.interface",        // interface contract revision
	"core.grammar",          // DDL and fold-read grammar revision
	"core.jsonata",          // JSONata implementation, version, subset, bounds
	"core.value-codec",      // logical value codec revision
	"core.sqlite",           // SQLite adapter, authorizer policy, engine/driver
	"dialect",               // source layout, events, topology, authority, limits
	"host.canonicalization", // input canonicalization and envelope revision
	"host.orchestration",    // orchestration and transaction visibility
	"host.projection",       // externally observable projection/query values
}

// Component is one independently versioned semantic participant.
type Component struct {
	Key   string
	Value string
}

// RuntimeIdentity is a validated, canonically ordered component set.
type RuntimeIdentity struct {
	components map[string]string
}

// ComposeIdentity validates and canonicalizes a component set: every
// required key exactly once with a non-empty value, no unknown keys, and
// no reserved characters in values. Order of the inputs never matters.
func ComposeIdentity(components ...Component) (RuntimeIdentity, error) {
	required := make(map[string]bool, len(requiredComponents))
	for _, key := range requiredComponents {
		required[key] = true
	}
	collected := make(map[string]string, len(components))
	for _, component := range components {
		if !required[component.Key] {
			return RuntimeIdentity{}, fmt.Errorf("unknown runtime identity component %q", component.Key)
		}
		if _, duplicate := collected[component.Key]; duplicate {
			return RuntimeIdentity{}, fmt.Errorf("duplicate runtime identity component %q", component.Key)
		}
		if component.Value == "" {
			return RuntimeIdentity{}, fmt.Errorf("empty runtime identity component %q", component.Key)
		}
		if strings.ContainsAny(component.Value, ";\n") {
			return RuntimeIdentity{}, fmt.Errorf("runtime identity component %q value contains a reserved character", component.Key)
		}
		collected[component.Key] = component.Value
	}
	for _, key := range requiredComponents {
		if _, present := collected[key]; !present {
			return RuntimeIdentity{}, fmt.Errorf("missing runtime identity component %q", key)
		}
	}
	return RuntimeIdentity{components: collected}, nil
}

// Descriptor is the human-readable canonical form: key=value pairs joined
// with "; " in sorted key order.
func (identity RuntimeIdentity) Descriptor() string {
	keys := make([]string, 0, len(identity.components))
	for key := range identity.components {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, key+"="+identity.components[key])
	}
	return strings.Join(pairs, "; ")
}

// Digest is the canonical identity digest: sha256 over the descriptor,
// hex-encoded with a scheme prefix.
func (identity RuntimeIdentity) Digest() string {
	sum := sha256.Sum256([]byte(identity.Descriptor()))
	return "jsonata-ddl-runtime:sha256:" + hex.EncodeToString(sum[:])
}

// Component reports one component's value.
func (identity RuntimeIdentity) Component(key string) (string, bool) {
	value, present := identity.components[key]
	return value, present
}

// CoreComponents are the core's own identity components: the interface
// contract it implements and the revisions of the grammar, JSONata subset,
// value codec, and SQLite policy it ships. The host composes these with its
// dialect and host components; the core never assembles a full identity,
// because orchestration and canonicalization are not its to name.
func CoreComponents() []Component {
	return []Component{
		{Key: "core.interface", Value: "jsonata-ddl-application-interface/2026-08-26"},
		{Key: "core.grammar", Value: "ddl/2"},
		{Key: "core.jsonata", Value: "jsonata-go-v206/bounded-2"},
		{Key: "core.value-codec", Value: "logical-values/2"},
		{Key: "core.sqlite", Value: "sqlite-3.53.4/read-authorizer-1"},
	}
}
