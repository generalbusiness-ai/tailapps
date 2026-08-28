// Package tailapps contains the first-party Tailapp source sets. The engine
// installs these through the same public service used for agent-authored apps;
// embedding is distribution, not a privileged execution path.
package tailapps

import (
	"embed"
	"fmt"

	"github.com/generalbusiness-ai/tailapp/internal/profile"
)

//go:embed agent-guard/application.sql agent-guard/folds/*.jsonata session-cost/application.sql session-cost/folds/*.jsonata
var sources embed.FS

func Load(name string) (*profile.Profile, error) {
	switch name {
	case "agent-guard", "session-cost":
		return profile.Load(sources, name, name)
	default:
		return nil, fmt.Errorf("unknown bundled tailapp %q", name)
	}
}

func Names() []string { return []string{"agent-guard", "session-cost"} }
