// Package tailapps contains the first-party Tailapp source sets. The engine
// installs these through the same public service used for agent-authored apps;
// embedding is distribution, not a privileged execution path.
package tailapps

import (
	"embed"
	"fmt"

	"github.com/generalbusiness-ai/tailapps/internal/profile"
)

//go:embed activity-stats/application.sql activity-stats/folds/*.jsonata agent-guard/application.sql agent-guard/folds/*.jsonata session-cost/application.sql session-cost/folds/*.jsonata
var sources embed.FS

func Load(name string) (*profile.Profile, error) {
	switch name {
	case "activity-stats", "agent-guard", "session-cost":
		return profile.Load(sources, name, name)
	default:
		return nil, fmt.Errorf("unknown bundled tailapp %q", name)
	}
}

func Names() []string { return []string{"activity-stats", "agent-guard", "session-cost"} }

// Source returns a private copy of a bundled Tailapp's ordinary source set.
// Bundles still travel through the public registry/compiler/activation path;
// this function is distribution, not a privileged runtime.
func Source(name string) (map[string][]byte, error) {
	compiled, err := Load(name)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]byte, len(compiled.Sources))
	for path, content := range compiled.Sources {
		result[path] = append([]byte(nil), content...)
	}
	return result, nil
}
