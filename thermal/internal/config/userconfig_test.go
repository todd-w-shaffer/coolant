package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsMatchTuningConstants(t *testing.T) {
	t.Helper()
	d := Defaults()

	// Memory
	if d.Memory.WarmPct != MemWarmPct {
		t.Errorf("Memory.WarmPct = %d, want %d", d.Memory.WarmPct, MemWarmPct)
	}
	if d.Memory.HotPct != MemHotPct {
		t.Errorf("Memory.HotPct = %d, want %d", d.Memory.HotPct, MemHotPct)
	}
	if d.Memory.CritPct != MemCritPct {
		t.Errorf("Memory.CritPct = %d, want %d", d.Memory.CritPct, MemCritPct)
	}

	// CPU
	if d.CPU.WarmPct != CPUWarmPct {
		t.Errorf("CPU.WarmPct = %d, want %d", d.CPU.WarmPct, CPUWarmPct)
	}
	if d.CPU.CritPct != CPUCritPct {
		t.Errorf("CPU.CritPct = %d, want %d", d.CPU.CritPct, CPUCritPct)
	}

	// Swap (GB → bytes)
	if d.Swap.WarmBytes != SwapWarmBytes {
		t.Errorf("Swap.WarmBytes = %d, want %d", d.Swap.WarmBytes, SwapWarmBytes)
	}
	if d.Swap.HotBytes != SwapHotBytes {
		t.Errorf("Swap.HotBytes = %d, want %d", d.Swap.HotBytes, SwapHotBytes)
	}
	if d.Swap.CritBytes != SwapCritBytes {
		t.Errorf("Swap.CritBytes = %d, want %d", d.Swap.CritBytes, SwapCritBytes)
	}

	// Headroom
	if d.Headroom.WarnBytes != HeadroomWarnBytes {
		t.Errorf("Headroom.WarnBytes = %d, want %d", d.Headroom.WarnBytes, HeadroomWarnBytes)
	}
	if d.Headroom.CritBytes != HeadroomCritBytes {
		t.Errorf("Headroom.CritBytes = %d, want %d", d.Headroom.CritBytes, HeadroomCritBytes)
	}

	// Spawn
	if d.Spawn.BurstThreshold != SpawnBurstThreshold {
		t.Errorf("Spawn.BurstThreshold = %d, want %d", d.Spawn.BurstThreshold, SpawnBurstThreshold)
	}
	if d.Spawn.RateEscalation != SpawnRateEscalation {
		t.Errorf("Spawn.RateEscalation = %f, want %f", d.Spawn.RateEscalation, SpawnRateEscalation)
	}

	// Score
	if d.Score.Warm != ScoreWarm {
		t.Errorf("Score.Warm = %d, want %d", d.Score.Warm, ScoreWarm)
	}
	if d.Score.Hot != ScoreHot {
		t.Errorf("Score.Hot = %d, want %d", d.Score.Hot, ScoreHot)
	}
	if d.Score.Meltdown != ScoreMeltdown {
		t.Errorf("Score.Meltdown = %d, want %d", d.Score.Meltdown, ScoreMeltdown)
	}

	// Sparklines
	if d.Sparklines.CPUWarn != CPUSparkWarn {
		t.Errorf("Sparklines.CPUWarn = %f, want %f", d.Sparklines.CPUWarn, CPUSparkWarn)
	}
	if d.Sparklines.CPUCrit != CPUSparkCrit {
		t.Errorf("Sparklines.CPUCrit = %f, want %f", d.Sparklines.CPUCrit, CPUSparkCrit)
	}
	if d.Sparklines.MemWarn != MemSparkWarn {
		t.Errorf("Sparklines.MemWarn = %f, want %f", d.Sparklines.MemWarn, MemSparkWarn)
	}
	if d.Sparklines.MemCrit != MemSparkCrit {
		t.Errorf("Sparklines.MemCrit = %f, want %f", d.Sparklines.MemCrit, MemSparkCrit)
	}

	// Categories
	thresh := d.CatThreshold("build")
	if thresh != CatThresholds["build"] {
		t.Errorf("CatThreshold(build) = %v, want %v", thresh, CatThresholds["build"])
	}
	thresh = d.CatThreshold("unknown_cat")
	if thresh != CatThresholdDefault {
		t.Errorf("CatThreshold(unknown) = %v, want %v", thresh, CatThresholdDefault)
	}
	if d.Categories.ShellExplosion != ShellExplosionThreshold {
		t.Errorf("Categories.ShellExplosion = %d, want %d", d.Categories.ShellExplosion, ShellExplosionThreshold)
	}
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	old := C
	defer func() { C = old }()

	err := Load("/nonexistent/path/config.toml")
	if err != nil {
		t.Fatalf("Load missing file returned error: %v", err)
	}
	// C should still equal defaults
	d := Defaults()
	if C.Memory.WarmPct != d.Memory.WarmPct {
		t.Errorf("after missing file, Memory.WarmPct = %d, want default %d", C.Memory.WarmPct, d.Memory.WarmPct)
	}
}

