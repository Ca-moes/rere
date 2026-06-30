// Package pr opens auto-merged GitHub pull requests carrying surgical manifest
// edits. It builds an atomic commit through the Git Data API (create branch,
// tree, commit, move the ref) and enables auto-merge through the GraphQL API —
// the only place that mutation exists.
package pr

import "context"

// FileEdit is the full new content of one file in the commit.
type FileEdit struct {
	Path    string
	Content string
}

// Request describes the pull request to open.
type Request struct {
	Owner           string
	Repo            string
	BaseBranch      string
	HeadBranch      string
	Title           string
	Body            string
	Edits           []FileEdit
	MergeMethod     string // squash | merge | rebase (empty defaults server-side)
	EnableAutoMerge bool
}

// Result reports the opened pull request.
type Result struct {
	Number           int
	URL              string
	NodeID           string
	AutoMergeEnabled bool
}

// Opener opens a pull request. The run loop depends on this interface so it is
// fully fake-testable.
type Opener interface {
	Open(ctx context.Context, req Request) (*Result, error)
}
