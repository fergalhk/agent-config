package update

import (
	"fmt"

	"github.com/fergalhk/agent-config/internal/claude"
	"github.com/fergalhk/agent-config/internal/decide"
)

// RunSummary returns the comment that the job posts on an existing PR after a run.
func RunSummary(outcome decide.Outcome, upstream string, events int, result claude.Result) string {
	return fmt.Sprintf("The update job ran (%s: %s).\nUpstream commit: %s.\nNew human events: %d.\nPR title: %s",
		outcome.Action, outcome.Reason, shortSHA(upstream), events, result.Title)
}
