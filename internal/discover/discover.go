// Package discover locates the manifest backing a workload by scanning a local
// checkout (no cluster). It matches on kind + name, and on namespace when the
// manifest carries one.
package discover

import (
	"context"
	"errors"
	"fmt"
)

// Workload identifies a target to locate. Namespace may be empty: KRR always
// provides one, but raw manifests often omit it (kustomize sets it later).
type Workload struct {
	Namespace string
	Kind      string
	Name      string
}

// Location is where a workload's manifest lives: a file and the 0-based index
// of the document within that file (multi-doc YAML).
type Location struct {
	File     string
	DocIndex int
}

// ErrNotFound is returned when no manifest matches the workload.
var ErrNotFound = errors.New("discover: no manifest matches workload")

// AmbiguousError is returned when more than one manifest matches and scoping
// did not narrow it to a single location.
type AmbiguousError struct {
	Workload   Workload
	Candidates []Location
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("discover: %d manifests match %s/%s (namespace %q); scope with include/exclude",
		len(e.Candidates), e.Workload.Kind, e.Workload.Name, e.Workload.Namespace)
}

// Discoverer maps a workload to the manifest that defines it.
type Discoverer interface {
	Discover(ctx context.Context, w Workload) (*Location, error)
}
