package providers

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"i9c/internal/agent"
	"i9c/internal/config"
)

func TestExtractFinalAgentText(t *testing.T) {
	raw := strings.Join([]string{
		`{"type":"thread.started","thread_id":"x"}`,
		`{"type":"item.completed","item":{"id":"1","type":"agent_message","text":"hello"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1,"cached_input_tokens":0}}`,
	}, "\n")
	got, err := extractFinalAgentText([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Fatalf("expected hello, got %q", got)
	}
}

func TestCodexProviderComplete(t *testing.T) {
	orig := codexExecCommand
	t.Cleanup(func() { codexExecCommand = orig })
	codexExecCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "printf '%s\n' '{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"ok\"}}'")
	}

	p := NewCodexProvider(&config.LLMConfig{Provider: "codex", Model: "gpt-5"})
	out, err := p.Complete(context.Background(), agent.CompletionRequest{
		Messages: []agent.Message{{Role: "user", Content: "test"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != "ok" {
		t.Fatalf("expected ok, got %q", out)
	}
}
