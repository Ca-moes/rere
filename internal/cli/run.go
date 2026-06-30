package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pmezard/go-difflib/difflib"

	"github.com/Ca-moes/rere/internal/adapter"
	"github.com/Ca-moes/rere/internal/config"
	"github.com/Ca-moes/rere/internal/discover"
	"github.com/Ca-moes/rere/internal/fieldmap"
	"github.com/Ca-moes/rere/internal/policy"
	"github.com/Ca-moes/rere/internal/pr"
	"github.com/Ca-moes/rere/internal/yamledit"
)

// Runner executes the pipeline for a set of targets. It depends on the
// Discoverer and Opener interfaces so it is fully fake-testable; Opener is nil
// in dry-run mode.
type Runner struct {
	Cfg        *config.Config
	Repo       string
	Discoverer discover.Discoverer
	Opener     pr.Opener
	Out        io.Writer
}

type workloadGroup struct {
	Namespace string
	Kind      string
	Name      string
	Targets   []adapter.Target
}

// groupByWorkload collapses per-container targets into one group per workload,
// so each workload yields a single PR.
func groupByWorkload(targets []adapter.Target) []workloadGroup {
	idx := map[string]int{}
	var groups []workloadGroup
	for _, t := range targets {
		key := t.Namespace + "/" + t.Kind + "/" + t.Name
		i, ok := idx[key]
		if !ok {
			groups = append(groups, workloadGroup{Namespace: t.Namespace, Kind: t.Kind, Name: t.Name})
			i = len(groups) - 1
			idx[key] = i
		}
		groups[i].Targets = append(groups[i].Targets, t)
	}
	return groups
}

// Run processes every workload group independently: a per-workload failure
// (e.g. a PR-open 422 from an existing branch, or a rate limit) is logged and
// counted, never aborting the rest — this tool runs repeatedly on a schedule.
// Run returns an aggregated error if any workload failed, so the exit code
// surfaces it. Expected skips (not found, ambiguous, unsupported kind, no
// changes) are not failures.
func (r *Runner) Run(ctx context.Context, targets []adapter.Target) error {
	var failed int
	for _, grp := range groupByWorkload(targets) {
		if err := r.processWorkload(ctx, grp); err != nil {
			fmt.Fprintf(r.Out, "error %s/%s: %v\n", grp.Kind, grp.Name, err)
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d workload(s) failed; see log above", failed)
	}
	return nil
}

func (r *Runner) processWorkload(ctx context.Context, grp workloadGroup) error {
	if !fieldmap.Tier1Supports(grp.Kind) {
		fmt.Fprintf(r.Out, "skip %s/%s: kind %q not supported in M1 (tier-1 only)\n", grp.Kind, grp.Name, grp.Kind)
		return nil
	}

	loc, err := r.Discoverer.Discover(ctx, discover.Workload{Namespace: grp.Namespace, Kind: grp.Kind, Name: grp.Name})
	if err != nil {
		// Only not-found and ambiguous are expected skips. Anything else (a
		// cancelled context, a cached index-build I/O error) is a real failure
		// — don't swallow it as a success.
		var amb *discover.AmbiguousError
		if errors.Is(err, discover.ErrNotFound) || errors.As(err, &amb) {
			fmt.Fprintf(r.Out, "skip %s: %v\n", workloadRef(grp), err)
			return nil
		}
		return fmt.Errorf("discover %s: %w", workloadRef(grp), err)
	}

	orig, err := os.ReadFile(loc.File)
	if err != nil {
		return fmt.Errorf("read %s: %w", loc.File, err)
	}

	var edits []yamledit.Edit
	for _, t := range grp.Targets {
		cur, err := yamledit.ReadCurrent(bytes.NewReader(orig), grp.Kind, grp.Name, t.Container)
		if err != nil {
			return fmt.Errorf("read current %s/%s/%s: %w", grp.Kind, grp.Name, t.Container, err)
		}
		_, containerEdits := policy.Decide(cur, t.Recommended, r.Cfg.Policy)
		for i := range containerEdits {
			containerEdits[i].Container = t.Container
		}
		edits = append(edits, containerEdits...)
	}
	if len(edits) == 0 {
		fmt.Fprintf(r.Out, "%s/%s: no changes (within deadband)\n", grp.Kind, grp.Name)
		return nil
	}

	var buf bytes.Buffer
	changed, err := yamledit.Apply(bytes.NewReader(orig), &buf, grp.Kind, grp.Name, edits)
	if err != nil {
		return fmt.Errorf("apply %s/%s: %w", grp.Kind, grp.Name, err)
	}
	if !changed {
		fmt.Fprintf(r.Out, "%s/%s: no changes\n", grp.Kind, grp.Name)
		return nil
	}

	rel, err := filepath.Rel(r.Repo, loc.File)
	if err != nil {
		rel = loc.File
	}
	rel = filepath.ToSlash(rel)

	if r.Cfg.DryRun {
		return r.printDiff(rel, orig, buf.Bytes())
	}
	return r.openPR(ctx, grp, rel, buf.String())
}

func (r *Runner) printDiff(path string, before, after []byte) error {
	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(before)),
		B:        difflib.SplitLines(string(after)),
		FromFile: "a/" + path,
		ToFile:   "b/" + path,
		Context:  3,
	})
	if err != nil {
		return err
	}
	fmt.Fprint(r.Out, diff)
	return nil
}

