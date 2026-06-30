// Package adapter normalizes recommender output (KRR) into rere's internal
// Target vocabulary: per-(workload, container) CPU/memory request & limit
// recommendations, stored as resource.Quantity.
package adapter

import (
	"errors"
	"fmt"
	"math"

	"k8s.io/apimachinery/pkg/api/resource"
)

// ResourceValues holds a CPU and/or memory quantity. A nil pointer means the
// value was not recommended and must not be written back.
type ResourceValues struct {
	CPU *resource.Quantity
	Mem *resource.Quantity
}

func (rv ResourceValues) empty() bool { return rv.CPU == nil && rv.Mem == nil }

// Max returns the per-field maximum of rv and other, treating nil as "no value".
func (rv ResourceValues) Max(other ResourceValues) ResourceValues {
	return ResourceValues{CPU: maxQuantity(rv.CPU, other.CPU), Mem: maxQuantity(rv.Mem, other.Mem)}
}

func maxQuantity(a, b *resource.Quantity) *resource.Quantity {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	case a.Cmp(*b) >= 0:
		return a
	default:
		return b
	}
}

// Recommended is a full recommendation for one container: requests and limits.
type Recommended struct {
	Requests ResourceValues
	Limits   ResourceValues
}

func (r Recommended) empty() bool { return r.Requests.empty() && r.Limits.empty() }

// Max returns the per-field maximum of r and other. When several recommender
// reports collapse onto one shared resources block (e.g. operator-CR instance
// pods), merging by max ensures the busiest instance's needs are met rather than
// whichever report was processed last.
func (r Recommended) Max(other Recommended) Recommended {
	return Recommended{
		Requests: r.Requests.Max(other.Requests),
		Limits:   r.Limits.Max(other.Limits),
	}
}

// Target is one (workload, container) pair with its recommended resources.
type Target struct {
	Namespace   string
	Kind        string
	Name        string
	Container   string
	Recommended Recommended
}

// Validate reports whether the target is well-formed: identity fields present
// and at least one recommended value to write.
func (t Target) Validate() error {
	switch {
	case t.Namespace == "":
		return errors.New("target: empty namespace")
	case t.Kind == "":
		return errors.New("target: empty kind")
	case t.Name == "":
		return errors.New("target: empty name")
	case t.Container == "":
		return errors.New("target: empty container")
	case t.Recommended.empty():
		return fmt.Errorf("target %s/%s: no recommended values", t.Kind, t.Name)
	}
	return nil
}

// cpuFromCores converts KRR's CPU value (cores, e.g. 0.25) to a milli-quantity
// (e.g. "250m"). KRR emits raw floats; resource.Quantity gives clean k8s units.
func cpuFromCores(c float64) *resource.Quantity {
	return resource.NewMilliQuantity(int64(math.Round(c*1000)), resource.DecimalSI)
}

// memFromBytes converts KRR's memory value (bytes, e.g. 134217728) to a binary
// quantity (e.g. "128Mi").
func memFromBytes(b float64) *resource.Quantity {
	return resource.NewQuantity(int64(b), resource.BinarySI)
}
