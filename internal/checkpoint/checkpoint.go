// Package checkpoint captures named snapshots of the installed
// toolchain so they can be rolled back to later. A Checkpoint is a
// declarative record (tools + versions + sources, plus the captured
// $PATH) — not a binary backup — so rollback is just a plan that
// drives the user back to the saved versions through the same PM
// commands `klim install` / `klim upgrade` already use.
//
// Storage layout:
//
//	~/.klim/checkpoints/<name>.yaml
//
// Names are validated to be safe filename components (no path
// separators, no leading dots) so a user can never accidentally
// write a checkpoint outside the dedicated directory.
package checkpoint

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/nassiharel/klim/internal/fileutil"
	"github.com/nassiharel/klim/internal/paths"
	"github.com/nassiharel/klim/internal/registry"
)

// ToolState is one installed tool as it existed at checkpoint time.
type ToolState struct {
	Name        string                 `yaml:"name"`
	DisplayName string                 `yaml:"display_name,omitempty"`
	Version     string                 `yaml:"version,omitempty"`
	Source      registry.InstallSource `yaml:"source,omitempty"`
	Path        string                 `yaml:"path,omitempty"`
}

// Checkpoint is the on-disk format. Schema is small on purpose —
// every field is human-meaningful so users can hand-edit before a
// rollback if they need to.
type Checkpoint struct {
	Name        string      `yaml:"name"`
	Description string      `yaml:"description,omitempty"`
	CreatedAt   time.Time   `yaml:"created_at"`
	GOOS        string      `yaml:"goos"`
	Tools       []ToolState `yaml:"tools"`
	PATH        string      `yaml:"path,omitempty"`
	// File is populated by Load/List; not serialized. Holds the
	// absolute path of the .yaml file on disk so callers can show
	// or delete it.
	File string `yaml:"-"`
}

// Capture turns a tool slice into a Checkpoint. Only installed tools
// land in the snapshot (the point of a rollback is to restore
// installed state). Order is alphabetical for deterministic file
// contents.
func Capture(name, description string, tools []registry.Tool) Checkpoint {
	c := Checkpoint{
		Name:        name,
		Description: description,
		CreatedAt:   time.Now().UTC(),
		GOOS:        runtime.GOOS,
		PATH:        os.Getenv("PATH"),
	}
	for _, t := range tools {
		if !t.IsInstalled() {
			continue
		}
		inst := t.PrimaryInstance()
		state := ToolState{
			Name:        t.Name,
			DisplayName: t.DisplayName,
		}
		if inst != nil {
			state.Version = inst.Version
			state.Source = inst.Source
			state.Path = inst.Path
		}
		c.Tools = append(c.Tools, state)
	}
	sort.Slice(c.Tools, func(i, j int) bool {
		return strings.ToLower(c.Tools[i].Name) < strings.ToLower(c.Tools[j].Name)
	})
	return c
}

// Save writes the checkpoint atomically to ~/.klim/checkpoints/<name>.yaml.
// Returns the absolute path written, or an error.
func Save(c Checkpoint) (string, error) {
	if err := validateName(c.Name); err != nil {
		return "", err
	}
	dir, err := paths.CheckpointsDir()
	if err != nil {
		return "", err
	}
	target := filepath.Join(dir, c.Name+".yaml")
	if err := fileutil.EnsureDir(target); err != nil {
		return "", err
	}
	header := "# klim checkpoint — captured by `klim checkpoint " + c.Name + "`.\n" +
		"# Roll back to it with: klim rollback " + c.Name + "\n"
	if err := fileutil.WriteYAML(target, &c, header); err != nil {
		return "", err
	}
	return target, nil
}

// Load reads a single checkpoint by name. Returns os.ErrNotExist
// when the file isn't on disk so callers can distinguish "no such
// checkpoint" from a parse error.
func Load(name string) (Checkpoint, error) {
	if err := validateName(name); err != nil {
		return Checkpoint{}, err
	}
	dir, err := paths.CheckpointsDir()
	if err != nil {
		return Checkpoint{}, err
	}
	file := filepath.Join(dir, name+".yaml")
	var c Checkpoint
	found, err := fileutil.ReadYAML(file, &c)
	if err != nil {
		return Checkpoint{}, err
	}
	if !found {
		return Checkpoint{}, fmt.Errorf("checkpoint %q: %w", name, os.ErrNotExist)
	}
	c.File = file
	if c.Name == "" {
		c.Name = name
	}
	return c, nil
}

// List returns every checkpoint in the directory, sorted newest
// first. Missing directory is not an error — we return an empty
// slice so the CLI can render "no checkpoints yet" cleanly.
func List() ([]Checkpoint, error) {
	dir, err := paths.CheckpointsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Checkpoint
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".yaml")
		c, err := Load(name)
		if err != nil {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// Delete removes the named checkpoint from disk. Idempotent: deleting
// a checkpoint that doesn't exist returns nil.
func Delete(name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	dir, err := paths.CheckpointsDir()
	if err != nil {
		return err
	}
	target := filepath.Join(dir, name+".yaml")
	if err := os.Remove(target); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return nil
}

// nameRegex enforces filename-safe names: alphanumerics, dashes,
// underscores, and dots — same charset git tag/branch names allow.
// Empty or "." / ".." are rejected so a malicious name can't escape
// the checkpoints directory.
var nameRegex = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func validateName(name string) error {
	if name == "" {
		return errors.New("checkpoint name is required")
	}
	if name == "." || name == ".." {
		return errors.New("checkpoint name must not be \".\" or \"..\"")
	}
	if !nameRegex.MatchString(name) {
		return fmt.Errorf("checkpoint name %q is invalid: use letters, digits, dots, dashes, underscores (max 128 chars, must start with alphanumeric)", name)
	}
	return nil
}
