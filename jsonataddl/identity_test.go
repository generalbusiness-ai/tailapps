package jsonataddl

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func fullComponents() []Component {
	return []Component{
		{Key: "core.interface", Value: "jsonata-ddl-application-interface/2026-08-26"},
		{Key: "core.grammar", Value: "ddl-jsonata/v206"},
		{Key: "core.jsonata", Value: "jsonata-go/1.8-json-v1-subset"},
		{Key: "core.value-codec", Value: "logical-values/1"},
		{Key: "core.sqlite", Value: "sqlite-3.53.4@5"},
		{Key: "dialect", Value: "tailapp-otlp/1"},
		{Key: "host.canonicalization", Value: "otlp-canonical/1"},
		{Key: "host.orchestration", Value: "two-stage-txn/1"},
		{Key: "host.projection", Value: "query-values/1"},
	}
}

func TestInputIdentityIsCanonicalCompleteAndImmutable(t *testing.T) {
	base := declaredInputDialect()
	identity := DialectComponent(base)
	mutations := map[string]func(*Dialect){
		"input depth":     func(d *Dialect) { d.Limits.MaxInputDepth++ },
		"input bytes":     func(d *Dialect) { d.Limits.MaxInputBytes++ },
		"meta absent":     func(d *Dialect) { d.Input.Meta = ObjectContract{} },
		"meta root null":  func(d *Dialect) { d.Input.Meta = NewObjectContract(true, d.Input.Meta.Fields()...) },
		"event root null": func(d *Dialect) { d.Input.Event = NewObjectContract(true, d.Input.Event.Fields()...) },
		"envelope optional": func(d *Dialect) {
			f := d.HostEvent.Fields()
			f[0].Optional = true
			d.HostEvent = NewEventContract(d.HostEvent.Name, f...)
		},
		"meta type": func(d *Dialect) {
			f := d.Input.Meta.Fields()
			f[0].Type = "REAL"
			d.Input.Meta = NewObjectContract(false, f...)
		},
		"meta optional": func(d *Dialect) {
			f := d.Input.Meta.Fields()
			f[0].Optional = true
			d.Input.Meta = NewObjectContract(false, f...)
		},
		"meta nullable": func(d *Dialect) {
			f := d.Input.Meta.Fields()
			f[0].Nullable = true
			d.Input.Meta = NewObjectContract(false, f...)
		},
		"event kind": func(d *Dialect) {
			f := d.Input.Event.Fields()
			f[0].Kind = InputStringArray
			d.Input.Event = NewObjectContract(false, f...)
		},
		"event name": func(d *Dialect) {
			f := d.Input.Event.Fields()
			f[0].Name = "other"
			d.Input.Event = NewObjectContract(false, f...)
		},
		"event optional": func(d *Dialect) {
			f := d.Input.Event.Fields()
			f[0].Optional = true
			d.Input.Event = NewObjectContract(false, f...)
		},
		"event nullable": func(d *Dialect) {
			f := d.Input.Event.Fields()
			f[0].Nullable = true
			d.Input.Event = NewObjectContract(false, f...)
		},
		"nested type": func(d *Dialect) {
			f := d.Input.Event.Fields()
			f[2].Members[0].Type = "BLOB"
			d.Input.Event = NewObjectContract(false, f...)
		},
		"nested optional": func(d *Dialect) {
			f := d.Input.Event.Fields()
			f[2].Members[0].Optional = true
			d.Input.Event = NewObjectContract(false, f...)
		},
		"nested nullable": func(d *Dialect) {
			f := d.Input.Event.Fields()
			f[2].Members[0].Nullable = true
			d.Input.Event = NewObjectContract(false, f...)
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			d := base
			mutate(&d)
			if DialectComponent(d) == identity {
				t.Fatal("semantic mutation retained identity")
			}
		})
	}
	reordered := base
	meta, event, envelope := base.Input.Meta.Fields(), base.Input.Event.Fields(), base.HostEvent.Fields()
	slices.Reverse(meta)
	slices.Reverse(event)
	slices.Reverse(envelope)
	for i := range event {
		slices.Reverse(event[i].Members)
	}
	reordered.Input = InputContract{Meta: NewObjectContract(false, meta...), Event: NewObjectContract(false, event...)}
	reordered.HostEvent = NewEventContract(base.HostEvent.Name, envelope...)
	if DialectComponent(reordered) != identity {
		t.Fatal("declaration ordering changed identity")
	}
	// Constructor arguments and accessor results must not mutate a shared copy.
	event[0].Members[0].Name = "constructor-leak"
	leaked := reordered.Input.Event.Fields()
	leaked[0].Members[0].Name = "accessor-leak"
	if DialectComponent(reordered) != identity || DialectComponent(base) != identity {
		t.Fatal("nested contract mutation escaped defensive copy")
	}
	app := inputTestApplication(t, base)
	appCopy := app.Dialect()
	fields := appCopy.Input.Event.Fields()
	fields[2].Members[0].Name = "compiled-leak"
	if DialectComponent(app.Dialect()) != identity {
		t.Fatal("compiled contract is mutable")
	}
	// Even strings that compilation refuses have an unambiguous serialization.
	escaped := base
	escaped.Layout.ProgramRoot = "a\n\";Type=TEXT\\b"
	var decoded any
	if err := json.Unmarshal([]byte(escaped.Canonical()), &decoded); err != nil {
		t.Fatalf("canonical strings are not escaped: %v", err)
	}
	if escaped.Canonical() == base.Canonical() {
		t.Fatal("escaped field disappeared")
	}
	separate := base
	separate.HostEvent = NewEventContract("host_record", EnvelopeField{Name: "id", Type: "TEXT"}, EnvelopeField{Name: "signal", Type: "TEXT"})
	injected := base
	injected.HostEvent = NewEventContract("host_record", EnvelopeField{Name: "id", Type: "TEXT/nullable=false\nhost-event.field.signal=TEXT"})
	if separate.Canonical() == injected.Canonical() || DialectComponent(separate) == DialectComponent(injected) {
		t.Fatal("newline-injected scalar type collided with two declarations")
	}
}

