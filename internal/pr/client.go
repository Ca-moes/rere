package pr

import (
	"context"

	"github.com/google/go-github/v79/github"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

// GitHubOpener implements Opener against github.com using a REST client (Git
// Data API) and a GraphQL client (auto-merge).
type GitHubOpener struct {
	rest *github.Client
	gql  *githubv4.Client
}

// NewGitHubOpener wires an opener from pre-built clients (used in tests).
func NewGitHubOpener(rest *github.Client, gql *githubv4.Client) *GitHubOpener {
	return &GitHubOpener{rest: rest, gql: gql}
}

// NewGitHubOpenerFromToken builds REST and GraphQL clients from a PAT. The
// token is resolved from an environment variable by config — never inline.
func NewGitHubOpenerFromToken(ctx context.Context, token string) *GitHubOpener {
	httpClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}))
	return &GitHubOpener{
		rest: github.NewClient(httpClient),
		gql:  githubv4.NewClient(httpClient),
	}
}
