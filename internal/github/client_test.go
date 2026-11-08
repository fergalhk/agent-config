package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New(srv.URL, "tok", "owner/repo", srv.Client())
}

func TestFindPRs_FollowsLinkHeader(t *testing.T) {
	var queries []string
	var srvURL string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("X-GitHub-Api-Version"); got != "2022-11-28" {
			t.Errorf("version header = %q", got)
		}
		if r.URL.Query().Get("page") == "2" {
			fmt.Fprint(w, `[{"number":2,"node_id":"N2","state":"open","head":{"ref":"h","sha":"s2"},"base":{"ref":"main"}}]`)
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/repos/owner/repo/pulls?head=owner:skill-update/x&state=all&per_page=100&page=2>; rel="next"`, srvURL))
		fmt.Fprint(w, `[{"number":1,"node_id":"N1","state":"open","head":{"ref":"skill-update/x","sha":"s1"},"base":{"ref":"main"},"title":"T","body":"B"}]`)
	})
	srvURL = c.baseURL
	prs, err := c.FindPRs(context.Background(), "skill-update/x")
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 2 || prs[0].Number != 1 || prs[1].Number != 2 {
		t.Fatalf("prs = %+v", prs)
	}
	if prs[0].NodeID != "N1" || prs[0].HeadRef != "skill-update/x" || prs[0].HeadSHA != "s1" || prs[0].BaseRef != "main" {
		t.Fatalf("mapping wrong: %+v", prs[0])
	}
	want := "head=owner:skill-update/x&state=all&per_page=100"
	if queries[0] != want {
		t.Fatalf("first query = %q, want %q", queries[0], want)
	}
}

func TestFindPRs_MapsMerged(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"number":1,"state":"closed","merged_at":"2026-01-01T00:00:00Z","head":{},"base":{}},{"number":2,"state":"closed","merged_at":null,"head":{},"base":{}}]`)
	})
	prs, err := c.FindPRs(context.Background(), "b")
	if err != nil {
		t.Fatal(err)
	}
	if !prs[0].Merged || prs[1].Merged {
		t.Fatalf("merged flags wrong: %+v", prs)
	}
}

func TestUpdatePullRequestBranch_Conflict(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"errors":[{"message":"There is a merge Conflict here"}]}`)
	})
	err := c.UpdatePullRequestBranch(context.Background(), "N1")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

func TestUpdatePullRequestBranch_OtherErrorIsNotConflict(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"errors":[{"message":"boom"}]}`)
	})
	err := c.UpdatePullRequestBranch(context.Background(), "N1")
	if err == nil || errors.Is(err, ErrConflict) || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdatePullRequestBranch_SendsRebase(t *testing.T) {
	var path, body string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		fmt.Fprint(w, `{"data":{}}`)
	})
	if err := c.UpdatePullRequestBranch(context.Background(), "NODE123"); err != nil {
		t.Fatal(err)
	}
	if path != "/graphql" {
		t.Fatalf("path = %q", path)
	}
	if !strings.Contains(body, "updateMethod: REBASE") {
		t.Fatalf("body lacks REBASE: %s", body)
	}
	if !strings.Contains(body, "NODE123") {
		t.Fatalf("body lacks node id: %s", body)
	}
}

func TestCompareBehindBy(t *testing.T) {
	var path string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		fmt.Fprint(w, `{"behind_by": 3}`)
	})
	n, err := c.CompareBehindBy(context.Background(), "main", "feat")
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 || path != "/repos/owner/repo/compare/main...feat" {
		t.Fatalf("n = %d path = %q", n, path)
	}
}

func TestNonSuccessStatusIsError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		fmt.Fprint(w, `{"message":"Validation Failed"}`)
	})
	_, err := c.GetPR(context.Background(), 1)
	if err == nil || !strings.Contains(err.Error(), "422") || !strings.Contains(err.Error(), "Validation Failed") {
		t.Fatalf("err = %v", err)
	}
}

func TestListPRCommits_NullAuthor(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"sha":"a","author":null,"commit":{"author":{"date":"2026-01-02T03:04:05Z"}}},{"sha":"b","author":{"login":"bob"},"commit":{"author":{"date":"2026-01-02T03:04:05Z"}}}]`)
	})
	cs, err := c.ListPRCommits(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if cs[0].AuthorLogin != "" || cs[1].AuthorLogin != "bob" || cs[1].AuthorDate.Year() != 2026 {
		t.Fatalf("commits = %+v", cs)
	}
}

func TestReopenAndBranchHead(t *testing.T) {
	var method, body string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/repos/owner/repo/git/ref/heads/") {
			fmt.Fprint(w, `{"object":{"sha":"abc"}}`)
			return
		}
		method = r.Method
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		fmt.Fprint(w, `{}`)
	})
	if err := c.ReopenPR(context.Background(), 5); err != nil {
		t.Fatal(err)
	}
	if method != "PATCH" || strings.TrimSpace(body) != `{"state":"open"}` {
		t.Fatalf("method=%s body=%s", method, body)
	}
	sha, err := c.BranchHead(context.Background(), "x")
	if err != nil || sha != "abc" {
		t.Fatalf("sha=%q err=%v", sha, err)
	}
}
