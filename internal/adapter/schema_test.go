package adapter

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestCPUFromCores(t *testing.T) {
	cases := []struct {
		cores float64
		want  string
	}{
		{0.25, "250m"},
		{2.0, "2"},
		{0.001, "1m"},
		{1.5, "1500m"},
		{0.1, "100m"},
	}
	for _, c := range cases {
		if got := cpuFromCores(c.cores).String(); got != c.want {
			t.Errorf("cpuFromCores(%v) = %q, want %q", c.cores, got, c.want)
		}
	}
}

func TestMemFromBytes(t *testing.T) {
	cases := []struct {
		bytes float64
		want  string
	}{
		{134217728, "128Mi"},
		{1073741824, "1Gi"},
		{268435456, "256Mi"},
		{0, "0"},
	}
	for _, c := range cases {
		if got := memFromBytes(c.bytes).String(); got != c.want {
			t.Errorf("memFromBytes(%v) = %q, want %q", c.bytes, got, c.want)
		}
	}
}

func q(s string) *resource.Quantity {
	v := resource.MustParse(s)
	return &v
}

func TestTargetValidate(t *testing.T) {
	base := Target{
		Namespace: "default", Kind: "Deployment", Name: "web", Container: "web",
		Recommended: Recommended{Requests: ResourceValues{CPU: q("250m"), Mem: q("128Mi")}},
	}
	cases := []struct {
		name    string
		mutate  func(*Target)
		wantErr bool
	}{
		{"valid full", func(*Target) {}, false},
		{"partial: only cpu request", func(t *Target) { t.Recommended.Requests.Mem = nil }, false},
		{"missing namespace", func(t *Target) { t.Namespace = "" }, true},
		{"missing kind", func(t *Target) { t.Kind = "" }, true},
		{"missing name", func(t *Target) { t.Name = "" }, true},
		{"missing container", func(t *Target) { t.Container = "" }, true},
		{"no recommended values", func(t *Target) { t.Recommended = Recommended{} }, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tgt := base
			c.mutate(&tgt)
			if err := tgt.Validate(); (err != nil) != c.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}

func TestQuantityRoundTrip(t *testing.T) {
	for _, s := range []string{"250m", "2", "128Mi", "1Gi", "1500m"} {
		orig := resource.MustParse(s)
		reparsed := resource.MustParse(orig.String())
		if orig.Cmp(reparsed) != 0 {
			t.Errorf("round trip %q: %v != %v", s, orig.String(), reparsed.String())
		}
	}
}
