package trader

import "os"

// KillFileTripped reports whether the kill-switch file exists. An empty path
// disables the file switch (DB halt flag still applies separately).
func KillFileTripped(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}
