package mcp

import (
	"fmt"
	"os"
	"path/filepath"
)

// The optional cross-harness skill: progressive-disclosure packaging for
// harnesses that index SKILL.md files (Claude Code under .claude/skills/,
// Codex under .agents/skills/, OpenCode reading both plus its own). It is
// never required for correct MCP orientation - the instructions, tools,
// and resource catalog are the contract - and it is generated from the
// same embedded content as the overview resource so the two cannot drift.
// Nothing installs it as a side effect: only the explicit
// `tailapp mcp emit-skill DIRECTORY` command writes it.

// skillDescription is the startup-catalog line harnesses index. OpenCode
// bounds skill descriptions at 1024 characters; this stays far under.
const skillDescription = "Query and manage Tailapp: local OTLP telemetry analytics for coding agents, over MCP. Use when inspecting agent activity, cost, or guard findings, or when authoring a Tailapp. Read tailapp://docs/overview first."

// SkillMarkdown renders the skill: frontmatter plus, byte-for-byte, the
// overview resource body.
func SkillMarkdown() string {
	overview, _ := readResourceText(docsScheme + "overview")
	return fmt.Sprintf(`---
name: tailapp
description: %q
---

%s`, skillDescription, overview)
}

// EmitSkill writes DIRECTORY/tailapp/SKILL.md and reports the written
// path. The directory is the skills root the operator chooses (for
// example .claude/skills or .agents/skills; OpenCode reads both).
func EmitSkill(directory string) (string, error) {
	target := filepath.Join(directory, "tailapp")
	if err := os.MkdirAll(target, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(target, "SKILL.md")
	if err := os.WriteFile(path, []byte(SkillMarkdown()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
