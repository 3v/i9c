package providers

import (
	"fmt"

	"i9c/internal/agent"
	"i9c/internal/config"
)

func NewProvider(cfg *config.LLMConfig) (agent.Provider, error) {
	switch cfg.Provider {
	case "codex":
		return NewCodexProvider(cfg), nil
	case "openai":
		return NewOpenAIProvider(cfg), nil
	case "claude":
		return NewClaudeProvider(cfg), nil
	case "bedrock":
		return NewBedrockProvider(cfg), nil
	case "ollama":
		return NewOllamaProvider(cfg), nil
	default:
		return nil, fmt.Errorf("unknown LLM provider: %s", cfg.Provider)
	}
}
