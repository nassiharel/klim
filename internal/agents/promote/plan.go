package promote

import (
	"fmt"
	"os"
	"path/filepath"
)

// Plan inspects the snapshot and the spec and returns a runnable
// Plan or one whose Conflict explains why it can't run. The planner
// never mutates the filesystem or shells out — it just decides what
// the executor would have to do.
func Build(snap Snapshot, spec Spec, opts BuildOpts) Plan {
	if err := spec.Validate(); err != nil {
		return Plan{Spec: spec, Conflict: ConflictUnsupported, ConflictMsg: err.Error()}
	}
	switch spec.Kind {
	case KindSkill:
		return planSkill(snap, spec, opts)
	case KindMCP:
		return planMCP(snap, spec, opts)
	case KindPlugin:
		return planPlugin(snap, spec, opts)
	}
	return Plan{Spec: spec, Conflict: ConflictUnsupported, ConflictMsg: "unknown kind"}
}

// BuildOpts is provider-installation paths needed by the file-copy
// planner. The TUI fills these from real filesystem locations; tests
// can substitute temp dirs.
type BuildOpts struct {
	// SkillDir returns the directory that holds skill folders for
	// (provider, scope). e.g. ~/.claude/skills (user, claude-code).
	SkillDir func(provider, scope string) (string, error)
}

func planSkill(snap Snapshot, spec Spec, opts BuildOpts) Plan {
	src, ok := snap.FindSkill(spec.SubjectID, spec.SourceProvider)
	if !ok {
		return Plan{Spec: spec, Conflict: ConflictMissing, ConflictMsg: "source skill not found"}
	}
	targetScope := spec.TargetScope
	if targetScope == "" {
		targetScope = "user"
	}
	if !spec.Force && snap.HasSkill(src.Name, spec.TargetProvider, targetScope) {
		return Plan{
			Spec:        spec,
			Conflict:    ConflictDuplicate,
			ConflictMsg: fmt.Sprintf("skill %q already exists for provider %s at scope %s; remove it first or pass --force", src.Name, spec.TargetProvider, targetScope),
		}
	}

	if opts.SkillDir == nil {
		return Plan{Spec: spec, Conflict: ConflictUnsupported, ConflictMsg: "skill directories not configured"}
	}
	targetRoot, err := opts.SkillDir(spec.TargetProvider, targetScope)
	if err != nil {
		return Plan{Spec: spec, Conflict: ConflictUnsupported, ConflictMsg: "no skill dir for target: " + err.Error()}
	}

	// Source must point at the SKILL.md file inside a skill directory.
	srcDir := filepath.Dir(src.Path)
	if filepath.Base(src.Path) != "SKILL.md" {
		// Some snapshots store the directory path directly — fall back.
		srcDir = src.Path
	}
	dstDir := filepath.Join(targetRoot, src.Name)

	body := convertSkillFrontmatter(src, spec.TargetProvider)

	plan := Plan{
		Spec:          spec,
		SourceSummary: fmt.Sprintf("%s skill %q from %s/%s", spec.SourceProvider, src.Name, spec.SourceProvider, src.Scope),
		TargetSummary: fmt.Sprintf("%s/%s/%s", spec.TargetProvider, targetScope, src.Name),
	}

	// Always emit the converted SKILL.md (the executor writes it,
	// creating parent dirs as needed). Then walk the source skill
	// directory and copy every other file alongside it.
	plan.FileCopies = append(plan.FileCopies, FileCopy{
		Src:     filepath.Join(srcDir, "SKILL.md"),
		Dst:     filepath.Join(dstDir, "SKILL.md"),
		Body:    body,
		Mode:    0o644,
		MkdirOK: true,
	})

	// Best-effort walk — if it fails we still ship just the SKILL.md.
	_ = filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if filepath.Base(path) == "SKILL.md" {
			return nil
		}
		rel, _ := filepath.Rel(srcDir, path)
		plan.FileCopies = append(plan.FileCopies, FileCopy{
			Src:     path,
			Dst:     filepath.Join(dstDir, rel),
			Mode:    0o644,
			MkdirOK: true,
		})
		return nil
	})

	return plan
}

func planMCP(snap Snapshot, spec Spec, opts BuildOpts) Plan {
	src, ok := snap.FindMCP(spec.SubjectID, spec.SourceProvider)
	if !ok {
		return Plan{Spec: spec, Conflict: ConflictMissing, ConflictMsg: "source MCP not found"}
	}
	scope := spec.TargetScope
	if scope == "" {
		scope = "user"
	}
	if !spec.Force && snap.HasMCP(src.Name, spec.TargetProvider) {
		return Plan{
			Spec:        spec,
			Conflict:    ConflictDuplicate,
			ConflictMsg: fmt.Sprintf("MCP %q already configured for provider %s; remove it first or pass --force", src.Name, spec.TargetProvider),
		}
	}
	plan := Plan{
		Spec:          spec,
		SourceSummary: fmt.Sprintf("%s MCP %q (%s)", spec.SourceProvider, src.Name, src.Transport),
		TargetSummary: fmt.Sprintf("%s/%s/%s", spec.TargetProvider, scope, src.Name),
	}
	plan.ProviderOps = append(plan.ProviderOps, ProviderOp{
		Kind:         OpAddMCP,
		MCPName:      src.Name,
		MCPTransport: src.Transport,
		MCPCommand:   src.Command,
		MCPArgs:      append([]string(nil), src.Args...),
		MCPURL:       src.URL,
		MCPScope:     scope,
	})
	return plan
}

func planPlugin(snap Snapshot, spec Spec, _ BuildOpts) Plan {
	src, ok := snap.FindPlugin(spec.SubjectID, spec.SourceProvider)
	if !ok {
		return Plan{Spec: spec, Conflict: ConflictMissing, ConflictMsg: "source plugin not found"}
	}
	if src.Marketplace == "" {
		return Plan{
			Spec:        spec,
			Conflict:    ConflictUnsupported,
			ConflictMsg: fmt.Sprintf("plugin %q has no marketplace; can't be promoted across providers", src.Name),
		}
	}
	if !spec.Force && snap.HasPlugin(src.Name, spec.TargetProvider) {
		return Plan{
			Spec:        spec,
			Conflict:    ConflictDuplicate,
			ConflictMsg: fmt.Sprintf("plugin %q already installed for provider %s; remove it first or pass --force", src.Name, spec.TargetProvider),
		}
	}
	plan := Plan{
		Spec:          spec,
		SourceSummary: fmt.Sprintf("%s plugin %q from marketplace %s", spec.SourceProvider, src.Name, src.Marketplace),
		TargetSummary: fmt.Sprintf("%s install of %s@%s", spec.TargetProvider, src.Name, src.Marketplace),
	}
	plan.ProviderOps = append(plan.ProviderOps, ProviderOp{
		Kind:          OpInstallPlugin,
		PluginRefName: src.Name,
		PluginRefMP:   src.Marketplace,
	})
	return plan
}
