package github

import "context"

// API lists the GitHub operations that the tool needs.
type API interface {
	FindPRs(ctx context.Context, headBranch string) ([]PR, error) // state=all
	CreatePR(ctx context.Context, head, base, title, body string) (*PR, error)
	UpdatePR(ctx context.Context, number int, title, body string) error
	ReopenPR(ctx context.Context, number int) error
	ListIssueComments(ctx context.Context, number int) ([]Comment, error)
	ListReviewComments(ctx context.Context, number int) ([]Comment, error)
	ListReviews(ctx context.Context, number int) ([]Review, error)
	ListPRCommits(ctx context.Context, number int) ([]Commit, error)
	CreateComment(ctx context.Context, number int, body string) error
	GetPR(ctx context.Context, number int) (*PR, error)
	BranchHead(ctx context.Context, branch string) (string, error)
	CompareBehindBy(ctx context.Context, base, head string) (int, error) // GET compare/base...head, field behind_by
	UpdatePullRequestBranch(ctx context.Context, nodeID string) error    // GraphQL mutation, updateMethod REBASE
}
