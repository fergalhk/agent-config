package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// ErrConflict means that GitHub cannot update a branch because of a merge conflict.
var ErrConflict = errors.New("merge conflict")

const updateBranchMutation = `mutation($id: ID!) { updatePullRequestBranch(input: {pullRequestId: $id, updateMethod: REBASE}) { clientMutationId } }`

var nextLink = regexp.MustCompile(`<([^>]+)>\s*;\s*rel="next"`)

// Client talks to GitHub over HTTP. It implements API.
type Client struct {
	baseURL string
	token   string
	repo    string // owner/repo
	owner   string
	hc      *http.Client
}

var _ API = (*Client)(nil)

// New makes a Client. ownerRepo has the form owner/repo.
func New(baseURL, token, ownerRepo string, hc *http.Client) *Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	owner, _, _ := strings.Cut(ownerRepo, "/")
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		repo:    ownerRepo,
		owner:   owner,
		hc:      hc,
	}
}

// NewFromEnv makes a Client from GITHUB_TOKEN, GITHUB_REPOSITORY and GITHUB_API_URL.
func NewFromEnv() (*Client, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, errors.New("environment variable GITHUB_TOKEN is not set")
	}
	repo := os.Getenv("GITHUB_REPOSITORY")
	if repo == "" {
		return nil, errors.New("environment variable GITHUB_REPOSITORY is not set")
	}
	if o, r, ok := strings.Cut(repo, "/"); !ok || o == "" || r == "" {
		return nil, fmt.Errorf("environment variable GITHUB_REPOSITORY must have the form owner/repo, got %q", repo)
	}
	base := os.Getenv("GITHUB_API_URL")
	if base == "" {
		base = "https://api.github.com"
	}
	return New(base, token, repo, nil), nil
}

// doURL sends one request to a full URL and decodes a JSON response into out.
// It returns the response headers.
func (c *Client) doURL(ctx context.Context, method, fullURL string, body, out any) (http.Header, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request body: %w", err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, fullURL, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response of %s %s: %w", method, fullURL, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("%s %s: status %d: %s", method, fullURL, resp.StatusCode, data)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return nil, fmt.Errorf("decode response of %s %s: %w", method, fullURL, err)
		}
	}
	return resp.Header, nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	_, err := c.doURL(ctx, method, c.baseURL+path, body, out)
	return err
}

// paginate reads every page of a list endpoint. It calls add for each item.
func paginate[T any](ctx context.Context, c *Client, path string, add func(T)) error {
	next := c.baseURL + path
	for next != "" {
		var page []T
		h, err := c.doURL(ctx, http.MethodGet, next, nil, &page)
		if err != nil {
			return err
		}
		for _, item := range page {
			add(item)
		}
		next = ""
		if m := nextLink.FindStringSubmatch(h.Get("Link")); m != nil {
			next = m[1]
		}
	}
	return nil
}

type prJSON struct {
	Number   int        `json:"number"`
	NodeID   string     `json:"node_id"`
	State    string     `json:"state"`
	MergedAt *time.Time `json:"merged_at"`
	Title    string     `json:"title"`
	Body     string     `json:"body"`
	Head     struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

func (p prJSON) pr() PR {
	return PR{
		Number:  p.Number,
		NodeID:  p.NodeID,
		State:   p.State,
		Merged:  p.MergedAt != nil,
		HeadRef: p.Head.Ref,
		HeadSHA: p.Head.SHA,
		BaseRef: p.Base.Ref,
		Title:   p.Title,
		Body:    p.Body,
	}
}

// FindPRs returns all PRs, open or closed, whose head is headBranch.
func (c *Client) FindPRs(ctx context.Context, headBranch string) ([]PR, error) {
	path := fmt.Sprintf("/repos/%s/pulls?head=%s:%s&state=all&per_page=100", c.repo, c.owner, headBranch)
	var prs []PR
	err := paginate(ctx, c, path, func(p prJSON) { prs = append(prs, p.pr()) })
	return prs, err
}

func (c *Client) CreatePR(ctx context.Context, head, base, title, body string) (*PR, error) {
	var out prJSON
	in := map[string]string{"head": head, "base": base, "title": title, "body": body}
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/pulls", c.repo), in, &out); err != nil {
		return nil, err
	}
	p := out.pr()
	return &p, nil
}

func (c *Client) GetPR(ctx context.Context, number int) (*PR, error) {
	var out prJSON
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/pulls/%d", c.repo, number), nil, &out); err != nil {
		return nil, err
	}
	p := out.pr()
	return &p, nil
}

