package update

import (
	"strings"
	"testing"

	"github.com/fergalhk/agent-config/internal/claude"
	"github.com/fergalhk/agent-config/internal/decide"
)

func TestRunSummary_NamesUpstreamAndEvents(t *testing.T) {
	outcome := decide.Outcome{Action: decide.Update, Reason: "branch needs an update"}
	out := RunSummary(outcome, newSHA, 3, claude.Result{Title: "chore: apply upstream", Body: "b"})
	for _, want := range []string{"2222222", "events: 3", "chore: apply upstream"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary %q does not contain %q", out, want)
		}
	}
}
