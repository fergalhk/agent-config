package activity

import (
	"testing"
	"time"

	"github.com/fergalhk/agent-config/internal/github"
)

const testBot = "agent-bot"

var t0 = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func TestLastBotCommit(t *testing.T) {
	commits := []github.Commit{
		{SHA: "a", AuthorLogin: testBot, AuthorDate: t0},
		{SHA: "b", AuthorLogin: "alice", AuthorDate: t0.Add(2 * time.Hour)},
		{SHA: "c", AuthorLogin: testBot, AuthorDate: t0.Add(time.Hour)},
	}
	if got := LastBotCommit(commits, testBot); !got.Equal(t0.Add(time.Hour)) {
		t.Errorf("LastBotCommit = %v, want %v", got, t0.Add(time.Hour))
	}
	if got := LastBotCommit(commits[1:2], testBot); !got.IsZero() {
		t.Errorf("LastBotCommit with no bot commits = %v, want zero time", got)
	}
}

func TestFilter_DropsBotAndEarlier(t *testing.T) {
	events := []Event{
		{Kind: "comment", Author: testBot, Body: "bot", At: t0.Add(time.Hour)},
		{Kind: "comment", Author: "alice", Body: "before", At: t0.Add(-time.Hour)},
		{Kind: "comment", Author: "alice", Body: "at", At: t0},
		{Kind: "comment", Author: "alice", Body: "after", At: t0.Add(time.Hour)},
	}
	got := Filter(events, testBot, t0)
	if len(got) != 1 || got[0].Body != "after" {
		t.Errorf("Filter = %+v, want only the event with body %q", got, "after")
	}
}

func TestFilter_DropsEmptyApproval(t *testing.T) {
	events := []Event{
		{Kind: "review", Author: "alice", State: "APPROVED", Body: "", At: t0.Add(time.Hour)},
		{Kind: "review", Author: "alice", State: "APPROVED", Body: "looks fine, but...", At: t0.Add(2 * time.Hour)},
		{Kind: "review", Author: "alice", State: "CHANGES_REQUESTED", Body: "", At: t0.Add(3 * time.Hour)},
	}
	got := Filter(events, testBot, t0)
	if len(got) != 2 {
		t.Fatalf("Filter kept %d events, want 2: %+v", len(got), got)
	}
	if got[0].Body != "looks fine, but..." || got[1].State != "CHANGES_REQUESTED" {
		t.Errorf("Filter kept the wrong events: %+v", got)
	}
}

func TestFilter_SortsByTime(t *testing.T) {
	events := []Event{
		{Kind: "comment", Author: "alice", Body: "second", At: t0.Add(2 * time.Hour)},
		{Kind: "comment", Author: "alice", Body: "first", At: t0.Add(time.Hour)},
	}
	got := Filter(events, testBot, t0)
	if len(got) != 2 || got[0].Body != "first" || got[1].Body != "second" {
		t.Errorf("Filter order = %+v, want first then second", got)
	}
}

func TestCollect_MapsReviewState(t *testing.T) {
	reviews := []github.Review{
		{Author: "alice", Body: "please fix", State: "CHANGES_REQUESTED", SubmittedAt: t0},
	}
	got := Collect(nil, nil, reviews, nil)
	if len(got) != 1 {
		t.Fatalf("Collect returned %d events, want 1", len(got))
	}
	e := got[0]
	if e.Kind != "review" || e.State != "CHANGES_REQUESTED" || e.Body != "please fix" {
		t.Errorf("Collect = %+v, want review with state CHANGES_REQUESTED and body %q", e, "please fix")
	}
}

func TestFormat(t *testing.T) {
	events := []Event{
		{Kind: "review", Author: "alice", Body: "please fix", State: "CHANGES_REQUESTED", At: t0},
		{Kind: "comment", Author: "bob", Body: "thanks", At: t0.Add(time.Hour)},
	}
	want := "### review by alice at 2026-11-01T12:00:00Z (CHANGES_REQUESTED)\n" +
		"please fix\n\n" +
		"### comment by bob at 2026-11-01T13:00:00Z\n" +
		"thanks\n\n"
	if got := Format(events); got != want {
		t.Errorf("Format =\n%q\nwant\n%q", got, want)
	}
}
