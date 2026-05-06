package envid

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ComputeHash returns a 12-character hex prefix of SHA256 over the
// canonical, deterministic encoding of p with Hash and GeneratedAt
// blanked out. Two profiles that differ only in capture time share a
// hash, which makes "are these the same env?" cheap and stable.
//
// ComputeHash is side-effect free: the input *p is never mutated.
// We deep-copy slice/map fields before canonicalize because the
// canonicalize helpers rewrite Tools, Packs, Favorites, and each
// Pack.Tools list.
//
// Determinism guarantees:
//   - The deep clone's Tools/Favorites/Packs are produced by
//     canonicalize() before hashing — sorted, deduped, with pack
//     tool lists also sorted+deduped.
//   - PackageManagers is a map[string]bool; yaml.v3 marshals maps
//     with sorted keys, so the encoding is stable across runs
//     without canonicalize touching the map.
func ComputeHash(p *Profile) string {
	if p == nil {
		return ""
	}
	clone := deepClone(p)
	canonicalize(clone)
	clone.Hash = ""
	clone.GeneratedAt = time.Time{}

	data, err := yaml.Marshal(clone)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:12]
}

// deepClone returns a copy of p with new backing arrays/maps for
// every slice and map field, so subsequent in-place mutation of the
// clone never affects the caller. ClimInfo, OSInfo, Security, and
// VerdictsCounts are value types — copied as part of the struct
// literal — and need no further treatment.
func deepClone(p *Profile) *Profile {
	if p == nil {
		return nil
	}
	out := *p

	if p.Tools != nil {
		out.Tools = append([]Tool(nil), p.Tools...)
	}
	if p.Favorites != nil {
		out.Favorites = append([]string(nil), p.Favorites...)
	}
	if p.Packs != nil {
		out.Packs = make([]Pack, len(p.Packs))
		for i, pk := range p.Packs {
			out.Packs[i] = pk
			if pk.Tools != nil {
				out.Packs[i].Tools = append([]string(nil), pk.Tools...)
			}
		}
	}
	if p.PackageManagers != nil {
		out.PackageManagers = make(map[string]bool, len(p.PackageManagers))
		for k, v := range p.PackageManagers {
			out.PackageManagers[k] = v
		}
	}
	return &out
}

// canonicalize normalises in-place the slices/maps that drive the
// hash so semantically-equivalent profiles produce identical bytes.
// Idempotent.
//
// MUTATES the receiver: Tools, Favorites, Packs, and each
// Pack.Tools are rewritten. Callers (notably ComputeHash) must
// either own the Profile already or pass a deep clone.
func canonicalize(p *Profile) {
	p.Favorites = dedupSorted(p.Favorites)
	p.Tools = dedupSortToolsByName(p.Tools)
	p.Packs = dedupSortPacksByName(p.Packs)
}

func dedupSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// dedupSortToolsByName trims Name whitespace, drops empty-name
// entries, sorts by Name, and deduplicates (first occurrence
// wins). A profile containing duplicates or whitespace-only names
// (manual edit, malformed token, future merge bug) shouldn't
// perturb the hash or smuggle ghost installs through the apply
// flow.
func dedupSortToolsByName(tools []Tool) []Tool {
	if len(tools) == 0 {
		return tools
	}
	cleaned := make([]Tool, 0, len(tools))
	for _, t := range tools {
		t.Name = strings.TrimSpace(t.Name)
		if t.Name == "" {
			continue
		}
		cleaned = append(cleaned, t)
	}
	sort.SliceStable(cleaned, func(i, j int) bool { return cleaned[i].Name < cleaned[j].Name })
	out := cleaned[:0]
	var prev string
	for i, t := range cleaned {
		if i > 0 && t.Name == prev {
			continue
		}
		prev = t.Name
		out = append(out, t)
	}
	return out
}

// dedupSortPacksByName trims pack and tool names, drops packs
// whose name or tool list is empty after trimming, sorts by Name
// (case-insensitive), and dedupes by name. Mirrors
// dedupSortToolsByName so canonicalize handles every user-
// editable list with the same hygiene.
func dedupSortPacksByName(packs []Pack) []Pack {
	if len(packs) == 0 {
		return packs
	}
	cleaned := make([]Pack, 0, len(packs))
	for _, p := range packs {
		p.Name = strings.TrimSpace(p.Name)
		p.DisplayName = strings.TrimSpace(p.DisplayName)
		if p.Name == "" {
			continue
		}
		p.Tools = dedupSorted(p.Tools)
		if len(p.Tools) == 0 {
			continue
		}
		cleaned = append(cleaned, p)
	}
	sort.SliceStable(cleaned, func(i, j int) bool {
		return strings.ToLower(cleaned[i].Name) < strings.ToLower(cleaned[j].Name)
	})
	out := cleaned[:0]
	var prev string
	for i, p := range cleaned {
		key := strings.ToLower(p.Name)
		if i > 0 && key == prev {
			continue
		}
		prev = key
		out = append(out, p)
	}
	return out
}

// (sortPacksByName has been folded into dedupSortPacksByName in
// hash.go; kept history note here in case grep traces back.)
