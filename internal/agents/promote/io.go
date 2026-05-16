package promote

import "os"

// readFileBridge funnels os.ReadFile through a single point so the
// rest of the package can avoid importing os.
func readFileBridge(path string) ([]byte, error) { return os.ReadFile(path) }