func TestLoadPartialTOML(t *testing.T) {
	old := C
	defer func() { C = old }()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
[memory]
warm_pct = 50

[cpu]
crit_pct = 95
`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Load(path); err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	// Overridden values
	if C.Memory.WarmPct != 50 {
		t.Errorf("Memory.WarmPct = %d, want 50", C.Memory.WarmPct)
	}
	if C.CPU.CritPct != 95 {
		t.Errorf("CPU.CritPct = %d, want 95", C.CPU.CritPct)
	}

	// Non-overridden values keep defaults
	if C.Memory.HotPct != MemHotPct {
		t.Errorf("Memory.HotPct = %d, want default %d", C.Memory.HotPct, MemHotPct)
	}
	if C.CPU.WarmPct != CPUWarmPct {
		t.Errorf("CPU.WarmPct = %d, want default %d", C.CPU.WarmPct, CPUWarmPct)
	}
	if C.Swap.WarmBytes != SwapWarmBytes {
		t.Errorf("Swap.WarmBytes = %d, want default %d", C.Swap.WarmBytes, SwapWarmBytes)
	}
}

func TestLoadSwapGB(t *testing.T) {
	old := C
	defer func() { C = old }()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
[swap]
warm_gb = 4
hot_gb  = 16
crit_gb = 32
`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Load(path); err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	const GB = 1 << 30
	if C.Swap.WarmBytes != 4*GB {
		t.Errorf("Swap.WarmBytes = %d, want %d", C.Swap.WarmBytes, 4*int64(GB))
	}
	if C.Swap.HotBytes != 16*GB {
		t.Errorf("Swap.HotBytes = %d, want %d", C.Swap.HotBytes, 16*int64(GB))
	}
	if C.Swap.CritBytes != 32*GB {
		t.Errorf("Swap.CritBytes = %d, want %d", C.Swap.CritBytes, 32*int64(GB))
	}
}

func TestLoadHeadroomGB(t *testing.T) {
	old := C
	defer func() { C = old }()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
[headroom]
warn_gb = 8
crit_gb = 4
`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Load(path); err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	const GB = 1 << 30
	if C.Headroom.WarnBytes != 8*GB {
		t.Errorf("Headroom.WarnBytes = %d, want %d", C.Headroom.WarnBytes, 8*int64(GB))
	}
	if C.Headroom.CritBytes != 4*GB {
		t.Errorf("Headroom.CritBytes = %d, want %d", C.Headroom.CritBytes, 4*int64(GB))
	}
}

func TestLoadCategoryThresholds(t *testing.T) {
	old := C
	defer func() { C = old }()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
[categories]
node    = [2, 10]
default = [5, 15]
shell_explosion = 50
`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Load(path); err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	thresh := C.CatThreshold("node")
	if thresh != [2]int{2, 10} {
		t.Errorf("CatThreshold(node) = %v, want [2, 10]", thresh)
	}
	// build should keep compiled default
	thresh = C.CatThreshold("build")
	if thresh != CatThresholds["build"] {
		t.Errorf("CatThreshold(build) = %v, want compiled default %v", thresh, CatThresholds["build"])
	}
	// unknown should use user-overridden default
	thresh = C.CatThreshold("unknown_cat")
	if thresh != [2]int{5, 15} {
		t.Errorf("CatThreshold(unknown) = %v, want [5, 15]", thresh)
	}
	if C.Categories.ShellExplosion != 50 {
		t.Errorf("Categories.ShellExplosion = %d, want 50", C.Categories.ShellExplosion)
	}
}

func TestValidationRejectsWarmAboveHot(t *testing.T) {
	old := C
	defer func() { C = old }()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
[memory]
warm_pct = 90
hot_pct  = 50
crit_pct = 95
`), 0644); err != nil {
		t.Fatal(err)
	}

	err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for warm > hot, got nil")
	}
}

func TestValidationRejectsNegative(t *testing.T) {
	old := C
	defer func() { C = old }()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
[swap]
warm_gb = -1
`), 0644); err != nil {
		t.Fatal(err)
	}

	err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for negative swap, got nil")
	}
}

