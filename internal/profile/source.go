package profile

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

var (
	tailappNameRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	identifierRE  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func ValidateName(name string) error {
	if !tailappNameRE.MatchString(name) {
		return fmt.Errorf("invalid tailapp name %q", name)
	}
	return nil
}

func ValidateSourceElement(name string, content []byte) error {
	if err := validateSourcePath(name); err != nil {
		return err
	}
	if name != "application.sql" && !(strings.HasPrefix(name, "folds/") && strings.HasSuffix(name, ".jsonata")) {
		return fmt.Errorf("source path %q is outside the tailapp source profile", name)
	}
	if len(content) == 0 || len(content) > MaxElementBytes {
		return fmt.Errorf("source element %q is empty or exceeds %d bytes", name, MaxElementBytes)
	}
	if !utf8.Valid(content) {
		return fmt.Errorf("source element %q is not UTF-8", name)
	}
	return nil
}

func Load(files fs.FS, root, name string) (*Profile, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	root = path.Clean(root)
	if root == "." {
		root = ""
	}
	sources, err := loadSources(files, root)
	if err != nil {
		return nil, err
	}
	compiler := newCompiler(name, sources)
	if err := compiler.compile(); err != nil {
		return nil, err
	}
	compiler.profile.Revision = digestSources(sources)
	return compiler.profile, nil
}

func loadSources(files fs.FS, root string) (map[string][]byte, error) {
	applicationPath := "application.sql"
	if root != "" {
		applicationPath = path.Join(root, applicationPath)
	}
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
		if rel != "application.sql" && !(strings.HasPrefix(rel, "folds/") && strings.HasSuffix(rel, ".jsonata")) {
			return fmt.Errorf("source path %q is outside the tailapp source profile", rel)
		}
		data, err := fs.ReadFile(files, filePath)
		if err != nil {
			return err
		}
		if len(data) == 0 || len(data) > MaxElementBytes {
			return fmt.Errorf("source element %q is empty or exceeds %d bytes", rel, MaxElementBytes)
		}
		if !utf8.Valid(data) {
			return fmt.Errorf("source element %q is not UTF-8", rel)
		}
		total += len(data)
		if total > MaxSourceBytes {
			return fmt.Errorf("tailapp source exceeds %d bytes", MaxSourceBytes)
		}
		sources[rel] = append([]byte(nil), data...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read tailapp source: %w", err)
	}
	if _, ok := sources["application.sql"]; !ok {
		if _, err := fs.Stat(files, applicationPath); errors.Is(err, fs.ErrNotExist) {
			return nil, errors.New("tailapp source requires application.sql")
		}
		return nil, errors.New("tailapp source requires application.sql")
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

func digestSources(sources map[string][]byte) string {
	hash := sha256.New()
	hash.Write([]byte(RuntimeID))
	paths := sortedKeys(sources)
	for _, name := range paths {
		hash.Write([]byte{0})
		hash.Write([]byte(name))
		hash.Write([]byte{0})
		hash.Write(sources[name])
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func digestJSON(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func sortedKeys[V any](values map[string]V) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
