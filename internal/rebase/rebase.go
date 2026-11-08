// Package rebase brings a pull request branch up to date with its base
// through the GitHub API.
package rebase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fergalhk/agent-config/internal/github"
)

// ErrTimeout means the branch head did not change before MaxWait passed.
var ErrTimeout = errors.New("rebase did not finish before the time limit")

// Options sets the polling behavior. Zero values select the defaults.
type Options struct {
	PollInterval time.Duration // default 5s
	MaxWait      time.Duration // default 3m
	Sleep        func(time.Duration)
}

// Rebase asks GitHub to rebase the PR branch onto its base. It then polls
// until the branch head changes. It returns the new head SHA.
// If the branch is not behind its base, it returns the current head SHA.
// A merge conflict gives an error that matches github.ErrConflict.
func Rebase(ctx context.Context, api github.API, pr github.PR, opt Options) (string, error) {
	if opt.PollInterval == 0 {
		opt.PollInterval = 5 * time.Second
	}
	if opt.MaxWait == 0 {
		opt.MaxWait = 3 * time.Minute
	}
	if opt.Sleep == nil {
		opt.Sleep = time.Sleep
	}

	behind, err := api.CompareBehindBy(ctx, pr.BaseRef, pr.HeadRef)
	if err != nil {
		return "", fmt.Errorf("compare %s with %s: %w", pr.HeadRef, pr.BaseRef, err)
	}
	if behind == 0 {
		return pr.HeadSHA, nil
	}

	if err := api.UpdatePullRequestBranch(ctx, pr.NodeID); err != nil {
		return "", err
	}

	// Sum the sleep times. This keeps the time limit deterministic.
	var elapsed time.Duration
	for {
		head, err := api.BranchHead(ctx, pr.HeadRef)
		if err != nil {
			return "", fmt.Errorf("read head of %s: %w", pr.HeadRef, err)
		}
		if head != pr.HeadSHA {
			return head, nil
		}
		if elapsed >= opt.MaxWait {
			return "", ErrTimeout
		}
		opt.Sleep(opt.PollInterval)
		elapsed += opt.PollInterval
	}
}
