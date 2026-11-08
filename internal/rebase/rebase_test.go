package rebase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fergalhk/agent-config/internal/github"
)

type fakeAPI struct {
	behind      int
	updateErr   error
	updateCalls int
	heads       []string // returned in order, the last one repeats
	headCalls   int
}

func (f *fakeAPI) FindPRs(context.Context, string) ([]github.PR, error) {
	panic("fakeAPI: FindPRs not expected")
}
func (f *fakeAPI) CreatePR(context.Context, string, string, string, string) (*github.PR, error) {
	panic("fakeAPI: CreatePR not expected")
}
func (f *fakeAPI) UpdatePR(context.Context, int, string, string) error {
	panic("fakeAPI: UpdatePR not expected")
}
func (f *fakeAPI) ReopenPR(context.Context, int) error { panic("fakeAPI: ReopenPR not expected") }
func (f *fakeAPI) ListIssueComments(context.Context, int) ([]github.Comment, error) {
	panic("fakeAPI: ListIssueComments not expected")
}
func (f *fakeAPI) ListReviewComments(context.Context, int) ([]github.Comment, error) {
	panic("fakeAPI: ListReviewComments not expected")
}
func (f *fakeAPI) ListReviews(context.Context, int) ([]github.Review, error) {
	panic("fakeAPI: ListReviews not expected")
}
func (f *fakeAPI) ListPRCommits(context.Context, int) ([]github.Commit, error) {
	panic("fakeAPI: ListPRCommits not expected")
}
func (f *fakeAPI) CreateComment(context.Context, int, string) error {
	panic("fakeAPI: CreateComment not expected")
}
func (f *fakeAPI) GetPR(context.Context, int) (*github.PR, error) {
	panic("fakeAPI: GetPR not expected")
}
func (f *fakeAPI) BranchHead(context.Context, string) (string, error) {
	i := f.headCalls
	if i >= len(f.heads) {
		i = len(f.heads) - 1
	}
	f.headCalls++
	return f.heads[i], nil
}
func (f *fakeAPI) CompareBehindBy(context.Context, string, string) (int, error) {
	return f.behind, nil
}
func (f *fakeAPI) UpdatePullRequestBranch(context.Context, string) error {
	f.updateCalls++
	return f.updateErr
}

var testPR = github.PR{NodeID: "node1", HeadRef: "feature", HeadSHA: "old", BaseRef: "main"}

func TestRebase_UpToDate(t *testing.T) {
	api := &fakeAPI{behind: 0}
	got, err := Rebase(context.Background(), api, testPR, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "old" {
		t.Errorf("got %q, want old", got)
	}
	if api.updateCalls != 0 {
		t.Errorf("mutation called %d times", api.updateCalls)
	}
}

func TestRebase_PollsUntilHeadMoves(t *testing.T) {
	api := &fakeAPI{behind: 2, heads: []string{"old", "old", "new"}}
	var sleeps []time.Duration
	got, err := Rebase(context.Background(), api, testPR, Options{
		Sleep: func(d time.Duration) { sleeps = append(sleeps, d) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "new" {
		t.Errorf("got %q, want new", got)
	}
	if len(sleeps) != 2 {
		t.Errorf("got %d sleeps, want 2", len(sleeps))
	}
}

func TestRebase_Conflict(t *testing.T) {
	api := &fakeAPI{behind: 1, updateErr: github.ErrConflict, heads: []string{"new"}}
	_, err := Rebase(context.Background(), api, testPR, Options{})
	if !errors.Is(err, github.ErrConflict) {
		t.Fatalf("got %v, want ErrConflict", err)
	}
	if api.headCalls != 0 {
		t.Errorf("polled %d times", api.headCalls)
	}
}

func TestRebase_Timeout(t *testing.T) {
	api := &fakeAPI{behind: 1, heads: []string{"old"}}
	var clock time.Duration
	_, err := Rebase(context.Background(), api, testPR, Options{
		PollInterval: time.Millisecond,
		MaxWait:      10 * time.Millisecond,
		Sleep:        func(d time.Duration) { clock += d },
	})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("got %v, want ErrTimeout", err)
	}
	if clock != 10*time.Millisecond {
		t.Errorf("slept %v, want 10ms", clock)
	}
}
