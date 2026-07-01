package fieldmap

import (
	"slices"
	"testing"
)

func TestMapConfig_Validate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     MapConfig
		wantErr bool
	}{
		{
			name: "valid single-component",
			cfg:  MapConfig{Maps: []CRMap{{Group: "example.com", Kind: "Foo", ResourcePath: []string{"spec", "resources"}}}},
		},
		{
			name: "valid multi-component",
			cfg: MapConfig{Maps: []CRMap{{Group: "example.com", Kind: "Foo", Components: []Component{
				{Name: "a", Path: []string{"spec", "a", "resources"}},
			}}}},
		},
		{
			name:    "missing group",
			cfg:     MapConfig{Maps: []CRMap{{Kind: "Foo", ResourcePath: []string{"spec", "resources"}}}},
			wantErr: true,
		},
		{
			name:    "missing kind",
			cfg:     MapConfig{Maps: []CRMap{{Group: "example.com", ResourcePath: []string{"spec", "resources"}}}},
			wantErr: true,
		},
		{
			name:    "neither path nor components",
			cfg:     MapConfig{Maps: []CRMap{{Group: "example.com", Kind: "Foo"}}},
			wantErr: true,
		},
		{
			name: "both path and components",
			cfg: MapConfig{Maps: []CRMap{{Group: "example.com", Kind: "Foo",
				ResourcePath: []string{"spec", "resources"},
				Components:   []Component{{Name: "a", Path: []string{"spec", "a"}}}}}},
			wantErr: true,
		},
		{
			name: "bad name pattern",
			cfg: MapConfig{Maps: []CRMap{{Group: "example.com", Kind: "Foo", ResourcePath: []string{"spec", "resources"},
				Match: MatchRule{NamePattern: "("}}}},
			wantErr: true,
		},
		{
			name: "name pattern without capture group",
			cfg: MapConfig{Maps: []CRMap{{Group: "example.com", Kind: "Foo", ResourcePath: []string{"spec", "resources"},
				Match: MatchRule{NamePattern: "foo-.*"}}}},
			wantErr: true,
		},
		{
			name: "name pattern with capture group",
			cfg: MapConfig{Maps: []CRMap{{Group: "example.com", Kind: "Foo", ResourcePath: []string{"spec", "resources"},
				Match: MatchRule{NamePattern: "^(.*)-[0-9]+$"}}}},
		},
		{
			name: "duplicate group+kind",
			cfg: MapConfig{Maps: []CRMap{
				{Group: "example.com", Kind: "Foo", ResourcePath: []string{"spec", "resources"}},
				{Group: "example.com", Kind: "Foo", ResourcePath: []string{"spec", "other"}},
			}},
			wantErr: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.cfg.Validate()
			if (err != nil) != c.wantErr {
				t.Errorf("Validate() err = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}

func TestBuiltinMaps(t *testing.T) {
	b := BuiltinMaps()
	if err := b.Validate(); err != nil {
		t.Fatalf("built-in maps must be valid: %v", err)
	}
	cnpg := findCRMap(b, "postgresql.cnpg.io", "Cluster")
	if cnpg == nil {
		t.Fatal("CNPG Cluster built-in missing")
	}
	if !slices.Equal(cnpg.ResourcePath, []string{"spec", "resources"}) {
		t.Errorf("CNPG resourcePath = %v", cnpg.ResourcePath)
	}
	if findCRMap(b, "opentelemetry.io", "OpenTelemetryCollector") == nil {
		t.Fatal("OpenTelemetryCollector built-in missing")
	}
}

func TestMergedMaps(t *testing.T) {
	user := MapConfig{Maps: []CRMap{
		{Group: "example.com", Kind: "MyApp", ResourcePath: []string{"spec", "resources"}},                    // new
		{Group: "postgresql.cnpg.io", Kind: "Cluster", ResourcePath: []string{"spec", "custom", "resources"}}, // override
	}}
	merged := MergedMaps(user)
	if err := merged.Validate(); err != nil {
		t.Fatalf("merged maps invalid: %v", err)
	}
	// user's CNPG override wins over the built-in
	cnpg := findCRMap(merged, "postgresql.cnpg.io", "Cluster")
	if cnpg == nil || !slices.Equal(cnpg.ResourcePath, []string{"spec", "custom", "resources"}) {
		t.Errorf("user override not applied: %+v", cnpg)
	}
	// the new user map is present
	if findCRMap(merged, "example.com", "MyApp") == nil {
		t.Error("user MyApp map missing from merge")
	}
	// a non-overridden built-in survives
	if findCRMap(merged, "opentelemetry.io", "OpenTelemetryCollector") == nil {
		t.Error("OTel built-in lost in merge")
	}
}

func TestMergedMaps_CompilesNamePattern(t *testing.T) {
	merged := MergedMaps(MapConfig{})
	cnpg := findCRMap(merged, "postgresql.cnpg.io", "Cluster") // has a NamePattern
	if cnpg == nil || cnpg.nameRE == nil {
		t.Fatal("MergedMaps must precompile and cache the NamePattern regexp")
	}
}

func TestMapConfigValidate_ContainerToComponentCrossChecks(t *testing.T) {
	// A component value on a resourcePath-only map is silently ignored at
	// resolve time today; a typo'd component name only fails per-target. Both
	// are config errors and must die at Validate.
	cases := []struct {
		name string
		m    CRMap
	}{
		{"component value on resourcePath map", CRMap{
			Group: "acme.io", Kind: "App", ResourcePath: []string{"spec", "resources"},
			Match: MatchRule{WorkloadKind: "Deployment", ContainerToComponent: map[string]string{"web": "server"}},
		}},
		{"unknown component name", CRMap{
			Group: "acme.io", Kind: "App",
			Components: []Component{{Name: "server", Path: []string{"spec", "server", "resources"}}},
			Match:      MatchRule{WorkloadKind: "Deployment", ContainerToComponent: map[string]string{"web": "sever"}},
		}},
		{"empty component value on components map", CRMap{
			Group: "acme.io", Kind: "App",
			Components: []Component{{Name: "server", Path: []string{"spec", "server", "resources"}}},
			Match:      MatchRule{WorkloadKind: "Deployment", ContainerToComponent: map[string]string{"web": ""}},
		}},
	}
	for _, tc := range cases {
		if err := (MapConfig{Maps: []CRMap{tc.m}}).Validate(); err == nil {
			t.Errorf("%s: expected Validate error", tc.name)
		}
	}
	// The valid shapes stay valid.
	good := MapConfig{Maps: []CRMap{
		{Group: "a.io", Kind: "A", ResourcePath: []string{"spec", "resources"},
			Match: MatchRule{WorkloadKind: "Pod", ContainerToComponent: map[string]string{"main": ""}}},
		{Group: "b.io", Kind: "B",
			Components: []Component{{Name: "server", Path: []string{"spec", "server", "resources"}}},
			Match:      MatchRule{WorkloadKind: "Pod", ContainerToComponent: map[string]string{"main": "server"}}},
	}}
	if err := good.Validate(); err != nil {
		t.Errorf("valid maps rejected: %v", err)
	}
}
