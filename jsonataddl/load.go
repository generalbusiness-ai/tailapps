package jsonataddl

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var identifierRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// LoadApplication reads one application source set through the dialect's
// source layout, compiles it under the dialect's event, topology, authority
// and limit policies, and returns an immutable handle. The runtimeProfile
// string names the complete runtime the host composed; the core never owns
// a global runtime constant, and the source revision digest is seeded with
// the given value so identity follows the host's declared runtime.
func LoadApplication(files fs.FS, root, name string, dialect Dialect, runtimeProfile string) (*Application, error) {
	if name == "" {
		return nil, errors.New("application name is required")
	}
	if runtimeProfile == "" {
		return nil, errors.New("runtime profile identity is required")
	}
	if err := validateLayout(dialect.Layout); err != nil {
		return nil, err
	}
	root = path.Clean(root)
	if root == "." {
		root = ""
	}
	sources, err := loadDialectSources(files, root, dialect)
	if err != nil {
		return nil, err
	}
	compiled, err := compileApplication(name, sources, dialect, runtimeProfile)
	if err != nil {
		return nil, err
	}
	compiled.revision = digestSourceSet(sources, runtimeProfile)
	return compiled, nil
}

// ValidateSource checks one source element against the dialect's layout and
// element bounds without compiling anything.
func ValidateSource(dialect Dialect, name string, content []byte) error {
	if err := validateLayout(dialect.Layout); err != nil {
		return err
	}
	if err := validateSourcePath(name); err != nil {
		return err
	}
	if !layoutAdmits(dialect.Layout, name) {
		return fmt.Errorf("source path %q is outside the application source layout", name)
	}
	if len(content) == 0 || len(content) > dialect.Limits.MaxElementBytes {
		return fmt.Errorf("source element %q is empty or exceeds %d bytes", name, dialect.Limits.MaxElementBytes)
	}
	if !utf8.Valid(content) {
		return fmt.Errorf("source element %q is not UTF-8", name)
	}
	return nil
}

func validateLayout(layout SourceLayout) error {
	if layout.DefinitionPath == "" || layout.ProgramRoot == "" || layout.ProgramSuffix == "" {
		return errors.New("dialect source layout is incomplete")
	}
	if validateSourcePath(layout.DefinitionPath) != nil || validateSourcePath(layout.ProgramRoot) != nil {
		return errors.New("dialect source layout has invalid paths")
	}
	if !strings.HasPrefix(layout.ProgramSuffix, ".") {
		return errors.New("dialect program suffix must begin with a dot")
	}
	return nil
}

func layoutAdmits(layout SourceLayout, name string) bool {
	if name == layout.DefinitionPath {
		return true
	}
	return strings.HasPrefix(name, layout.ProgramRoot+"/") && strings.HasSuffix(name, layout.ProgramSuffix)
}

func loadDialectSources(files fs.FS, root string, dialect Dialect) (map[string][]byte, error) {
	sources := make(map[string][]byte)
	total := 0
	err := fs.WalkDir(files, rootOrDot(root), func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(filePath, root+"/")
		if root == "" {
			rel = filePath
		}
		if err := validateSourcePath(rel); err != nil {
			return err
		}
		if !layoutAdmits(dialect.Layout, rel) {
			return fmt.Errorf("source path %q is outside the application source layout", rel)
		}
		data, err := fs.ReadFile(files, filePath)
		if err != nil {
			return err
		}
		if len(data) == 0 || len(data) > dialect.Limits.MaxElementBytes {
			return fmt.Errorf("source element %q is empty or exceeds %d bytes", rel, dialect.Limits.MaxElementBytes)
		}
		if !utf8.Valid(data) {
			return fmt.Errorf("source element %q is not UTF-8", rel)
		}
		total += len(data)
		if total > dialect.Limits.MaxSourceBytes {
			return fmt.Errorf("application source exceeds %d bytes", dialect.Limits.MaxSourceBytes)
		}
		sources[rel] = append([]byte(nil), data...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read application source: %w", err)
	}
	if _, ok := sources[dialect.Layout.DefinitionPath]; !ok {
		return nil, fmt.Errorf("application source requires %s", dialect.Layout.DefinitionPath)
	}
	return sources, nil
}

func rootOrDot(root string) string {
	if root == "" {
		return "."
	}
	return root
}

func validateSourcePath(name string) error {
	if name == "" || !utf8.ValidString(name) || strings.Contains(name, `\`) || path.Clean(name) != name || strings.HasPrefix(name, "/") {
		return fmt.Errorf("invalid source path %q", name)
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("invalid source path %q", name)
		}
	}
	return nil
}

func digestSourceSet(sources map[string][]byte, runtimeProfile string) string {
	hash := sha256.New()
	hash.Write([]byte(runtimeProfile))
	for _, name := range sortedNames(sources) {
		hash.Write([]byte{0})
		hash.Write([]byte(name))
		hash.Write([]byte{0})
		hash.Write(sources[name])
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func digestEncoded(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func sortedNames[V any](values map[string]V) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
