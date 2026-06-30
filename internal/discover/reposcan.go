package discover

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// RepoScanner is a cluster-free Discoverer: it indexes the YAML manifests under
// Root once, then matches workloads against that index. Include/Exclude are
// path globs (relative to Root, slash-separated) that scope which files are
// indexed — use them to disambiguate overlapping overlays.
type RepoScanner struct {
	Root    string
	Include []string
	Exclude []string

	once  sync.Once
	index map[metaKey][]Location
	err   error
}

type metaKey struct {
	kind      string
	name      string
	namespace string
}

func (s *RepoScanner) build() {
	s.index, s.err = buildIndex(s.Root, s.Include, s.Exclude)
}

// Discover implements Discoverer. The index is built lazily on first call.
func (s *RepoScanner) Discover(ctx context.Context, w Workload) (*Location, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.once.Do(s.build)
	if s.err != nil {
		return nil, s.err
	}

	var candidates []Location
	for key, locs := range s.index {
		if key.kind != w.Kind || key.name != w.Name {
			continue
		}
		// Match when the manifest omits a namespace (raw manifests do), the
		// namespaces agree, or the query left namespace unspecified.
		if w.Namespace == "" || key.namespace == "" || key.namespace == w.Namespace {
			candidates = append(candidates, locs...)
		}
	}

	switch len(candidates) {
	case 0:
		return nil, fmt.Errorf("%w: %s/%s", ErrNotFound, w.Kind, w.Name)
	case 1:
		return &candidates[0], nil
	default:
		sortLocations(candidates)
		return nil, &AmbiguousError{Workload: w, Candidates: candidates}
	}
}

func sortLocations(locs []Location) {
	sort.Slice(locs, func(i, j int) bool {
		if locs[i].File != locs[j].File {
			return locs[i].File < locs[j].File
		}
		return locs[i].DocIndex < locs[j].DocIndex
	})
}
