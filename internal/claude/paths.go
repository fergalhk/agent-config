package claude

import (
	"fmt"
	"path"
	"strings"
)

// CheckChangedPaths returns an error that lists every path that Claude Code
// may not change. It allows skills/<skill>/... and docs/agents/skills/<skill>.md.
func CheckChangedPaths(changed []string, skill string) error {
	allowedDir := "skills/" + skill + "/"
	allowedNotes := "docs/agents/skills/" + skill + ".md"

	var rejected []string
	for _, p := range changed {
		c := path.Clean(p)
		if c == allowedNotes || strings.HasPrefix(c, allowedDir) {
			continue
		}
		rejected = append(rejected, p)
	}
	if len(rejected) > 0 {
		return fmt.Errorf("claude changed files outside the allowed paths: %s", strings.Join(rejected, ", "))
	}
	return nil
}
