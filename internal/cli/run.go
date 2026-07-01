package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
	"sigs.k8s.io/kustomize/kyaml/yaml"

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
// in dry-run mode. Mappers is the ordered FieldMapper registry; when empty it
// defaults to tier-1 only.
type Runner struct {
	Cfg        *config.Config
	Repo       string
	Discoverer discover.Discoverer
	Opener     pr.Opener
	Mappers    []fieldmap.FieldMapper
	FieldMaps  fieldmap.MapConfig   // merged maps, for translating CR workloads pre-grouping
	ChartMaps  fieldmap.ChartConfig // merged chart maps, for translating HelmRelease workloads pre-grouping
	Out        io.Writer
}

// selectMapper returns the first mapper that supports root, or nil if none do.
func selectMapper(ms []fieldmap.FieldMapper, root *yaml.RNode) fieldmap.FieldMapper {
	for _, m := range ms {
		if m.Supports(root) {
			return m
		}
	}
	return nil
}

type workloadGroup struct {
	Namespace string
	Kind      string
	Name      string
	Targets   []adapter.Target
}

// translateTargets rewrites workloads named by the recommender as the generated
// Deployment/Pod to the owning resource that lives in the repo — an operator CR
// (tier-2) or a Flux HelmRelease (tier-3) — before grouping, so a resource's
// several workloads collapse into one. Raw workloads pass through. A no-op when
// no maps are configured.
//
// The rewrite is committed only when the target actually exists in the repo: a
// real workload whose name coincidentally matches a built-in match rule (e.g. a
// Deployment "metrics-collector", or a bare Pod "foo-3") would otherwise be
// rewritten to a nonexistent resource and silently skipped, instead of being
// right-sized by tier-1.
func (r *Runner) translateTargets(ctx context.Context, targets []adapter.Target) []adapter.Target {
	if len(r.FieldMaps.Maps) == 0 && len(r.ChartMaps.Maps) == 0 {
		return targets
	}
	out := make([]adapter.Target, len(targets))
	for i, t := range targets {
		out[i] = t
		// Tier-2 operator CRs first, then tier-3 HelmReleases.
		if ct, ok := fieldmap.TranslateTarget(t, r.FieldMaps); ok && r.crResolvable(ctx, ct) {
			out[i] = ct
			continue
		}
		if ht, ok := fieldmap.TranslateHelmTarget(t, r.ChartMaps); ok && r.crResolvable(ctx, ht) {
			out[i] = ht
		}
	}
	return out
}

