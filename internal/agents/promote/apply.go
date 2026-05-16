package promote

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Executor is the runtime interface a Plan needs to be applied.
// It's a slim adapter over a real *agents.Service so the promote
// package never imports the agents tree directly (which would create
// a cycle with the snapshot type the planner uses).
type Executor interface {
	// AddMCP routes to the target provider's AddMCP. The op carries
	// every field needed.
	AddMCP(ctx context.Context, providerID string, op ProviderOp) error
	// InstallPlugin routes to the target provider's InstallPlugin.
	InstallPlugin(ctx context.Context, providerID string, op ProviderOp) error
}

// Apply runs the plan's file copies and provider ops in order. Any
// failure aborts the rest of the plan and returns immediately so the
// user gets a clear partial-state message (we don't try to roll back
// — that's a complication better solved at a higher level).
func (p Plan) Apply(ctx context.Context, ex Executor) error {
	if p.Conflict != ConflictNone {
		return fmt.Errorf("promote: %s — %s", p.Conflict, p.ConflictMsg)
	}
	for _, fc := range p.FileCopies {
		if err := applyFileCopy(fc); err != nil {
			return fmt.Errorf("file copy %s -> %s: %w", fc.Src, fc.Dst, err)
		}
	}
	for _, op := range p.ProviderOps {
		switch op.Kind {
		case OpAddMCP:
			if err := ex.AddMCP(ctx, p.Spec.TargetProvider, op); err != nil {
				return fmt.Errorf("add MCP %q: %w", op.MCPName, err)
			}
		case OpInstallPlugin:
			if err := ex.InstallPlugin(ctx, p.Spec.TargetProvider, op); err != nil {
				return fmt.Errorf("install plugin %q: %w", op.PluginRefName, err)
			}
		default:
			return fmt.Errorf("unknown provider op kind %d", op.Kind)
		}
	}
	return nil
}

func applyFileCopy(fc FileCopy) error {
	if fc.Dst == "" {
		return errors.New("destination path is empty")
	}
	if fc.MkdirOK {
		if err := os.MkdirAll(filepath.Dir(fc.Dst), 0o755); err != nil {
			return err
		}
	}
	mode := os.FileMode(fc.Mode)
	if mode == 0 {
		mode = 0o644
	}
	if fc.Body != nil {
		return atomicWrite(fc.Dst, fc.Body, mode)
	}
	srcF, err := os.Open(fc.Src)
	if err != nil {
		return err
	}
	defer func() { _ = srcF.Close() }()
	tmp := fc.Dst + ".klim-tmp"
	dstF, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dstF, srcF); err != nil {
		_ = dstF.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := dstF.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, fc.Dst)
}

func atomicWrite(path string, body []byte, mode os.FileMode) error {
	tmp := path + ".klim-tmp"
	if err := os.WriteFile(tmp, body, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
