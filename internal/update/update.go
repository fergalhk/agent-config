// Package update runs the update job for one forked skill. The job applies
// upstream changes to the fork through Claude Code and raises or updates a PR.
package update

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fergalhk/agent-config/internal/activity"
	"github.com/fergalhk/agent-config/internal/claude"
	"github.com/fergalhk/agent-config/internal/config"
	"github.com/fergalhk/agent-config/internal/decide"
	"github.com/fergalhk/agent-config/internal/discover"
	"github.com/fergalhk/agent-config/internal/github"
	"github.com/fergalhk/agent-config/internal/gitx"
	"github.com/fergalhk/agent-config/internal/naming"
	"github.com/fergalhk/agent-config/internal/rebase"
)

// baseBranch is the branch that update PRs target.
const baseBranch = "main"

// Git is the set of local repository operations that the job uses.
// *gitx.Repo satisfies it.
type Git interface {
	Fetch(ctx context.Context) error
	ShowFile(ctx context.Context, ref, path string) (string, error)
	CheckoutNewBranch(ctx context.Context, branch, startPoint string) error
	ChangedPaths(ctx context.Context) ([]string, error)
	CommitWith(ctx context.Context, paths []string, message string, opts gitx.CommitOptions) error
	Push(ctx context.Context, branch string, force bool) error
}

// UpstreamRepo is a clone of the upstream repository. *gitx.Repo satisfies it.
type UpstreamRepo interface {
	LastCommitTouching(ctx context.Context, ref, dir string) (string, error)
	DefaultBranch(ctx context.Context) (string, error)
	ExtractDir(ctx context.Context, sha, dir, dest string) error
	DiffDir(ctx context.Context, old, new, dir string) (string, error)
}

// Deps holds the dependencies of the update job.
type Deps struct {
	API github.API
	// Git is the local checkout of this repository.
	Git Git
	// Upstream clones the upstream repository at url.
	Upstream func(ctx context.Context, url string) (UpstreamRepo, error)
	// Claude runs Claude Code in repoDir with the prompt.
	Claude interface {
		Run(ctx context.Context, repoDir, scratch, prompt string) error
	}
	// Root is the directory of the local checkout.
	Root string
	// Scratch is a directory outside the checkout for the job files.
	Scratch string
	// RunChecks runs the repository checks. It returns an error when a check fails.
	RunChecks func(root string) error
	// Now returns the current time.
	Now func() time.Time
}

