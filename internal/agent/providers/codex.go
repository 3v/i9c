package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"i9c/internal/agent"
	"i9c/internal/config"
)

var codexExecCommand = exec.CommandContext

type CodexProvider struct {
	model string
}

func NewCodexProvider(cfg *config.LLMConfig) *CodexProvider {
	model := cfg.Model
	if model == "" {
		model = "gpt-5"
	}
	return &CodexProvider{model: model}
}

func (p *CodexProvider) Name() string { return "codex" }

func (p *CodexProvider) Complete(ctx context.Context, req agent.CompletionRequest) (string, error) {
	prompt := buildPromptFromMessages(req.Messages)
	if strings.TrimSpace(prompt) == "" {
		return "", nil
	}
	args := []string{"exec", "--skip-git-repo-check", "--json"}
	if req.Model != "" {
		args = append(args, "-m", req.Model)
	} else if p.model != "" {
		args = append(args, "-m", p.model)
	}
	args = append(args, "-")

	cmd := codexExecCommand(ctx, "codex", args...)
	cmd.Stdin = strings.NewReader(prompt)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("codex exec failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	content, parseErr := extractFinalAgentText(stdout.Bytes())
	if parseErr != nil {
		return "", fmt.Errorf("failed to parse codex output: %w", parseErr)
	}
	if content == "" {
		return "", fmt.Errorf("codex returned no assistant content")
	}
	return content, nil
}

func (p *CodexProvider) Stream(ctx context.Context, req agent.CompletionRequest) (<-chan agent.StreamChunk, error) {
	out := make(chan agent.StreamChunk, 2)
	go func() {
		defer close(out)
		text, err := p.Complete(ctx, req)
		if err != nil {
			out <- agent.StreamChunk{Error: err, Done: true}
			return
		}
		out <- agent.StreamChunk{Content: text}
		out <- agent.StreamChunk{Done: true}
	}()
	return out, nil
}

func (p *CodexProvider) CompleteWithTools(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	// codex exec integration currently returns assistant text only.
	// Tool-calling is handled by other providers with native function-call APIs.
	text, err := p.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	return &agent.CompletionResponse{Content: text}, nil
}

func buildPromptFromMessages(messages []agent.Message) string {
	var sb strings.Builder
	for _, m := range messages {
		role := strings.ToUpper(strings.TrimSpace(m.Role))
		if role == "" {
			role = "USER"
		}
		sb.WriteString(role)
		sb.WriteString(":\n")
		sb.WriteString(strings.TrimSpace(m.Content))
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String())
}

func extractFinalAgentText(raw []byte) (string, error) {
	var lastAgentText string
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var envelope map[string]any
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}
		t, _ := envelope["type"].(string)
		if t == "error" {
			msg, _ := envelope["message"].(string)
			if msg != "" {
				lastAgentText = ""
			}
			continue
		}
		if t != "item.completed" {
			continue
		}
		itemObj, ok := envelope["item"].(map[string]any)
		if !ok {
			continue
		}
		itemType, _ := itemObj["type"].(string)
		if itemType != "agent_message" {
			continue
		}
		txt, _ := itemObj["text"].(string)
		if strings.TrimSpace(txt) != "" {
			lastAgentText = txt
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return strings.TrimSpace(lastAgentText), nil
}
