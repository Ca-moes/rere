package discover

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot() string { return filepath.Join("testdata", "repo") }

func TestDiscover_UniqueMatch(t *testing.T) {
	s := &RepoScanner{Root: repoRoot()}
	loc, err := s.Discover(t.Context(), Workload{Namespace: "default", Kind: "Deployment", Name: "web"})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !strings.HasSuffix(filepath.ToSlash(loc.File), "base/deploy.yaml") {
		t.Errorf("File = %q, want suffix base/deploy.yaml", loc.File)
	}
	if loc.DocIndex != 0 {
		t.Errorf("DocIndex = %d, want 0", loc.DocIndex)
	}
}

func TestDiscover_MultiDocIndex(t *testing.T) {
	s := &RepoScanner{Root: repoRoot()}
	loc, err := s.Discover(t.Context(), Workload{Kind: "Deployment", Name: "cache"})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !strings.HasSuffix(filepath.ToSlash(loc.File), "base/multi.yaml") {
		t.Errorf("File = %q, want suffix base/multi.yaml", loc.File)
	}
	if loc.DocIndex != 1 {
		t.Errorf("DocIndex = %d, want 1 (second doc)", loc.DocIndex)
	}
}

func TestDiscover_NotFound(t *testing.T) {
	s := &RepoScanner{Root: repoRoot()}
	_, err := s.Discover(t.Context(), Workload{Kind: "Deployment", Name: "ghost"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestDiscover_Ambiguous(t *testing.T) {
	s := &RepoScanner{Root: repoRoot()}
	_, err := s.Discover(t.Context(), Workload{Kind: "Deployment", Name: "api"})
	var amb *AmbiguousError
	if !errors.As(err, &amb) {
		t.Fatalf("err = %v, want *AmbiguousError", err)
	}
	if len(amb.Candidates) != 2 {
		t.Errorf("candidates = %d, want 2", len(amb.Candidates))
	}
}

func TestDiscover_NamespaceNarrows(t *testing.T) {
	s := &RepoScanner{Root: repoRoot()}
	loc, err := s.Discover(t.Context(), Workload{Namespace: "prod", Kind: "Deployment", Name: "api"})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !strings.Contains(filepath.ToSlash(loc.File), "overlays/prod/") {
		t.Errorf("File = %q, want overlays/prod", loc.File)
	}
}

func TestDiscover_IncludeScoping(t *testing.T) {
	s := &RepoScanner{Root: repoRoot(), Include: []string{"overlays/prod/*"}}
	loc, err := s.Discover(t.Context(), Workload{Kind: "Deployment", Name: "api"})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !strings.Contains(filepath.ToSlash(loc.File), "overlays/prod/") {
		t.Errorf("File = %q, want overlays/prod", loc.File)
	}
}

func TestDiscover_ExcludeScoping(t *testing.T) {
	s := &RepoScanner{Root: repoRoot(), Exclude: []string{"overlays/staging"}}
	loc, err := s.Discover(t.Context(), Workload{Kind: "Deployment", Name: "api"})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !strings.Contains(filepath.ToSlash(loc.File), "overlays/prod/") {
		t.Errorf("File = %q, want overlays/prod", loc.File)
	}
}

func TestDiscover_SkipsUnparseableFiles(t *testing.T) {
	// testdata/repo contains a Helm template (Go-template directives) that kyaml
	// cannot parse. One bad file must not abort the whole scan.
	s := &RepoScanner{Root: repoRoot()}
	loc, err := s.Discover(t.Context(), Workload{Namespace: "default", Kind: "Deployment", Name: "web"})
	if err != nil {
		t.Fatalf("unparseable file aborted the scan: %v", err)
	}
	if !strings.HasSuffix(filepath.ToSlash(loc.File), "base/deploy.yaml") {
		t.Errorf("File = %q, want base/deploy.yaml", loc.File)
	}
}

func TestDiscover_IgnoresNonManifests(t *testing.T) {
	// values.yaml has no kind/name and must never match.
	s := &RepoScanner{Root: repoRoot()}
	_, err := s.Discover(t.Context(), Workload{Kind: "", Name: ""})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty workload err = %v, want ErrNotFound", err)
	}
}
