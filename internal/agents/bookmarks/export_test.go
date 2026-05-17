package bookmarks

import (
	"github.com/nassiharel/klim/internal/fileutil"
	"github.com/nassiharel/klim/internal/paths"
)

func pathsAgentBookmarksLegacy() (string, error) { return paths.AgentBookmarksLegacy() }
func fileutilWriteYAML(path string, v any) error {
	return fileutil.WriteYAML(path, v, "# legacy klim agent session bookmarks (test fixture)\n")
}
