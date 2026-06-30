package policy

import (
	"math"

	"k8s.io/apimachinery/pkg/api/resource"
)

const mi = 1024 * 1024

// withinDeadband reports whether desired is within pct of cur in either
// direction (so it suppresses churny up- and down-sizes alike). A nil or zero
// current value is never "within" — those always apply (create). It compares
// quantities numerically, so 1Gi and 1024Mi are equal.
func withinDeadband(cur, desired *resource.Quantity, pct float64) bool {
	if cur == nil || cur.IsZero() {
		return false
	}
	c := cur.AsApproximateFloat64()
	d := desired.AsApproximateFloat64()
	if c == 0 {
		return false
	}
	return math.Abs(d-c)/c < pct
}

// scaleCPU multiplies a CPU quantity by mult, keeping milli precision.
func scaleCPU(q *resource.Quantity, mult float64) *resource.Quantity {
	milli := float64(q.MilliValue()) * mult
	return resource.NewMilliQuantity(int64(math.Round(milli)), resource.DecimalSI)
}

// scaleMem multiplies a memory quantity by mult and rounds up to a whole Mi, so
// values stay clean and idempotent across runs.
func scaleMem(q *resource.Quantity, mult float64) *resource.Quantity {
	bytes := float64(q.Value()) * mult
	miCount := int64(math.Ceil(bytes / mi))
	return resource.NewQuantity(miCount*mi, resource.BinarySI)
}
