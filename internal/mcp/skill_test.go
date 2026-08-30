package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillByteDerivesFromTheOverviewResource(t *testing.T) {
	skill := SkillMarkdown()
	overview, known := readResourceText(docsScheme + "overview")
	if !known {
		t.Fatal("overview unknown")
	}
	if !strings.HasSuffix(skill, overview) {
		t.Fatal("the skill body does not byte-derive from the overview resource")
	}
	body := strings.TrimSuffix(skill, overview)
	if strings.Contains(body, "#") || !strings.HasPrefix(body, "---\n") || !strings.HasSuffix(body, "---\n\n") {
		t.Fatalf("everything before the overview must be frontmatter alone: %q", body)
	}
}

func TestSkillFrontmatterFitsHarnessBounds(t *testing.T) {
	if len("tailapp") < 1 || len("tailapp") > 64 {
		t.Fatal("skill name outside the 1-64 bound")
	}
	if len(skillDescription) < 1 || len(skillDescription) > 1024 {
		t.Fatalf("skill description is %d characters; OpenCode bounds descriptions at 1024", len(skillDescription))
	}
	skill := SkillMarkdown()
	for _, needed := range []string{"name: tailapp\n", "description: \"" + skillDescription + "\"\n", "tailapp://docs/overview"} {
		if !strings.Contains(skill, needed) {
			t.Fatalf("skill missing %q", needed)
		}
	}
}

func TestEmitSkillWritesOnlyWhereDirected(t *testing.T) {
	root := t.TempDir()
	path, err := EmitSkill(root)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(root, "tailapp", "SKILL.md") {
		t.Fatalf("skill path = %q", path)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != SkillMarkdown() {
		t.Fatal("written skill differs from the rendered skill")
	}
	entries, err := os.ReadDir(filepath.Join(root, "tailapp"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("skill folder must contain exactly SKILL.md: %v %v", entries, err)
	}
}
