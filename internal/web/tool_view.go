package web

import (
	"sort"
	"strings"

	"github.com/nassiharel/clim/internal/recommend"
	"github.com/nassiharel/clim/internal/registry"
)

// buildPMRows lists every package manager the tool has a package id
// for, plus an availability flag so the UI can tell the user "winget
// installed" vs "winget configured but not installed locally". This
// mirrors the TUI's per-tool Package Managers table.
func buildPMRows(t registry.Tool) []pmRow {
	rows := []pmRow{}
	// AllPMStatusForOS returns every source relevant to the host OS
	// plus an availability flag, so we can list configured-but-
	// unavailable sources alongside ready-to-use ones.
	for _, st := range registry.AllPMStatusForOS() {
		args := t.Packages.InstallArgs(st.Source)
		if args == nil {
			continue
		}
		rows = append(rows, pmRow{
			Source:     string(st.Source),
			PackageID:  packageIDFor(t.Packages, st.Source),
			Available:  st.Available,
			InstallCmd: strings.Join(args, " "),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Source < rows[j].Source })
	return rows
}

// packageIDFor reads the package id for source out of PackageIDs.
// PackageIDs.pkgID() is unexported, so we mirror its switch here.
func packageIDFor(p registry.PackageIDs, source registry.InstallSource) string {
	switch source {
	case registry.SourceWinget:
		return p.Winget
	case registry.SourceChoco:
		return p.Choco
	case registry.SourceScoop:
		return p.Scoop
	case registry.SourceBrew:
		return p.Brew
	case registry.SourceApt:
		return p.Apt
	case registry.SourceSnap:
		return p.Snap
	case registry.SourceNPM:
		return p.NPM
	}
	return ""
}

// mergedTagsAndTopics returns a sorted, deduped list of the tool's
// tags plus its GitHub topics. The TUI's About section shows them in
// one merged list and that's what users actually want to filter on.
func mergedTagsAndTopics(t registry.Tool) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, tag := range t.Tags {
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	if t.GitHubInfo != nil {
		for _, topic := range t.GitHubInfo.Topics {
			if topic == "" {
				continue
			}
			if _, ok := seen[topic]; ok {
				continue
			}
			seen[topic] = struct{}{}
			out = append(out, topic)
		}
	}
	sort.Strings(out)
	return out
}

// buildRelatedTools returns "you might also like" candidates for the
// detail page. We feed Compute() the same catalog so the scoring
// matches /foryou; then keep only entries that share at least one tag
// or topic with the focus tool, and aren't already installed.
//
// Limit to 5 to keep the section a sensible "next thing to look at"
// rather than a full search result.
func buildRelatedTools(focus registry.Tool, tools []registry.Tool, favs map[string]bool) []relatedTool {
	if len(tools) == 0 {
		return nil
	}
	focusTags := tagSet(focus)
	if len(focusTags) == 0 {
		return nil
	}
	all := recommend.Compute(tools)
	out := make([]relatedTool, 0, 5)
	for _, r := range all {
		if r.ToolIdx < 0 || r.ToolIdx >= len(tools) {
			continue
		}
		t := tools[r.ToolIdx]
		if strings.EqualFold(t.Name, focus.Name) {
			continue
		}
		if !sharesTag(focusTags, t) {
			continue
		}
		out = append(out, relatedTool{
			Name:        t.Name,
			DisplayName: t.DisplayName,
			Category:    t.Category,
			Description: r.Description,
			Stars:       r.Stars,
			MatchPct:    r.MatchPct,
			Reason:      r.Reason,
			IsFavorite:  favs[t.Name],
		})
		if len(out) >= 5 {
			break
		}
	}
	return out
}

func tagSet(t registry.Tool) map[string]struct{} {
	out := map[string]struct{}{}
	for _, tag := range t.Tags {
		if tag != "" {
			out[strings.ToLower(tag)] = struct{}{}
		}
	}
	if t.GitHubInfo != nil {
		for _, topic := range t.GitHubInfo.Topics {
			if topic != "" {
				out[strings.ToLower(topic)] = struct{}{}
			}
		}
	}
	return out
}

func sharesTag(want map[string]struct{}, t registry.Tool) bool {
	for _, tag := range t.Tags {
		if _, ok := want[strings.ToLower(tag)]; ok {
			return true
		}
	}
	if t.GitHubInfo != nil {
		for _, topic := range t.GitHubInfo.Topics {
			if _, ok := want[strings.ToLower(topic)]; ok {
				return true
			}
		}
	}
	return false
}
