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
