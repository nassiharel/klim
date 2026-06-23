// Command gen-docs generates klim's shell completions and man pages into the
// given output directory. It is run at release time by GoReleaser (and can be
// run locally) so the artifacts can be packaged into archives and Linux
// packages. Not shipped in the klim binary itself.
//
// The output directory MUST live outside GoReleaser's managed `dist/`
// directory: GoReleaser cleans `dist/` and then asserts it is empty after
// the `before` hooks run, so a hook writing into `dist/` makes the release
// fail with "dist is not empty". We default to `extras/` at the repo root.
//
// Usage:
//
//	go run ./cmd/gen-docs -o extras
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nassiharel/klim/internal/cli"
)

func main() {
	out := flag.String("o", "extras", "output directory for completions/ and man/")
	flag.Parse()

	completionsDir := filepath.Join(*out, "completions")
	manDir := filepath.Join(*out, "man")

	if err := cli.GenCompletions(completionsDir); err != nil {
		fmt.Fprintln(os.Stderr, "gen-docs: completions:", err)
		os.Exit(1)
	}
	if err := cli.GenManTree(manDir); err != nil {
		fmt.Fprintln(os.Stderr, "gen-docs: man pages:", err)
		os.Exit(1)
	}
	fmt.Printf("Wrote completions to %s and man pages to %s\n", completionsDir, manDir)
}