// crResolvable reports whether a translated CR identity matches a manifest in
// the repo. Anything but a definitive not-found counts as resolvable — a found
// or even ambiguous CR is a real CR (ambiguity is surfaced later by
// processWorkload), and a transient discover error is better handled there than
// by silently falling back — so we only revert to the untranslated workload
// when the CR is genuinely absent.
func (r *Runner) crResolvable(ctx context.Context, t adapter.Target) bool {
	_, err := r.Discoverer.Discover(ctx, discover.Workload{Namespace: t.Namespace, Kind: t.Kind, Name: t.Name})
	return !errors.Is(err, discover.ErrNotFound)
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
	if len(r.Mappers) == 0 {
		r.Mappers = []fieldmap.FieldMapper{fieldmap.Tier1{}}
	}
	targets = r.translateTargets(ctx, targets)
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

	// Parse the addressed doc and pick the FieldMapper that handles it. Selection
	// needs the parsed manifest, so it happens after discover/read — an
	// unsupported manifest is a skip, not a failure.
	root, err := yamledit.SelectDoc(orig, grp.Kind, grp.Name)
	if err != nil {
		return fmt.Errorf("parse %s: %w", loc.File, err)
	}
	if root == nil {
		fmt.Fprintf(r.Out, "skip %s: %s not found in %s\n", workloadRef(grp), grp.Kind, loc.File)
		return nil
	}
	mapper := selectMapper(r.Mappers, root)
	if mapper == nil {
		fmt.Fprintf(r.Out, "skip %s: no field mapper supports kind %q\n", workloadRef(grp), grp.Kind)
		return nil
	}

	var edits []yamledit.PathEdit
	for _, t := range mergeByContainer(grp.Targets) {
		tg := fieldmap.Target{Kind: grp.Kind, Container: t.Container}
		paths, err := resolveCells(mapper, root, tg)
		if err != nil {
			// The addressed container/subtree is absent — skip this target, not
			// the whole workload.
			fmt.Fprintf(r.Out, "skip %s container %q: %v\n", workloadRef(grp), t.Container, err)
			continue
		}
		cur, err := yamledit.ReadCurrentAt(root, func(section, res string) ([]string, error) {
			return paths[fieldmap.ResourceField{Section: section, Name: res}], nil
		})
		if err != nil {
			return fmt.Errorf("read current %s/%s: %w", workloadRef(grp), t.Container, err)
		}
		_, containerEdits := policy.Decide(cur, t.Recommended, r.Cfg.Policy)
		for _, e := range containerEdits {
			edits = append(edits, yamledit.PathEdit{
				Path:   paths[fieldmap.ResourceField{Section: e.Section, Name: e.Resource}],
				Value:  e.Value,
				Delete: e.Delete,
			})
		}
	}
	if len(edits) == 0 {
		fmt.Fprintf(r.Out, "%s/%s: no changes (within deadband)\n", grp.Kind, grp.Name)
		return nil
	}

	var buf bytes.Buffer
	changed, err := yamledit.ApplyPaths(bytes.NewReader(orig), &buf, grp.Kind, grp.Name, edits)
	if err != nil {
		return fmt.Errorf("apply %s/%s: %w", grp.Kind, grp.Name, err)
	}
	if !changed {
		fmt.Fprintf(r.Out, "%s/%s: no changes\n", grp.Kind, grp.Name)
		return nil
	}

	// The repo-relative path becomes the PR tree path; the absolute local path
	// must never leak there.
	rel, err := filepath.Rel(r.Repo, loc.File)
	if err != nil {
		return fmt.Errorf("relative path for %s under repo %s: %w", loc.File, r.Repo, err)
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

// mergeByContainer collapses targets that resolve to the same container or
// component into one, taking the max recommendation per field. Several
// operator-CR instance pods (e.g. CNPG mycluster-1..N) translate to one CR with
// the same component, so without this their shared resources block would get
// last-write-wins edits from whichever instance the recommender reported last —
// dropping the busier replicas' needs. Distinct containers (tier-1
// multi-container pods) are left untouched.
func mergeByContainer(targets []adapter.Target) []adapter.Target {
	idx := map[string]int{}
	var out []adapter.Target
	for _, t := range targets {
		if i, ok := idx[t.Container]; ok {
			out[i].Recommended = out[i].Recommended.Max(t.Recommended)
			continue
		}
		idx[t.Container] = len(out)
		out = append(out, t)
	}
	return out
}

// resolveCells resolves the four requests/limits × cpu/memory paths for a
// target once, so the current-value read and the edits share them and an absent
// container/subtree is caught before any work begins.
func resolveCells(m fieldmap.FieldMapper, root *yaml.RNode, tg fieldmap.Target) (map[fieldmap.ResourceField][]string, error) {
	cells := []fieldmap.ResourceField{
		{Section: "requests", Name: "cpu"}, {Section: "requests", Name: "memory"},
		{Section: "limits", Name: "cpu"}, {Section: "limits", Name: "memory"},
	}
	paths := make(map[fieldmap.ResourceField][]string, len(cells))
	for _, f := range cells {
		p, err := m.ResolvePath(root, tg, f)
		if err != nil {
			return nil, err
		}
		paths[f] = p
	}
	return paths, nil
}

// branchName builds a head branch unique per workload, including the namespace
// so same-name workloads in different namespaces do not collide. The identity
// comes from untrusted recommender JSON, so it is sanitized to a ref-safe form.
func (r *Runner) branchName(grp workloadGroup) string {
	parts := make([]string, 0, 3)
	if grp.Namespace != "" {
		parts = append(parts, grp.Namespace)
	}
	parts = append(parts, strings.ToLower(grp.Kind), grp.Name)
	return r.Cfg.Git.BranchPrefix + sanitizeRefComponent(strings.Join(parts, "-"))
}

var refUnsafe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// sanitizeRefComponent maps a workload identity to a git-ref-safe branch
// component: unsafe characters collapse to "-", ".." sequences (invalid in
// refs) collapse, and leading/trailing separators are trimmed.
func sanitizeRefComponent(s string) string {
	s = refUnsafe.ReplaceAllString(s, "-")
	for strings.Contains(s, "..") {
		s = strings.ReplaceAll(s, "..", ".")
	}
	return strings.Trim(s, ".-")
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
