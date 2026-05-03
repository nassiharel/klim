package trail

import "sort"

// DiffResult describes the change set between two Snapshots.
//
// All four collections are tool-keyed and sorted by name for stable output.
// Platform changes (OS or Arch) are surfaced explicitly so a diff between
// captures from different machines doesn't silently report "no changes" —
// trail history is explicitly designed to be sync/copy-friendly across
// machines, so the diff must reflect the platform delta.
type DiffResult struct {
	Added          []Tool          `yaml:"added"                     json:"added"`
	Removed        []Tool          `yaml:"removed"                   json:"removed"`
	VersionChanged []VersionChange `yaml:"version_changed"           json:"version_changed"`
	SourceChanged  []SourceChange  `yaml:"source_changed"            json:"source_changed"`
	PlatformChange *PlatformChange `yaml:"platform_change,omitempty" json:"platform_change,omitempty"`
}

// PlatformChange records the OS/Arch delta between two snapshots when at
// least one of those fields differs. nil on DiffResult means the
// platforms match.
type PlatformChange struct {
	FromOS   string `yaml:"from_os"   json:"from_os"`
	ToOS     string `yaml:"to_os"     json:"to_os"`
	FromArch string `yaml:"from_arch" json:"from_arch"`
	ToArch   string `yaml:"to_arch"   json:"to_arch"`
}

// VersionChange records a tool whose version differs between two snapshots.
type VersionChange struct {
	Name   string `yaml:"name" json:"name"`
	From   string `yaml:"from" json:"from"`
	To     string `yaml:"to"   json:"to"`
	Source string `yaml:"source,omitempty" json:"source,omitempty"`
}

// SourceChange records a tool that switched install source. When the tool
// also bumped its version in the same step, FromVersion / ToVersion carry
// that delta — recording only the source switch would under-report common
// migrations like `winget@1.2 → brew@1.3`.
type SourceChange struct {
	Name        string `yaml:"name"                   json:"name"`
	From        string `yaml:"from"                   json:"from"`
	To          string `yaml:"to"                     json:"to"`
	FromVersion string `yaml:"from_version,omitempty" json:"from_version,omitempty"`
	ToVersion   string `yaml:"to_version,omitempty"   json:"to_version,omitempty"`
}

// Diff returns the change set going from snapshot a to snapshot b.
// Snapshots are looked up by refSpec — see Resolve for accepted forms.
//
// Both lookups are performed under a single trail lock so the result is
// consistent under concurrent writes (a concurrent prune that drops the
// older object after we've resolved a but before we've resolved b would
// otherwise corrupt the diff).
func Diff(a, b string) (*DiffResult, error) {
	r, err := resolveRoots()
	if err != nil {
		return nil, err
	}
	release, err := acquireLock(r.lock)
	if err != nil {
		return nil, err
	}
	defer release()

	snapA, err := resolveAndLoad(r, a)
	if err != nil {
		return nil, err
	}
	snapB, err := resolveAndLoad(r, b)
	if err != nil {
		return nil, err
	}
	out := diffSnapshots(snapA, snapB)
	return &out, nil
}

// resolveAndLoad combines resolveSpec+readObject under an existing lock.
// Caller must hold the trail lock.
func resolveAndLoad(r fsRoots, refSpec string) (*Snapshot, error) {
	lf, err := loadLog(r)
	if err != nil {
		return nil, err
	}
	entry, err := resolveSpec(lf, refSpec)
	if err != nil {
		return nil, err
	}
	return readObject(r, entry.Object)
}

// diffSnapshots is the pure diff helper used by both Diff and capture
// summarization.
func diffSnapshots(a, b *Snapshot) DiffResult {
	out := DiffResult{
		Added:          []Tool{},
		Removed:        []Tool{},
		VersionChanged: []VersionChange{},
		SourceChanged:  []SourceChange{},
	}
	if a == nil && b == nil {
		return out
	}
	if a != nil && b != nil {
		if a.OS != b.OS || a.Arch != b.Arch {
			out.PlatformChange = &PlatformChange{
				FromOS: a.OS, ToOS: b.OS,
				FromArch: a.Arch, ToArch: b.Arch,
			}
		}
	}
	aMap := indexBySource(a)
	bMap := indexBySource(b)

	// Added / version-changed / source-changed: walk b.
	for key, bt := range bMap {
		at, ok := aMap[key]
		if !ok {
			// Same name might exist under another source — treat as source change.
			if other, otherKey := findByName(aMap, bt.Name); other != nil {
				out.SourceChanged = append(out.SourceChanged, SourceChange{
					Name:        bt.Name,
					From:        other.Source,
					To:          bt.Source,
					FromVersion: other.Version,
					ToVersion:   bt.Version,
				})
				delete(aMap, otherKey) // mark consumed so it isn't reported as Removed
				continue
			}
			out.Added = append(out.Added, bt)
			continue
		}
		if at.Version != bt.Version {
			out.VersionChanged = append(out.VersionChanged, VersionChange{
				Name: bt.Name, From: at.Version, To: bt.Version, Source: bt.Source,
			})
		}
		delete(aMap, key)
	}
	// Anything left in aMap was removed.
	for _, at := range aMap {
		out.Removed = append(out.Removed, at)
	}

	sort.SliceStable(out.Added, func(i, j int) bool { return out.Added[i].Name < out.Added[j].Name })
	sort.SliceStable(out.Removed, func(i, j int) bool { return out.Removed[i].Name < out.Removed[j].Name })
	sort.SliceStable(out.VersionChanged, func(i, j int) bool { return out.VersionChanged[i].Name < out.VersionChanged[j].Name })
	sort.SliceStable(out.SourceChanged, func(i, j int) bool { return out.SourceChanged[i].Name < out.SourceChanged[j].Name })
	return out
}

func indexBySource(s *Snapshot) map[string]Tool {
	m := make(map[string]Tool)
	if s == nil {
		return m
	}
	for _, t := range s.Tools {
		m[t.Name+"\x00"+t.Source] = t
	}
	return m
}

// findByName returns the first tool in m whose Name matches.
func findByName(m map[string]Tool, name string) (*Tool, string) {
	for k, t := range m {
		if t.Name == name {
			tt := t
			return &tt, k
		}
	}
	return nil, ""
}
