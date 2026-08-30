// Package jsonataddl is the Tailapps-local home of the JSONata-with-DDL
// application core described by notes/2026-08-28-jsonata-ddl-core-extraction.md.
// At this migration stage it defines the host policy contract (Dialect) and
// the composed runtime identity; behavior still lives in internal/profile
// and moves here boundary by boundary.
package jsonataddl

// Dialect is the immutable host policy a compiler receives instead of
// hard-coded host names: where sources live, what the host and private
// events are called and carry, which program topology is admitted, and the
// evaluation limits. Core code never tests a literal path or event name;
// diagnostics may include configured names, but no host name is a special
// case in core code.
type Dialect struct {
	Identity     DialectIdentity
	Layout       SourceLayout
	HostEvent    EventContract
	PrivateEvent PrivateEventPolicy
	Topology     TopologyPolicy
	Limits       Limits
}

// DialectIdentity names the dialect for the composed runtime identity.
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

// EventContract describes the host event a normalizer receives: its name
// and the scalar envelope fields a program may read directly.
type EventContract struct {
	Name         string
	ScalarFields []string
}

// PrivateEventPolicy constrains the application-declared private event.
type PrivateEventPolicy struct {
	// Name is the required private event name.
	Name string
	// ExactlyOne requires the application to declare exactly one private
	// event.
	ExactlyOne bool
}

// TopologyPolicy is the admitted program topology. Parameterization is not
// permission to build an arbitrary dataflow graph: a core admits only
// topology policies it implements, and this closed two-stage policy is the
// one Tailapp selects.
type TopologyPolicy struct {
	// NormalizerConsumes is the event one normalizer consumes.
	NormalizerConsumes string
	// NormalizerEmits is the only event the normalizer may emit.
	NormalizerEmits string
	// FoldsConsume is the event analytic folds consume.
	FoldsConsume string
	// FoldsMayEmitEvents is whether folds may emit events at all.
	FoldsMayEmitEvents bool
	// SingleWriterTables requires each table to have exactly one writer.
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
// events, closed two-stage topology, and bounds that internal/profile
// enforces today. The consistency tests bind every field to observed
// compiler and evaluator behavior so this description cannot drift from
// the implementation while behavior migrates.
func Tailapp() Dialect {
	return Dialect{
		Identity: DialectIdentity{Name: "tailapp-otlp", Version: "1"},
		Layout: SourceLayout{
			DefinitionPath: "application.sql",
			ProgramRoot:    "folds",
			ProgramSuffix:  ".jsonata",
		},
		HostEvent: EventContract{
			Name: "otlp_record",
			ScalarFields: []string{
				"id", "signal", "name", "source",
				"time_unix_nano", "observed_unix_nano",
				"trace_id", "span_id", "content_digest",
			},
		},
		PrivateEvent: PrivateEventPolicy{Name: "otel_event", ExactlyOne: true},
		Topology: TopologyPolicy{
			NormalizerConsumes: "otlp_record",
			NormalizerEmits:    "otel_event",
			FoldsConsume:       "otel_event",
			FoldsMayEmitEvents: false,
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
