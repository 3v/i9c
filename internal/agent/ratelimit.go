package agent

import (
	"context"
	"strings"
	"sync"
	"time"
)

var ModelTPMLimits = map[string]int{
	"gpt-4o":            30000,
	"gpt-4o-mini":       200000,
	"gpt-4-turbo":       30000,
	"gpt-4":             10000,
	"gpt-3.5-turbo":     60000,
	"claude-3-opus":     20000,
	"claude-3-sonnet":   40000,
	"claude-3-haiku":    50000,
	"claude-3.5-sonnet": 40000,
	"claude-3.5-haiku":  50000,
}

const (
	defaultTPM     = 30000
	safetyFactor   = 0.80
	chatGPTHardCap = 25000
	charsPerToken  = 4
)

type TokenRateLimiter struct {
	mu         sync.Mutex
	maxTPM     int
	usedTokens int
	resetAt    time.Time
}

func NewTokenRateLimiter(model string) *TokenRateLimiter {
	tpm := lookupTPM(model)
	effectiveTPM := int(float64(tpm) * safetyFactor)

	if isChatGPTModel(model) && effectiveTPM > chatGPTHardCap {
		effectiveTPM = chatGPTHardCap
	}

	return &TokenRateLimiter{
		maxTPM:  effectiveTPM,
		resetAt: time.Now().Add(time.Minute),
	}
}

func (r *TokenRateLimiter) Wait(ctx context.Context, estimatedTokens int) error {
	for {
		r.mu.Lock()
		now := time.Now()
		if now.After(r.resetAt) {
			r.usedTokens = 0
			r.resetAt = now.Add(time.Minute)
		}

		if r.usedTokens+estimatedTokens <= r.maxTPM {
			r.usedTokens += estimatedTokens
			r.mu.Unlock()
			return nil
		}

		waitUntil := r.resetAt
		r.mu.Unlock()

		sleepDur := time.Until(waitUntil)
		if sleepDur <= 0 {
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleepDur):
		}
	}
}

func (r *TokenRateLimiter) Record(tokensUsed int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	if now.After(r.resetAt) {
		r.usedTokens = tokensUsed
		r.resetAt = now.Add(time.Minute)
		return
	}
	r.usedTokens += tokensUsed
}

func EstimateTokens(messages []Message) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content) / charsPerToken
		for _, tc := range m.ToolCalls {
			total += len(tc.Arguments) / charsPerToken
			total += 20 // overhead per tool call
		}
		total += 4 // per-message overhead
	}
	return total
}

func lookupTPM(model string) int {
	lower := strings.ToLower(model)
	if tpm, ok := ModelTPMLimits[lower]; ok {
		return tpm
	}
	for key, tpm := range ModelTPMLimits {
		if strings.Contains(lower, key) {
			return tpm
		}
	}
	return defaultTPM
}

func isChatGPTModel(model string) bool {
	lower := strings.ToLower(model)
	return strings.HasPrefix(lower, "gpt-")
}
