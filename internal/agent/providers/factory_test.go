package providers

import (
	"testing"

	"i9c/internal/config"
)

func TestFactorySupportsCodex(t *testing.T) {
	p, err := NewProvider(&config.LLMConfig{Provider: "codex", Model: "gpt-5"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "codex" {
		t.Fatalf("expected codex provider, got %s", p.Name())
	}
}
