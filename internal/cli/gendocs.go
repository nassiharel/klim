package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra/doc"
)

// GenManTree writes man pages for klim and every subcommand into dir.
// Used at release time (see cmd/gen-docs) to package man pages into archives
// and Linux packages. dir is created if it does not exist.
func GenManTree(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating man dir: %w", err)
	}
	header := &doc.GenManHeader{
		Title:   "KLIM",
		Section: "1",
		Source:  "klim",
		Manual:  "klim Manual",
	}
	return doc.GenManTree(rootCmd, header, dir)
}

// GenCompletions writes bash, zsh, fish, and powershell completion scripts into
// dir. Filenames follow the conventions each shell's completion loader expects.
func GenCompletions(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating completions dir: %w", err)
	}
	// Fixed order so generated artifacts are reproducible run-to-run.
	writers := []struct {
		name string
		gen  func(io.Writer) error
	}{
		{"klim.bash", func(w io.Writer) error { return rootCmd.GenBashCompletionV2(w, true) }},
		{"klim.zsh", func(w io.Writer) error { return rootCmd.GenZshCompletion(w) }},
		{"klim.fish", func(w io.Writer) error { return rootCmd.GenFishCompletion(w, true) }},
		{"klim.ps1", func(w io.Writer) error { return rootCmd.GenPowerShellCompletionWithDesc(w) }},
	}
	for _, c := range writers {
		if err := writeFile(filepath.Join(dir, c.name), c.gen); err != nil {
			return fmt.Errorf("generating %s: %w", c.name, err)
		}
	}
	return nil
}

func writeFile(path string, gen func(io.Writer) error) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	return gen(f)
}
