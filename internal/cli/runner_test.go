package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/Ca-moes/rere/internal/adapter"
	"github.com/Ca-moes/rere/internal/config"
	"github.com/Ca-moes/rere/internal/discover"
	"github.com/Ca-moes/rere/internal/fieldmap"
	"github.com/Ca-moes/rere/internal/pr"
)

const deployManifest = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  template:
    spec:
      containers:
        - name: web
          resources:
            requests:
              cpu: "1"
              memory: 512Mi
`

func q(s string) *resource.Quantity {
	v := resource.MustParse(s)
	return &v
}

type fakeDiscoverer struct {
	loc *discover.Location
	err error
}

func (f fakeDiscoverer) Discover(context.Context, discover.Workload) (*discover.Location, error) {
	return f.loc, f.err
}

type fakeOpener struct {
	called int
	got    pr.Request
}

func (f *fakeOpener) Open(_ context.Context, req pr.Request) (*pr.Result, error) {
	f.called++
	f.got = req
	return &pr.Result{Number: 7, URL: "https://example/pull/7", NodeID: "N", AutoMergeEnabled: req.EnableAutoMerge}, nil
}

func writeManifest(t *testing.T) (dir, path string) {
	t.Helper()
	dir = t.TempDir()
	path = filepath.Join(dir, "deploy.yaml")
	if err := os.WriteFile(path, []byte(deployManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, path
}

func cpuTarget(value string) []adapter.Target {
	return []adapter.Target{{
		Namespace: "default", Kind: "Deployment", Name: "web", Container: "web",
		Recommended: adapter.Recommended{Requests: adapter.ResourceValues{CPU: q(value)}},
	}}
}

func TestRunner_DryRunPrintsDiffAndDoesNotWrite(t *testing.T) {
	dir, path := writeManifest(t)
	cfg := config.Defaults()
	cfg.DryRun = true
	var out bytes.Buffer
	r := &Runner{
		Cfg:        cfg,
		Repo:       dir,
		Discoverer: fakeDiscoverer{loc: &discover.Location{File: path, DocIndex: 0}},
		Out:        &out,
	}
	if err := r.Run(context.Background(), cpuTarget("250m")); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "250m") {
		t.Errorf("diff output missing new value:\n%s", out.String())
	}
	after, _ := os.ReadFile(path)
	if string(after) != deployManifest {
		t.Error("dry-run modified the file on disk")
	}
}

func TestRunner_LiveOpensPR(t *testing.T) {
	dir, path := writeManifest(t)
	cfg := config.Defaults()
	cfg.Git.Repo = "acme/widgets"
	opener := &fakeOpener{}
	var out bytes.Buffer
	r := &Runner{
		Cfg:        cfg,
		Repo:       dir,
		Discoverer: fakeDiscoverer{loc: &discover.Location{File: path, DocIndex: 0}},
		Opener:     opener,
		Out:        &out,
	}
	if err := r.Run(context.Background(), cpuTarget("250m")); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if opener.called != 1 {
		t.Fatalf("Open called %d times, want 1", opener.called)
	}
	if opener.got.Owner != "acme" || opener.got.Repo != "widgets" {
		t.Errorf("owner/repo = %q/%q", opener.got.Owner, opener.got.Repo)
	}
	if len(opener.got.Edits) != 1 || opener.got.Edits[0].Path != "deploy.yaml" {
		t.Fatalf("edits = %+v", opener.got.Edits)
	}
	if !strings.Contains(opener.got.Edits[0].Content, "250m") {
		t.Errorf("edit content missing new value:\n%s", opener.got.Edits[0].Content)
	}
	// Live mode writes via the API, never the working tree.
	after, _ := os.ReadFile(path)
	if string(after) != deployManifest {
		t.Error("live run modified the file on disk")
	}
}

func TestRunner_NoChangeWithinDeadband(t *testing.T) {
	dir, path := writeManifest(t)
	cfg := config.Defaults()
	opener := &fakeOpener{}
	var out bytes.Buffer
	r := &Runner{
		Cfg:        cfg,
		Repo:       dir,
		Discoverer: fakeDiscoverer{loc: &discover.Location{File: path, DocIndex: 0}},
		Opener:     opener,
		Out:        &out,
	}
	// Current cpu is 1 (1000m); recommend 1050m -> within 10% deadband.
	if err := r.Run(context.Background(), cpuTarget("1050m")); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if opener.called != 0 {
		t.Errorf("Open should not be called for a no-op")
	}
}

// flakyOpener fails the Nth Open call (1-based) and succeeds otherwise.
type flakyOpener struct {
	called int
	failOn int
}

func (f *flakyOpener) Open(_ context.Context, req pr.Request) (*pr.Result, error) {
	f.called++
	if f.called == f.failOn {
		return nil, fmt.Errorf("simulated 422: branch %q already exists", req.HeadBranch)
	}
	return &pr.Result{Number: f.called, URL: "u", NodeID: "n", AutoMergeEnabled: req.EnableAutoMerge}, nil
}

func namedDeployment(name string) string {
	return strings.Replace(deployManifest, "name: web", "name: "+name, 1)
}

func TestRunner_ContinuesPastPRFailure(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"web", "api"} {
		if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(namedDeployment(name)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Defaults()
	cfg.Git.Repo = "acme/widgets"
	opener := &flakyOpener{failOn: 1} // first workload's PR fails
	var out bytes.Buffer
	r := &Runner{
		Cfg:        cfg,
		Repo:       dir,
		Discoverer: &discover.RepoScanner{Root: dir},
		Opener:     opener,
		Out:        &out,
	}
	targets := []adapter.Target{
		{Namespace: "default", Kind: "Deployment", Name: "web", Container: "web",
			Recommended: adapter.Recommended{Requests: adapter.ResourceValues{CPU: q("250m")}}},
		{Namespace: "default", Kind: "Deployment", Name: "api", Container: "web",
			Recommended: adapter.Recommended{Requests: adapter.ResourceValues{CPU: q("250m")}}},
	}

	err := r.Run(context.Background(), targets)
	if opener.called != 2 {
		t.Errorf("both workloads should be attempted despite the first failing, got %d Open calls", opener.called)
	}
	if err == nil {
		t.Error("expected an aggregated error reporting the failed workload")
	}
}

type recordingOpener struct {
	reqs []pr.Request
}

func (o *recordingOpener) Open(_ context.Context, req pr.Request) (*pr.Result, error) {
	o.reqs = append(o.reqs, req)
	return &pr.Result{Number: len(o.reqs), URL: "u", NodeID: "n", AutoMergeEnabled: req.EnableAutoMerge}, nil
}

func namespacedDeployment(name, ns string) string {
	return strings.Replace(deployManifest,
		"metadata:\n  name: web",
		"metadata:\n  name: "+name+"\n  namespace: "+ns, 1)
}

func targetNs(name, ns, value string) adapter.Target {
	return adapter.Target{
		Namespace: ns, Kind: "Deployment", Name: name, Container: "web",
		Recommended: adapter.Recommended{Requests: adapter.ResourceValues{CPU: q(value)}},
	}
}

func TestRunner_BranchIncludesNamespace(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(namespacedDeployment("web", "team-a")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.yaml"), []byte(namespacedDeployment("web", "team-b")), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Git.Repo = "acme/widgets"
	opener := &recordingOpener{}
	r := &Runner{
		Cfg: cfg, Repo: dir,
		Discoverer: &discover.RepoScanner{Root: dir},
		Opener:     opener,
		Out:        &bytes.Buffer{},
	}
	targets := []adapter.Target{targetNs("web", "team-a", "250m"), targetNs("web", "team-b", "250m")}
	if err := r.Run(context.Background(), targets); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(opener.reqs) != 2 {
		t.Fatalf("expected 2 PRs, got %d", len(opener.reqs))
	}
	if opener.reqs[0].HeadBranch == opener.reqs[1].HeadBranch {
		t.Errorf("branches collide across namespaces: both %q", opener.reqs[0].HeadBranch)
	}
	if !strings.Contains(opener.reqs[0].HeadBranch, "team-a") || !strings.Contains(opener.reqs[1].HeadBranch, "team-b") {
		t.Errorf("branches missing namespace: %q, %q", opener.reqs[0].HeadBranch, opener.reqs[1].HeadBranch)
	}
}

type autoMergeFailOpener struct{ called int }

func (o *autoMergeFailOpener) Open(_ context.Context, _ pr.Request) (*pr.Result, error) {
	o.called++
	// The PR opened, but auto-merge could not be enabled (repo setting).
	return &pr.Result{Number: 5, URL: "https://example/pull/5", NodeID: "n", AutoMergeEnabled: false},
		fmt.Errorf("enable auto-merge: not allowed on this repository")
}

func TestRunner_AutoMergeFailureNotFatal(t *testing.T) {
	dir, path := writeManifest(t)
	cfg := config.Defaults()
	cfg.Git.Repo = "acme/widgets"
	opener := &autoMergeFailOpener{}
	var out bytes.Buffer
	r := &Runner{
		Cfg: cfg, Repo: dir,
		Discoverer: fakeDiscoverer{loc: &discover.Location{File: path, DocIndex: 0}},
		Opener:     opener,
		Out:        &out,
	}
	if err := r.Run(context.Background(), cpuTarget("250m")); err != nil {
		t.Errorf("auto-merge failure should not fail the run, got: %v", err)
	}
	if !strings.Contains(out.String(), "https://example/pull/5") {
		t.Errorf("opened PR URL must be surfaced:\n%s", out.String())
	}
	if !strings.Contains(strings.ToLower(out.String()), "auto-merge") {
		t.Errorf("should warn about auto-merge:\n%s", out.String())
	}
}

func TestRunner_Tier2OperatorCR(t *testing.T) {
	// The recommender names the generated Deployment "otel-collector"; rere must
	// translate that to the OpenTelemetryCollector CR "otel" and edit its
	// spec.resources — proving the full tier-2 chain end-to-end.
	dir := t.TempDir()
	cr := `apiVersion: opentelemetry.io/v1beta1
