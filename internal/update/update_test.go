package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fergalhk/agent-config/internal/config"
	"github.com/fergalhk/agent-config/internal/github"
	"github.com/fergalhk/agent-config/internal/gitx"
)

const (
	skill     = "sp-a"
	storedSHA = "1111111111111111111111111111111111111111"
	newSHA    = "2222222222222222222222222222222222222222"
	branch    = "skill-update/sp-a"
)

var t0 = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

type fakeAPI struct {
	prs           []github.PR
	issueComments []github.Comment
	commits       []github.Commit
	getPR         *github.PR
	compareCalls  int
	reopened      []int
	created       []string
	updated       []string
	comments      []string
}

func (f *fakeAPI) FindPRs(ctx context.Context, head string) ([]github.PR, error) {
	return f.prs, nil
}

func (f *fakeAPI) CreatePR(ctx context.Context, head, base, title, body string) (*github.PR, error) {
	f.created = append(f.created, title)
	return &github.PR{Number: 99}, nil
}

func (f *fakeAPI) UpdatePR(ctx context.Context, number int, title, body string) error {
	f.updated = append(f.updated, title)
	return nil
}

func (f *fakeAPI) ReopenPR(ctx context.Context, number int) error {
	f.reopened = append(f.reopened, number)
	return nil
}

func (f *fakeAPI) ListIssueComments(ctx context.Context, number int) ([]github.Comment, error) {
	return f.issueComments, nil
}

func (f *fakeAPI) ListReviewComments(ctx context.Context, number int) ([]github.Comment, error) {
	return nil, nil
}

func (f *fakeAPI) ListReviews(ctx context.Context, number int) ([]github.Review, error) {
	return nil, nil
}

func (f *fakeAPI) ListPRCommits(ctx context.Context, number int) ([]github.Commit, error) {
	return f.commits, nil
}

func (f *fakeAPI) CreateComment(ctx context.Context, number int, body string) error {
	f.comments = append(f.comments, body)
	return nil
}

func (f *fakeAPI) GetPR(ctx context.Context, number int) (*github.PR, error) {
	return f.getPR, nil
}

func (f *fakeAPI) BranchHead(ctx context.Context, branch string) (string, error) {
	return "", errors.New("not used")
}

// CompareBehindBy reports that the branch is current, so the rebase stops early.
func (f *fakeAPI) CompareBehindBy(ctx context.Context, base, head string) (int, error) {
	f.compareCalls++
	return 0, nil
}

func (f *fakeAPI) UpdatePullRequestBranch(ctx context.Context, nodeID string) error {
	return errors.New("not used")
}

type commitCall struct {
	message string
	opts    gitx.CommitOptions
}

type pushCall struct {
	branch string
	force  bool
}

type fakeGit struct {
	files     map[string]string
	changed   []string
	checkouts []string
	commits   []commitCall
	pushes    []pushCall
	// nothingToCommit makes a commit that is not empty return ErrNothingToCommit.
	nothingToCommit bool
}

func (g *fakeGit) Fetch(ctx context.Context) error { return nil }

func (g *fakeGit) ShowFile(ctx context.Context, ref, path string) (string, error) {
	content, ok := g.files[ref+":"+path]
	if !ok {
		return "", errors.New("no such file")
	}
	return content, nil
}

func (g *fakeGit) CheckoutNewBranch(ctx context.Context, branch, startPoint string) error {
	g.checkouts = append(g.checkouts, branch+"@"+startPoint)
	return nil
}

func (g *fakeGit) ChangedPaths(ctx context.Context) ([]string, error) { return g.changed, nil }

func (g *fakeGit) CommitWith(ctx context.Context, paths []string, message string, opts gitx.CommitOptions) error {
	if g.nothingToCommit && !opts.AllowEmpty {
		return gitx.ErrNothingToCommit
	}
	g.commits = append(g.commits, commitCall{message: message, opts: opts})
	return nil
}

func (g *fakeGit) Push(ctx context.Context, branch string, force bool) error {
	g.pushes = append(g.pushes, pushCall{branch: branch, force: force})
	return nil
}

type fakeUpstream struct{ head string }

func (u *fakeUpstream) LastCommitTouching(ctx context.Context, ref, dir string) (string, error) {
	return u.head, nil
}

func (u *fakeUpstream) DefaultBranch(ctx context.Context) (string, error) { return "main", nil }

func (u *fakeUpstream) ExtractDir(ctx context.Context, sha, dir, dest string) error {
	return os.MkdirAll(dest, 0o755)
}

func (u *fakeUpstream) DiffDir(ctx context.Context, old, new, dir string) (string, error) {
	return "diff\n", nil
}

