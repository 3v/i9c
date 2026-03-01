package config

import "testing"

func TestDefaultConfigDriftInterval(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Terraform.DriftCheckIntervalMin != 15 {
		t.Fatalf("expected default drift interval 15, got %d", cfg.Terraform.DriftCheckIntervalMin)
	}
	if cfg.LLM.Provider != "codex" {
		t.Fatalf("expected default llm provider codex, got %s", cfg.LLM.Provider)
	}
}

func TestValidateRejectsInvalidDriftInterval(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Terraform.DriftCheckIntervalMin = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validate to fail for drift interval < 1")
	}
}
