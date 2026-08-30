package profile

import "errors"

type EvaluationInput struct {
	Meta  map[string]any `json:"meta"`
	Event map[string]any `json:"event"`
	Rows  map[string]any `json:"rows"`
}

type TableChanges struct {
	Insert []map[string]any `json:"insert,omitempty"`
	Upsert []map[string]any `json:"upsert,omitempty"`
	Delete []map[string]any `json:"delete,omitempty"`
}

type EvaluationResult struct {
	Decision string                      `json:"decision"`
	Facts    []map[string]any            `json:"facts"`
	Events   map[string][]map[string]any `json:"events,omitempty"`
	Tables   map[string]TableChanges     `json:"tables"`
}

// Evaluate runs one named program through the core handle and returns its
// validated result; bounded evaluation and strict result and mutation-plan
// validation live in the core.
func (p *Profile) Evaluate(programName string, input EvaluationInput) (EvaluationResult, error) {
	if p.core == nil {
		return EvaluationResult{}, errors.New("profile has no compiled application handle")
	}
	return evaluateViaCore(p.core, programName, input)
}
