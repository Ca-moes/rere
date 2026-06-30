package pr

import (
	"context"
	"strings"

	"github.com/shurcooL/githubv4"
)

// enableAutoMerge runs the enablePullRequestAutoMerge GraphQL mutation. It must
// be given the PR's node_id (GraphQL global ID), not its number.
func (o *GitHubOpener) enableAutoMerge(ctx context.Context, prNodeID, mergeMethod string) error {
	var mutation struct {
		EnablePullRequestAutoMerge struct {
			PullRequest struct {
				ID githubv4.ID
			}
		} `graphql:"enablePullRequestAutoMerge(input: $input)"`
	}
	input := githubv4.EnablePullRequestAutoMergeInput{
		PullRequestID: githubv4.ID(prNodeID),
	}
	if m := mergeMethodFor(mergeMethod); m != nil {
		input.MergeMethod = m
	}
	return o.gql.Mutate(ctx, &mutation, input, nil)
}

func mergeMethodFor(s string) *githubv4.PullRequestMergeMethod {
	var m githubv4.PullRequestMergeMethod
	switch strings.ToUpper(s) {
	case "SQUASH":
		m = githubv4.PullRequestMergeMethodSquash
	case "REBASE":
		m = githubv4.PullRequestMergeMethodRebase
	case "MERGE":
		m = githubv4.PullRequestMergeMethodMerge
	default:
		return nil
	}
	return &m
}
