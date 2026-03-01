package agent

import (
	"context"
	"fmt"
	"strings"

	"i9c/internal/agent/prompts"
)

type AgentType string

const (
	AgentAWSAPI    AgentType = "aws_api"
	AgentTerraform AgentType = "terraform"
	AgentBlueprint AgentType = "blueprint"
)

type Context struct {
	Profiles           []ProfileContext
	DriftEntries       []DriftContext
	ResourceCount      int
	ResourcesByService []ServiceCount
	HCLFiles           []HCLFileContext
	UserMessage        string
	IACDir             string
	IACBinary          string
}

type ProfileContext struct {
	Name   string
	Region string
}

type DriftContext struct {
	Address string
	Type    string
	Action  string
	Before  string
	After   string
}

type ServiceCount struct {
	Service string
	Count   int
}

type HCLFileContext struct {
	Path      string
	Resources []HCLResourceContext
}

type HCLResourceContext struct {
	Type string
	Name string
}

type Agent struct {
	provider    Provider
	agentType   AgentType
	history     []Message
	rateLimiter *TokenRateLimiter
}

func NewAgent(provider Provider, agentType AgentType) *Agent {
	return &Agent{
		provider:  provider,
		agentType: agentType,
	}
}

func NewAgentWithModel(provider Provider, agentType AgentType, model string) *Agent {
	return &Agent{
		provider:    provider,
		agentType:   agentType,
		rateLimiter: NewTokenRateLimiter(model),
	}
}

func (a *Agent) Chat(ctx context.Context, userMessage string, agentCtx *Context) (string, error) {
	agentCtx.UserMessage = userMessage

	messages := a.buildMessages(agentCtx)

	if a.rateLimiter != nil {
		est := EstimateTokens(messages)
		if err := a.rateLimiter.Wait(ctx, est); err != nil {
			return "", fmt.Errorf("rate limit wait: %w", err)
		}
	}

	req := CompletionRequest{
		Messages:    messages,
		Temperature: 0.3,
		MaxTokens:   4096,
	}

	response, err := a.provider.Complete(ctx, req)
	if err != nil {
		return "", fmt.Errorf("agent completion: %w", err)
	}

	if a.rateLimiter != nil {
		a.rateLimiter.Record(len(response) / charsPerToken)
	}

	a.history = append(a.history, Message{Role: "user", Content: userMessage})
	a.history = append(a.history, Message{Role: "assistant", Content: response})

	return response, nil
}

func (a *Agent) ChatStream(ctx context.Context, userMessage string, agentCtx *Context) (<-chan StreamChunk, error) {
	agentCtx.UserMessage = userMessage

	messages := a.buildMessages(agentCtx)

	req := CompletionRequest{
		Messages:    messages,
		Temperature: 0.3,
		MaxTokens:   4096,
		Stream:      true,
	}

	ch, err := a.provider.Stream(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("agent stream: %w", err)
	}

	collectCh := make(chan StreamChunk)
	go func() {
		defer close(collectCh)
		var fullResponse strings.Builder
		for chunk := range ch {
			collectCh <- chunk
			if chunk.Content != "" {
				fullResponse.WriteString(chunk.Content)
			}
			if chunk.Done {
				a.history = append(a.history,
					Message{Role: "user", Content: userMessage},
					Message{Role: "assistant", Content: fullResponse.String()},
				)
			}
		}
	}()

	return collectCh, nil
}

