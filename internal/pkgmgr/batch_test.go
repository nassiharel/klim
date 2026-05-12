package pkgmgr

import (
	"strings"
	"testing"
)

// realisticWingetList mirrors the layout `winget list` actually
// emits: Name, Id, Version, Available, Source columns with
// whitespace-aligned content. The "Available" column is blank for
// most rows; the test exercises both the present-Available and
// missing-Available cases to make sure Version slicing doesn't
// overrun into Source.
const realisticWingetList = `Name                     Id                              Version       Available Source
-----------------------------------------------------------------------------------------------
Microsoft.Edge           Microsoft.Edge                  126.0.2592.81           winget
Mozilla.Firefox          Mozilla.Firefox                 127.0.2       128.0.0   winget
Git.Git                  Git.Git                         2.45.2                  winget
`

const realisticWingetUpgrade = `Name              Id                    Version       Available     Source
-------------------------------------------------------------------------------
Mozilla.Firefox   Mozilla.Firefox       127.0.2       128.0.0       winget
Git.Git           Git.Git               2.45.2        2.46.0        winget
`

func TestParseWingetTable_VersionDoesNotSwallowSource(t *testing.T) {
	got := map[string]string{}
	parseWingetTable(realisticWingetList, got)

	want := map[string]string{
		"Microsoft.Edge":  "126.0.2592.81",
		"Mozilla.Firefox": "127.0.2",
		"Git.Git":         "2.45.2",
	}
	if len(got) != len(want) {
		t.Fatalf("want %d entries, got %d (%v)", len(want), len(got), got)
	}
	for id, v := range want {
		gotV, ok := got[id]
		if !ok {
			t.Errorf("missing %q in parsed output", id)
			continue
		}
		if gotV != v {
			t.Errorf("%s: got %q want %q", id, gotV, v)
		}
		// Most importantly: the version must NOT contain spaces or
		// the trailing "winget" source column.
		if strings.Contains(gotV, " ") || strings.Contains(gotV, "winget") {
			t.Errorf("%s: version %q swallowed an adjacent column", id, gotV)
		}
	}
}

func TestParseWingetUpgradeTable_AvailableDoesNotSwallowSource(t *testing.T) {
	got := map[string]string{}
	parseWingetUpgradeTable(realisticWingetUpgrade, got)

	want := map[string]string{
		"Mozilla.Firefox": "128.0.0",
		"Git.Git":         "2.46.0",
	}
	if len(got) != len(want) {
		t.Fatalf("want %d entries, got %d (%v)", len(want), len(got), got)
	}
	for id, v := range want {
		gotV, ok := got[id]
		if !ok {
			t.Errorf("missing %q in parsed output", id)
			continue
		}
		if gotV != v {
			t.Errorf("%s: got %q want %q", id, gotV, v)
		}
		if strings.Contains(gotV, " ") || strings.Contains(gotV, "winget") {
			t.Errorf("%s: available %q swallowed an adjacent column", id, gotV)
		}
	}
}

func TestWingetColumnRanges_BoundsByFullHeaderSet(t *testing.T) {
	header := "Name              Id                    Version       Available     Source"
	ranges := wingetColumnRanges(header+"\n----", "Id", "Version")
	if ranges == nil {
		t.Fatalf("expected non-nil ranges for valid header")
	}
	if len(ranges) != 2 {
		t.Fatalf("expected 2 ranges, got %d", len(ranges))
	}
	// Version's end must stop at the "Available" header (not at end-of-line).
	verEnd := ranges[1][1]
	available := strings.Index(header, "Available")
	if verEnd != available {
		t.Errorf("Version's end-bound should be the Available column start %d, got %d", available, verEnd)
	}
}

func TestParseWingetTable_HandlesEmptyAvailableColumn(t *testing.T) {
	// A row with an empty Available column (the usual case for
	// up-to-date tools). Version must still parse correctly.
	out := `Name           Id              Version       Available Source
---------------------------------------------------------------------
Foo.Bar        Foo.Bar         1.2.3                   winget
`
	got := map[string]string{}
	parseWingetTable(out, got)
	if got["Foo.Bar"] != "1.2.3" {
		t.Errorf("Foo.Bar: got %q want %q", got["Foo.Bar"], "1.2.3")
	}
}
