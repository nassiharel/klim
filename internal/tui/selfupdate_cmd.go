package tui

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/nassiharel/klim/internal/build"
	"github.com/nassiharel/klim/internal/selfupdate"
)

// selfUpdateCheckMsg arrives after a CheckOnly self-update probe.
// `background` distinguishes the startup auto-check from a user-
// initiated `u` press on the Config tab: background results land in
// the persistent title-bar hint, user results land in the status
// line and announce verbose results regardless of state.
type selfUpdateCheckMsg struct {
	current    string
	latest     string
	available  bool
	devBuild   bool
	background bool
	err        error
}

// checkSelfUpdateCmd queries the configured release endpoint for the
// latest klim release. CheckOnly so the running binary is never
// touched — the TUI only surfaces the result and lets the user run
// `klim update` from a fresh shell when they're ready.
func checkSelfUpdateCmd() tea.Cmd {
	return func() tea.Msg {
		current := build.VersionOnly()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		res, err := selfupdate.Update(ctx, current, &selfupdate.Options{CheckOnly: true})
		if err != nil {
			if errors.Is(err, selfupdate.ErrDevBuild) {
				return selfUpdateCheckMsg{current: current, devBuild: true}
			}
			return selfUpdateCheckMsg{current: current, err: err}
		}
		return selfUpdateCheckMsg{
			current:   res.CurrentVersion,
			latest:    res.LatestVersion,
			available: res.UpdateAvailable(),
		}
	}
}
