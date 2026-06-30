package policy

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/Ca-moes/rere/internal/adapter"
)

func q(s string) *resource.Quantity {
	v := resource.MustParse(s)
	return &v
}

func TestDefaults(t *testing.T) {
	d := Defaults()
	if d.DeadbandPct != 0.10 || d.CPUHeadroom != 1.0 || d.MemHeadroom != 1.15 {
		t.Errorf("defaults = %+v", d)
	}
	if !d.RemoveCPULimit || d.MemLimitRatio != 1.0 {
		t.Errorf("limit defaults = %+v", d)
	}
}

// findEdit returns the edit for section/resource, or nil.
func findEdit(edits []editLike, section, res string) *editLike {
	for i := range edits {
		if edits[i].Section == section && edits[i].Resource == res {
			return &edits[i]
		}
	}
	return nil
}

type editLike struct {
	Section, Resource, Value string
	Delete                   bool
}

func decideEdits(cur, rec adapter.Recommended, cfg Config) []editLike {
	_, edits := Decide(cur, rec, cfg)
	out := make([]editLike, len(edits))
	for i, e := range edits {
		out[i] = editLike{Section: e.Section, Resource: e.Resource, Value: e.Value, Delete: e.Delete}
	}
	return out
}

func TestDecide_Downsize(t *testing.T) {
	cfg := Defaults()
	cur := adapter.Recommended{Requests: adapter.ResourceValues{CPU: q("1000m")}}
	rec := adapter.Recommended{Requests: adapter.ResourceValues{CPU: q("250m")}}
	edits := decideEdits(cur, rec, cfg)
	e := findEdit(edits, "requests", "cpu")
	if e == nil || e.Value != "250m" {
		t.Fatalf("downsize edit = %+v", e)
	}
}

func TestDecide_DeadbandSkip(t *testing.T) {
	cfg := Defaults() // 10%
	cur := adapter.Recommended{Requests: adapter.ResourceValues{CPU: q("100m")}}
	rec := adapter.Recommended{Requests: adapter.ResourceValues{CPU: q("105m")}}
	if e := findEdit(decideEdits(cur, rec, cfg), "requests", "cpu"); e != nil {
		t.Fatalf("expected skip within deadband, got edit %+v", e)
	}
}

func TestDecide_CreateWhenAbsent(t *testing.T) {
	cfg := Defaults()
	rec := adapter.Recommended{Requests: adapter.ResourceValues{CPU: q("250m")}}
	if e := findEdit(decideEdits(adapter.Recommended{}, rec, cfg), "requests", "cpu"); e == nil {
		t.Fatal("expected create edit when current is nil")
	}
}

func TestDecide_CrossUnitDeadband(t *testing.T) {
	cfg := Defaults()
	cfg.MemHeadroom = 1.0
	// 1Gi current vs 1073741824 bytes recommended == identical; must skip.
	cur := adapter.Recommended{Requests: adapter.ResourceValues{Mem: q("1Gi")}}
	rec := adapter.Recommended{Requests: adapter.ResourceValues{Mem: q("1073741824")}}
	if e := findEdit(decideEdits(cur, rec, cfg), "requests", "memory"); e != nil {
		t.Fatalf("1Gi vs 1024Mi must be equal (skip), got %+v", e)
	}
}

func TestDecide_RemoveCPULimit(t *testing.T) {
	cfg := Defaults()
	cur := adapter.Recommended{Limits: adapter.ResourceValues{CPU: q("500m")}}
	rec := adapter.Recommended{Requests: adapter.ResourceValues{CPU: q("250m")}}
	e := findEdit(decideEdits(cur, rec, cfg), "limits", "cpu")
	if e == nil || !e.Delete {
		t.Fatalf("expected cpu-limit delete, got %+v", e)
	}
}

func TestDecide_MemLimitEqualsRequest(t *testing.T) {
	cfg := Defaults() // ratio 1.0, headroom 1.0 would tie; use headroom 1.0 for clarity
	cfg.MemHeadroom = 1.0
	rec := adapter.Recommended{Requests: adapter.ResourceValues{Mem: q("256Mi")}}
	edits := decideEdits(adapter.Recommended{}, rec, cfg)
	req := findEdit(edits, "requests", "memory")
	lim := findEdit(edits, "limits", "memory")
	if req == nil || lim == nil {
		t.Fatalf("want request+limit mem edits, got %+v", edits)
	}
	if req.Value != lim.Value {
		t.Errorf("mem limit %q != mem request %q", lim.Value, req.Value)
	}
}

