package plan

import (
	"time"

	"github.com/nassiharel/klim/internal/registry"
)

// Per-source rough wall-clock estimates. These are deliberately
// pessimistic — users prefer "took less time than expected" over
// being surprised by a slow run. Values come from empirical
// observations on warm caches; cold-cache runs will exceed them.
var (
	defaultInstallTime = map[registry.InstallSource]time.Duration{
		registry.SourceBrew:   30 * time.Second,
		registry.SourceWinget: 25 * time.Second,
		registry.SourceChoco:  35 * time.Second,
		registry.SourceScoop:  15 * time.Second,
		registry.SourceApt:    10 * time.Second,
		registry.SourceSnap:   30 * time.Second,
		registry.SourceNPM:    8 * time.Second,
	}
	defaultUpgradeTime = map[registry.InstallSource]time.Duration{
		registry.SourceBrew:   20 * time.Second,
		registry.SourceWinget: 20 * time.Second,
		registry.SourceChoco:  25 * time.Second,
		registry.SourceScoop:  10 * time.Second,
		registry.SourceApt:    8 * time.Second,
		registry.SourceSnap:   25 * time.Second,
		registry.SourceNPM:    6 * time.Second,
	}
	defaultRemoveTime = map[registry.InstallSource]time.Duration{
		registry.SourceBrew:   5 * time.Second,
		registry.SourceWinget: 8 * time.Second,
		registry.SourceChoco:  10 * time.Second,
		registry.SourceScoop:  3 * time.Second,
		registry.SourceApt:    5 * time.Second,
		registry.SourceSnap:   8 * time.Second,
		registry.SourceNPM:    3 * time.Second,
	}
)

// Per-source rough disk deltas in MB. Same caveat as the time
// estimates — empirical, pessimistic, and meant to give the user a
// ballpark rather than a precise number.
var (
	defaultInstallMB = map[registry.InstallSource]int{
		registry.SourceBrew:   200,
		registry.SourceWinget: 300,
		registry.SourceChoco:  250,
		registry.SourceScoop:  100,
		registry.SourceApt:    50,
		registry.SourceSnap:   250,
		registry.SourceNPM:    30,
	}
	defaultUpgradeMB = map[registry.InstallSource]int{
		registry.SourceBrew:   50,
		registry.SourceWinget: 50,
		registry.SourceChoco:  60,
		registry.SourceScoop:  30,
		registry.SourceApt:    10,
		registry.SourceSnap:   60,
		registry.SourceNPM:    10,
	}
	defaultRemoveMB = map[registry.InstallSource]int{
		registry.SourceBrew:   100,
		registry.SourceWinget: 150,
		registry.SourceChoco:  120,
		registry.SourceScoop:  50,
		registry.SourceApt:    25,
		registry.SourceSnap:   150,
		registry.SourceNPM:    20,
	}
)

func timeFor(kind ChangeKind, src registry.InstallSource) time.Duration {
	var m map[registry.InstallSource]time.Duration
	switch kind {
	case ChangeInstall:
		m = defaultInstallTime
	case ChangeUpgrade:
		m = defaultUpgradeTime
	case ChangeRemove:
		m = defaultRemoveTime
	}
	if d, ok := m[src]; ok {
		return d
	}
	// Unknown source — assume a middle-of-the-road 20s install.
	return 20 * time.Second
}

func diskFor(kind ChangeKind, src registry.InstallSource) int {
	var m map[registry.InstallSource]int
	switch kind {
	case ChangeInstall:
		m = defaultInstallMB
	case ChangeUpgrade:
		m = defaultUpgradeMB
	case ChangeRemove:
		m = defaultRemoveMB
	}
	if v, ok := m[src]; ok {
		return v
	}
	return 100
}
