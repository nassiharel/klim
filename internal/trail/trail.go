// Package trail records every change to the local toolchain as a
// content-addressed environment snapshot, exposing git-style history
// inspection (capture, log, show, diff, prune).
//
// Storage layout under TrailDir():
//
//	HEAD              ASCII line: integer index of newest entry, or "-1".
//	log.yaml          ordered list of Entry records (newest at the end of the slice).
//	log.lock          advisory cross-process lock guarding mutations to HEAD/log.
//	objects/<aa>/<bb...>.yaml  content-addressed Snapshot bodies (sha256 hex).
//
// Two-type model:
//
//   - Snapshot is the canonical, deduplicable content of an
//     environment (sorted tools + os/arch + schema_version).
//   - Entry is one record in the linear history that points at a
//     Snapshot by its ObjectID. Multiple entries can share an
//     Object — that's how identical-environment captures dedupe.
//
// Reads are strict: any unknown YAML field or unknown
// schema_version returns an error. clim owns this on-disk format,
// so we do not need lenient forward-compat.
package trail

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/registry"
)

// SchemaVersion is the on-disk format version. Bumped on
// backwards-incompatible changes; readers reject unknown values.
const SchemaVersion = 1

// ObjectID is the content hash of a canonical Snapshot body.
type ObjectID string

// Short returns the conventional 7-char hash prefix (git-style).
func (o ObjectID) Short() string {
	if len(o) <= 7 {
		return string(o)
	}
	return string(o[:7])
}

// IsValid reports whether o is a 64-char lowercase hex string.
func (o ObjectID) IsValid() bool {
	if len(o) != 64 {
		return false
	}
	for _, c := range o {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		default:
			return false
		}
	}
	return true
}

// Snapshot is the canonical environment content.
//
// Two snapshots with the same OS/Arch and the same set of installed
// tools (by Name + Version + Source) hash to the same ObjectID, which
// is why Tools must be sorted before marshalling.
//
// Note: Tool.Path is intentionally NOT part of the canonical content —
// paths are machine-specific (e.g. `C:\Users\alice\...`), so including
// them would prevent dedupe across machines and break future cross-host
// scenarios (clim sync). Path lives on the Tool struct because it's
// useful in `show` output for forensic / revert work, but it is hashed
// only via the per-instance fields below.
type Snapshot struct {
	SchemaVersion int    `yaml:"schema_version" json:"schema_version"`
	OS            string `yaml:"os"             json:"os"`
	Arch          string `yaml:"arch"           json:"arch"`
	Tools         []Tool `yaml:"tools"          json:"tools"`
}

// Tool is the trail-internal projection of a registry.Tool. Name +
// Version + Source define content (and participate in hashing); Path is
// metadata for inspection and is excluded from canonical hashing.
type Tool struct {
	Name    string `yaml:"name"              json:"name"`
	Version string `yaml:"version,omitempty" json:"version,omitempty"`
	Source  string `yaml:"source,omitempty"  json:"source,omitempty"`
	Path    string `yaml:"path,omitempty"    json:"path,omitempty"`
}

// Operation kinds for entries.
const (
	OpCapture = "capture"
	OpImport  = "import"
	OpInstall = "install"
	OpUpgrade = "upgrade"
	OpRemove  = "remove"
)

// Entry is one record in the linear trail history. Multiple Entries
// can reference the same Object — that's how identical environments
// captured at different times dedupe storage.
type Entry struct {
	Index     int       `yaml:"index"             json:"index"`
	Object    ObjectID  `yaml:"object"            json:"object"`
	Time      time.Time `yaml:"time"              json:"time"`
	Operation string    `yaml:"op"                json:"op"`
	Label     string    `yaml:"label,omitempty"   json:"label,omitempty"`
	Summary   string    `yaml:"summary,omitempty" json:"summary,omitempty"`
}

// canonicalSnapshot returns a Snapshot with deterministic field order
// and tool sorting. The output bytes are content-hashable.
func canonicalSnapshot(osName, arch string, tools []registry.Tool) Snapshot {
	out := Snapshot{
		SchemaVersion: SchemaVersion,
		OS:            osName,
		Arch:          arch,
	}
	for _, t := range tools {
		if !t.IsInstalled() {
			continue
		}
		// PrimaryInstance returns the PATH-precedence instance; trail records
		// only the primary so the env is reproducible from the user's shell.
		primary := t.PrimaryInstance()
		st := Tool{Name: t.Name}
		if primary != nil {
			st.Version = primary.Version
			st.Source = string(primary.Source)
			st.Path = primary.Path
		}
		out.Tools = append(out.Tools, st)
	}
	sort.SliceStable(out.Tools, func(i, j int) bool {
		if out.Tools[i].Name != out.Tools[j].Name {
			return out.Tools[i].Name < out.Tools[j].Name
		}
		return out.Tools[i].Source < out.Tools[j].Source
	})
	return out
}

// marshalSnapshot returns the canonical YAML bytes of s. Used both for
// hashing (input must be sorted by canonicalSnapshot) and for storage.
func marshalSnapshot(s Snapshot) ([]byte, error) {
	data, err := yaml.Marshal(&s)
	if err != nil {
		return nil, fmt.Errorf("marshalling snapshot: %w", err)
	}
	return data, nil
}

// hashSnapshot returns the ObjectID for a canonical Snapshot. Identical
// environments produce identical hashes regardless of capture time AND
// regardless of per-machine binary paths (Path is excluded from hashing
// even though it's stored in the Snapshot body for inspection).
func hashSnapshot(s Snapshot) (ObjectID, []byte, error) {
	body, err := marshalSnapshot(s)
	if err != nil {
		return "", nil, err
	}
	hashed := stripPathsForHash(s)
	hashedBody, err := marshalSnapshot(hashed)
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(hashedBody)
	return ObjectID(hex.EncodeToString(sum[:])), body, nil
}

// stripPathsForHash returns a copy of s with Tool.Path zeroed so that
// content hashing is path-independent. The returned value is only used
// for hashing; storage retains the original (path-bearing) Snapshot.
func stripPathsForHash(s Snapshot) Snapshot {
	tools := make([]Tool, len(s.Tools))
	for i, t := range s.Tools {
		t.Path = ""
		tools[i] = t
	}
	return Snapshot{
		SchemaVersion: s.SchemaVersion,
		OS:            s.OS,
		Arch:          s.Arch,
		Tools:         tools,
	}
}
