package pr

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-github/v79/github"
	"github.com/shurcooL/githubv4"
)

const (
	baseSHA    = "base000000000000000000000000000000000000"
	baseTree   = "tree000000000000000000000000000000000000"
	newTree    = "tree111111111111111111111111111111111111"
	newCommit  = "cmmt111111111111111111111111111111111111"
	prNodeID   = "PR_kwDONODEID"
	prHTMLURL  = "https://github.com/acme/widgets/pull/42"
	prNumber   = 42
	mergeQuery = "enablePullRequestAutoMerge"
)

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatal(err)
	}
}

func TestGitHubOpener_Open(t *testing.T) {
	var (
		createTreeBody string
		graphqlBody    string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/git/ref/"):
			writeJSON(t, w, map[string]any{"ref": "refs/heads/main", "object": map[string]any{"sha": baseSHA, "type": "commit"}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/git/refs"):
			writeJSON(t, w, map[string]any{"ref": "refs/heads/rere/web", "object": map[string]any{"sha": baseSHA}})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/git/commits/"):
			writeJSON(t, w, map[string]any{"sha": baseSHA, "tree": map[string]any{"sha": baseTree}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/git/trees"):
			createTreeBody = string(body)
			writeJSON(t, w, map[string]any{"sha": newTree})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/git/commits"):
			writeJSON(t, w, map[string]any{"sha": newCommit})
		case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/git/refs/"):
			writeJSON(t, w, map[string]any{"ref": "refs/heads/rere/web", "object": map[string]any{"sha": newCommit}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/pulls"):
			writeJSON(t, w, map[string]any{"number": prNumber, "node_id": prNodeID, "html_url": prHTMLURL})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/graphql"):
			graphqlBody = string(body)
			writeJSON(t, w, map[string]any{"data": map[string]any{mergeQuery: map[string]any{"pullRequest": map[string]any{"id": prNodeID}}}})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	rest := github.NewClient(srv.Client())
	rest.BaseURL, _ = url.Parse(srv.URL + "/")
	gql := githubv4.NewEnterpriseClient(srv.URL+"/graphql", srv.Client())
	opener := NewGitHubOpener(rest, gql)

	req := Request{
		Owner: "acme", Repo: "widgets",
		BaseBranch: "main", HeadBranch: "rere/web",
		Title: "chore: right-size web", Body: "by rere",
		Edits:           []FileEdit{{Path: "base/deploy.yaml", Content: "kind: Deployment\n"}},
		MergeMethod:     "squash",
		EnableAutoMerge: true,
	}
	res, err := opener.Open(t.Context(), req)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if res.Number != prNumber || res.NodeID != prNodeID || res.URL != prHTMLURL {
		t.Errorf("result = %+v", res)
	}
	if !res.AutoMergeEnabled {
		t.Error("AutoMergeEnabled = false")
	}
	if !strings.Contains(createTreeBody, "base/deploy.yaml") || !strings.Contains(createTreeBody, "kind: Deployment") {
		t.Errorf("tree request missing file edit: %s", createTreeBody)
	}
	// Auto-merge must use the PR node_id (not the number) and the squash method.
	if !strings.Contains(graphqlBody, prNodeID) {
		t.Errorf("graphql mutation did not use node_id: %s", graphqlBody)
	}
	if !strings.Contains(graphqlBody, "SQUASH") {
		t.Errorf("graphql mutation missing SQUASH merge method: %s", graphqlBody)
	}
}

func TestGitHubOpener_NoAutoMerge(t *testing.T) {
	graphqlCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/git/ref/"):
			writeJSON(t, w, map[string]any{"ref": "refs/heads/main", "object": map[string]any{"sha": baseSHA}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/git/refs"):
			writeJSON(t, w, map[string]any{"object": map[string]any{"sha": baseSHA}})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/git/commits/"):
			writeJSON(t, w, map[string]any{"sha": baseSHA, "tree": map[string]any{"sha": baseTree}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/git/trees"):
			writeJSON(t, w, map[string]any{"sha": newTree})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/git/commits"):
			writeJSON(t, w, map[string]any{"sha": newCommit})
		case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/git/refs/"):
			writeJSON(t, w, map[string]any{"object": map[string]any{"sha": newCommit}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/pulls"):
			writeJSON(t, w, map[string]any{"number": prNumber, "node_id": prNodeID, "html_url": prHTMLURL})
		case strings.HasSuffix(r.URL.Path, "/graphql"):
			graphqlCalled = true
			writeJSON(t, w, map[string]any{"data": map[string]any{}})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	rest := github.NewClient(srv.Client())
	rest.BaseURL, _ = url.Parse(srv.URL + "/")
	gql := githubv4.NewEnterpriseClient(srv.URL+"/graphql", srv.Client())
	opener := NewGitHubOpener(rest, gql)

	res, err := opener.Open(t.Context(), Request{
		Owner: "acme", Repo: "widgets", BaseBranch: "main", HeadBranch: "rere/web",
		Title: "t", Edits: []FileEdit{{Path: "a.yaml", Content: "x\n"}},
		EnableAutoMerge: false,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if res.AutoMergeEnabled {
		t.Error("AutoMergeEnabled should be false")
	}
	if graphqlCalled {
		t.Error("graphql should not be called when auto-merge disabled")
	}
}
