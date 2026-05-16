package tui

import (
	"context"
	"errors"
	"time"

	"github.com/nassiharel/klim/internal/build"
	"github.com/nassiharel/klim/internal/fileutil"
	"github.com/nassiharel/klim/internal/paths"
	"github.com/nassiharel/klim/internal/selfupdate"

	tea "charm.land/bubbletea/v2"
)

// selfUpdateCache persists the last update-check result so klim
// doesn't hit GitHub on every launch. The 24h TTL strikes a balance
// between freshness (users see new versions within a day) and
// politeness (no per-launch API calls).
type selfUpdateCache struct {
	Version        int       `yaml:"version"`
	CheckedAt      time.Time `yaml:"checked_at"`
	CurrentVersion string    `yaml:"current_version,omitempty"`
	LatestVersion  string    `yaml:"latest_version,omitempty"`
	Available      bool      `yaml:"available,omitempty"`
}

// selfUpdateCheckTTL is how long a cached check is considered fresh.
const selfUpdateCheckTTL = 24 * time.Hour

// loadSelfUpdateCache reads the cached check result. Returns
// (nil, false) when the file is missing or unreadable so callers
// treat that as "no cache, trigger a fresh check".
func loadSelfUpdateCache() (*selfUpdateCache, bool) {
	path, err := selfUpdateCachePath()
	if err != nil {
		return nil, false
	}
	c := &selfUpdateCache{}
	found, err := fileutil.ReadYAML(path, c)
	if err != nil || !found {
		return nil, false
	}
	return c, true
}

func saveSelfUpdateCache(c *selfUpdateCache) error {
	if c == nil {
		return nil
	}
	path, err := selfUpdateCachePath()
	if err != nil {
		return err
	}
	c.Version = 1
	return fileutil.WriteYAML(path, c, "# klim self-update check cache - auto-generated\n")
}

func selfUpdateCachePath() (string, error) {
	return paths.Join("cache", "selfupdate.yaml")
}

// backgroundSelfUpdateCheck runs a non-intrusive self-update check at
// startup. If the cache is fresh (within TTL) it returns the cached
// result without hitting the network; otherwise it queries the
// release endpoint and caches the answer.
//
// The result is the same selfUpdateCheckMsg used by the user-driven
// check, but with `background=true` so the message handler routes it
// to the title-bar hint slot instead of the status line.
func backgroundSelfUpdateCheck() tea.Cmd {
	return func() tea.Msg {
		current := build.VersionOnly()
		// PR #77 review: actually short-circuit on dev builds rather
		// than discovering them by ErrDevBuild after a full network
		// round-trip. selfupdate.Update treats VersionOnly()=="dev"
		// as a dev build, so we can pre-empt the call here.
		if current == "dev" {
			return selfUpdateCheckMsg{current: current, devBuild: true, background: true}
		}
		if c, ok := loadSelfUpdateCache(); ok && time.Since(c.CheckedAt) < selfUpdateCheckTTL {
			return selfUpdateCheckMsg{
				current:    c.CurrentVersion,
				latest:     c.LatestVersion,
				available:  c.Available,
				background: true,
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		res, err := selfupdate.Update(ctx, current, &selfupdate.Options{CheckOnly: true})
		if err != nil {
			if errors.Is(err, selfupdate.ErrDevBuild) {
				return selfUpdateCheckMsg{current: current, devBuild: true, background: true}
			}
			// Background failures are silent — surfacing every network
			// hiccup at startup would be more annoying than helpful.
			return selfUpdateCheckMsg{current: current, err: err, background: true}
		}
		msg := selfUpdateCheckMsg{
			current:    res.CurrentVersion,
			latest:     res.LatestVersion,
			available:  res.UpdateAvailable(),
			background: true,
		}
		// Cache regardless of result so we don't re-probe for 24h.
		_ = saveSelfUpdateCache(&selfUpdateCache{
			CheckedAt:      time.Now(),
			CurrentVersion: msg.current,
			LatestVersion:  msg.latest,
			Available:      msg.available,
		})
		return msg
	}
}
