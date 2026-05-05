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
// Determinism guarantees:
//   - The clone's Tools/Favorites/Packs/PackageManagers are produced
//     by canonicalize() before hashing — sorted, deduped, with
//     pack tool lists also sorted+deduped.
//   - yaml.v3 marshals maps with sorted keys, so PackageManagers
//     stays stable across runs.
func ComputeHash(p *Profile) string {
	if p == nil {
		return ""
	}
	clone := *p
	canonicalize(&clone)
	clone.Hash = ""
	clone.GeneratedAt = time.Time{}

	data, err := yaml.Marshal(&clone)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:12]
}

// canonicalize normalises in-place the slices/maps that drive the
// hash so semantically-equivalent profiles produce identical bytes.
// Idempotent.
func canonicalize(p *Profile) {
	p.Favorites = dedupSorted(p.Favorites)
	p.Tools = sortToolsByName(p.Tools)
	for i := range p.Packs {
		p.Packs[i].Tools = dedupSorted(p.Packs[i].Tools)
	}
	sortPacksByName(p.Packs)
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

func sortToolsByName(tools []Tool) []Tool {
	if len(tools) == 0 {
		return tools
	}
	sort.SliceStable(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools
}

func sortPacksByName(packs []Pack) {
	sort.SliceStable(packs, func(i, j int) bool { return packs[i].Name < packs[j].Name })
}