kind: OpenTelemetryCollector
metadata:
  name: otel
  namespace: default
spec:
  mode: deployment
  resources:
    requests:
      cpu: "1"
      memory: 256Mi
  config: |
    receivers: {}
`
	path := filepath.Join(dir, "otel.yaml")
	if err := os.WriteFile(path, []byte(cr), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.DryRun = true
	maps := fieldmap.MergedMaps(fieldmap.MapConfig{})
	var out bytes.Buffer
	r := &Runner{
		Cfg:        cfg,
		Repo:       dir,
		Discoverer: &discover.RepoScanner{Root: dir},
		Mappers:    []fieldmap.FieldMapper{fieldmap.Tier2{Maps: maps}, fieldmap.Tier1{}},
		FieldMaps:  maps,
		Out:        &out,
	}
	targets := []adapter.Target{{
		Namespace: "default", Kind: "Deployment", Name: "otel-collector", Container: "otc-container",
		Recommended: adapter.Recommended{Requests: adapter.ResourceValues{CPU: q("250m")}},
	}}
	if err := r.Run(context.Background(), targets); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "250m") {
		t.Errorf("expected the CR's spec.resources to be downsized to 250m:\n%s", out.String())
	}
	after, _ := os.ReadFile(path)
	if string(after) != cr {
		t.Error("dry-run modified the CR on disk")
	}
}

func TestRunner_SkipsUnsupportedManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cr.yaml")
	cr := `apiVersion: example.com/v1
