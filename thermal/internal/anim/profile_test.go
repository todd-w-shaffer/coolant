package anim

import "testing"

func TestAllProfilesHaveBloomFields(t *testing.T) {
	profiles := []*Profile{Default(), Calm(), Intense()}
	for _, p := range profiles {
		if p.BloomBreatheSecCool <= 0 || p.BloomBreatheSecHot <= 0 {
			t.Errorf("profile %q missing breathe seconds: cool=%v hot=%v", p.Name, p.BloomBreatheSecCool, p.BloomBreatheSecHot)
		}
		if p.BloomScaleAmpCool < 0 || p.BloomScaleAmpHot < 0 {
			t.Errorf("profile %q has negative scale amp", p.Name)
		}
		if p.BloomOpacityMinCool <= 0 || p.BloomOpacityMaxCool <= 0 {
			t.Errorf("profile %q has non-positive cool opacity", p.Name)
		}
		if p.BloomOpacityMinHot <= 0 || p.BloomOpacityMaxHot <= 0 {
			t.Errorf("profile %q has non-positive hot opacity", p.Name)
		}
		if p.BloomSpringFreq <= 0 || p.BloomSpringDamping <= 0 {
			t.Errorf("profile %q has non-positive spring params", p.Name)
		}
	}
}