// fakeClaude edits the fork, records the change in the fake git, and writes a result file.
type fakeClaude struct {
	git         *fakeGit
	extraChange string
	err         error
	prompts     []string
}

func (c *fakeClaude) Run(ctx context.Context, repoDir, scratch, prompt string) error {
	c.prompts = append(c.prompts, prompt)
	if c.err != nil {
		return c.err
	}
	if err := os.WriteFile(filepath.Join(repoDir, "skills", skill, "SKILL.md"), []byte("changed\n"), 0o644); err != nil {
		return err
	}
	c.git.changed = append(c.git.changed, "skills/"+skill+"/SKILL.md")
	if c.extraChange != "" {
		c.git.changed = append(c.git.changed, c.extraChange)
	}
	result := "chore: apply upstream\n\n## What changed upstream\nThings.\n"
	return os.WriteFile(filepath.Join(scratch, "result.md"), []byte(result), 0o644)
}

type harness struct {
	deps   Deps
	api    *fakeAPI
	git    *fakeGit
	claude *fakeClaude
}

func newHarness(t *testing.T, upstream string) *harness {
	t.Helper()
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(config.SkillsPath, "sp-a:\n  type: forked\n  fork:\n    source:\n      repo: github.com/obra/superpowers\n      directory: skills/a\n")
	write(config.ShaPath(skill), storedSHA+"\n")
	write("skills/sp-a/SKILL.md", "original\n")

	api := &fakeAPI{}
	g := &fakeGit{files: map[string]string{}}
	c := &fakeClaude{git: g}
	h := &harness{api: api, git: g, claude: c}
	h.deps = Deps{
		API: api,
		Git: g,
		Upstream: func(ctx context.Context, url string) (UpstreamRepo, error) {
			if url != "https://github.com/obra/superpowers.git" {
				t.Errorf("upstream url = %q", url)
			}
			return &fakeUpstream{head: upstream}, nil
		},
		Claude:    c,
		Root:      root,
		Scratch:   t.TempDir(),
		RunChecks: func(string) error { return nil },
		Now:       func() time.Time { return t0 },
	}
	return h
}

// withPR adds a PR for the update branch and records branchSHA on the branch.
func (h *harness) withPR(state, branchSHA string) {
	h.api.prs = []github.PR{{Number: 7, NodeID: "N7", State: state, HeadRef: branch, HeadSHA: "head", BaseRef: "main"}}
	h.git.files["origin/"+branch+":"+config.ShaPath(skill)] = branchSHA + "\n"
}

func TestRun_StopsWhenUpstreamUnchanged(t *testing.T) {
	h := newHarness(t, storedSHA)
	if err := Run(context.Background(), h.deps, skill); err != nil {
		t.Fatal(err)
	}
	if len(h.claude.prompts) != 0 || len(h.git.pushes) != 0 || len(h.api.created) != 0 {
		t.Errorf("want no work, got prompts=%d pushes=%v created=%v", len(h.claude.prompts), h.git.pushes, h.api.created)
	}
}

func TestRun_CreatesPROnNewUpstream(t *testing.T) {
	h := newHarness(t, newSHA)
	if err := Run(context.Background(), h.deps, skill); err != nil {
		t.Fatal(err)
	}
	if len(h.claude.prompts) != 1 {
		t.Fatalf("claude calls = %d, want 1", len(h.claude.prompts))
	}
	for _, want := range []string{filepath.Join(h.deps.Scratch, "old"), filepath.Join(h.deps.Scratch, "new")} {
		if !strings.Contains(h.claude.prompts[0], want) {
			t.Errorf("prompt does not name %s", want)
		}
	}
	sha, err := config.ReadSha(h.deps.Root, skill)
	if err != nil || sha != newSHA {
		t.Errorf("sha file = %q, %v, want %s", sha, err, newSHA)
	}
	if len(h.git.checkouts) != 1 || h.git.checkouts[0] != branch+"@origin/main" {
		t.Errorf("checkouts = %v", h.git.checkouts)
	}
	if len(h.git.pushes) != 1 || h.git.pushes[0] != (pushCall{branch: branch, force: true}) {
		t.Errorf("pushes = %v, want one force push of %s", h.git.pushes, branch)
	}
	if len(h.api.created) != 1 || h.api.created[0] != "chore: apply upstream" {
		t.Errorf("created = %v", h.api.created)
	}
}

