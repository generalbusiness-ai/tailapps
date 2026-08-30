package jsonataddl

import (
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
	if base.Key != "dialect" || !strings.HasPrefix(base.Value, "tailapp-otlp/1+sha256:") {
		t.Fatalf("dialect component = %#v", base)
	}
	// Changing any semantic field changes the component value mechanically,
	// with no version bump required: the digest of the canonical form
	// participates in the composed identity.
	mutations := map[string]func(*Dialect){
		"layout":        func(d *Dialect) { d.Layout.ProgramSuffix = ".changed" },
		"envelope name": func(d *Dialect) { d.HostEvent.ScalarFields[0].Name = "changed" },
		"envelope type": func(d *Dialect) { d.HostEvent.ScalarFields[0].Nullable = !d.HostEvent.ScalarFields[0].Nullable },
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
	// Value semantics: mutating one value never affects a fresh one.
	if DialectComponent(Tailapp()).Value != base.Value {
		t.Fatal("Tailapp() does not return an independent value")
	}
}
