// Package policy decides whether and how to apply a recommendation: it applies
// a symmetric deadband (skip small changes either direction), headroom
// multipliers, and a limits policy, emitting yamledit.Edits for what survives.
package policy

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/Ca-moes/rere/internal/adapter"
	"github.com/Ca-moes/rere/internal/yamledit"
)

// Config tunes the decision engine. All knobs are overridable per repo.
//
// Note: this models the M1 default (remove CPU limit, set mem limit = request)
// with explicit fields rather than a single LimitsPolicy enum, so CPU and
// memory limit handling stay independent and extensible.
type Config struct {
	DeadbandPct    float64 `json:"deadbandPct"`    // skip when |desired-current|/current < pct, both directions
	CPUHeadroom    float64 `json:"cpuHeadroom"`    // multiplier on the recommended CPU request (e.g. 1.0)
	MemHeadroom    float64 `json:"memHeadroom"`    // multiplier on the recommended memory request (e.g. 1.15)
	RemoveCPULimit bool    `json:"removeCpuLimit"` // drop CPU limits when present (compressible; limits cause throttling)
	MemLimitRatio  float64 `json:"memLimitRatio"`  // memory limit = memory request * ratio; 0 leaves it untouched
}

// Defaults returns rere's out-of-the-box policy.
func Defaults() Config {
	return Config{
		DeadbandPct:    0.10,
		CPUHeadroom:    1.0,
		MemHeadroom:    1.15,
		RemoveCPULimit: true,
		MemLimitRatio:  1.0,
	}
}

// Decision is the human-readable record of one field's outcome.
type Decision struct {
	Field   string // e.g. "requests.cpu"
	Current *resource.Quantity
	Desired *resource.Quantity // nil means "remove"
	Apply   bool
	Reason  string
}

// Decide compares current values (from the manifest) against recommendations
// (from KRR) under cfg, returning a decision per field and the edits to apply.
// The returned edits carry no Container — the caller scopes them to a target.
func Decide(cur, rec adapter.Recommended, cfg Config) ([]Decision, []yamledit.Edit) {
	var decisions []Decision
	var edits []yamledit.Edit

	add := func(d Decision, e *yamledit.Edit) {
		decisions = append(decisions, d)
		if e != nil {
			edits = append(edits, *e)
		}
	}

	// requests.cpu
	if rec.Requests.CPU != nil {
		add(decideSet("requests", "cpu", cur.Requests.CPU, scaleCPU(rec.Requests.CPU, cfg.CPUHeadroom), cfg.DeadbandPct))
	}

	// requests.memory (also the basis for the memory limit)
	var effectiveMemReq *resource.Quantity
	if rec.Requests.Mem != nil {
		desiredMemReq := scaleMem(rec.Requests.Mem, cfg.MemHeadroom)
		dec, edit := decideSet("requests", "memory", cur.Requests.Mem, desiredMemReq, cfg.DeadbandPct)
		add(dec, edit)
		// Derive the limit from the request that will actually be in the
		// manifest: the new value when we apply it, otherwise the retained
		// current value. Using the desired (lower) request unconditionally
		// could set a limit below a retained, higher request — invalid.
		if dec.Apply {
			effectiveMemReq = desiredMemReq
		} else {
			effectiveMemReq = cur.Requests.Mem
		}
	}

	// limits.cpu: remove (CPU is compressible).
	if cfg.RemoveCPULimit && cur.Limits.CPU != nil {
		add(
			Decision{Field: "limits.cpu", Current: cur.Limits.CPU, Apply: true, Reason: "remove cpu limit"},
			&yamledit.Edit{Section: "limits", Resource: "cpu", Delete: true},
		)
	}

	// limits.memory = effective memory request * ratio, but never below the
	// request itself: Kubernetes rejects limits.memory < requests.memory.
	if cfg.MemLimitRatio > 0 && effectiveMemReq != nil {
		desiredMemLimit := scaleMem(effectiveMemReq, cfg.MemLimitRatio)
		if desiredMemLimit.Cmp(*effectiveMemReq) < 0 { // guard a misconfigured ratio < 1
			desiredMemLimit = effectiveMemReq
		}
		dec, edit := decideSet("limits", "memory", cur.Limits.Mem, desiredMemLimit, cfg.DeadbandPct)
		// The deadband must not suppress an increase needed to keep the limit
		// at or above the (applied) request — that would write an invalid
		// manifest. cur.Limits.Mem is non-nil whenever the edit was suppressed.
		if !dec.Apply && cur.Limits.Mem.Cmp(*effectiveMemReq) < 0 {
			dec = Decision{
				Field: "limits.memory", Current: cur.Limits.Mem, Desired: desiredMemLimit,
				Apply: true, Reason: "raise limit to stay >= request",
			}
			edit = &yamledit.Edit{Section: "limits", Resource: "memory", Value: desiredMemLimit.String()}
		}
		add(dec, edit)
	}

	return decisions, edits
}

// decideSet builds the decision and edit for setting one resource field,
// honoring the deadband against the current value.
func decideSet(section, res string, cur, desired *resource.Quantity, pct float64) (Decision, *yamledit.Edit) {
	field := section + "." + res
	if cur != nil && withinDeadband(cur, desired, pct) {
		return Decision{
			Field: field, Current: cur, Desired: desired, Apply: false,
			Reason: fmt.Sprintf("within %.0f%% deadband", pct*100),
		}, nil
	}
	reason := "apply"
	if cur == nil {
		reason = "create (no current value)"
	}
	return Decision{Field: field, Current: cur, Desired: desired, Apply: true, Reason: reason},
		&yamledit.Edit{Section: section, Resource: res, Value: desired.String()}
}