func TestComposeIdentityOrderDoesNotMatter(t *testing.T) {
	forward := fullComponents()
	reversed := make([]Component, len(forward))
	for index, component := range forward {
		reversed[len(forward)-1-index] = component
	}
	first, err := ComposeIdentity(forward...)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ComposeIdentity(reversed...)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest() != second.Digest() || first.Descriptor() != second.Descriptor() {
		t.Fatalf("component order changed the identity:\n%s\n%s", first.Descriptor(), second.Descriptor())
	}
}

func TestEveryComponentChangeAltersDigest(t *testing.T) {
	base, err := ComposeIdentity(fullComponents()...)
	if err != nil {
		t.Fatal(err)
	}
	for index := range fullComponents() {
		changed := fullComponents()
		changed[index].Value += "-changed"
		altered, err := ComposeIdentity(changed...)
		if err != nil {
			t.Fatal(err)
		}
		if altered.Digest() == base.Digest() {
			t.Fatalf("changing %q did not change the digest", changed[index].Key)
		}
	}
}

func TestComposeIdentityRejectsMalformedSets(t *testing.T) {
	if _, err := ComposeIdentity(fullComponents()[:len(fullComponents())-1]...); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing component admitted: %v", err)
	}
	duplicated := append(fullComponents(), Component{Key: "dialect", Value: "again/1"})
	if _, err := ComposeIdentity(duplicated...); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate component admitted: %v", err)
	}
	unknown := append(fullComponents(), Component{Key: "core.surprise", Value: "1"})
	if _, err := ComposeIdentity(unknown...); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unknown component admitted: %v", err)
	}
	empty := fullComponents()
	empty[0].Value = ""
	if _, err := ComposeIdentity(empty...); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty component admitted: %v", err)
	}
	reserved := fullComponents()
	reserved[0].Value = "a;b"
	if _, err := ComposeIdentity(reserved...); err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("reserved character admitted: %v", err)
	}
}

func TestDescriptorIsSortedAndReadable(t *testing.T) {
	identity, err := ComposeIdentity(fullComponents()...)
	if err != nil {
		t.Fatal(err)
	}
	descriptor := identity.Descriptor()
	if !strings.HasPrefix(descriptor, "core.grammar=") {
		t.Fatalf("descriptor does not start at the first sorted key: %q", descriptor)
	}
	if !strings.Contains(descriptor, "dialect=tailapp-otlp/1") {
		t.Fatalf("descriptor omits the dialect: %q", descriptor)
	}
	if !strings.HasPrefix(identity.Digest(), "jsonata-ddl-runtime:sha256:") {
		t.Fatalf("digest scheme: %q", identity.Digest())
	}
}

func TestDialectComponentBindsEverySemanticField(t *testing.T) {
	base := DialectComponent(Tailapp())
	if base.Key != "dialect" || !strings.HasPrefix(base.Value, "tailapp-otlp/2+sha256:") {
		t.Fatalf("dialect component = %#v", base)
	}
	// Changing any semantic field changes the component value mechanically,
	// with no version bump required: the digest of the canonical form
	// participates in the composed identity.
	reviseEnvelope := func(d *Dialect, revise func([]EnvelopeField) []EnvelopeField) {
		d.HostEvent = NewEventContract(d.HostEvent.Name, revise(d.HostEvent.Fields())...)
	}
	mutations := map[string]func(*Dialect){
		"layout": func(d *Dialect) { d.Layout.ProgramSuffix = ".changed" },
		"envelope name": func(d *Dialect) {
			reviseEnvelope(d, func(fields []EnvelopeField) []EnvelopeField { fields[0].Name = "changed"; return fields })
		},
		"envelope nullability": func(d *Dialect) {
			reviseEnvelope(d, func(fields []EnvelopeField) []EnvelopeField { fields[0].Nullable = !fields[0].Nullable; return fields })
		},
		"private event": func(d *Dialect) { d.PrivateEvent.Name = "changed" },
		"topology":      func(d *Dialect) { d.Topology.AtLeastOneFold = false },
		"authority":     func(d *Dialect) { d.Authority.FoldReads = ReadOwnTables },
		"limit":         func(d *Dialect) { d.Limits.MaxFacts++ },
	}
	for name, mutate := range mutations {
		changed := Tailapp()
		mutate(&changed)
		if DialectComponent(changed).Value == base.Value {
			t.Fatalf("changing the %s did not change the dialect component", name)
		}
	}
	// Immutability through encapsulation: the envelope is only reachable as
	// a copy, so no holder of any Dialect copy - including a plain struct
	// copy, which aliases unexported state - can mutate the shared contract.
	original := Tailapp()
	aliased := original
	leaked := aliased.HostEvent.Fields()
	leaked[0].Name = "corrupted"
	if original.HostEvent.Fields()[0].Name == "corrupted" || DialectComponent(original).Value != base.Value {
		t.Fatal("mutating a Fields() copy reached the shared contract")
	}
	if DialectComponent(Tailapp()).Value != base.Value {
		t.Fatal("Tailapp() does not return an independent value")
	}
	// The full digest participates: the component carries the complete
	// sha256, not a truncation.
	if got := len(strings.TrimPrefix(base.Value, "tailapp-otlp/2+sha256:")); got != 64 {
		t.Fatalf("dialect component digest is %d hex characters, want the full 64", got)
	}
}
