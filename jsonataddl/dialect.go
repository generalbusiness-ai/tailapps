// Package jsonataddl compiles and evaluates deterministic JSONata applications
// whose DDL declares typed events, state reads, and bounded table changes.
// Hosts supply their source layout, event vocabulary, authority policy, limits,
// runtime identity, storage, and transaction orchestration.
package jsonataddl

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Dialect is the complete host policy a compiler receives instead of
// hard-coded host names: where sources live, what the host and private
// events are called and carry, which program topology and read/write/
// emission authority are admitted, and the evaluation limits. Core code
// never tests a literal path or event name; diagnostics may include
// configured names, but no host name is a special case in core code.
//
// Dialect values have value semantics: every constructor returns an
// independent value, so no caller can mutate a shared configuration. The
// semantic configuration is mechanically bound to the composed runtime
// identity through Canonical and DialectComponent - changing any field
// changes the identity digest whether or not anyone remembers to bump the
// version.
type Dialect struct {
	Identity     DialectIdentity
	Layout       SourceLayout
	HostEvent    EventContract
	PrivateEvent PrivateEventPolicy
	Topology     TopologyPolicy
	Authority    AuthorityPolicy
	Limits       Limits
}

// DialectIdentity names the dialect; the composed identity additionally
// carries the canonical digest of the whole value.
type DialectIdentity struct {
	Name    string
	Version string
}

// SourceLayout tells the loader where a source set keeps its parts. The
// loader receives this; it never tests literal paths.
type SourceLayout struct {
	// DefinitionPath is the DDL document, relative to the source root.
	DefinitionPath string
	// ProgramRoot is the directory holding program sources.
	ProgramRoot string
	// ProgramSuffix is the required program file suffix.
	ProgramSuffix string
}

// EnvelopeField is one typed scalar the host event exposes to programs.
type EnvelopeField struct {
	Name string
	// Type is the field's logical type in the shared value model.
	Type string
	// Nullable is whether the host may deliver the field as null.
	Nullable bool
}

// EventContract describes the host event a normalizer receives: its name
// and the typed scalar envelope programs may read directly (as read
// parameters and program input). The envelope is encapsulated: Fields
// returns a copy, so no holder of any Dialect copy can mutate the shared
// contract through aliasing.
type EventContract struct {
	Name         string
	scalarFields []EnvelopeField
}

// NewEventContract builds an event contract from a defensive copy of the
// given fields.
func NewEventContract(name string, fields ...EnvelopeField) EventContract {
	owned := make([]EnvelopeField, len(fields))
	copy(owned, fields)
	return EventContract{Name: name, scalarFields: owned}
}

// Fields returns a copy of the typed scalar envelope.
func (contract EventContract) Fields() []EnvelopeField {
	owned := make([]EnvelopeField, len(contract.scalarFields))
	copy(owned, contract.scalarFields)
	return owned
}

// PrivateEventPolicy constrains the application-declared private event.
type PrivateEventPolicy struct {
	// Name is the required private event name.
	Name string
	// ExactlyOne requires the application to declare exactly one private
	// event.
	ExactlyOne bool
}

// TopologyPolicy is the admitted program topology. The event flow is
// structural: the single normalizer consumes the host event and emits only
// the private event; analytic folds consume the private event. Names are
// never duplicated here - HostEvent and PrivateEvent are the single source
// of each name. Parameterization is not permission to build an arbitrary
// dataflow graph: a core admits only topology policies it implements, and
// this closed two-stage policy is the one Tailapp selects.
type TopologyPolicy struct {
	// ExactlyOneNormalizer requires exactly one normalizer.
	ExactlyOneNormalizer bool
	// AtLeastOneFold requires at least one analytic fold.
	AtLeastOneFold bool
	// FoldsMayEmitEvents is whether folds may emit events at all.
	FoldsMayEmitEvents bool
}

// ReadVisibility names a closed read-authority rule.
type ReadVisibility string

const (
	// ReadOwnTables admits reads only of tables the program itself writes.
	ReadOwnTables ReadVisibility = "own-tables"
	// ReadOwnAndNormalizerTables admits reads of tables the program itself
	// writes and tables the normalizer writes - never another fold's.
	ReadOwnAndNormalizerTables ReadVisibility = "own-and-normalizer-tables"
)

// AuthorityPolicy is the read/write/emission ownership contract.
type AuthorityPolicy struct {
	// NormalizerReads is the normalizer's read visibility.
	NormalizerReads ReadVisibility
	// FoldReads is an analytic fold's read visibility.
	FoldReads ReadVisibility
	// SingleWriterTables requires each table to have exactly one writing
	// program.
	SingleWriterTables bool
}

// Limits are the evaluation and source bounds, in the same units the
// runtime enforces.
type Limits struct {
	MaxElementBytes int
	MaxSourceBytes  int
	MaxProgramBytes int
	MaxInputBytes   int
	MaxOutputBytes  int
	MaxDepth        int
	MaxRange        int
	MaxEvents       int
	MaxFacts        int
	MaxRowChanges   int
	MaxManyRows     int
}

