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

type OpenAIProvider struct {
	apiKey  string
	baseURL string
	model   string
}

func NewOpenAIProvider(cfg *config.LLMConfig) *OpenAIProvider {
	apiKey := cfg.ResolveAPIKey()
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	model := cfg.Model
	if model == "" {
		model = "gpt-4o"
	}
	return &OpenAIProvider{apiKey: apiKey, baseURL: baseURL, model: model}
}

func (p *OpenAIProvider) Name() string { return "openai" }

func (p *OpenAIProvider) Complete(ctx context.Context, req agent.CompletionRequest) (string, error) {
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
		return "", fmt.Errorf("OpenAI API error %d: %s", resp.StatusCode, string(data))
	}

	var result openAIResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return result.Choices[0].Message.Content, nil
}

func (p *OpenAIProvider) Stream(ctx context.Context, req agent.CompletionRequest) (<-chan agent.StreamChunk, error) {
	body := p.buildRequest(req, true)
	resp, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("OpenAI API error %d: %s", resp.StatusCode, string(data))
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
			if data == "[DONE]" {
				ch <- agent.StreamChunk{Done: true}
				return
			}
			var chunk openAIStreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				ch <- agent.StreamChunk{Content: chunk.Choices[0].Delta.Content}
			}
		}
		ch <- agent.StreamChunk{Done: true}
	}()

	return ch, nil
}

func (p *OpenAIProvider) CompleteWithTools(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
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
		return nil, fmt.Errorf("OpenAI API error %d: %s", resp.StatusCode, string(data))
	}

	var result openAIToolResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := result.Choices[0]
	cr := &agent.CompletionResponse{Content: choice.Message.Content}

	for _, tc := range choice.Message.ToolCalls {
		cr.ToolCalls = append(cr.ToolCalls, agent.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}

	return cr, nil
}

func (p *OpenAIProvider) buildToolRequest(req agent.CompletionRequest) []byte {
	model := req.Model
	if model == "" {
		model = p.model
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	var messages []interface{}
	for _, m := range req.Messages {
		if m.ToolCallID != "" {
			messages = append(messages, map[string]interface{}{
				"role":         "tool",
				"content":      m.Content,
				"tool_call_id": m.ToolCallID,
			})
		} else if len(m.ToolCalls) > 0 {
			var tcs []map[string]interface{}
			for _, tc := range m.ToolCalls {
				tcs = append(tcs, map[string]interface{}{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]interface{}{
						"name":      tc.Name,
						"arguments": tc.Arguments,
					},
				})
			}
			msg := map[string]interface{}{
				"role":       "assistant",
				"tool_calls": tcs,
			}
			if m.Content != "" {
				msg["content"] = m.Content
			}
			messages = append(messages, msg)
		} else {
			messages = append(messages, map[string]string{"role": m.Role, "content": m.Content})
		}
	}

	payload := map[string]interface{}{
		"model":       model,
		"messages":    messages,
		"max_tokens":  maxTokens,
		"temperature": req.Temperature,
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
				"type": "function",
				"function": map[string]interface{}{
					"name":        t.Name,
					"description": t.Description,
					"parameters": map[string]interface{}{
						"type":       "object",
						"properties": props,
						"required":   t.Required,
					},
				},
			})
		}
		payload["tools"] = tools
	}

	data, _ := json.Marshal(payload)
	return data
}

func (p *OpenAIProvider) buildRequest(req agent.CompletionRequest, stream bool) []byte {
	model := req.Model
	if model == "" {
		model = p.model
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	messages := make([]map[string]string, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = map[string]string{"role": m.Role, "content": m.Content}
	}

	payload := map[string]interface{}{
		"model":       model,
		"messages":    messages,
		"max_tokens":  maxTokens,
		"temperature": req.Temperature,
		"stream":      stream,
	}

	data, _ := json.Marshal(payload)
	return data
}

func (p *OpenAIProvider) doRequest(ctx context.Context, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	return http.DefaultClient.Do(req)
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type openAIToolResponse struct {
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}