func (r *Runner) openPR(ctx context.Context, grp workloadGroup, path, content string) error {
	owner, repo, ok := strings.Cut(r.Cfg.Git.Repo, "/")
	if !ok {
		return fmt.Errorf("git.repo must be owner/name, got %q", r.Cfg.Git.Repo)
	}
	res, err := r.Opener.Open(ctx, pr.Request{
		Owner:           owner,
		Repo:            repo,
		BaseBranch:      r.Cfg.Git.BaseBranch,
		HeadBranch:      r.branchName(grp),
		Title:           "chore(rere): right-size " + workloadRef(grp),
		Body:            "Automated resource right-sizing by rere.",
		Edits:           []pr.FileEdit{{Path: path, Content: content}},
		MergeMethod:     r.Cfg.Git.MergeMethod,
		EnableAutoMerge: r.Cfg.Git.AutoMerge,
	})
	if res != nil {
		// The PR opened. A non-nil error here means only that auto-merge could
		// not be enabled (a repo setting) — surface it as a warning, not a
		// failure, so the run continues and the URL is not lost.
		fmt.Fprintf(r.Out, "opened PR #%d %s (auto-merge=%v)\n", res.Number, res.URL, res.AutoMergeEnabled)
		if err != nil {
			fmt.Fprintf(r.Out, "  warning: auto-merge not enabled: %v\n", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("open PR for %s: %w", workloadRef(grp), err)
	}
	return nil
}

// branchName builds a head branch unique per workload, including the namespace
// so same-name workloads in different namespaces do not collide.
func (r *Runner) branchName(grp workloadGroup) string {
	parts := make([]string, 0, 3)
	if grp.Namespace != "" {
		parts = append(parts, grp.Namespace)
	}
	parts = append(parts, strings.ToLower(grp.Kind), grp.Name)
	return r.Cfg.Git.BranchPrefix + strings.Join(parts, "-")
}

// workloadRef is the human-readable identity of a workload, namespace-qualified.
func workloadRef(grp workloadGroup) string {
	if grp.Namespace != "" {
		return grp.Namespace + "/" + grp.Kind + "/" + grp.Name
	}
	return grp.Kind + "/" + grp.Name
}

// readRecommendations parses the configured KRR input (file or stdin).
func readRecommendations(cfg *config.Config) ([]adapter.Target, error) {
	in := cfg.Recommender.Input
	if in == "" || in == "-" {
		return adapter.ParseKRR(os.Stdin)
	}
	f, err := os.Open(in)
	if err != nil {
		return nil, fmt.Errorf("open recommender input: %w", err)
	}
	defer func() { _ = f.Close() }()
	return adapter.ParseKRR(f)
}
