package profile

import (
	"fmt"
	"regexp"

	"github.com/generalbusiness-ai/tailapps/jsonataddl"
)

var tailappNameRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

func ValidateName(name string) error {
	if !tailappNameRE.MatchString(name) {
		return fmt.Errorf("invalid tailapp name %q", name)
	}
	return nil
}

// ValidateSourceElement checks one draft source element against the Tailapp
// dialect's layout and element bounds without compiling anything.
func ValidateSourceElement(name string, content []byte) error {
	return jsonataddl.ValidateSource(jsonataddl.Tailapp(), name, content)
}