func TestRun_UpdatesExistingPRWithActivity(t *testing.T) {
	h := newHarness(t, newSHA)
	h.withPR("open", newSHA)
	h.api.commits = []github.Commit{{SHA: "c1", AuthorLogin: gitx.BotName, AuthorDate: t0}}
	h.api.issueComments = []github.Comment{{Author: "fergal", Body: "keep the old wording", CreatedAt: t0.Add(time.Hour)}}
	if err := Run(context.Background(), h.deps, skill); err != nil {
		t.Fatal(err)
	}
	if h.api.compareCalls == 0 {
		t.Error("rebase was not attempted")
	}
	activityPath := filepath.Join(h.deps.Scratch, "activity.md")
	if len(h.claude.prompts) != 1 || !strings.Contains(h.claude.prompts[0], activityPath) {
		t.Error("prompt does not name the activity file")
	}
	activity, err := os.ReadFile(activityPath)
	if err != nil || !strings.Contains(string(activity), "keep the old wording") {
		t.Errorf("activity file = %q, %v", activity, err)
	}
	if len(h.git.checkouts) != 1 || h.git.checkouts[0] != branch+"@origin/"+branch {
		t.Errorf("checkouts = %v", h.git.checkouts)
	}
	if len(h.git.commits) != 1 || !h.git.commits[0].opts.AuthorDate.Equal(t0) {
		t.Errorf("commits = %+v, want one commit dated %s", h.git.commits, t0)
	}
	if len(h.api.updated) != 1 || len(h.api.comments) != 1 {
		t.Errorf("updated = %v, comments = %v", h.api.updated, h.api.comments)
	}
	if len(h.api.created) != 0 {
		t.Errorf("created = %v, want none", h.api.created)
	}
}

func TestRun_EmptyCommitWhenActivityChangesNothing(t *testing.T) {
	h := newHarness(t, newSHA)
	h.withPR("open", newSHA)
	h.git.nothingToCommit = true
	h.api.commits = []github.Commit{{SHA: "c1", AuthorLogin: gitx.BotName, AuthorDate: t0}}
	h.api.issueComments = []github.Comment{{Author: "fergal", Body: "why?", CreatedAt: t0.Add(time.Hour)}}
	if err := Run(context.Background(), h.deps, skill); err != nil {
		t.Fatal(err)
	}
	want := commitCall{message: "chore: respond to activity on sp-a", opts: gitx.CommitOptions{AllowEmpty: true, AuthorDate: t0}}
	if len(h.git.commits) != 1 || h.git.commits[0] != want {
		t.Errorf("commits = %+v, want [%+v]", h.git.commits, want)
	}
	if len(h.git.pushes) != 1 {
		t.Errorf("pushes = %v, want one push", h.git.pushes)
	}
}

func TestRun_ReopensClosedPR(t *testing.T) {
	h := newHarness(t, newSHA)
	h.withPR("closed", storedSHA)
	if err := Run(context.Background(), h.deps, skill); err != nil {
		t.Fatal(err)
	}
	if len(h.api.reopened) != 1 || h.api.reopened[0] != 7 {
		t.Errorf("reopened = %v, want [7]", h.api.reopened)
	}
	if len(h.api.updated) != 1 || len(h.api.created) != 0 {
		t.Errorf("updated = %v, created = %v", h.api.updated, h.api.created)
	}
	if len(h.git.pushes) != 1 || h.git.pushes[0].force {
		t.Errorf("pushes = %v, want one push without force", h.git.pushes)
	}
}

func TestRun_RejectsChangesOutsideAllowedPaths(t *testing.T) {
	h := newHarness(t, newSHA)
	h.claude.extraChange = config.SkillsPath
	if err := Run(context.Background(), h.deps, skill); err == nil {
		t.Fatal("want an error")
	}
	if len(h.git.pushes) != 0 || len(h.git.commits) != 0 {
		t.Errorf("pushes = %v, commits = %v, want none", h.git.pushes, h.git.commits)
	}
}

func TestRun_CommentsOnClaudeFailure(t *testing.T) {
	h := newHarness(t, newSHA)
	h.withPR("open", storedSHA)
	h.claude.err = errors.New("boom")
	if err := Run(context.Background(), h.deps, skill); err == nil {
		t.Fatal("want an error")
	}
	if len(h.api.comments) != 1 || !strings.Contains(h.api.comments[0], "Claude step failed") {
		t.Errorf("comments = %v", h.api.comments)
	}
	if len(h.git.pushes) != 0 {
		t.Errorf("pushes = %v, want none", h.git.pushes)
	}
}

func TestSkillForPR(t *testing.T) {
	for _, tc := range []struct {
		head string
		name string
		ok   bool
	}{
		{"skill-update/sp-a", "sp-a", true},
		{"skill-add/sp-a", "", false},
	} {
		api := &fakeAPI{getPR: &github.PR{Number: 3, HeadRef: tc.head}}
		name, ok, err := SkillForPR(context.Background(), api, 3)
		if err != nil || name != tc.name || ok != tc.ok {
			t.Errorf("%s: got (%q, %v, %v), want (%q, %v, nil)", tc.head, name, ok, err, tc.name, tc.ok)
		}
	}
}
