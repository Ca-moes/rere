package fieldmap

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/Ca-moes/rere/internal/adapter"
)

func krrTarget(kind, name, container string) adapter.Target {
	q := resource.MustParse("250m")
	return adapter.Target{
		Namespace: "default", Kind: kind, Name: name, Container: container,
		Recommended: adapter.Recommended{Requests: adapter.ResourceValues{CPU: &q}},
	}
}

func TestTranslateTarget_OTel(t *testing.T) {
	got, ok := TranslateTarget(krrTarget("Deployment", "otel-collector", "otc-container"), BuiltinMaps())
	if !ok {
		t.Fatal("expected OTel collector to translate")
	}
	if got.Kind != "OpenTelemetryCollector" || got.Name != "otel" || got.Container != "" {
		t.Errorf("got %s/%s container=%q, want OpenTelemetryCollector/otel container=\"\"", got.Kind, got.Name, got.Container)
	}
	if got.Namespace != "default" || got.Recommended.Requests.CPU == nil {
		t.Errorf("namespace/recommendation not preserved: %+v", got)
	}
}

func TestTranslateTarget_CNPG(t *testing.T) {
	got, ok := TranslateTarget(krrTarget("Pod", "mycluster-1", "postgres"), BuiltinMaps())
	if !ok {
		t.Fatal("expected CNPG pod to translate")
	}
	if got.Kind != "Cluster" || got.Name != "mycluster" || got.Container != "" {
		t.Errorf("got %s/%s container=%q, want Cluster/mycluster container=\"\"", got.Kind, got.Name, got.Container)
	}
}

func TestTranslateTarget_PassthroughTier1(t *testing.T) {
	in := krrTarget("Deployment", "web", "web")
	got, ok := TranslateTarget(in, BuiltinMaps())
	if ok {
		t.Errorf("a plain Deployment must not translate, got %+v", got)
	}
	if got.Kind != "Deployment" || got.Name != "web" || got.Container != "web" {
		t.Errorf("passthrough altered the target: %+v", got)
	}
}

func TestTranslateTarget_UserMapTakesPrecedence(t *testing.T) {
	// A user operator whose generated workload collides with the OTel built-in
	// (Deployment + "-collector" suffix + otc-container) must resolve to the
	// user's CR, not OpenTelemetryCollector.
	user := MapConfig{Maps: []CRMap{{
		Group: "acme.io", Kind: "Widget", ResourcePath: []string{"spec", "resources"},
		Match: MatchRule{
			WorkloadKind:         "Deployment",
			NameSuffix:           "-collector",
			ContainerToComponent: map[string]string{"otc-container": ""},
		},
	}}}
	merged := MergedMaps(user)
	got, ok := TranslateTarget(krrTarget("Deployment", "my-collector", "otc-container"), merged)
	if !ok {
		t.Fatal("expected the colliding workload to translate")
	}
	if got.Kind != "Widget" || got.Name != "my" {
		t.Errorf("user map must win over the built-in OTel rule, got %s/%s", got.Kind, got.Name)
	}
}

func TestTranslateTarget_UnmappedContainerNotTranslated(t *testing.T) {
	// CNPG built-in names only the "postgres" container. A monitoring sidecar in
	// the same pod must not be rewritten onto the CR's shared spec.resources.
	maps := BuiltinMaps()
	if got, ok := TranslateTarget(krrTarget("Pod", "mycluster-1", "sidecar"), maps); ok {
		t.Errorf("unmapped sidecar must not translate, got %+v", got)
	}
	got, ok := TranslateTarget(krrTarget("Pod", "mycluster-1", "postgres"), maps)
	if !ok || got.Kind != "Cluster" || got.Container != "" {
		t.Errorf("mapped postgres container should translate to Cluster/component \"\", got %+v ok=%v", got, ok)
	}
}

func TestTranslateTarget_MultiComponent(t *testing.T) {
	maps := MapConfig{Maps: []CRMap{{
		Group: "example.com", Kind: "MyApp",
		Components: []Component{
			{Name: "server", Path: []string{"spec", "server", "resources"}},
			{Name: "worker", Path: []string{"spec", "worker", "resources"}},
		},
		Match: MatchRule{
			WorkloadKind: "Deployment",
			NamePattern:  `^(.*)-(?:server|worker)$`,
			ContainerToComponent: map[string]string{
				"myapp-server": "server",
				"myapp-worker": "worker",
			},
		},
	}}}
	got, ok := TranslateTarget(krrTarget("Deployment", "myapp-server", "myapp-server"), maps)
	if !ok {
		t.Fatal("expected multi-component translate")
	}
	if got.Kind != "MyApp" || got.Name != "myapp" || got.Container != "server" {
		t.Errorf("got %s/%s container=%q, want MyApp/myapp container=server", got.Kind, got.Name, got.Container)
	}
}
