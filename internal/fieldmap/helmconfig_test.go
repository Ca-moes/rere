package fieldmap

import (
	"slices"
	"testing"
)

func TestChartConfig_Validate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     ChartConfig
		wantErr bool
	}{
		{
			name: "valid single-component",
			cfg:  ChartConfig{Maps: []ChartMap{{Chart: "keycloakx", ResourcePath: []string{"spec", "values", "resources"}}}},
		},
		{
			name: "valid multi-component",
			cfg: ChartConfig{Maps: []ChartMap{{Chart: "ingress-nginx", Components: []ChartComponent{
				{Name: "controller", Path: []string{"spec", "values", "controller", "resources"}},
			}}}},
		},
		{
			name:    "missing chart",
			cfg:     ChartConfig{Maps: []ChartMap{{ResourcePath: []string{"spec", "values", "resources"}}}},
			wantErr: true,
		},
		{
			name:    "neither path nor components",
			cfg:     ChartConfig{Maps: []ChartMap{{Chart: "foo"}}},
			wantErr: true,
		},
		{
			name: "both path and components",
			cfg: ChartConfig{Maps: []ChartMap{{Chart: "foo",
				ResourcePath: []string{"spec", "values", "resources"},
				Components:   []ChartComponent{{Name: "a", Path: []string{"spec", "values", "a"}}}}}},
			wantErr: true,
		},
		{
			name:    "component missing path",
			cfg:     ChartConfig{Maps: []ChartMap{{Chart: "foo", Components: []ChartComponent{{Name: "a"}}}}},
			wantErr: true,
		},
		{
			name: "bad top-level name pattern",
			cfg: ChartConfig{Maps: []ChartMap{{Chart: "foo", ResourcePath: []string{"spec", "values", "resources"},
				Match: MatchRule{NamePattern: "("}}}},
			wantErr: true,
		},
		{
			name: "top-level pattern without capture group",
			cfg: ChartConfig{Maps: []ChartMap{{Chart: "foo", ResourcePath: []string{"spec", "values", "resources"},
				Match: MatchRule{NamePattern: "foo-.*"}}}},
			wantErr: true,
		},
		{
			name: "bad component name pattern",
			cfg: ChartConfig{Maps: []ChartMap{{Chart: "foo", Components: []ChartComponent{
				{Name: "a", Path: []string{"spec", "values", "a", "resources"}, Match: MatchRule{NamePattern: "("}},
			}}}},
			wantErr: true,
		},
		{
			name: "valid pattern with capture group",
			cfg: ChartConfig{Maps: []ChartMap{{Chart: "foo", ResourcePath: []string{"spec", "values", "resources"},
				Match: MatchRule{NamePattern: "^(.*)-foo$"}}}},
		},
		{
			name: "duplicate chart",
			cfg: ChartConfig{Maps: []ChartMap{
				{Chart: "foo", ResourcePath: []string{"spec", "values", "resources"}},
				{Chart: "foo", ResourcePath: []string{"spec", "values", "other"}},
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

func TestBuiltinChartMaps(t *testing.T) {
	b := BuiltinChartMaps()
	if err := b.Validate(); err != nil {
		t.Fatalf("built-in chart maps must be valid: %v", err)
	}
	kc := findChartMap(b, "keycloakx")
	if kc == nil {
		t.Fatal("keycloakx built-in missing")
	}
	if !slices.Equal(kc.ResourcePath, []string{"spec", "values", "resources"}) {
		t.Errorf("keycloakx resourcePath = %v", kc.ResourcePath)
	}
	ing := findChartMap(b, "ingress-nginx")
	if ing == nil {
		t.Fatal("ingress-nginx built-in missing")
	}
	if len(ing.Components) < 2 {
		t.Errorf("ingress-nginx should be multi-component, got %d components", len(ing.Components))
	}
	for _, m := range b.Maps {
		if m.Chart == "" {
			t.Error("built-ins must not carry a blank chart key")
		}
	}
}

func TestMergedChartMaps(t *testing.T) {
	user := ChartConfig{Maps: []ChartMap{
		{Chart: "my-app", ResourcePath: []string{"spec", "values", "resources"}},              // new
		{Chart: "keycloakx", ResourcePath: []string{"spec", "values", "custom", "resources"}}, // override built-in
	}}
	merged := MergedChartMaps(user)
	if err := merged.Validate(); err != nil {
		t.Fatalf("merged chart maps invalid: %v", err)
	}
	// user's keycloakx override wins over the built-in
	kc := findChartMap(merged, "keycloakx")
	if kc == nil || !slices.Equal(kc.ResourcePath, []string{"spec", "values", "custom", "resources"}) {
		t.Errorf("user override not applied: %+v", kc)
	}
	// the new user map is present
	if findChartMap(merged, "my-app") == nil {
		t.Error("user my-app map missing from merge")
	}
	// a non-overridden built-in survives
	if findChartMap(merged, "ingress-nginx") == nil {
		t.Error("ingress-nginx built-in lost in merge")
	}
}

func TestMergedChartMaps_CompilesNamePatterns(t *testing.T) {
	user := ChartConfig{Maps: []ChartMap{{
		Chart:        "patterned",
		ResourcePath: []string{"spec", "values", "resources"},
		Match:        MatchRule{NamePattern: `^(.*)-patterned$`},
	}, {
		Chart: "multi",
		Components: []ChartComponent{
			{Name: "web", Path: []string{"spec", "values", "web", "resources"},
				Match: MatchRule{NamePattern: `^(.*)-web$`}},
		},
	}}}
	merged := MergedChartMaps(user)
	top := findChartMap(merged, "patterned")
	if top == nil || top.nameRE == nil {
		t.Fatal("MergedChartMaps must precompile the top-level NamePattern")
	}
	multi := findChartMap(merged, "multi")
	if multi == nil || multi.Components[0].nameRE == nil {
		t.Fatal("MergedChartMaps must precompile each component's NamePattern")
	}
}
