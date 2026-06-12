package derive

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
	"gopkg.in/yaml.v3"
)

type goldenCase struct {
	Memory       model.Memory        `yaml:"memory"`
	Observations []model.Observation `yaml:"observations"`
	Expected     struct {
		Status         model.Status      `yaml:"status"`
		Risk           model.Risk        `yaml:"risk"`
		Confidence     model.Confidence  `yaml:"confidence"`
		Enforcement    model.Enforcement `yaml:"enforcement"`
		EffectiveScope []string          `yaml:"effective_scope"`
	} `yaml:"expected"`
}

func TestDeriveGolden(t *testing.T) {
	files, err := filepath.Glob("testdata/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no golden fixtures found")
	}
	for _, f := range files {
		f := f
		t.Run(filepath.Base(f), func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var gc goldenCase
			if err := yaml.Unmarshal(data, &gc); err != nil {
				t.Fatalf("parse fixture: %v", err)
			}
			got := Derive(gc.Memory, gc.Observations, policy.Default())
			if got.Status != gc.Expected.Status {
				t.Errorf("status = %q, want %q", got.Status, gc.Expected.Status)
			}
			if got.Risk != gc.Expected.Risk {
				t.Errorf("risk = %q, want %q", got.Risk, gc.Expected.Risk)
			}
			if got.Confidence != gc.Expected.Confidence {
				t.Errorf("confidence = %q, want %q", got.Confidence, gc.Expected.Confidence)
			}
			if got.Enforcement != gc.Expected.Enforcement {
				t.Errorf("enforcement = %q, want %q", got.Enforcement, gc.Expected.Enforcement)
			}
			if len(got.EffectiveScope.Paths) != len(gc.Expected.EffectiveScope) {
				t.Fatalf("effective scope = %v, want %v", got.EffectiveScope.Paths, gc.Expected.EffectiveScope)
			}
			for i, p := range gc.Expected.EffectiveScope {
				if got.EffectiveScope.Paths[i] != p {
					t.Errorf("effective scope[%d] = %q, want %q", i, got.EffectiveScope.Paths[i], p)
				}
			}
		})
	}
}