func (c *Client) UpdatePR(ctx context.Context, number int, title, body string) error {
	in := map[string]string{"title": title, "body": body}
	return c.do(ctx, http.MethodPatch, fmt.Sprintf("/repos/%s/pulls/%d", c.repo, number), in, nil)
}

func (c *Client) ReopenPR(ctx context.Context, number int) error {
	in := map[string]string{"state": "open"}
	return c.do(ctx, http.MethodPatch, fmt.Sprintf("/repos/%s/pulls/%d", c.repo, number), in, nil)
}

type commentJSON struct {
	User struct {
		Login string `json:"login"`
	} `json:"user"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

func (c *Client) listComments(ctx context.Context, path string) ([]Comment, error) {
	var out []Comment
	err := paginate(ctx, c, path, func(j commentJSON) {
		out = append(out, Comment{Author: j.User.Login, Body: j.Body, CreatedAt: j.CreatedAt})
	})
	return out, err
}

func (c *Client) ListIssueComments(ctx context.Context, number int) ([]Comment, error) {
	return c.listComments(ctx, fmt.Sprintf("/repos/%s/issues/%d/comments?per_page=100", c.repo, number))
}

func (c *Client) ListReviewComments(ctx context.Context, number int) ([]Comment, error) {
	return c.listComments(ctx, fmt.Sprintf("/repos/%s/pulls/%d/comments?per_page=100", c.repo, number))
}

func (c *Client) ListReviews(ctx context.Context, number int) ([]Review, error) {
	type reviewJSON struct {
		User struct {
			Login string `json:"login"`
		} `json:"user"`
		Body        string    `json:"body"`
		State       string    `json:"state"`
		SubmittedAt time.Time `json:"submitted_at"`
	}
	var out []Review
	path := fmt.Sprintf("/repos/%s/pulls/%d/reviews?per_page=100", c.repo, number)
	err := paginate(ctx, c, path, func(j reviewJSON) {
		out = append(out, Review{Author: j.User.Login, Body: j.Body, State: j.State, SubmittedAt: j.SubmittedAt})
	})
	return out, err
}

func (c *Client) ListPRCommits(ctx context.Context, number int) ([]Commit, error) {
	type commitJSON struct {
		SHA    string `json:"sha"`
		Author *struct {
			Login string `json:"login"`
		} `json:"author"`
		Commit struct {
			Author struct {
				Date time.Time `json:"date"`
			} `json:"author"`
		} `json:"commit"`
	}
	var out []Commit
	path := fmt.Sprintf("/repos/%s/pulls/%d/commits?per_page=100", c.repo, number)
	err := paginate(ctx, c, path, func(j commitJSON) {
		login := ""
		if j.Author != nil {
			login = j.Author.Login
		}
		out = append(out, Commit{SHA: j.SHA, AuthorLogin: login, AuthorDate: j.Commit.Author.Date})
	})
	return out, err
}

func (c *Client) CreateComment(ctx context.Context, number int, body string) error {
	in := map[string]string{"body": body}
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/issues/%d/comments", c.repo, number), in, nil)
}

func (c *Client) BranchHead(ctx context.Context, branch string) (string, error) {
	var out struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	path := fmt.Sprintf("/repos/%s/git/ref/heads/%s", c.repo, branch)
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return "", err
	}
	return out.Object.SHA, nil
}

func (c *Client) CompareBehindBy(ctx context.Context, base, head string) (int, error) {
	var out struct {
		BehindBy int `json:"behind_by"`
	}
	path := fmt.Sprintf("/repos/%s/compare/%s...%s", c.repo, base, head)
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return 0, err
	}
	return out.BehindBy, nil
}

// UpdatePullRequestBranch rebases the PR branch onto its base with GraphQL.
// It wraps ErrConflict when GitHub reports a conflict.
func (c *Client) UpdatePullRequestBranch(ctx context.Context, nodeID string) error {
	in := map[string]any{
		"query":     updateBranchMutation,
		"variables": map[string]string{"id": nodeID},
	}
	var out struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := c.do(ctx, http.MethodPost, "/graphql", in, &out); err != nil {
		return err
	}
	if len(out.Errors) == 0 {
		return nil
	}
	msgs := make([]string, len(out.Errors))
	conflict := false
	for i, e := range out.Errors {
		msgs[i] = e.Message
		if strings.Contains(strings.ToLower(e.Message), "conflict") {
			conflict = true
		}
	}
	joined := strings.Join(msgs, ", ")
	if conflict {
		return fmt.Errorf("update pull request branch: %s: %w", joined, ErrConflict)
	}
	return fmt.Errorf("update pull request branch: %s", joined)
}
