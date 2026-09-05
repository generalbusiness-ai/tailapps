package profile

import (
	"encoding/json"
	"fmt"
)

// ContinueCompatible requires the same runtime for a continue activation. Every
// existing writable table must retain its complete stored shape. The next
// revision may add tables; the activation code creates those empty at the
// delivery boundary.
func ContinueCompatible(existing, next *Profile) error {
	if existing == nil || next == nil {
		return fmt.Errorf("both existing and next profiles are required")
	}
	if existing.RuntimeProfile != next.RuntimeProfile {
		return fmt.Errorf("runtime profile changed; acknowledged reset is required")
	}
	for name, prior := range existing.Tables {
		candidate, ok := next.Tables[name]
		if !ok {
			return fmt.Errorf("existing writable table %q was removed", name)
		}
		prior.SQL, prior.Writer = "", ""
		candidate.SQL, candidate.Writer = "", ""
		priorJSON, _ := json.Marshal(prior)
		candidateJSON, _ := json.Marshal(candidate)
		if string(priorJSON) != string(candidateJSON) {
			return fmt.Errorf("existing writable table %q changed stored shape", name)
		}
	}
	return nil
}
