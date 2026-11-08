// Package activity turns pull request comments, reviews and commits
// into a filtered list of human activity.
package activity

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fergalhk/agent-config/internal/github"
)

// Event is one item of activity on a pull request.
// State is set only for reviews.
type Event struct {
	Kind   string
	Author string
	Body   string
	State  string
	At     time.Time
}

// LastBotCommit returns the latest author date of a commit by bot.
// It returns the zero time when bot has no commits.
func LastBotCommit(commits []github.Commit, bot string) time.Time {
	var last time.Time
	for _, c := range commits {
		if c.AuthorLogin == bot && c.AuthorDate.After(last) {
			last = c.AuthorDate
		}
	}
	return last
}

// Collect converts comments, reviews and commits into events.
// A commit has no message in the github package, so its body is the SHA.
func Collect(comments, reviewComments []github.Comment, reviews []github.Review, commits []github.Commit) []Event {
	var events []Event
	for _, c := range comments {
		events = append(events, Event{Kind: "comment", Author: c.Author, Body: c.Body, At: c.CreatedAt})
	}
	for _, c := range reviewComments {
		events = append(events, Event{Kind: "review-comment", Author: c.Author, Body: c.Body, At: c.CreatedAt})
	}
	for _, r := range reviews {
		events = append(events, Event{Kind: "review", Author: r.Author, Body: r.Body, State: r.State, At: r.SubmittedAt})
	}
	for _, c := range commits {
		events = append(events, Event{Kind: "commit", Author: c.AuthorLogin, Body: c.SHA, At: c.AuthorDate})
	}
	return events
}

// Filter keeps human events that happened after the given time.
// It drops empty approvals and sorts the result by time, earliest first.
func Filter(events []Event, bot string, after time.Time) []Event {
	var kept []Event
	for _, e := range events {
		if e.Author == bot || !e.At.After(after) {
			continue
		}
		if e.Kind == "review" && e.State == "APPROVED" && e.Body == "" {
			continue
		}
		kept = append(kept, e)
	}
	sort.SliceStable(kept, func(i, j int) bool {
		return kept[i].At.Before(kept[j].At)
	})
	return kept
}

// Format renders each event as a heading line, its body and a blank line.
func Format(events []Event) string {
	var b strings.Builder
	for _, e := range events {
		fmt.Fprintf(&b, "### %s by %s at %s", e.Kind, e.Author, e.At.Format(time.RFC3339))
		if e.Kind == "review" {
			fmt.Fprintf(&b, " (%s)", e.State)
		}
		b.WriteString("\n")
		b.WriteString(e.Body)
		b.WriteString("\n\n")
	}
	return b.String()
}