func TestValidationRejectsBadSwapOrder(t *testing.T) {
	old := C
	defer func() { C = old }()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
[swap]
warm_gb = 10
hot_gb  = 5
crit_gb = 20
`), 0644); err != nil {
		t.Fatal(err)
	}

	err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for warm > hot swap, got nil")
	}
}

func TestLoadBadTOML(t *testing.T) {
	old := C
	defer func() { C = old }()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`this is not valid toml {{{{`), 0644); err != nil {
		t.Fatal(err)
	}

	err := Load(path)
	if err == nil {
		t.Fatal("expected parse error for bad TOML, got nil")
	}
}

// ── Brainstem-proof coercion tests ────────────────────────────

func TestLoadQuotedInts(t *testing.T) {
	old := C
	defer func() { C = old }()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
[memory]
warm_pct = "50"
hot_pct  = "80"
crit_pct = "95"

[cpu]
warm_pct = "70"
crit_pct = "92"

[spawn]
burst_threshold = "12"

[score]
warm     = "1"
hot      = "3"
meltdown = "5"
`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Load(path); err != nil {
		t.Fatalf("Load with quoted ints returned error: %v", err)
	}

	if C.Memory.WarmPct != 50 {
		t.Errorf("Memory.WarmPct = %d, want 50", C.Memory.WarmPct)
	}
	if C.Memory.HotPct != 80 {
		t.Errorf("Memory.HotPct = %d, want 80", C.Memory.HotPct)
	}
	if C.CPU.WarmPct != 70 {
		t.Errorf("CPU.WarmPct = %d, want 70", C.CPU.WarmPct)
	}
	if C.CPU.CritPct != 92 {
		t.Errorf("CPU.CritPct = %d, want 92", C.CPU.CritPct)
	}
	if C.Spawn.BurstThreshold != 12 {
		t.Errorf("Spawn.BurstThreshold = %d, want 12", C.Spawn.BurstThreshold)
	}
	if C.Score.Warm != 1 {
		t.Errorf("Score.Warm = %d, want 1", C.Score.Warm)
	}
}

func TestLoadQuotedFloats(t *testing.T) {
	old := C
	defer func() { C = old }()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
[swap]
warm_gb = "4"
hot_gb  = "16"
crit_gb = "32"

[headroom]
warn_gb = "8"
crit_gb = "4"

[sparklines]
cpu_warn = "75.0"
cpu_crit = "95"

[spawn]
rate_escalation = "12.5"
`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Load(path); err != nil {
		t.Fatalf("Load with quoted floats returned error: %v", err)
	}

	const GB = 1 << 30
	if C.Swap.WarmBytes != 4*GB {
		t.Errorf("Swap.WarmBytes = %d, want %d", C.Swap.WarmBytes, 4*int64(GB))
	}
	if C.Headroom.WarnBytes != 8*GB {
		t.Errorf("Headroom.WarnBytes = %d, want %d", C.Headroom.WarnBytes, 8*int64(GB))
	}
	if C.Sparklines.CPUWarn != 75.0 {
		t.Errorf("Sparklines.CPUWarn = %f, want 75.0", C.Sparklines.CPUWarn)
	}
	if C.Sparklines.CPUCrit != 95.0 {
		t.Errorf("Sparklines.CPUCrit = %f, want 95.0", C.Sparklines.CPUCrit)
	}
	if C.Spawn.RateEscalation != 12.5 {
		t.Errorf("Spawn.RateEscalation = %f, want 12.5", C.Spawn.RateEscalation)
	}
}

func TestLoadFloatForInt(t *testing.T) {
	old := C
	defer func() { C = old }()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
[memory]
warm_pct = 50.0
hot_pct  = 80.0
crit_pct = 95.0
`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Load(path); err != nil {
		t.Fatalf("Load with float-for-int returned error: %v", err)
	}

	if C.Memory.WarmPct != 50 {
		t.Errorf("Memory.WarmPct = %d, want 50", C.Memory.WarmPct)
	}
}

func TestLoadIntForFloat(t *testing.T) {
	old := C
	defer func() { C = old }()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
[sparklines]
cpu_warn = 70
cpu_crit = 90
`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Load(path); err != nil {
		t.Fatalf("Load with int-for-float returned error: %v", err)
	}

	if C.Sparklines.CPUWarn != 70.0 {
		t.Errorf("Sparklines.CPUWarn = %f, want 70.0", C.Sparklines.CPUWarn)
	}
}

func TestLoadQuotedCategories(t *testing.T) {
	old := C
	defer func() { C = old }()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
[categories]
shell_explosion = "50"
`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Load(path); err != nil {
		t.Fatalf("Load with quoted category int returned error: %v", err)
	}

	if C.Categories.ShellExplosion != 50 {
		t.Errorf("Categories.ShellExplosion = %d, want 50", C.Categories.ShellExplosion)
	}
}

func TestLoadTrailingPercentQuoted(t *testing.T) {
	old := C
	defer func() { C = old }()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
[memory]
warm_pct = "50%"
hot_pct  = "80%"
crit_pct = "95%"

[cpu]
warm_pct = "70%"
crit_pct = "92%"

[sparklines]
cpu_warn = "75%"
gpu_crit = "85%"
`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Load(path); err != nil {
		t.Fatalf("Load with quoted trailing %% returned error: %v", err)
	}

	if C.Memory.WarmPct != 50 {
		t.Errorf("Memory.WarmPct = %d, want 50", C.Memory.WarmPct)
	}
	if C.Memory.HotPct != 80 {
		t.Errorf("Memory.HotPct = %d, want 80", C.Memory.HotPct)
	}
	if C.CPU.WarmPct != 70 {
		t.Errorf("CPU.WarmPct = %d, want 70", C.CPU.WarmPct)
	}
	if C.Sparklines.CPUWarn != 75.0 {
		t.Errorf("Sparklines.CPUWarn = %f, want 75.0", C.Sparklines.CPUWarn)
	}
	if C.Sparklines.GPUCrit != 85.0 {
		t.Errorf("Sparklines.GPUCrit = %f, want 85.0", C.Sparklines.GPUCrit)
	}
}

func TestLoadBarePercent(t *testing.T) {
	old := C
	defer func() { C = old }()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	// Bare 65% is invalid TOML — our pre-processor should fix it
	if err := os.WriteFile(path, []byte(`
[memory]
warm_pct = 50%
hot_pct  = 80%
crit_pct = 95%

[cpu]
warm_pct = 70%
crit_pct = 92%
`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Load(path); err != nil {
		t.Fatalf("Load with bare %% returned error: %v", err)
	}

	if C.Memory.WarmPct != 50 {
		t.Errorf("Memory.WarmPct = %d, want 50", C.Memory.WarmPct)
	}
	if C.CPU.CritPct != 92 {
		t.Errorf("CPU.CritPct = %d, want 92", C.CPU.CritPct)
	}
}

func TestLoadBareGB(t *testing.T) {
	old := C
	defer func() { C = old }()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
[swap]
warm_gb = 4GB
hot_gb  = 16gb
crit_gb = 32GB
`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Load(path); err != nil {
		t.Fatalf("Load with bare GB returned error: %v", err)
	}

	const GB = 1 << 30
	if C.Swap.WarmBytes != 4*GB {
		t.Errorf("Swap.WarmBytes = %d, want %d", C.Swap.WarmBytes, 4*int64(GB))
	}
	if C.Swap.HotBytes != 16*GB {
		t.Errorf("Swap.HotBytes = %d, want %d", C.Swap.HotBytes, 16*int64(GB))
	}
}

func TestLoadTrailingGB(t *testing.T) {
	old := C
	defer func() { C = old }()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
[swap]
warm_gb = "4GB"
hot_gb  = "16gb"
crit_gb = "32 GB"
`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Load(path); err != nil {
		t.Fatalf("Load with trailing GB returned error: %v", err)
	}

	const GB = 1 << 30
	if C.Swap.WarmBytes != 4*GB {
		t.Errorf("Swap.WarmBytes = %d, want %d", C.Swap.WarmBytes, 4*int64(GB))
	}
	if C.Swap.HotBytes != 16*GB {
		t.Errorf("Swap.HotBytes = %d, want %d", C.Swap.HotBytes, 16*int64(GB))
	}
	if C.Swap.CritBytes != 32*GB {
		t.Errorf("Swap.CritBytes = %d, want %d", C.Swap.CritBytes, 32*int64(GB))
	}
}

func TestValidationRejectsBadScoreOrder(t *testing.T) {
	old := C
	defer func() { C = old }()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
[score]
warm     = 5
hot      = 3
meltdown = 1
`), 0644); err != nil {
		t.Fatal(err)
	}

	err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for bad score ordering, got nil")
	}
}
