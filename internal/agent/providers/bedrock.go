package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"i9c/internal/agent"
	"i9c/internal/config"
)

type BedrockProvider struct {
	cfg    *config.LLMConfig
	model  string
	region string
}

func NewBedrockProvider(cfg *config.LLMConfig) *BedrockProvider {
	model := cfg.Model
	if model == "" {
		model = "anthropic.claude-sonnet-4-20250514-v1:0"
	}
	region := "us-east-1"
	if cfg.BaseURL != "" {
		region = cfg.BaseURL
	}
	return &BedrockProvider{cfg: cfg, model: model, region: region}
}

func (p *BedrockProvider) Name() string { return "bedrock" }

func (p *BedrockProvider) Complete(ctx context.Context, req agent.CompletionRequest) (string, error) {
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
		"anthropic_version": "bedrock-2023-05-31",
		"messages":          messages,
		"max_tokens":        maxTokens,
		"temperature":       req.Temperature,
	}
	if systemPrompt != "" {
		payload["system"] = systemPrompt
	}

	body, _ := json.Marshal(payload)
	endpoint := fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com/model/%s/invoke", p.region, p.model)

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(p.region))
	if err != nil {
		return "", fmt.Errorf("loading AWS config for Bedrock: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	creds, err := awsCfg.Credentials.Retrieve(ctx)
	if err != nil {
		return "", fmt.Errorf("retrieving AWS credentials: %w", err)
	}

	signer := v4.NewSigner()
	hash := "UNSIGNED-PAYLOAD"
	err = signer.SignHTTP(ctx, creds, httpReq, hash, "bedrock", p.region, time.Now())
	if err != nil {
		return "", fmt.Errorf("signing request: %w", err)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Bedrock API error %d: %s", resp.StatusCode, string(data))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}

	var content string
	for _, block := range result.Content {
		content += block.Text
	}
	return content, nil
}

func (p *BedrockProvider) CompleteWithTools(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
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

func (p *BedrockProvider) Stream(ctx context.Context, req agent.CompletionRequest) (<-chan agent.StreamChunk, error) {
	// Bedrock streaming uses a different content type and binary event stream.
	// For MVP, fall back to non-streaming and emit the full result as one chunk.
	result, err := p.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	ch := make(chan agent.StreamChunk, 1)
	ch <- agent.StreamChunk{Content: result, Done: true}
	close(ch)
	return ch, nil
}
