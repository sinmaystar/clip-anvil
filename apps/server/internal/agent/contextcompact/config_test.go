package contextcompact

import "testing"

func TestAgentContextCompactionDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Enabled {
		t.Fatal("Enabled = false, want true")
	}
	if cfg.ModelContextWindowTokens != 256000 {
		t.Fatalf("ModelContextWindowTokens = %d, want 256000", cfg.ModelContextWindowTokens)
	}
	if cfg.MicroTriggerTokens != 180000 || cfg.MicroTargetTokens != 150000 {
		t.Fatalf("micro thresholds = %d -> %d, want 180000 -> 150000", cfg.MicroTriggerTokens, cfg.MicroTargetTokens)
	}
	if cfg.MicroMinReductionTokens != 8000 {
		t.Fatalf("MicroMinReductionTokens = %d, want 8000", cfg.MicroMinReductionTokens)
	}
	if cfg.FullTriggerTokens != 200000 || cfg.FullTargetTokens != 140000 {
		t.Fatalf("full thresholds = %d -> %d, want 200000 -> 140000", cfg.FullTriggerTokens, cfg.FullTargetTokens)
	}
	if cfg.PreserveRecentUserMessages != 6 || cfg.PreserveRecentTotalMessages != 40 {
		t.Fatalf("preserve counts = %d/%d, want 6/40", cfg.PreserveRecentUserMessages, cfg.PreserveRecentTotalMessages)
	}
	if cfg.SearchMaxResults != 50 {
		t.Fatalf("SearchMaxResults = %d, want 50", cfg.SearchMaxResults)
	}
	if cfg.MediaImageInputTokenWeight != 1500 || cfg.MediaCardMaxChars != 1200 {
		t.Fatalf("media defaults = %d/%d, want 1500/1200", cfg.MediaImageInputTokenWeight, cfg.MediaCardMaxChars)
	}
}

func TestAgentContextCompactionWithDefaultsPreservesExplicitDisable(t *testing.T) {
	cfg := Config{Enabled: false, EnabledSet: true}.WithDefaults()
	if cfg.Enabled {
		t.Fatal("Enabled = true, want explicit false preserved")
	}
	if cfg.ModelContextWindowTokens != 256000 {
		t.Fatalf("ModelContextWindowTokens = %d, want default", cfg.ModelContextWindowTokens)
	}
}
