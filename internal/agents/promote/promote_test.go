package promote

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidate_RequiresFields(t *testing.T) {
	if err := (Spec{}).Validate(); err == nil {
		t.Error("empty spec should be invalid")
	}
	if err := (Spec{Kind: KindSkill, SubjectID: "x"}).Validate(); err == nil {
		t.Error("missing providers should fail")
	}
	if err := (Spec{Kind: KindMCP, SubjectID: "x", SourceProvider: "a", TargetProvider: "a"}).Validate(); err == nil {
		t.Error("same-provider MCP should fail")
	}
	if err := (Spec{Kind: KindSkill, SubjectID: "x", SourceProvider: "a", TargetProvider: "a"}).Validate(); err != nil {
		t.Error("same-provider skill (scope change) should be valid")
	}
}

func TestBuild_SkillConflict_BlocksOverwrite(t *testing.T) {
	tmp := t.TempDir()
	srcPath := filepath.Join(tmp, "src", "SKILL.md")
	mustWrite(t, srcPath, "---\nname: ship-it\n---\nbody\n")

	snap := Snapshot{
		Skills: []SkillRef{
			{Name: "ship-it", Provider: "claude-code", Scope: "user", Path: srcPath, Description: "Ship the change"},
			{Name: "ship-it", Provider: "copilot-cli", Scope: "user", Path: filepath.Join(tmp, "tgt", "SKILL.md")},
		},
	}
	spec := Spec{Kind: KindSkill, SubjectID: "ship-it", SourceProvider: "claude-code", TargetProvider: "copilot-cli", TargetScope: "user"}
	opts := BuildOpts{SkillDir: func(p, sc string) (string, error) { return filepath.Join(tmp, p, sc), nil }}
	plan := Build(snap, spec, opts)
	if plan.Conflict != ConflictDuplicate {
		t.Errorf("expected duplicate conflict, got %v: %s", plan.Conflict, plan.ConflictMsg)
	}
	// Same spec with Force should succeed.
	spec.Force = true
	if p := Build(snap, spec, opts); p.Conflict != ConflictNone {
		t.Errorf("with force expected no conflict, got %v: %s", p.Conflict, p.ConflictMsg)
	}
}

func TestBuild_SkillPlan_HasFileCopiesAndFrontmatter(t *testing.T) {
	tmp := t.TempDir()
	srcDir := filepath.Join(tmp, "src")
	srcPath := filepath.Join(srcDir, "SKILL.md")
	mustWrite(t, srcPath, "---\nname: summary\ndescription: Old desc\n---\nbody text\n")
	mustWrite(t, filepath.Join(srcDir, "helper.md"), "extra content")

	snap := Snapshot{
		Skills: []SkillRef{
			{Name: "summary", Provider: "claude-code", Scope: "user", Path: srcPath, Description: "New desc", WhenToUse: "when summarising"},
		},
	}
	spec := Spec{Kind: KindSkill, SubjectID: "summary", SourceProvider: "claude-code", TargetProvider: "copilot-cli", TargetScope: "user"}
	opts := BuildOpts{SkillDir: func(p, sc string) (string, error) { return filepath.Join(tmp, p, sc), nil }}
	plan := Build(snap, spec, opts)
	if plan.Conflict != ConflictNone {
		t.Fatalf("unexpected conflict %v: %s", plan.Conflict, plan.ConflictMsg)
	}
	// We expect 2 file copies: SKILL.md (with Body) + helper.md verbatim.
	if len(plan.FileCopies) < 2 {
		t.Fatalf("expected ≥2 copies, got %d: %+v", len(plan.FileCopies), plan.FileCopies)
	}
	var skillMd *FileCopy
	for i := range plan.FileCopies {
		if strings.HasSuffix(plan.FileCopies[i].Dst, "SKILL.md") {
			skillMd = &plan.FileCopies[i]
		}
	}
	if skillMd == nil {
		t.Fatal("SKILL.md copy missing from plan")
	}
	if len(skillMd.Body) == 0 {
		t.Error("SKILL.md copy should have a converted Body")
	}
	if !strings.Contains(string(skillMd.Body), "description: New desc") {
		t.Errorf("converted frontmatter missing description: %s", skillMd.Body)
	}
	if !strings.Contains(string(skillMd.Body), "klim: promoted from claude-code to copilot-cli") {
		t.Error("converted SKILL.md missing provenance comment")
	}
}