func (a *Agent) ChatWithTools(ctx context.Context, userMessage string, agentCtx *Context, executor *ToolExecutor, statusFn func(string)) (string, error) {
	agentCtx.UserMessage = userMessage
	messages := a.buildMessages(agentCtx)
	tools := a.getToolDefs()

	const maxIterations = 10
	for i := 0; i < maxIterations; i++ {
		if a.rateLimiter != nil {
			est := EstimateTokens(messages)
			if statusFn != nil {
				statusFn(fmt.Sprintf("Rate limiter estimate: %d tokens (iteration %d)", est, i+1))
			}
			if err := a.rateLimiter.Wait(ctx, est); err != nil {
				return "", fmt.Errorf("rate limit wait: %w", err)
			}
		}

		req := CompletionRequest{
			Messages:    messages,
			Temperature: 0.3,
			MaxTokens:   4096,
			Tools:       tools,
		}

		resp, err := a.provider.CompleteWithTools(ctx, req)
		if err != nil {
			return "", fmt.Errorf("agent tool completion: %w", err)
		}

		if a.rateLimiter != nil {
			respTokens := len(resp.Content) / charsPerToken
			for _, tc := range resp.ToolCalls {
				respTokens += len(tc.Arguments) / charsPerToken
			}
			a.rateLimiter.Record(respTokens)
		}

		if len(resp.ToolCalls) == 0 {
			a.history = append(a.history,
				Message{Role: "user", Content: userMessage},
				Message{Role: "assistant", Content: resp.Content},
			)
			return resp.Content, nil
		}

		assistantMsg := Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		messages = append(messages, assistantMsg)

		for _, tc := range resp.ToolCalls {
			if statusFn != nil {
				statusFn(fmt.Sprintf("Calling %s...", tc.Name))
			}

			result, err := executor.Execute(ctx, tc.Name, tc.Arguments)
			if err != nil {
				result = fmt.Sprintf("Error executing %s: %v", tc.Name, err)
			}

			messages = append(messages, Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
	}

	return "", fmt.Errorf("tool calling exceeded %d iterations", maxIterations)
}

func (a *Agent) getToolDefs() []ToolDef {
	switch a.agentType {
	case AgentAWSAPI:
		return AllToolDefs
	case AgentTerraform:
		return AllToolDefs
	default:
		return AllToolDefs
	}
}

func (a *Agent) ClearHistory() {
	a.history = nil
}

func (a *Agent) buildMessages(agentCtx *Context) []Message {
	var messages []Message

	systemPrompt := a.systemPrompt()
	contextStr := a.buildContext(agentCtx)
	if contextStr != "" {
		systemPrompt += "\n\n" + contextStr
	}

	messages = append(messages, Message{Role: "system", Content: systemPrompt})

	for _, h := range a.history {
		messages = append(messages, h)
	}

	messages = append(messages, Message{Role: "user", Content: agentCtx.UserMessage})

	return messages
}

func (a *Agent) systemPrompt() string {
	switch a.agentType {
	case AgentAWSAPI:
		return prompts.AWSAPISystemPrompt
	case AgentTerraform:
		return prompts.TerraformSystemPrompt
	case AgentBlueprint:
		return prompts.BlueprintSystemPrompt
	default:
		return prompts.AWSAPISystemPrompt
	}
}

func (a *Agent) buildContext(agentCtx *Context) string {
	var sb strings.Builder

	if len(agentCtx.Profiles) > 0 {
		sb.WriteString("## Active AWS Profiles\n")
		for _, p := range agentCtx.Profiles {
			fmt.Fprintf(&sb, "- %s (region: %s)\n", p.Name, p.Region)
		}
		sb.WriteString("\n")
	}

	if len(agentCtx.DriftEntries) > 0 {
		sb.WriteString("## Current Drift\n")
		for _, d := range agentCtx.DriftEntries {
			fmt.Fprintf(&sb, "- %s (%s): %s\n", d.Address, d.Type, d.Action)
			if d.Before != "" {
				fmt.Fprintf(&sb, "  Before: %s\n", d.Before)
			}
			if d.After != "" {
				fmt.Fprintf(&sb, "  After: %s\n", d.After)
			}
		}
		sb.WriteString("\n")
	}

	if agentCtx.ResourceCount > 0 {
		fmt.Fprintf(&sb, "## Resources: %d total\n", agentCtx.ResourceCount)
		for _, sc := range agentCtx.ResourcesByService {
			fmt.Fprintf(&sb, "- %s: %d\n", sc.Service, sc.Count)
		}
		sb.WriteString("\n")
	}

	if len(agentCtx.HCLFiles) > 0 {
		sb.WriteString("## IaC Files\n")
		for _, f := range agentCtx.HCLFiles {
			fmt.Fprintf(&sb, "### %s\n", f.Path)
			for _, r := range f.Resources {
				fmt.Fprintf(&sb, "- resource \"%s\" \"%s\"\n", r.Type, r.Name)
			}
		}
		sb.WriteString("\n")
	}

	if agentCtx.IACDir != "" {
		fmt.Fprintf(&sb, "## IaC Directory: %s\n", agentCtx.IACDir)
	}
	if agentCtx.IACBinary != "" {
		fmt.Fprintf(&sb, "## IaC Tool: %s\n", agentCtx.IACBinary)
	}

	return sb.String()
}
