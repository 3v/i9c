package agent

import (
	"context"
)

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type CompletionRequest struct {
	Messages    []Message
	Model       string
	MaxTokens   int
	Temperature float64
	Stream      bool
	Tools       []ToolDef
}

type StreamChunk struct {
	Content string
	Done    bool
	Error   error
}

type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]ToolParam
	Required    []string
}

type ToolParam struct {
	Type        string
	Description string
	Enum        []string
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

type CompletionResponse struct {
	Content   string
	ToolCalls []ToolCall
}

type Provider interface {
	Name() string
	Complete(ctx context.Context, req CompletionRequest) (string, error)
	Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error)
	CompleteWithTools(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
}
