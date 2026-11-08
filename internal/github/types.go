// Package github is a small client for the GitHub REST and GraphQL APIs.
package github

import "time"

// PR is a pull request.
type PR struct {
	Number  int
	NodeID  string
	State   string
	Merged  bool
	HeadRef string
	HeadSHA string
	BaseRef string
	Title   string
	Body    string
}

// Comment is an issue comment or a review comment.
type Comment struct {
	Author    string
	Body      string
	CreatedAt time.Time
}

// Review is a pull request review.
type Review struct {
	Author      string
	Body        string
	State       string
	SubmittedAt time.Time
}

// Commit is a commit on a pull request.
type Commit struct {
	SHA         string
	AuthorLogin string
	AuthorDate  time.Time
}