// Run runs the update job for one forked skill.
func Run(ctx context.Context, d Deps, skill string) error {
	skills, err := config.LoadSkills(filepath.Join(d.Root, config.SkillsPath))
	if err != nil {
		return err
	}
	entry, ok := skills[skill]
	if !ok || entry.Fork == nil {
		return fmt.Errorf("skill %s is not a forked skill", skill)
	}
	source := entry.Fork.Source
	storedSHA, err := config.ReadSha(d.Root, skill)
	if err != nil {
		return err
	}

	up, err := d.Upstream(ctx, "https://"+source.Repo+".git")
	if err != nil {
		return err
	}
	defaultBranch, err := up.DefaultBranch(ctx)
	if err != nil {
		return fmt.Errorf("default branch of %s: %w", source.Repo, err)
	}
	upstream, err := up.LastCommitTouching(ctx, "origin/"+defaultBranch, source.Directory)
	if err != nil {
		return fmt.Errorf("last commit for %s: %w", source.Directory, err)
	}
	if upstream == "" {
		return fmt.Errorf("no commit changes %s in %s", source.Directory, source.Repo)
	}

	branch := naming.UpdateBranch(skill)
	pr, err := findPR(ctx, d.API, branch)
	if err != nil {
		return err
	}

	var branchSHA string
	var events []activity.Event
	// collectedAt dates the bot commit. Activity after this time counts on the next run.
	var collectedAt time.Time
	if pr != nil {
		branchSHA, err = readBranchSHA(ctx, d.Git, branch, skill)
		if err != nil {
			return err
		}
		collectedAt = d.Now()
		events, err = collectActivity(ctx, d.API, pr.Number)
		if err != nil {
			return err
		}
	}

	outcome := decide.Decide(decide.Input{
		Upstream:    upstream,
		StoredSHA:   storedSHA,
		HasPR:       pr != nil,
		BranchSHA:   branchSHA,
		HasActivity: len(events) > 0,
	})
	if outcome.Action == decide.Stop {
		log.Printf("%s: stop: %s", skill, outcome.Reason)
		return nil
	}
	log.Printf("%s: %s: %s", skill, outcome.Action, outcome.Reason)

	// fail posts a comment on the PR, if one exists, and returns err.
	fail := func(prefix string, err error) error {
		if pr != nil {
			if cerr := d.API.CreateComment(ctx, pr.Number, prefix+err.Error()); cerr != nil {
				return errors.Join(err, fmt.Errorf("comment on PR #%d: %w", pr.Number, cerr))
			}
		}
		return err
	}

	if pr != nil {
		if _, err := rebase.Rebase(ctx, d.API, *pr, rebase.Options{}); err != nil {
			return fail("The rebase of the PR branch failed: ", err)
		}
		if err := d.Git.Fetch(ctx); err != nil {
			return err
		}
		if err := d.Git.CheckoutNewBranch(ctx, branch, "origin/"+branch); err != nil {
			return err
		}
	} else if err := d.Git.CheckoutNewBranch(ctx, branch, "origin/"+baseBranch); err != nil {
		return err
	}

	result, err := runClaude(ctx, d, skill, source, outcome.OldSHA, upstream, events, up)
	if err != nil {
		return fail("The Claude step failed: ", err)
	}

	if err := commit(ctx, d, skill, upstream, branchSHA, collectedAt); err != nil {
		return err
	}
	if err := d.RunChecks(d.Root); err != nil {
		return fail("The checks failed: ", err)
	}
	// With no PR, force the push. The branch may still exist from an earlier PR.
	if err := d.Git.Push(ctx, branch, pr == nil); err != nil {
		return err
	}

	if pr == nil {
		created, err := d.API.CreatePR(ctx, branch, baseBranch, result.Title, result.Body)
		if err != nil {
			return fmt.Errorf("create PR for %s: %w", branch, err)
		}
		log.Printf("%s: created PR #%d", skill, created.Number)
		return nil
	}
	if err := d.API.UpdatePR(ctx, pr.Number, result.Title, result.Body); err != nil {
		return fmt.Errorf("update PR #%d: %w", pr.Number, err)
	}
	if err := d.API.CreateComment(ctx, pr.Number, RunSummary(outcome, upstream, len(events), result)); err != nil {
		return fmt.Errorf("comment on PR #%d: %w", pr.Number, err)
	}
	log.Printf("%s: updated PR #%d", skill, pr.Number)
	return nil
}

// SkillForPR returns the skill name for the PR with the given number.
// It returns false when the PR head is not an update branch.
func SkillForPR(ctx context.Context, api github.API, number int) (string, bool, error) {
	pr, err := api.GetPR(ctx, number)
	if err != nil {
		return "", false, err
	}
	name, ok := naming.SkillFromUpdateBranch(pr.HeadRef)
	return name, ok, nil
}

// findPR returns the open PR for branch. If no PR is open, it reopens a
// closed unmerged PR and returns it. It returns nil when no such PR exists.
func findPR(ctx context.Context, api github.API, branch string) (*github.PR, error) {
	prs, err := api.FindPRs(ctx, branch)
	if err != nil {
		return nil, fmt.Errorf("find PRs for %s: %w", branch, err)
	}
	for i := range prs {
		if prs[i].State == "open" {
			return &prs[i], nil
		}
	}
	for i := range prs {
		if !prs[i].Merged {
			if err := api.ReopenPR(ctx, prs[i].Number); err != nil {
				return nil, fmt.Errorf("reopen PR #%d: %w", prs[i].Number, err)
			}
			log.Printf("reopened PR #%d", prs[i].Number)
			prs[i].State = "open"
			return &prs[i], nil
		}
	}
	return nil, nil
}

// readBranchSHA reads the recorded upstream commit from the PR branch.
func readBranchSHA(ctx context.Context, git Git, branch, skill string) (string, error) {
	if err := git.Fetch(ctx); err != nil {
		return "", err
	}
	content, err := git.ShowFile(ctx, "origin/"+branch, config.ShaPath(skill))
	if err != nil {
		return "", fmt.Errorf("read %s on %s: %w", config.ShaPath(skill), branch, err)
	}
	return strings.TrimSpace(content), nil
}

