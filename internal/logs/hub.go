package logs

import (
	"fmt"
	"sync"
	"time"
)

const (
	ChannelSystem = "System"
	ChannelApp    = "App"
	ChannelDrift  = "Drift"
	ChannelAgent  = "Agent"
)

type Hub struct {
	mu       sync.RWMutex
	maxLines int
	lines    map[string][]string
}

func NewHub(maxLines int) *Hub {
	if maxLines < 100 {
		maxLines = 100
	}
	return &Hub{maxLines: maxLines, lines: map[string][]string{ChannelSystem: {}, ChannelApp: {}, ChannelDrift: {}, ChannelAgent: {}}}
}

func (h *Hub) Add(channel, msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.lines[channel]; !ok {
		h.lines[channel] = []string{}
	}
	line := fmt.Sprintf("%s %s", time.Now().Format("15:04:05"), msg)
	h.lines[channel] = append(h.lines[channel], line)
	if len(h.lines[channel]) > h.maxLines {
		h.lines[channel] = h.lines[channel][len(h.lines[channel])-h.maxLines:]
	}
}

func (h *Hub) Snapshot(channel string) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	cc := h.lines[channel]
	out := make([]string, len(cc))
	copy(out, cc)
	return out
}
