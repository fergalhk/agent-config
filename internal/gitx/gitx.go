// Package gitx is a thin wrapper around the git command line program.
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Identity of the bot that makes commits.
const (
	BotName  = "github-actions[bot]"
	BotEmail = "41898282+github-actions[bot]@users.noreply.github.com"
)

// ErrNothingToCommit means the index is clean after "git add".
var ErrNothingToCommit = errors.New("nothing to commit")

// Repo is a local git repository.
type Repo struct {
	Dir string
}

// Open returns a Repo for an existing directory.
func Open(dir string) *Repo {
	return &Repo{Dir: dir}
}

// Clone makes a full clone of url in dir.
func Clone(ctx context.Context, url, dir string) (*Repo, error) {
	cmd := exec.CommandContext(ctx, "git", "clone", "--", url, dir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git clone %s: %w: %s", url, err, strings.TrimSpace(stderr.String()))
	}
	return Open(dir), nil
}

// runRaw runs git in the repository and returns stdout without any change.
func (r *Repo) runRaw(ctx context.Context, args ...string) (string, error) {
	return r.runEnv(ctx, nil, args...)
}

// runEnv runs git with extra environment variables, in KEY=value form.
// It returns stdout without any change.
func (r *Repo) runEnv(ctx context.Context, env []string, args ...string) (string, error) {
	full := append([]string{"-C", r.Dir}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// Run runs git in the repository and returns trimmed stdout.
func (r *Repo) Run(ctx context.Context, args ...string) (string, error) {
	out, err := r.runRaw(ctx, args...)
	return strings.TrimSpace(out), err
}

// LastCommitTouching returns the last commit on ref that changed dir.
func (r *Repo) LastCommitTouching(ctx context.Context, ref, dir string) (string, error) {
	return r.Run(ctx, "log", "-1", "--format=%H", ref, "--", dir)
}

// DefaultBranch returns the name of the default branch of origin.
func (r *Repo) DefaultBranch(ctx context.Context) (string, error) {
	out, err := r.Run(ctx, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(out, "refs/remotes/origin/"), nil
}

// ExtractDir writes the files of dir at commit sha into dest.
// It removes the path dir from the start of each file path.
func (r *Repo) ExtractDir(ctx context.Context, sha, dir, dest string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dest, err)
	}
	depth := len(strings.Split(strings.Trim(dir, "/"), "/"))
	archive := exec.CommandContext(ctx, "git", "-C", r.Dir, "archive", sha, dir)
	extract := exec.CommandContext(ctx, "tar", "-x", "-C", dest, fmt.Sprintf("--strip-components=%d", depth))
	var archiveErr, extractErr bytes.Buffer
	archive.Stderr = &archiveErr
	extract.Stderr = &extractErr
	pipe, err := archive.StdoutPipe()
	if err != nil {
		return fmt.Errorf("git archive pipe: %w", err)
	}
	extract.Stdin = pipe
	if err := archive.Start(); err != nil {
		return fmt.Errorf("git archive %s %s: %w", sha, dir, err)
	}
	if err := extract.Start(); err != nil {
		_ = archive.Process.Kill()
		_ = archive.Wait()
		return fmt.Errorf("tar: %w", err)
	}
	extractWaitErr := extract.Wait()
	// Close the read end so that git archive gets a broken pipe if tar stopped early.
	_ = pipe.Close()
	archiveWaitErr := archive.Wait()
	var errs []error
	if extractWaitErr != nil {
		errs = append(errs, fmt.Errorf("tar extract: %w: %s", extractWaitErr, strings.TrimSpace(extractErr.String())))
	}
	if archiveWaitErr != nil {
		errs = append(errs, fmt.Errorf("git archive %s %s: %w: %s", sha, dir, archiveWaitErr, strings.TrimSpace(archiveErr.String())))
	}
	return errors.Join(errs...)
}

// DiffDir returns the diff of dir between two commits.
func (r *Repo) DiffDir(ctx context.Context, oldSha, newSha, dir string) (string, error) {
	return r.runRaw(ctx, "diff", oldSha, newSha, "--", dir)
}

// ShowFile returns the content of path at ref.
func (r *Repo) ShowFile(ctx context.Context, ref, path string) (string, error) {
	return r.runRaw(ctx, "show", ref+":"+path)
}

// CheckoutNewBranch creates branch at startPoint, or resets it, and checks it out.
func (r *Repo) CheckoutNewBranch(ctx context.Context, branch, startPoint string) error {
	_, err := r.Run(ctx, "checkout", "-B", branch, startPoint)
	return err
}

// ChangedPaths returns the paths that have changes in the working tree.
func (r *Repo) ChangedPaths(ctx context.Context) ([]string, error) {
	out, err := r.runRaw(ctx, "status", "--porcelain=v1", "-z")
	if err != nil {
		return nil, err
	}
	return ParseStatusPorcelain(out), nil
}

// CommitOptions changes how CommitWith makes a commit.
type CommitOptions struct {
	// AllowEmpty makes a commit when the index is clean.
	AllowEmpty bool
	// AuthorDate sets the author date of the commit. The zero value uses the current time.
	AuthorDate time.Time
}

// CommitAll stages paths and commits them as the bot.
// It returns ErrNothingToCommit when the index is clean.
func (r *Repo) CommitAll(ctx context.Context, paths []string, message string) error {
	return r.CommitWith(ctx, paths, message, CommitOptions{})
}

// CommitWith stages paths and commits them as the bot with the given options.
// It returns ErrNothingToCommit when the index is clean and AllowEmpty is false.
func (r *Repo) CommitWith(ctx context.Context, paths []string, message string, opts CommitOptions) error {
	if _, err := r.Run(ctx, append([]string{"add", "-A", "--"}, paths...)...); err != nil {
		return err
	}
	if !opts.AllowEmpty {
		_, err := r.Run(ctx, "diff", "--cached", "--quiet")
		if err == nil {
			return ErrNothingToCommit
		}
		// Exit code 1 means the index has changes. Any other failure is a real error.
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return err
		}
	}
	args := []string{
		"-c", "user.name=" + BotName,
		"-c", "user.email=" + BotEmail,
		"commit", "-m", message,
	}
	if opts.AllowEmpty {
		args = append(args, "--allow-empty")
	}
	var env []string
	if !opts.AuthorDate.IsZero() {
		env = []string{"GIT_AUTHOR_DATE=" + opts.AuthorDate.Format(time.RFC3339)}
	}
	_, err := r.runEnv(ctx, env, args...)
	return err
}

// Push pushes branch to origin. With force, it uses --force-with-lease.
func (r *Repo) Push(ctx context.Context, branch string, force bool) error {
	args := []string{"push"}
	if force {
		args = append(args, "--force-with-lease")
	}
	_, err := r.Run(ctx, append(args, "origin", branch)...)
	return err
}

// Fetch fetches from origin.
func (r *Repo) Fetch(ctx context.Context) error {
	_, err := r.Run(ctx, "fetch", "origin")
	return err
}