// collectActivity returns the human activity on the PR after the last bot commit.
func collectActivity(ctx context.Context, api github.API, number int) ([]activity.Event, error) {
	comments, err := api.ListIssueComments(ctx, number)
	if err != nil {
		return nil, fmt.Errorf("list comments on PR #%d: %w", number, err)
	}
	reviewComments, err := api.ListReviewComments(ctx, number)
	if err != nil {
		return nil, fmt.Errorf("list review comments on PR #%d: %w", number, err)
	}
	reviews, err := api.ListReviews(ctx, number)
	if err != nil {
		return nil, fmt.Errorf("list reviews on PR #%d: %w", number, err)
	}
	commits, err := api.ListPRCommits(ctx, number)
	if err != nil {
		return nil, fmt.Errorf("list commits on PR #%d: %w", number, err)
	}
	events := activity.Collect(comments, reviewComments, reviews, commits)
	return activity.Filter(events, gitx.BotName, activity.LastBotCommit(commits, gitx.BotName)), nil
}

// runClaude writes the job files to the scratch directory, runs Claude Code,
// reads the result file and checks the changed paths.
func runClaude(ctx context.Context, d Deps, skill string, source config.Source, oldSHA, newSHA string, events []activity.Event, up UpstreamRepo) (claude.Result, error) {
	oldDir := filepath.Join(d.Scratch, "old")
	newDir := filepath.Join(d.Scratch, "new")
	diffPath := filepath.Join(d.Scratch, "diff.patch")
	activityPath := filepath.Join(d.Scratch, "activity.md")
	resultPath := filepath.Join(d.Scratch, "result.md")

	if err := up.ExtractDir(ctx, oldSHA, source.Directory, oldDir); err != nil {
		return claude.Result{}, err
	}
	if err := up.ExtractDir(ctx, newSHA, source.Directory, newDir); err != nil {
		return claude.Result{}, err
	}
	diff, err := up.DiffDir(ctx, oldSHA, newSHA, source.Directory)
	if err != nil {
		return claude.Result{}, err
	}
	if err := os.WriteFile(diffPath, []byte(diff), 0o644); err != nil {
		return claude.Result{}, err
	}
	if len(events) > 0 {
		if err := os.WriteFile(activityPath, []byte(activity.Format(events)), 0o644); err != nil {
			return claude.Result{}, err
		}
	}

	prompt, err := claude.BuildPrompt(claude.PromptData{
		Skill:        skill,
		Source:       source,
		ForkDir:      "skills/" + skill,
		NotesPath:    discover.NotesPath(skill),
		OldDir:       oldDir,
		NewDir:       newDir,
		DiffPath:     diffPath,
		ActivityPath: activityPath,
		ResultPath:   resultPath,
		OldSHA:       oldSHA,
		NewSHA:       newSHA,
		HasActivity:  len(events) > 0,
		Date:         d.Now().Format("2006-01-02"),
	})
	if err != nil {
		return claude.Result{}, err
	}
	if err := d.Claude.Run(ctx, d.Root, d.Scratch, prompt); err != nil {
		return claude.Result{}, err
	}

	f, err := os.Open(resultPath)
	if err != nil {
		return claude.Result{}, fmt.Errorf("read result file: %w", err)
	}
	defer f.Close()
	result, err := claude.ParseResult(f)
	if err != nil {
		return claude.Result{}, err
	}

	changed, err := d.Git.ChangedPaths(ctx)
	if err != nil {
		return claude.Result{}, err
	}
	if err := claude.CheckChangedPaths(changed, skill); err != nil {
		return claude.Result{}, err
	}
	return result, nil
}

// commit writes the upstream commit to the sha file and commits the skill files.
// The commit has the author date at. When the branch already records upstream
// and nothing changed, the run answers activity only. Then commit makes an
// empty commit, so that the next run does not count the same activity again.
func commit(ctx context.Context, d Deps, skill, upstream, branchSHA string, at time.Time) error {
	shaFile := filepath.Join(d.Root, config.ShaPath(skill))
	if err := os.WriteFile(shaFile, []byte(upstream+"\n"), 0o644); err != nil {
		return err
	}
	paths := []string{"skills/" + skill, discover.NotesPath(skill), "internal/forked-skills/" + skill}
	message := fmt.Sprintf("chore: update %s to upstream %s", skill, shortSHA(upstream))
	opts := gitx.CommitOptions{AuthorDate: at}
	err := d.Git.CommitWith(ctx, paths, message, opts)
	if !errors.Is(err, gitx.ErrNothingToCommit) || branchSHA != upstream {
		return err
	}
	log.Printf("%s: no changes, make an empty commit", skill)
	opts.AllowEmpty = true
	return d.Git.CommitWith(ctx, paths, "chore: respond to activity on "+skill, opts)
}

// shortSHA returns the first 7 characters of sha.
func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
