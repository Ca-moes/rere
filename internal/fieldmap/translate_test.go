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

func TestTranslateHelmTarget_SingleComponent(t *testing.T) {
	// keycloakx renders one StatefulSet "<release>-keycloakx".
	got, ok := TranslateHelmTarget(krrTarget("StatefulSet", "auth-keycloakx", "keycloak"), MergedChartMaps(ChartConfig{}))
	if !ok {
		t.Fatal("expected keycloakx workload to translate")
	}
	if got.Kind != "HelmRelease" || got.Name != "auth" || got.Container != "" {
		t.Errorf("got %s/%s container=%q, want HelmRelease/auth container=\"\"", got.Kind, got.Name, got.Container)
	}
	if got.Namespace != "default" || got.Recommended.Requests.CPU == nil {
		t.Errorf("namespace/recommendation not preserved: %+v", got)
	}
}

func TestTranslateHelmTarget_MultiComponent(t *testing.T) {
	maps := MergedChartMaps(ChartConfig{})
	// The controller and defaultBackend are separate Deployments of the same
	// ingress-nginx release; each must recover the release and its own component.
	ctrl, ok := TranslateHelmTarget(krrTarget("Deployment", "ingress-nginx-controller", "controller"), maps)
	if !ok || ctrl.Kind != "HelmRelease" || ctrl.Name != "ingress-nginx" || ctrl.Container != "controller" {
		t.Errorf("controller: got %s/%s container=%q ok=%v, want HelmRelease/ingress-nginx container=controller",
			ctrl.Kind, ctrl.Name, ctrl.Container, ok)
	}
	db, ok := TranslateHelmTarget(krrTarget("Deployment", "ingress-nginx-defaultbackend", "default-backend"), maps)
	if !ok || db.Container != "defaultBackend" || db.Name != "ingress-nginx" {
		t.Errorf("defaultBackend: got %s/%s container=%q ok=%v, want HelmRelease/ingress-nginx container=defaultBackend",
			db.Kind, db.Name, db.Container, ok)
	}
}

func TestTranslateHelmTarget_ContainerGateBlocksFalsePositive(t *testing.T) {
	// A plain "cert-manager-controller" Deployment ends with the built-in
	// "-controller" suffix, but its container is not the ingress-nginx controller
	// container, so the container gate must reject it (it stays a tier-1 workload).
	got, ok := TranslateHelmTarget(krrTarget("Deployment", "cert-manager-controller", "cert-manager"), MergedChartMaps(ChartConfig{}))
	if ok {
		t.Errorf("a non-ingress -controller Deployment must not translate, got %+v", got)
	}
}

func TestTranslateHelmTarget_Passthrough(t *testing.T) {
	in := krrTarget("Deployment", "web", "web")
	got, ok := TranslateHelmTarget(in, MergedChartMaps(ChartConfig{}))
	if ok {
		t.Errorf("a plain Deployment must not translate, got %+v", got)
	}
	if got.Kind != "Deployment" || got.Name != "web" {
		t.Errorf("passthrough altered the target: %+v", got)
	}
}

func TestTranslateHelmTarget_UserPatternRecoversRelease(t *testing.T) {
	user := ChartConfig{Maps: []ChartMap{{
		Chart:        "myapp",
		ResourcePath: []string{"spec", "values", "resources"},
		Match:        MatchRule{WorkloadKind: "Deployment", NamePattern: `^(.*)-myapp$`},
	}}}
	got, ok := TranslateHelmTarget(krrTarget("Deployment", "prod-myapp", "app"), MergedChartMaps(user))
	if !ok || got.Kind != "HelmRelease" || got.Name != "prod" {
		t.Errorf("got %s/%s ok=%v, want HelmRelease/prod", got.Kind, got.Name, ok)
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

func TestTranslateTarget_ResolveOnlyMapNeverMatches(t *testing.T) {
	// A resolve-only user map (resourcePath, no match:) is legitimate config, but
	// its zero-value MatchRule must never translate: it would otherwise pass
	// every gate — user maps come first — and rewrite every recommender target
	// to the CR's kind, shadowing tier-1 and every built-in.
	user := MapConfig{Maps: []CRMap{{
		Group: "acme.io", Kind: "MyApp", ResourcePath: []string{"spec", "resources"},
	}}}
	maps := MergedMaps(user)
	if got, ok := TranslateTarget(krrTarget("Deployment", "web", "web"), maps); ok {
		t.Errorf("zero-value match rule must not translate, got %+v", got)
	}
	// Built-ins behind the resolve-only user map must still fire.
	got, ok := TranslateTarget(krrTarget("Deployment", "otel-collector", "otc-container"), maps)
	if !ok || got.Kind != "OpenTelemetryCollector" {
		t.Errorf("built-in OTel rule must still translate, got %+v ok=%v", got, ok)
	}
}

func TestTranslateHelmTarget_ResolveOnlyMapNeverMatches(t *testing.T) {
	// Same guard for tier-3: a chart map (or a component) without match: must
	// never claim a workload.
	user := ChartConfig{Maps: []ChartMap{
		{Chart: "my-chart", ResourcePath: []string{"spec", "values", "resources"}},
		{Chart: "multi", Components: []ChartComponent{
			{Name: "server", Path: []string{"spec", "values", "server", "resources"}},
		}},
	}}
	maps := MergedChartMaps(user)
	if got, ok := TranslateHelmTarget(krrTarget("Deployment", "web", "web"), maps); ok {
		t.Errorf("zero-value match rules must not translate, got %+v", got)
	}
	// Built-ins behind the resolve-only user maps must still fire.
	got, ok := TranslateHelmTarget(krrTarget("StatefulSet", "auth-keycloakx", "keycloak"), maps)
	if !ok || got.Name != "auth" {
		t.Errorf("built-in keycloakx rule must still translate, got %+v ok=%v", got, ok)
	}
}

func TestTranslateTarget_SuffixOnlyNameNeverMatches(t *testing.T) {
	// A workload named exactly the suffix would recover CR name "" — a
	// nonexistent identity; it must be a non-match, not a translation.
	got, ok := TranslateTarget(krrTarget("Deployment", "-collector", "otc-container"), BuiltinMaps())
	if ok {
		t.Errorf("suffix-only workload name must not translate, got %+v", got)
	}
}
