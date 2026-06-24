// Package costs aggregates per-session token usage across agent
// providers. Each Provider that wants to participate implements a
// TokenSamples method that returns a flat list of TokenSample records
// (one per assistant message it can parse out of the local transcripts).
// The Build function rolls those samples up into a Report scoped to a
// requested time range, which the TUI's Costs sub-tab renders.
//
// klim does not call provider APIs to fetch token counts — everything
// is parsed locally from the on-disk transcripts the agent CLIs
// already write. Missing fields are tolerated; a sample with zero
// tokens contributes nothing to the totals.
package costs

import (
	"sort"
	"time"
)

// ProviderID is duplicated here to avoid an import cycle with the
// parent agents package. Callers pass through the agents.ProviderID
// value as a string.
type ProviderID string

// TokenSample is one assistant-turn worth of usage emitted by a
// transcript parser. Every field is best-effort: an unknown Model is
// the empty string, and zero tokens means "the event didn't report
// usage". Day is truncated to the local-time midnight so range
// aggregations don't have to think about timezones.
type TokenSample struct {
	Provider  ProviderID
	SessionID string
	Title     string // optional — used to label TopSessions
	Model     string
	Day       time.Time
	Input     int
	Output    int
}

// Totals carries the input/output sum for a slice/group of samples.
type Totals struct {
	Input  int
	Output int
}

// ScanResult is what a provider returns from TokenSamples. It supports
// incremental scanning: the provider checks each transcript's mtime
// against Prior and only parses files that are new or changed.
//
//   - Samples holds freshly-parsed samples for changed/new transcripts
//     only (unchanged files are NOT re-read).
//   - Seen maps every session id that exists on disk to its current
//     transcript mtime — including the unchanged ones the provider
//     skipped. Callers use it to (a) keep cached entries for skipped
//     sessions and (b) prune cache entries whose session vanished.
type ScanResult struct {
	Samples []TokenSample
	Seen    map[string]time.Time
}

// ScanInput carries the prior per-session transcript mtimes into a
// provider scan so it can skip files that haven't changed. A nil/empty
// Prior forces a full parse (cold start).
type ScanInput struct {
	Prior map[string]time.Time
}

// Total returns input + output as a single number for ranking.
func (t Totals) Total() int { return t.Input + t.Output }

// Range names the time windows the Report supports. Today, 7d, 30d
// are all rolling windows ending now; AllTime is exactly that.
type Range int

// Range values.
const (
	RangeToday Range = iota
	Range7d
	Range30d
	RangeAllTime
	RangeCount
)

// Label returns a short human label for a Range.
func (r Range) Label() string {
	switch r {
	case RangeToday:
		return "Today"
	case Range7d:
		return "7 days"
	case Range30d:
		return "30 days"
	case RangeAllTime:
		return "All time"
	}
	return "?"
}

// Days returns the size of the sparkline window for this range. All
// time is capped at 30 buckets so it stays renderable.
func (r Range) Days() int {
	switch r {
	case RangeToday:
		return 1
	case Range7d:
		return 7
	case Range30d, RangeAllTime:
		return 30
	}
	return 7
}

// Report is the aggregated view of token usage scoped to a Range.
//
// ByProvider / ByModel are total-input + total-output for that key.
// TopSessions is sorted descending by total token count, capped to a
// reasonable display size by the caller.
// DailySparkline is one bucket per Range.Days() entries; element zero
// is the oldest day, the last element is today. Empty buckets are 0.
type Report struct {
	Range          Range
	Totals         Totals
	ByProvider     map[ProviderID]Totals
	ByModel        map[string]Totals
	TopSessions    []SessionCost
	DailySparkline []int
	Days           int // length of DailySparkline (always == Range.Days())
}

// SessionCost is one row in the TopSessions table.
type SessionCost struct {
	SessionID string
	Provider  ProviderID
	Title     string
	Model     string
	Totals    Totals
}

// startOfLocalDay returns the local-time midnight for t. Using
// time.Truncate(24h) bites in non-UTC timezones because it truncates
// against the zero time in UTC — startOfLocalDay produces the
// correct local-day boundary instead.
func startOfLocalDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// Build aggregates the supplied samples within the requested range. A
// nil or empty samples slice produces an empty (but non-nil) report so
// callers don't have to nil-check every field.
func Build(samples []TokenSample, r Range) Report {
	now := time.Now()
	cutoff := rangeStart(now, r)
	rep := Report{
		Range:      r,
		ByProvider: map[ProviderID]Totals{},
		ByModel:    map[string]Totals{},
		Days:       r.Days(),
	}
	rep.DailySparkline = make([]int, rep.Days)

	todayStart := startOfLocalDay(now)

	// Group per-session running totals so TopSessions reflects in-range
	// usage only.
	sess := map[string]*SessionCost{}

	for _, s := range samples {
		if s.Day.Before(cutoff) {
			continue
		}
		// Spark bucket — local-day diff so timezones don't shift the
		// histogram by one bucket.
		dayDiff := int(todayStart.Sub(startOfLocalDay(s.Day)).Hours() / 24)
		idx := rep.Days - 1 - dayDiff
		if idx >= 0 && idx < rep.Days {
			rep.DailySparkline[idx] += s.Input + s.Output
		}
		// Totals
		rep.Totals.Input += s.Input
		rep.Totals.Output += s.Output
		// Per provider / model.
		p := rep.ByProvider[s.Provider]
		p.Input += s.Input
		p.Output += s.Output
		rep.ByProvider[s.Provider] = p
		modelKey := string(s.Provider) + "/" + s.Model
		m := rep.ByModel[modelKey]
		m.Input += s.Input
		m.Output += s.Output
		rep.ByModel[modelKey] = m
		// Per-session running totals.
		sc, ok := sess[s.SessionID]
		if !ok {
			sc = &SessionCost{
				SessionID: s.SessionID,
				Provider:  s.Provider,
				Title:     s.Title,
				Model:     s.Model,
			}
			sess[s.SessionID] = sc
		}
		sc.Totals.Input += s.Input
		sc.Totals.Output += s.Output
		if sc.Title == "" {
			sc.Title = s.Title
		}
		if sc.Model == "" {
			sc.Model = s.Model
		}
	}

	// Sort sessions desc by total tokens.
	rep.TopSessions = make([]SessionCost, 0, len(sess))
	for _, sc := range sess {
		if sc.Totals.Total() == 0 {
			continue
		}
		rep.TopSessions = append(rep.TopSessions, *sc)
	}
	sort.Slice(rep.TopSessions, func(i, j int) bool {
		return rep.TopSessions[i].Totals.Total() > rep.TopSessions[j].Totals.Total()
	})
	return rep
}

// rangeStart returns the inclusive start time for a Range relative to now.
func rangeStart(now time.Time, r Range) time.Time {
	switch r {
	case RangeToday:
		return startOfLocalDay(now)
	case Range7d:
		return startOfLocalDay(now).AddDate(0, 0, -6)
	case Range30d:
		return startOfLocalDay(now).AddDate(0, 0, -29)
	}
	// AllTime — return Go's zero value; everything is after this.
	return time.Time{}
}