func TestDecide_MemHeadroomCeilMi(t *testing.T) {
	cfg := Defaults() // mem headroom 1.15
	rec := adapter.Recommended{Requests: adapter.ResourceValues{Mem: q("100Mi")}}
	e := findEdit(decideEdits(adapter.Recommended{}, rec, cfg), "requests", "memory")
	if e == nil || e.Value != "115Mi" {
		t.Fatalf("100Mi * 1.15 ceil = 115Mi, got %+v", e)
	}
}

// TestDecide_MemLimitNeverBelowRetainedRequest guards the deadband interaction:
// when the memory request is within the deadband (so it is NOT lowered), the
// memory limit must not be set below the retained (higher) request.
func TestDecide_MemLimitNeverBelowRetainedRequest(t *testing.T) {
	cfg := Defaults()
	cfg.MemHeadroom = 1.0 // isolate the deadband interaction
	// Current request 1000Mi, no limit; recommend 950Mi -> within 10% deadband,
	// so the request edit is suppressed and 1000Mi is retained.
	cur := adapter.Recommended{Requests: adapter.ResourceValues{Mem: q("1000Mi")}}
	rec := adapter.Recommended{Requests: adapter.ResourceValues{Mem: q("950Mi")}}

	edits := decideEdits(cur, rec, cfg)
	if e := findEdit(edits, "requests", "memory"); e != nil {
		t.Fatalf("memory request should be within deadband (no edit), got %+v", e)
	}
	lim := findEdit(edits, "limits", "memory")
	if lim == nil {
		return // skipping the limit edit entirely is also valid
	}
	got := resource.MustParse(lim.Value)
	retained := q("1000Mi")
	if got.Cmp(*retained) < 0 {
		t.Errorf("limits.memory %s < retained requests.memory 1000Mi (invalid manifest)", lim.Value)
	}
}

// TestDecide_MemLimitNeverBelowAppliedRequest guards the other deadband
// interaction: when the request is RAISED (applies), the limit's own deadband
// must not suppress the increase it needs to stay >= the new request.
func TestDecide_MemLimitNeverBelowAppliedRequest(t *testing.T) {
	cfg := Defaults()
	cfg.MemHeadroom = 1.0 // isolate the deadband interaction
	// request 400Mi -> 560Mi is +40% (applies); but limit 512Mi -> 560Mi is
	// +9.4% (within deadband) and would be dropped, leaving 512 < 560.
	cur := adapter.Recommended{
		Requests: adapter.ResourceValues{Mem: q("400Mi")},
		Limits:   adapter.ResourceValues{Mem: q("512Mi")},
	}
	rec := adapter.Recommended{Requests: adapter.ResourceValues{Mem: q("560Mi")}}

	edits := decideEdits(cur, rec, cfg)

	writtenReq := q("400Mi")
	if e := findEdit(edits, "requests", "memory"); e != nil {
		writtenReq = q(e.Value)
	}
	writtenLimit := q("512Mi")
	if e := findEdit(edits, "limits", "memory"); e != nil {
		writtenLimit = q(e.Value)
	}
	if writtenLimit.Cmp(*writtenReq) < 0 {
		t.Errorf("written limits.memory %v < requests.memory %v (invalid manifest)", writtenLimit, writtenReq)
	}
}

// TestDecide_Idempotent is the acceptance test: feeding the applied state back
// produces zero edits.
func TestDecide_Idempotent(t *testing.T) {
	cfg := Defaults()
	cur := adapter.Recommended{Limits: adapter.ResourceValues{CPU: q("500m")}} // only a cpu limit to remove
	rec := adapter.Recommended{
		Requests: adapter.ResourceValues{CPU: q("250m"), Mem: q("200Mi")},
	}
	decisions, edits := Decide(cur, rec, cfg)
	if len(edits) == 0 {
		t.Fatal("first pass produced no edits")
	}

	// Build the applied state from the decisions.
	var applied adapter.Recommended
	for _, d := range decisions {
		if !d.Apply {
			continue
		}
		switch d.Field {
		case "requests.cpu":
			applied.Requests.CPU = d.Desired
		case "requests.memory":
			applied.Requests.Mem = d.Desired
		case "limits.memory":
			applied.Limits.Mem = d.Desired
		case "limits.cpu":
			// removed -> stays nil
		}
	}

	_, edits2 := Decide(applied, rec, cfg)
	if len(edits2) != 0 {
		t.Fatalf("second pass not idempotent: %d edits remain", len(edits2))
	}
}