kind: WidgetSet
metadata:
  name: w
spec:
  resources:
    requests:
      cpu: "1"
`
	if err := os.WriteFile(path, []byte(cr), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.DryRun = true
	var out bytes.Buffer
	r := &Runner{
		Cfg:        cfg,
		Repo:       dir,
		Discoverer: fakeDiscoverer{loc: &discover.Location{File: path, DocIndex: 0}},
		Out:        &out,
	}
	target := []adapter.Target{{
		Namespace: "default", Kind: "WidgetSet", Name: "w", Container: "main",
		Recommended: adapter.Recommended{Requests: adapter.ResourceValues{CPU: q("250m")}},
	}}
	if err := r.Run(context.Background(), target); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(strings.ToLower(out.String()), "no field mapper") {
		t.Errorf("expected a 'no field mapper' skip, got:\n%s", out.String())
	}
	if strings.Contains(out.String(), "250m") {
		t.Errorf("unsupported manifest must not be edited:\n%s", out.String())
	}
}

func TestRunner_SkipsNotFound(t *testing.T) {
	cfg := config.Defaults()
	cfg.DryRun = true
	var out bytes.Buffer
	r := &Runner{
		Cfg:        cfg,
		Repo:       t.TempDir(),
		Discoverer: fakeDiscoverer{err: discover.ErrNotFound},
		Out:        &out,
	}
	if err := r.Run(context.Background(), cpuTarget("250m")); err != nil {
		t.Fatalf("Run should not fail on not-found: %v", err)
	}
	if !strings.Contains(strings.ToLower(out.String()), "skip") {
		t.Errorf("expected skip message, got:\n%s", out.String())
	}
}

func TestRunner_SkipsAmbiguous(t *testing.T) {
	cfg := config.Defaults()
	cfg.DryRun = true
	amb := &discover.AmbiguousError{
		Workload:   discover.Workload{Kind: "Deployment", Name: "web"},
		Candidates: []discover.Location{{File: "a.yaml"}, {File: "b.yaml"}},
	}
	r := &Runner{Cfg: cfg, Repo: t.TempDir(), Discoverer: fakeDiscoverer{err: amb}, Out: &bytes.Buffer{}}
	if err := r.Run(context.Background(), cpuTarget("250m")); err != nil {
		t.Errorf("ambiguous match should be a skip, not a failure: %v", err)
	}
}

func TestRunner_DiscoverErrorIsFailure(t *testing.T) {
	cfg := config.Defaults()
	cfg.DryRun = true
	// A systemic discover error (e.g. cached index-build I/O failure, or a
	// cancelled context) must fail the run, not silently no-op as success.
	r := &Runner{
		Cfg:        cfg,
		Repo:       t.TempDir(),
		Discoverer: fakeDiscoverer{err: errors.New("index build: permission denied")},
		Out:        &bytes.Buffer{},
	}
	if err := r.Run(context.Background(), cpuTarget("250m")); err == nil {
		t.Error("a non-skip discover error must fail the run, got nil")
	}
}
