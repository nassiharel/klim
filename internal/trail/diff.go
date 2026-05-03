package trail

import "sort"

// DiffResult describes the change set between two Snapshots.
//
// All four collections are tool-keyed and sorted by name for stable output.
type DiffResult struct {
	Added          []Tool          `yaml:"added"            json:"added"`
	Removed        []Tool          `yaml:"removed"          json:"removed"`
	VersionChanged []VersionChange `yaml:"version_changed"  json:"version_changed"`
	SourceChanged  []SourceChange  `yaml:"source_changed"   json:"source_changed"`
}

// VersionChange records a tool whose version differs between two snapshots.
type VersionChange struct {
	Name   string `yaml:"name" json:"name"`
	From   string `yaml:"from" json:"from"`
	To     string `yaml:"to"   json:"to"`
	Source string `yaml:"source,omitempty" json:"source,omitempty"`
}

// SourceChange records a tool that switched install source.
type SourceChange struct {
	Name string `yaml:"name" json:"name"`
	From string `yaml:"from" json:"from"`
	To   string `yaml:"to"   json:"to"`
}

// Diff returns the change set going from snapshot a to snapshot b.
// Snapshots are looked up by refSpec — see Resolve for accepted forms.
func Diff(a, b string) (*DiffResult, error) {
	snapA, _, err := Show(a)
	if err != nil {
		return nil, err
	}
	snapB, _, err := Show(b)
	if err != nil {
		return nil, err
	}
	out := diffSnapshots(snapA, snapB)
	return &out, nil
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
	aMap := indexBySource(a)
	bMap := indexBySource(b)

	// Added / version-changed / source-changed: walk b.
	for key, bt := range bMap {
		at, ok := aMap[key]
		if !ok {
			// Same name might exist under another source — treat as source change.
			if other, otherKey := findByName(aMap, bt.Name); other != nil {
				out.SourceChanged = append(out.SourceChanged, SourceChange{
					Name: bt.Name, From: other.Source, To: bt.Source,
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
