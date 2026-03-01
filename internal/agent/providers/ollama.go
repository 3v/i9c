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

type OllamaProvider struct {
	baseURL string
	model   string
}

func NewOllamaProvider(cfg *config.LLMConfig) *OllamaProvider {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	model := cfg.Model
	if model == "" {
		model = "llama3"
	}
	return &OllamaProvider{baseURL: baseURL, model: model}
}

func (p *OllamaProvider) Name() string { return "ollama" }

func (p *OllamaProvider) Complete(ctx context.Context, req agent.CompletionRequest) (string, error) {
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
		return "", fmt.Errorf("Ollama API error %d: %s", resp.StatusCode, string(data))
	}

	var result ollamaResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	return result.Message.Content, nil
}

func (p *OllamaProvider) CompleteWithTools(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	if len(req.Tools) > 0 {
		var sb strings.Builder
		sb.WriteString("\n\n## Available Tools\nYou have access to the following tools. Describe which tool you would call and with what arguments, but you cannot execute them directly.\n")
		for _, t := range req.Tools {
			fmt.Fprintf(&sb, "- %s: %s\n", t.Name, t.Description)
		}
		if len(req.Messages) > 0 && req.Messages[0].Role == "system" {
			req.Messages[0].Content += sb.String()
		}
	}
	result, err := p.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	return &agent.CompletionResponse{Content: result}, nil
}

func (p *OllamaProvider) Stream(ctx context.Context, req agent.CompletionRequest) (<-chan agent.StreamChunk, error) {
	body := p.buildRequest(req, true)
	resp, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("Ollama API error %d: %s", resp.StatusCode, string(data))
	}

	ch := make(chan agent.StreamChunk)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			var chunk ollamaStreamChunk
			if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
				continue
			}
			if chunk.Message.Content != "" {
				ch <- agent.StreamChunk{Content: chunk.Message.Content}
			}
			if chunk.Done {
				ch <- agent.StreamChunk{Done: true}
				return
			}
		}
		ch <- agent.StreamChunk{Done: true}
	}()

	return ch, nil
}

func (p *OllamaProvider) buildRequest(req agent.CompletionRequest, stream bool) []byte {
	model := req.Model
	if model == "" {
		model = p.model
	}

	messages := make([]map[string]string, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = map[string]string{"role": m.Role, "content": m.Content}
	}

	payload := map[string]interface{}{
		"model":    model,
		"messages": messages,
		"stream":   stream,
	}
	if req.Temperature > 0 {
		payload["options"] = map[string]interface{}{"temperature": req.Temperature}
	}

	data, _ := json.Marshal(payload)
	return data
}

func (p *OllamaProvider) doRequest(ctx context.Context, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}

type ollamaResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

type ollamaStreamChunk struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done bool `json:"done"`
}
