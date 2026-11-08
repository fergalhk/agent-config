package decide

import "testing"

func TestDecide(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		want Outcome
	}{
		{
			name: "no PR and upstream unchanged stops",
			in:   Input{Upstream: "abc", StoredSHA: "abc"},
			want: Outcome{Action: Stop, Reason: "upstream did not move"},
		},
		{
			name: "no PR and upstream moved creates",
			in:   Input{Upstream: "def", StoredSHA: "abc"},
			want: Outcome{Action: Create, OldSHA: "abc", Reason: "upstream moved and no PR exists"},
		},
		{
			name: "PR, branch current, no activity stops",
			in:   Input{Upstream: "abc", HasPR: true, BranchSHA: "abc"},
			want: Outcome{Action: Stop, Reason: "branch is current and there is no activity"},
		},
		{
			name: "PR, branch current, with activity updates",
			in:   Input{Upstream: "abc", HasPR: true, BranchSHA: "abc", HasActivity: true},
			want: Outcome{Action: Update, OldSHA: "abc", Reason: "branch needs an update"},
		},
		{
			name: "PR, branch behind, no activity updates",
			in:   Input{Upstream: "def", HasPR: true, BranchSHA: "abc"},
			want: Outcome{Action: Update, OldSHA: "abc", Reason: "branch needs an update"},
		},
		{
			name: "PR, branch behind, with activity updates",
			in:   Input{Upstream: "def", HasPR: true, BranchSHA: "abc", HasActivity: true},
			want: Outcome{Action: Update, OldSHA: "abc", Reason: "branch needs an update"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Decide(tt.in)
			if got != tt.want {
				t.Fatalf("Decide(%+v) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestActionString(t *testing.T) {
	want := map[Action]string{Stop: "stop", Create: "create", Update: "update"}
	for a, s := range want {
		if got := a.String(); got != s {
			t.Errorf("Action(%d).String() = %q, want %q", a, got, s)
		}
	}
}
