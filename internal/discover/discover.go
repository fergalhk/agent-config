package discover

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"

	"github.com/fergalhk/agent-config/internal/config"
	"github.com/fergalhk/agent-config/internal/github"
	"github.com/fergalhk/agent-config/internal/gitx"
	"github.com/fergalhk/agent-config/internal/naming"
)

// baseBranch is the branch that discovery PRs target.
const baseBranch = "main"

// Deps holds the dependencies of the discovery job.
type Deps struct {
	API github.API
	// Repo is the local checkout of this repository.
	Repo *gitx.Repo
	// Root is the directory of the local checkout.
	Root string
	// Scratch is a directory for the upstream clones.
	Scratch string
	// RunChecks runs the repository checks. It returns an error when a check fails.
	RunChecks func(root string) error
}

// NotesPath returns the path of the notes file for a skill.
func NotesPath(name string) string {
	return "docs/agents/skills/" + name + ".md"
}

// Run finds new skills in each fork source and raises one PR for each new skill.
// It continues after a failed candidate. It returns the joined candidate errors.
func Run(ctx context.Context, d Deps) error {
	skills, err := config.LoadSkills(filepath.Join(d.Root, config.SkillsPath))
	if err != nil {
		return err
	}
	sources, err := config.LoadForkSources(filepath.Join(d.Root, config.ForkSourcesPath))
	if err != nil {
		return err
	}

	var errs []error
	for i, src := range sources {
		upstream, ref, err := cloneSource(ctx, d.Scratch, i, src)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		cands, err := ListCandidates(upstream.Dir, src)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, c := range NewCandidates(cands, skills, src.Ignore) {
			outcome, err := addCandidate(ctx, d, upstream, ref, src, c)
			if err != nil {
				log.Printf("%s: failed: %v", c.Name, err)
				errs = append(errs, fmt.Errorf("%s: %w", c.Name, err))
			} else {
				log.Printf("%s: %s", c.Name, outcome)
			}
			if err := resetToMain(ctx, d.Repo); err != nil {
				errs = append(errs, fmt.Errorf("%s: reset to %s: %w", c.Name, baseBranch, err))
				return errors.Join(errs...)
			}
		}
	}
	return errors.Join(errs...)
}

// cloneSource clones the upstream repository of src. It returns the clone and
// the ref of its default branch.
func cloneSource(ctx context.Context, scratch string, i int, src config.ForkSource) (*gitx.Repo, string, error) {
	url := "https://" + src.Source.Repository + ".git"
	upstream, err := gitx.Clone(ctx, url, filepath.Join(scratch, "upstream-"+strconv.Itoa(i)))
	if err != nil {
		return nil, "", err
	}
	branch, err := upstream.DefaultBranch(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("default branch of %s: %w", src.Source.Repository, err)
	}
	return upstream, "origin/" + branch, nil
}

// addCandidate raises the PR for one new skill. It returns a short outcome for the log.
// Each add branch changes skills.yaml. So after one discovery PR merges, the
// other open discovery PRs conflict. When the branch of an open PR is behind
// main, addCandidate builds the branch again from main and pushes it with
// force. It does not change the title or the body of that PR.
func addCandidate(ctx context.Context, d Deps, upstream *gitx.Repo, ref string, src config.ForkSource, c Candidate) (string, error) {
	branch := naming.AddBranch(c.Name)
	prs, err := d.API.FindPRs(ctx, branch)
	if err != nil {
		return "", fmt.Errorf("find PRs for %s: %w", branch, err)
	}
	behindBy := 0
	if OpenPR(prs) != nil {
		behindBy, err = d.API.CompareBehindBy(ctx, baseBranch, branch)
		if err != nil {
			return "", fmt.Errorf("compare %s with %s: %w", branch, baseBranch, err)
		}
	}
	action, existing := DecideAdd(prs, behindBy)
	switch action {
	case AddSkipOpen:
		return fmt.Sprintf("skipped, PR #%d is open", existing.Number), nil
	case AddSkipClosed:
		return fmt.Sprintf("skipped, PR #%d is closed", existing.Number), nil
	}

	sha, err := upstream.LastCommitTouching(ctx, ref, c.Dir)
	if err != nil {
		return "", fmt.Errorf("last commit for %s: %w", c.Dir, err)
	}
	if sha == "" {
		return "", fmt.Errorf("no commit changes %s on %s", c.Dir, ref)
	}
	source := config.Source{Repo: src.Source.Repository, Directory: c.Dir}

	if err := d.Repo.CheckoutNewBranch(ctx, branch, "origin/"+baseBranch); err != nil {
		return "", err
	}
	skillDir := "skills/" + c.Name
	if err := upstream.ExtractDir(ctx, sha, c.Dir, filepath.Join(d.Root, skillDir)); err != nil {
		return "", err
	}
	if err := AddSkillEntry(filepath.Join(d.Root, config.SkillsPath), c.Name, source); err != nil {
		return "", err
	}
	if err := writeFile(filepath.Join(d.Root, config.ShaPath(c.Name)), sha+"\n"); err != nil {
		return "", err
	}
	notes, err := NotesTemplate(c.Name, source)
	if err != nil {
		return "", err
	}
	if err := writeFile(filepath.Join(d.Root, NotesPath(c.Name)), notes); err != nil {
		return "", err
	}
	if err := d.RunChecks(d.Root); err != nil {
		return "", fmt.Errorf("checks: %w", err)
	}

	title := "feat: add forked skill " + c.Name
	paths := []string{skillDir, config.SkillsPath, config.ShaPath(c.Name), NotesPath(c.Name)}
	if err := d.Repo.CommitAll(ctx, paths, title); err != nil {
		return "", err
	}
	// Force the push. A branch with no open or closed unmerged PR may still exist from an earlier PR.
	// A rebuilt branch also replaces the old branch.
	if err := d.Repo.Push(ctx, branch, true); err != nil {
		return "", err
	}
	if action == AddRebuild {
		return fmt.Sprintf("rebuilt the branch of PR #%d from %s", existing.Number, baseBranch), nil
	}
	body, err := PRBody(c.Name, source, sha, path.Base(c.Dir))
	if err != nil {
		return "", err
	}
	pr, err := d.API.CreatePR(ctx, branch, baseBranch, title, body)
	if err != nil {
		return "", fmt.Errorf("create PR for %s: %w", branch, err)
	}
	return fmt.Sprintf("created PR #%d", pr.Number), nil
}

// resetToMain checks out the base branch and removes all local changes.
func resetToMain(ctx context.Context, repo *gitx.Repo) error {
	for _, args := range [][]string{
		{"checkout", "--", "."},
		{"clean", "-fd"},
		{"checkout", baseBranch},
	} {
		if _, err := repo.Run(ctx, args...); err != nil {
			return err
		}
	}
	return nil
}

// writeFile writes content to path and creates the parent directories.
func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
