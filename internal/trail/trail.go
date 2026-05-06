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
// schema_version returns an error. klim owns this on-disk format,
// so we do not need lenient forward-compat.
package trail

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/klim/internal/registry"
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
// Note: per-binary paths are NOT recorded in the trail at all. Paths
// are machine-specific (e.g. `C:\Users\alice\...`), so storing them
// would either prevent cross-machine dedupe or — worse — let stale
// paths from the first capture survive deduped re-captures and
// mislead `klim trail show`. Per-binary paths still live in the
// regular catalog/scan output (`klim list`, `klim info`); the trail
// captures only the env-defining content.
type Snapshot struct {
	SchemaVersion int    `yaml:"schema_version" json:"schema_version"`
	OS            string `yaml:"os"             json:"os"`
	Arch          string `yaml:"arch"           json:"arch"`
	Tools         []Tool `yaml:"tools"          json:"tools"`
}

// Tool is the trail-internal projection of a registry.Tool. Only the
// content-defining fields (Name + Version + Source) are kept so the
// stored body matches the canonical hash and remains correct under
// dedupe and cross-machine sharing.
type Tool struct {
	Name    string `yaml:"name"              json:"name"`
	Version string `yaml:"version,omitempty" json:"version,omitempty"`
	Source  string `yaml:"source,omitempty"  json:"source,omitempty"`
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

// hashSnapshot returns the ObjectID for a canonical Snapshot. The body
// bytes are exactly what's stored on disk under objects/<aa>/<bb...>.yaml,
// so reading an object back and re-hashing the bytes yields the same
// ObjectID — which is what `decodeSnapshot` relies on to verify
// content-addressed integrity.
func hashSnapshot(s Snapshot) (ObjectID, []byte, error) {
	body, err := marshalSnapshot(s)
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(body)
	return ObjectID(hex.EncodeToString(sum[:])), body, nil
}
