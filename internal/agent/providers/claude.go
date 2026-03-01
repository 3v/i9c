package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"i9c/internal/agent"
	"i9c/internal/config"
)

type ClaudeProvider struct {
	apiKey  string
	baseURL string
	model   string
}

func NewClaudeProvider(cfg *config.LLMConfig) *ClaudeProvider {
	apiKey := cfg.ResolveAPIKey()
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	model := cfg.Model
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	return &ClaudeProvider{apiKey: apiKey, baseURL: baseURL, model: model}
}

func (p *ClaudeProvider) Name() string { return "claude" }

func (p *ClaudeProvider) Complete(ctx context.Context, req agent.CompletionRequest) (string, error) {
	body := p.buildRequest(req, false)
	resp, err := p.doRequest(ctx, body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Claude API error %d: %s", resp.StatusCode, string(data))
	}

	var result claudeResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	var content string
	for _, block := range result.Content {
		if block.Type == "text" {
			content += block.Text
		}
	}
	return content, nil
}

func (p *ClaudeProvider) Stream(ctx context.Context, req agent.CompletionRequest) (<-chan agent.StreamChunk, error) {
	body := p.buildRequest(req, true)
	resp, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("Claude API error %d: %s", resp.StatusCode, string(data))
	}

	ch := make(chan agent.StreamChunk)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")

			var event claudeStreamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			switch event.Type {
			case "content_block_delta":
				if event.Delta != nil && event.Delta.Text != "" {
					ch <- agent.StreamChunk{Content: event.Delta.Text}
				}
			case "message_stop":
				ch <- agent.StreamChunk{Done: true}
				return
			}
		}
		ch <- agent.StreamChunk{Done: true}
	}()

	return ch, nil
}

func (p *ClaudeProvider) CompleteWithTools(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	body := p.buildToolRequest(req)
	resp, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Claude API error %d: %s", resp.StatusCode, string(data))
	}

	var result claudeToolResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	cr := &agent.CompletionResponse{}
	for _, block := range result.Content {
		switch block.Type {
		case "text":
			cr.Content += block.Text
		case "tool_use":
			args, _ := json.Marshal(block.Input)
			cr.ToolCalls = append(cr.ToolCalls, agent.ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: string(args),
			})
		}
	}

	return cr, nil
}

func (p *ClaudeProvider) buildToolRequest(req agent.CompletionRequest) []byte {
	model := req.Model
	if model == "" {
		model = p.model
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	var systemPrompt string
	var messages []interface{}
	for _, m := range req.Messages {
		if m.Role == "system" {
			systemPrompt = m.Content
			continue
		}
		if m.ToolCallID != "" {
			messages = append(messages, map[string]interface{}{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type":        "tool_result",
						"tool_use_id": m.ToolCallID,
						"content":     m.Content,
					},
				},
			})
		} else if len(m.ToolCalls) > 0 {
			var content []map[string]interface{}
			if m.Content != "" {
				content = append(content, map[string]interface{}{
					"type": "text",
					"text": m.Content,
				})
			}
			for _, tc := range m.ToolCalls {
				var input interface{}
				json.Unmarshal([]byte(tc.Arguments), &input)
				content = append(content, map[string]interface{}{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": input,
				})
			}
			messages = append(messages, map[string]interface{}{
				"role":    "assistant",
				"content": content,
			})
		} else {
			messages = append(messages, map[string]string{"role": m.Role, "content": m.Content})
		}
	}

	payload := map[string]interface{}{
		"model":      model,
		"messages":   messages,
		"max_tokens": maxTokens,
	}
	if systemPrompt != "" {
		payload["system"] = systemPrompt
	}

	if len(req.Tools) > 0 {
		var tools []map[string]interface{}
		for _, t := range req.Tools {
			props := make(map[string]interface{})
			for name, param := range t.Parameters {
				p := map[string]interface{}{"type": param.Type, "description": param.Description}
				if len(param.Enum) > 0 {
					p["enum"] = param.Enum
				}
				props[name] = p
			}
			tools = append(tools, map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"input_schema": map[string]interface{}{
					"type":       "object",
					"properties": props,
					"required":   t.Required,
				},
			})
		}
		payload["tools"] = tools
	}

	data, _ := json.Marshal(payload)
	return data
}

func (p *ClaudeProvider) buildRequest(req agent.CompletionRequest, stream bool) []byte {
	model := req.Model
	if model == "" {
		model = p.model
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	var systemPrompt string
	var messages []map[string]string
	for _, m := range req.Messages {
		if m.Role == "system" {
			systemPrompt = m.Content
			continue
		}
		messages = append(messages, map[string]string{"role": m.Role, "content": m.Content})
	}

	payload := map[string]interface{}{
		"model":      model,
		"messages":   messages,
		"max_tokens": maxTokens,
		"stream":     stream,
	}
	if systemPrompt != "" {
		payload["system"] = systemPrompt
	}

	data, _ := json.Marshal(payload)
	return data
}

func (p *ClaudeProvider) doRequest(ctx context.Context, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	return http.DefaultClient.Do(req)
}

type claudeResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

type claudeToolResponse struct {
	Content []struct {
		Type  string                 `json:"type"`
		Text  string                 `json:"text,omitempty"`
		ID    string                 `json:"id,omitempty"`
		Name  string                 `json:"name,omitempty"`
		Input map[string]interface{} `json:"input,omitempty"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
}

type claudeStreamEvent struct {
	Type  string `json:"type"`
	Delta *struct {
		Text string `json:"text"`
	} `json:"delta,omitempty"`
}