// Tailapp is the versioned dialect the Tailapp host supplies: the layout,
// typed envelope, closed two-stage topology, ownership authority, and
// bounds that the Tailapp host enforces. Host-side consistency tests bind
// every load-bearing field to observed compiler, evaluator, and producer
// behavior so this description cannot drift from the integration contract.
// Envelope types describe Tailapp's canonical record contract; envelope names
// are bound mechanically.
func Tailapp() Dialect {
	return Dialect{
		Identity: DialectIdentity{Name: "tailapp-otlp", Version: "1"},
		Layout: SourceLayout{
			DefinitionPath: "application.sql",
			ProgramRoot:    "folds",
			ProgramSuffix:  ".jsonata",
		},
		HostEvent: NewEventContract("otlp_record",
			EnvelopeField{Name: "id", Type: "TEXT", Nullable: false},
			EnvelopeField{Name: "signal", Type: "TEXT", Nullable: false},
			EnvelopeField{Name: "name", Type: "TEXT", Nullable: false},
			EnvelopeField{Name: "source", Type: "TEXT", Nullable: false},
			EnvelopeField{Name: "time_unix_nano", Type: "TEXT", Nullable: true},
			EnvelopeField{Name: "observed_unix_nano", Type: "TEXT", Nullable: true},
			EnvelopeField{Name: "trace_id", Type: "TEXT", Nullable: true},
			EnvelopeField{Name: "span_id", Type: "TEXT", Nullable: true},
			EnvelopeField{Name: "content_digest", Type: "TEXT", Nullable: false},
		),
		PrivateEvent: PrivateEventPolicy{Name: "otel_event", ExactlyOne: true},
		Topology: TopologyPolicy{
			ExactlyOneNormalizer: true,
			AtLeastOneFold:       true,
			FoldsMayEmitEvents:   false,
		},
		Authority: AuthorityPolicy{
			NormalizerReads:    ReadOwnTables,
			FoldReads:          ReadOwnAndNormalizerTables,
			SingleWriterTables: true,
		},
		Limits: Limits{
			MaxElementBytes: 64 << 10,
			MaxSourceBytes:  512 << 10,
			MaxProgramBytes: 64 << 10,
			MaxInputBytes:   256 << 10,
			MaxOutputBytes:  256 << 10,
			MaxDepth:        64,
			MaxRange:        4096,
			MaxEvents:       256,
			MaxFacts:        64,
			MaxRowChanges:   1024,
			MaxManyRows:     1024,
		},
	}
}

// Canonical is the deterministic full serialization of every semantic
// field, in a fixed order. It is the basis of the dialect's identity
// digest: any field change changes it.
func (dialect Dialect) Canonical() string {
	var builder strings.Builder
	write := func(key string, value any) {
		fmt.Fprintf(&builder, "%s=%v\n", key, value)
	}
	write("identity.name", dialect.Identity.Name)
	write("identity.version", dialect.Identity.Version)
	write("layout.definition", dialect.Layout.DefinitionPath)
	write("layout.program-root", dialect.Layout.ProgramRoot)
	write("layout.program-suffix", dialect.Layout.ProgramSuffix)
	write("host-event.name", dialect.HostEvent.Name)
	for _, field := range dialect.HostEvent.Fields() {
		write("host-event.field."+field.Name, field.Type+"/nullable="+fmt.Sprint(field.Nullable))
	}
	write("private-event.name", dialect.PrivateEvent.Name)
	write("private-event.exactly-one", dialect.PrivateEvent.ExactlyOne)
	write("topology.exactly-one-normalizer", dialect.Topology.ExactlyOneNormalizer)
	write("topology.at-least-one-fold", dialect.Topology.AtLeastOneFold)
	write("topology.folds-may-emit-events", dialect.Topology.FoldsMayEmitEvents)
	write("authority.normalizer-reads", dialect.Authority.NormalizerReads)
	write("authority.fold-reads", dialect.Authority.FoldReads)
	write("authority.single-writer-tables", dialect.Authority.SingleWriterTables)
	write("limits.element-bytes", dialect.Limits.MaxElementBytes)
	write("limits.source-bytes", dialect.Limits.MaxSourceBytes)
	write("limits.program-bytes", dialect.Limits.MaxProgramBytes)
	write("limits.input-bytes", dialect.Limits.MaxInputBytes)
	write("limits.output-bytes", dialect.Limits.MaxOutputBytes)
	write("limits.depth", dialect.Limits.MaxDepth)
	write("limits.range", dialect.Limits.MaxRange)
	write("limits.events", dialect.Limits.MaxEvents)
	write("limits.facts", dialect.Limits.MaxFacts)
	write("limits.row-changes", dialect.Limits.MaxRowChanges)
	write("limits.many-rows", dialect.Limits.MaxManyRows)
	return builder.String()
}

// DialectComponent renders a dialect as its runtime identity component:
// the name and version for readers, plus the complete canonical digest so
// every semantic field mechanically participates in the composed identity
// at full strength.
func DialectComponent(dialect Dialect) Component {
	sum := sha256.Sum256([]byte(dialect.Canonical()))
	return Component{
		Key:   "dialect",
		Value: dialect.Identity.Name + "/" + dialect.Identity.Version + "+sha256:" + hex.EncodeToString(sum[:]),
	}
}