func TestBuild_MCPPlan_EmitsAddMCPOp(t *testing.T) {
	snap := Snapshot{
		MCPs: []MCPRef{
			{Name: "github", Provider: "claude-code", Scope: "user", Transport: "stdio", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-github"}},
		},
	}
	spec := Spec{Kind: KindMCP, SubjectID: "github", SourceProvider: "claude-code", TargetProvider: "copilot-cli", TargetScope: "user"}
	plan := Build(snap, spec, BuildOpts{})
	if plan.Conflict != ConflictNone {
		t.Fatalf("conflict: %v %s", plan.Conflict, plan.ConflictMsg)
	}
	if len(plan.ProviderOps) != 1 || plan.ProviderOps[0].Kind != OpAddMCP {
		t.Fatalf("expected one OpAddMCP, got %+v", plan.ProviderOps)
	}
	op := plan.ProviderOps[0]
	if op.MCPName != "github" || op.MCPCommand != "npx" {
		t.Errorf("op fields not copied: %+v", op)
	}
}

func TestBuild_PluginPlan_NeedsMarketplace(t *testing.T) {
	snap := Snapshot{
		Plugins: []PluginRef{
			{Name: "no-mp", Provider: "claude-code", Installed: true},
			{Name: "with-mp", Provider: "claude-code", Marketplace: "official", Installed: true},
		},
	}
	if p := Build(snap, Spec{Kind: KindPlugin, SubjectID: "no-mp", SourceProvider: "claude-code", TargetProvider: "copilot-cli"}, BuildOpts{}); p.Conflict != ConflictUnsupported {
		t.Errorf("expected unsupported conflict for plugin without marketplace, got %v", p.Conflict)
	}
	if p := Build(snap, Spec{Kind: KindPlugin, SubjectID: "with-mp", SourceProvider: "claude-code", TargetProvider: "copilot-cli"}, BuildOpts{}); p.Conflict != ConflictNone {
		t.Errorf("expected no conflict for plugin with marketplace, got %v: %s", p.Conflict, p.ConflictMsg)
	}
}

func TestApply_FileCopiesAndExecutor(t *testing.T) {
	tmp := t.TempDir()
	srcDir := filepath.Join(tmp, "src")
	srcPath := filepath.Join(srcDir, "SKILL.md")
	mustWrite(t, srcPath, "---\nname: applied\n---\nbody")

	plan := Plan{
		Spec: Spec{Kind: KindSkill, SubjectID: "applied", SourceProvider: "x", TargetProvider: "y", TargetScope: "user"},
		FileCopies: []FileCopy{
			{Src: srcPath, Dst: filepath.Join(tmp, "out", "SKILL.md"), Body: []byte("---\nname: applied\n---\nnew\n"), Mode: 0o644, MkdirOK: true},
		},
		ProviderOps: []ProviderOp{
			{Kind: OpAddMCP, MCPName: "from-promote"},
		},
	}
	ex := &fakeExecutor{}
	if err := plan.Apply(context.Background(), ex); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(tmp, "out", "SKILL.md")); err != nil {
		t.Errorf("output missing: %v", err)
	} else if !strings.Contains(string(got), "new") {
		t.Errorf("output content wrong: %q", got)
	}
	if len(ex.mcps) != 1 || ex.mcps[0] != "from-promote" {
		t.Errorf("executor calls = %+v, want [from-promote]", ex.mcps)
	}
}

func TestApply_ReportsConflict(t *testing.T) {
	plan := Plan{Conflict: ConflictDuplicate, ConflictMsg: "already there"}
	if err := plan.Apply(context.Background(), &fakeExecutor{}); err == nil || !strings.Contains(err.Error(), "already there") {
		t.Errorf("expected conflict surfaced, got %v", err)
	}
}

type fakeExecutor struct {
	mcps    []string
	plugins []string
	fail    error
}

func (f *fakeExecutor) AddMCP(_ context.Context, _ string, op ProviderOp) error {
	if f.fail != nil {
		return f.fail
	}
	f.mcps = append(f.mcps, op.MCPName)
	return nil
}
func (f *fakeExecutor) InstallPlugin(_ context.Context, _ string, op ProviderOp) error {
	if f.fail != nil {
		return f.fail
	}
	f.plugins = append(f.plugins, op.PluginRefName)
	return nil
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

var _ = errors.New // keep import alive in case future tests use it
