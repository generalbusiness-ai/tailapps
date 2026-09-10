package jsonataddl

import (
	"errors"
	"strings"
)

type sourceElement struct {
	name    string
	content []byte
}

// snapshotDialectSources takes ownership of an already acquired source set.
// It does no filesystem I/O. The caller must keep the input stable during the
// call; bounding the host's acquisition and input backing storage is separate.
// This private constructor is not an extension admission path by itself.
// A counted slice avoids traversing a caller's potentially oversized map.
//
// These metadata bounds follow from existing successful-compilation rules:
// each source is nonempty, and each non-definition source must occur in a USING
// clause in the bounded definition. Unique program paths therefore occupy at
// most MaxElementBytes altogether. The configured definition path is separate.
// Reject impossible sets before validating or copying their strings and bytes.
func snapshotDialectSources(sources []sourceElement, dialect Dialect) (map[string][]byte, error) {
	limits := dialect.Limits
	if limits.MaxElementBytes < 1 || limits.MaxSourceBytes < 1 {
		return nil, errors.New("source limits must be positive")
	}
	if len(sources) > limits.MaxSourceBytes {
		return nil, errors.New("source entry count exceeds the content bound")
	}
	remainingContent, remainingPaths := limits.MaxSourceBytes, limits.MaxElementBytes
	definition := dialect.Layout.DefinitionPath
	foundDefinition := false
	for _, element := range sources {
		name, content := element.name, element.content
		// Check lengths before comparing, scanning or quoting an untrusted path.
		if len(name) > limits.MaxElementBytes && len(name) != len(definition) {
			return nil, errors.New("source path exceeds the definition bound")
		}
		if name == definition {
			if foundDefinition {
				return nil, errors.New("source path is declared twice")
			}
			foundDefinition = true
		} else {
			if len(name) > remainingPaths {
				return nil, errors.New("source paths exceed the definition bound")
			}
			remainingPaths -= len(name)
		}
		if len(content) == 0 || len(content) > limits.MaxElementBytes {
			return nil, errors.New("source element is empty or exceeds the element bound")
		}
		if len(content) > remainingContent {
			return nil, errors.New("source content exceeds the cumulative bound")
		}
		remainingContent -= len(content)
	}
	if !foundDefinition {
		return nil, errors.New("source set has no definition")
	}
	seen := make(map[string]bool, len(sources))
	for _, element := range sources {
		name, content := element.name, element.content
		if seen[name] {
			return nil, errors.New("source path is declared twice")
		}
		seen[name] = true
		if err := ValidateSource(dialect, name, content); err != nil {
			return nil, err
		}
	}
	owned := make(map[string][]byte, len(sources))
	for _, element := range sources {
		name, content := element.name, element.content
		// Neither a substring of a larger name nor a subslice of a larger file
		// may keep its caller's backing storage alive through the snapshot.
		owned[strings.Clone(name)] = append([]byte(nil), content...)
	}
	return owned, nil
}
