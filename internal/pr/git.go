package pr

import (
	"context"
	"fmt"

	"github.com/google/go-github/v79/github"
)

// Open creates a branch, an atomic commit carrying all edits, the pull request,
// and (optionally) enables auto-merge. If auto-merge fails the PR is still
// returned, with AutoMergeEnabled=false and the error.
func (o *GitHubOpener) Open(ctx context.Context, req Request) (*Result, error) {
	g := o.rest.Git

	baseRef, _, err := g.GetRef(ctx, req.Owner, req.Repo, "heads/"+req.BaseBranch)
	if err != nil {
		return nil, fmt.Errorf("get base ref: %w", err)
	}
	baseSHA := baseRef.GetObject().GetSHA()

	if _, _, err := g.CreateRef(ctx, req.Owner, req.Repo, github.CreateRef{
		Ref: "refs/heads/" + req.HeadBranch,
		SHA: baseSHA,
	}); err != nil {
		return nil, fmt.Errorf("create head ref %q: %w", req.HeadBranch, err)
	}

	baseCommit, _, err := g.GetCommit(ctx, req.Owner, req.Repo, baseSHA)
	if err != nil {
		return nil, fmt.Errorf("get base commit: %w", err)
	}

	entries := make([]*github.TreeEntry, 0, len(req.Edits))
	for _, e := range req.Edits {
		entries = append(entries, &github.TreeEntry{
			Path:    github.Ptr(e.Path),
			Mode:    github.Ptr("100644"),
			Type:    github.Ptr("blob"),
			Content: github.Ptr(e.Content),
		})
	}
	tree, _, err := g.CreateTree(ctx, req.Owner, req.Repo, baseCommit.GetTree().GetSHA(), entries)
	if err != nil {
		return nil, fmt.Errorf("create tree: %w", err)
	}

	commit, _, err := g.CreateCommit(ctx, req.Owner, req.Repo, github.Commit{
		Message: github.Ptr(commitMessage(req)),
		Tree:    tree,
		Parents: []*github.Commit{{SHA: github.Ptr(baseSHA)}},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("create commit: %w", err)
	}

	if _, _, err := g.UpdateRef(ctx, req.Owner, req.Repo, "heads/"+req.HeadBranch, github.UpdateRef{
		SHA: commit.GetSHA(),
	}); err != nil {
		return nil, fmt.Errorf("update head ref: %w", err)
	}

	pull, _, err := o.rest.PullRequests.Create(ctx, req.Owner, req.Repo, &github.NewPullRequest{
		Title: github.Ptr(req.Title),
		Head:  github.Ptr(req.HeadBranch),
		Base:  github.Ptr(req.BaseBranch),
		Body:  github.Ptr(req.Body),
	})
	if err != nil {
		return nil, fmt.Errorf("create pull request: %w", err)
	}

	res := &Result{Number: pull.GetNumber(), URL: pull.GetHTMLURL(), NodeID: pull.GetNodeID()}

	if req.EnableAutoMerge {
		if err := o.enableAutoMerge(ctx, pull.GetNodeID(), req.MergeMethod); err != nil {
			return res, fmt.Errorf("enable auto-merge: %w", err)
		}
		res.AutoMergeEnabled = true
	}
	return res, nil
}

func commitMessage(req Request) string {
	if req.Body == "" {
		return req.Title
	}
	return req.Title + "\n\n" + req.Body
}
