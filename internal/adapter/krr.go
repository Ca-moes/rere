package adapter

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
)

// KRR JSON wire shape, pinned against robusta-dev/krr v1.28.0
// (robusta_krr/core/models). Only the `recommended` side is consumed: current
// values are read from the manifest, not from KRR's cluster-side allocations.
//
// In `recommended.requests/limits`, each value is Union[RecommendationValue,
// Recommendation] where RecommendationValue = float | "?" | null and
// Recommendation = {value, severity}. recValue tolerates both forms.
type krrResult struct {
	Scans []krrScan `json:"scans"`
}

type krrScan struct {
	Object      krrObject `json:"object"`
	Recommended krrRec    `json:"recommended"`
}

type krrObject struct {
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Container string `json:"container"`
}

type krrRec struct {
	Requests map[string]json.RawMessage `json:"requests"`
	Limits   map[string]json.RawMessage `json:"limits"`
}

// ParseKRR reads `krr ... -f json` output and returns one Target per
// (workload, container) scan that carries at least one usable recommendation.
// GroupedJob scans and scans with no usable value are skipped (debug-logged).
func ParseKRR(r io.Reader) ([]Target, error) {
	var res krrResult
	if err := json.NewDecoder(r).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode krr json: %w", err)
	}

	targets := make([]Target, 0, len(res.Scans))
	for _, s := range res.Scans {
		if s.Object.Kind == "GroupedJob" {
			slog.Debug("krr: skipping GroupedJob scan", "name", s.Object.Name)
			continue
		}
		rec := Recommended{
			Requests: resourceValues(s.Recommended.Requests),
			Limits:   resourceValues(s.Recommended.Limits),
		}
		if rec.empty() {
			slog.Debug("krr: skipping scan with no usable recommendation",
				"kind", s.Object.Kind, "name", s.Object.Name, "container", s.Object.Container)
			continue
		}
		targets = append(targets, Target{
			Namespace:   s.Object.Namespace,
			Kind:        s.Object.Kind,
			Name:        s.Object.Name,
			Container:   s.Object.Container,
			Recommended: rec,
		})
	}
	return targets, nil
}

// resourceValues extracts cpu (cores) and memory (bytes) from one
// requests/limits map, converting to k8s quantities. Missing/unset -> nil.
func resourceValues(m map[string]json.RawMessage) ResourceValues {
	var rv ResourceValues
	if c := recValue(m["cpu"]); c != nil {
		rv.CPU = cpuFromCores(*c)
	}
	if mem := recValue(m["memory"]); mem != nil {
		rv.Mem = memFromBytes(*mem)
	}
	return rv
}

// recValue extracts a numeric recommended value, returning nil for unset
// (null, "?", "unset", or absent). It accepts both the wrapped
// {value, severity} object and a bare scalar, for version robustness.
func recValue(raw json.RawMessage) *float64 {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	// Unwrap the {value, severity} object form if present.
	var obj struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && len(obj.Value) > 0 {
		raw = obj.Value
	}
	if string(raw) == "null" {
		return nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return nil // "?" / "unset" string sentinel
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return &f
	}
	return nil
}
