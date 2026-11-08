// Package decide chooses the update action for one skill.
// It does not read or write anything. It only applies the rules.
package decide

// Action is the step the update job takes for one skill.
type Action int

const (
	// Stop means the job does nothing for the skill.
	Stop Action = iota
	// Create means the job opens a new pull request.
	Create
	// Update means the job updates the existing pull request branch.
	Update
)

// String returns the lowercase name of the action.
func (a Action) String() string {
	switch a {
	case Stop:
		return "stop"
	case Create:
		return "create"
	case Update:
		return "update"
	}
	return "unknown"
}

// Input holds the facts the decision needs.
type Input struct {
	Upstream    string
	StoredSHA   string
	HasPR       bool
	BranchSHA   string
	HasActivity bool
}

// Outcome is the decision for one skill.
type Outcome struct {
	Action Action
	OldSHA string
	Reason string
}

// Decide returns the action for one skill.
func Decide(in Input) Outcome {
	if !in.HasPR {
		if in.Upstream == in.StoredSHA {
			return Outcome{Action: Stop, Reason: "upstream did not move"}
		}
		return Outcome{Action: Create, OldSHA: in.StoredSHA, Reason: "upstream moved and no PR exists"}
	}
	if in.BranchSHA == in.Upstream && !in.HasActivity {
		return Outcome{Action: Stop, Reason: "branch is current and there is no activity"}
	}
	return Outcome{Action: Update, OldSHA: in.BranchSHA, Reason: "branch needs an update"}
}
