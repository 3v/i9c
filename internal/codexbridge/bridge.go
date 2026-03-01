package codexbridge

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type EventType string

const (
	EventAgentDelta   EventType = "agent_delta"
	EventTurnDone     EventType = "turn_done"
	EventTurnFailed   EventType = "turn_failed"
	EventApprovalReq  EventType = "approval_request"
	EventQuestionReq  EventType = "question_request"
	EventRequestClear EventType = "request_resolved"
	EventInfo         EventType = "info"
	EventError        EventType = "error"
)

type Event struct {
	Type        EventType
	Text        string
	RequestKey  string
	RequestKind string
	QuestionIDs []string
}

type requestState struct {
	RawID      json.RawMessage
	Method     string
	QuestionID []string
}

var execCommandContext = exec.CommandContext
var reconnectPattern = regexp.MustCompile(`(?i)reconnect`)

type AppServerBridge struct {
	Command string
	Cwd     string
	Model   string

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	events chan Event

	nextID    int64
	readerWG  sync.WaitGroup
	writeMu   sync.Mutex
	stateMu   sync.Mutex
	threadID  string
	turnID    string
	requests  map[string]requestState
	requestSN int64
}

func NewAppServerBridge(command, cwd, model string) *AppServerBridge {
	if command == "" {
		command = "codex"
	}
	if strings.TrimSpace(cwd) == "" {
		cwd = "."
	}
	return &AppServerBridge{
		Command:  command,
		Cwd:      cwd,
		Model:    model,
		events:   make(chan Event, 1024),
		requests: make(map[string]requestState),
	}
}

func (b *AppServerBridge) Events() <-chan Event { return b.events }

func (b *AppServerBridge) Start(ctx context.Context) error {
	b.cmd = execCommandContext(ctx, b.Command, "app-server")
	if b.Cwd != "" {
		b.cmd.Dir = b.Cwd
	}
	stdin, err := b.cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := b.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := b.cmd.StderrPipe()
	if err != nil {
		return err
	}
	b.stdin = stdin

	if err := b.cmd.Start(); err != nil {
		return err
	}

	b.readerWG.Add(2)
	go b.readStdout(stdout)
	go b.readStderr(stderr)

	if _, err := b.request(ctx, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "i9c",
			"title":   "i9c",
			"version": "0.1.0",
		},
		"capabilities": map[string]any{
			"experimentalApi": true,
		},
	}); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	_ = b.notify("initialized", map[string]any{})

	threadResult, err := b.request(ctx, "thread/start", map[string]any{
		"cwd":   b.Cwd,
		"model": b.Model,
	})
	if err != nil {
		return fmt.Errorf("thread/start: %w", err)
	}
	var start struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(threadResult, &start); err == nil && start.Thread.ID != "" {
		b.stateMu.Lock()
		b.threadID = start.Thread.ID
		b.stateMu.Unlock()
	}
	b.emit(Event{Type: EventInfo, Text: "Codex app-server connected"})
	return nil
}

func (b *AppServerBridge) Stop(ctx context.Context) error {
	_ = ctx
	if b.stdin != nil {
		_ = b.stdin.Close()
	}
	if b.cmd != nil && b.cmd.Process != nil {
		_ = b.cmd.Process.Kill()
		_ = b.cmd.Wait()
	}
	b.readerWG.Wait()
	close(b.events)
	return nil
}

func (b *AppServerBridge) SendUserTurn(ctx context.Context, text string) error {
	b.stateMu.Lock()
	threadID := b.threadID
	b.stateMu.Unlock()
	if threadID == "" {
		return fmt.Errorf("missing thread id")
	}
	_, err := b.request(ctx, "turn/start", map[string]any{
		"threadId": threadID,
		"input": []map[string]any{
			{"type": "text", "text": text},
		},
	})
	if err == nil {
		b.emit(Event{Type: EventInfo, Text: "progress: turn request accepted"})
	}
	return err
}

func (b *AppServerBridge) InterruptActiveTurn(ctx context.Context) error {
	b.stateMu.Lock()
	threadID := b.threadID
	turnID := b.turnID
	b.stateMu.Unlock()
	if threadID == "" || turnID == "" {
		return fmt.Errorf("no active turn")
	}
	_, err := b.request(ctx, "turn/interrupt", map[string]any{
		"threadId": threadID,
		"turnId":   turnID,
	})
	if err != nil {
		return err
	}
	b.emit(Event{Type: EventInfo, Text: "progress: turn interrupt requested"})
	return nil
}

func (b *AppServerBridge) RespondApproval(ctx context.Context, requestKey, decision string) error {
	_ = ctx
	req, ok := b.takeRequest(requestKey)
	if !ok {
		return fmt.Errorf("unknown request key %s", requestKey)
	}
	payload := map[string]any{
		"id":     json.RawMessage(req.RawID),
		"result": map[string]any{"decision": decision},
	}
	return b.writeJSON(payload)
}

func (b *AppServerBridge) RespondQuestion(ctx context.Context, requestKey, answer string) error {
	_ = ctx
	req, ok := b.takeRequest(requestKey)
	if !ok {
		return fmt.Errorf("unknown request key %s", requestKey)
	}
	answers := map[string]any{}
	for _, qid := range req.QuestionID {
		answers[qid] = map[string]any{"answers": []string{answer}}
	}
	payload := map[string]any{
		"id": json.RawMessage(req.RawID),
		"result": map[string]any{
			"answers": answers,
		},
	}
	return b.writeJSON(payload)
}

func (b *AppServerBridge) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := atomic.AddInt64(&b.nextID, 1)
	respCh := make(chan struct {
		result json.RawMessage
		err    error
	}, 1)

	waitKey := fmt.Sprintf("__wait_%d", id)
	b.stateMu.Lock()
	b.requests[waitKey] = requestState{}
	b.stateMu.Unlock()

	msg := map[string]any{
		"id":     id,
		"method": method,
		"params": params,
	}
	if err := b.writeJSON(msg); err != nil {
		return nil, err
	}

	go func() {
		deadline := time.NewTimer(45 * time.Second)
		defer deadline.Stop()
		t := time.NewTicker(50 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				respCh <- struct {
					result json.RawMessage
					err    error
				}{nil, ctx.Err()}
				return
			case <-deadline.C:
				respCh <- struct {
					result json.RawMessage
					err    error
				}{nil, fmt.Errorf("timeout waiting for %s response", method)}
				return
			case <-t.C:
				b.stateMu.Lock()
				key := fmt.Sprintf("__resp_%d", id)
				st, ok := b.requests[key]
				if ok {
					delete(b.requests, key)
					b.stateMu.Unlock()
					if len(st.RawID) > 0 {
						respCh <- struct {
							result json.RawMessage
							err    error
						}{st.RawID, nil}
					} else {
						respCh <- struct {
							result json.RawMessage
							err    error
						}{nil, fmt.Errorf("empty response for %s", method)}
					}
					return
				}
				errKey := fmt.Sprintf("__err_%d", id)
				er, ok := b.requests[errKey]
				if ok {
					delete(b.requests, errKey)
					b.stateMu.Unlock()
					respCh <- struct {
						result json.RawMessage
						err    error
					}{nil, fmt.Errorf("rpc error: %s", string(er.RawID))}
					return
				}
				b.stateMu.Unlock()
			}
		}
	}()

	resp := <-respCh
	return resp.result, resp.err
}

func (b *AppServerBridge) notify(method string, params any) error {
	return b.writeJSON(map[string]any{
		"method": method,
		"params": params,
	})
}

func (b *AppServerBridge) writeJSON(v any) error {
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := b.stdin.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func (b *AppServerBridge) readStdout(r io.Reader) {
	defer b.readerWG.Done()
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		b.handleLine(line)
	}
}

func (b *AppServerBridge) readStderr(r io.Reader) {
	defer b.readerWG.Done()
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		b.emit(Event{Type: EventInfo, Text: "[codex] " + line})
	}
}

func (b *AppServerBridge) handleLine(line string) {
	var msg struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		b.emit(Event{Type: EventError, Text: "invalid codex message: " + err.Error()})
		return
	}

	if len(msg.ID) > 0 && msg.Method == "" {
		var id int64
		_ = json.Unmarshal(msg.ID, &id)
		b.stateMu.Lock()
		if len(msg.Error) > 0 {
			b.requests[fmt.Sprintf("__err_%d", id)] = requestState{RawID: msg.Error}
		} else {
			b.requests[fmt.Sprintf("__resp_%d", id)] = requestState{RawID: msg.Result}
		}
		b.stateMu.Unlock()
		return
	}

	if len(msg.ID) > 0 && msg.Method != "" {
		b.handleServerRequest(msg.ID, msg.Method, msg.Params)
		return
	}

	if msg.Method != "" {
		b.handleNotification(msg.Method, msg.Params)
	}
}

func (b *AppServerBridge) handleServerRequest(id json.RawMessage, method string, params json.RawMessage) {
	switch method {
	case "item/commandExecution/requestApproval":
		key := b.putRequest(id, method, nil)
		cmd := extractCommandPreview(params)
		text := "Codex requests command approval."
		if cmd != "" {
			text += "\nCommand: " + cmd
		}
		text += "\nReply: approve | session | decline | cancel"
		b.emit(Event{Type: EventApprovalReq, RequestKey: key, RequestKind: "command", Text: text})
	case "item/fileChange/requestApproval":
		key := b.putRequest(id, method, nil)
		b.emit(Event{Type: EventApprovalReq, RequestKey: key, RequestKind: "file", Text: "Codex requests file-change approval. Reply: approve | session | decline | cancel"})
	case "item/tool/requestUserInput":
		var p struct {
			Questions []struct {
				ID       string `json:"id"`
				Header   string `json:"header"`
				Question string `json:"question"`
			} `json:"questions"`
		}
		_ = json.Unmarshal(params, &p)
		qids := make([]string, 0, len(p.Questions))
		prompt := "Codex requested user input."
		if len(p.Questions) > 0 {
			prompt = p.Questions[0].Question
		}
		for _, q := range p.Questions {
			qids = append(qids, q.ID)
		}
		key := b.putRequest(id, method, qids)
		b.emit(Event{Type: EventQuestionReq, RequestKey: key, RequestKind: "question", QuestionIDs: qids, Text: prompt})
	default:
		_ = b.writeJSON(map[string]any{
			"id":    json.RawMessage(id),
			"error": map[string]any{"code": -32601, "message": "unsupported request method"},
		})
	}
}

func (b *AppServerBridge) handleNotification(method string, params json.RawMessage) {
	switch method {
	case "turn/started":
		var p struct {
			Turn struct {
				ID string `json:"id"`
			} `json:"turn"`
		}
		_ = json.Unmarshal(params, &p)
		if p.Turn.ID != "" {
			b.stateMu.Lock()
			b.turnID = p.Turn.ID
			b.stateMu.Unlock()
		}
		b.emit(Event{Type: EventInfo, Text: "progress: turn started"})
	case "item/started":
		var p struct {
			Item struct {
				Type string `json:"type"`
			} `json:"item"`
		}
		_ = json.Unmarshal(params, &p)
		itemType := strings.TrimSpace(p.Item.Type)
		if itemType == "" {
			itemType = "unknown"
		}
		b.emit(Event{Type: EventInfo, Text: "progress: item started: " + itemType})
	case "item/completed":
		var p struct {
			Item struct {
				Type string `json:"type"`
			} `json:"item"`
		}
		_ = json.Unmarshal(params, &p)
		itemType := strings.TrimSpace(p.Item.Type)
		if itemType == "" {
			itemType = "unknown"
		}
		b.emit(Event{Type: EventInfo, Text: "progress: item completed: " + itemType})
	case "item/agentMessage/delta":
		var p struct {
			Delta string `json:"delta"`
		}
		_ = json.Unmarshal(params, &p)
		if p.Delta != "" {
			b.emit(Event{Type: EventAgentDelta, Text: p.Delta})
		}
	case "turn/completed":
		b.stateMu.Lock()
		b.turnID = ""
		b.stateMu.Unlock()
		b.emit(Event{Type: EventTurnDone})
	case "turn/failed", "error":
		var p struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(params, &p)
		msg := p.Error.Message
		if msg == "" {
			msg = "Codex turn failed"
		}
		if reconnectPattern.MatchString(msg) {
			b.emit(Event{Type: EventInfo, Text: "progress: " + msg})
			return
		}
		b.stateMu.Lock()
		b.turnID = ""
		b.stateMu.Unlock()
		b.emit(Event{Type: EventTurnFailed, Text: msg})
	case "serverRequest/resolved":
		var p struct {
			RequestID json.RawMessage `json:"requestId"`
		}
		_ = json.Unmarshal(params, &p)
		if len(p.RequestID) > 0 {
			b.clearByRawID(p.RequestID)
		}
		b.emit(Event{Type: EventRequestClear})
	}
}

func (b *AppServerBridge) putRequest(rawID json.RawMessage, method string, qids []string) string {
	key := fmt.Sprintf("req-%d", atomic.AddInt64(&b.requestSN, 1))
	b.stateMu.Lock()
	b.requests[key] = requestState{RawID: rawID, Method: method, QuestionID: qids}
	b.stateMu.Unlock()
	return key
}

func (b *AppServerBridge) takeRequest(key string) (requestState, bool) {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	req, ok := b.requests[key]
	if ok {
		delete(b.requests, key)
	}
	return req, ok
}

func (b *AppServerBridge) clearByRawID(raw json.RawMessage) {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	target := strings.TrimSpace(string(raw))
	for k, v := range b.requests {
		if strings.TrimSpace(string(v.RawID)) == target {
			delete(b.requests, k)
		}
	}
}

func (b *AppServerBridge) emit(ev Event) {
	select {
	case b.events <- ev:
	default:
	}
}

func extractCommandPreview(params json.RawMessage) string {
	if len(params) == 0 {
		return ""
	}
	var raw map[string]any
	if err := json.Unmarshal(params, &raw); err != nil {
		return ""
	}
	return firstCommandLike(raw)
}

func firstCommandLike(v any) string {
	switch t := v.(type) {
	case map[string]any:
		for _, key := range []string{"command", "cmd", "executable", "program"} {
			if s, ok := t[key].(string); ok && strings.TrimSpace(s) != "" {
				args := joinArgs(t["args"])
				if args == "" {
					args = joinArgs(t["argv"])
				}
				if args != "" {
					return strings.TrimSpace(s + " " + args)
				}
				return strings.TrimSpace(s)
			}
		}
		for _, key := range []string{"input", "item", "request", "payload", "commandExecution"} {
			if nested, ok := t[key]; ok {
				if out := firstCommandLike(nested); out != "" {
					return out
				}
			}
		}
		for _, val := range t {
			if out := firstCommandLike(val); out != "" {
				return out
			}
		}
	case []any:
		for _, item := range t {
			if out := firstCommandLike(item); out != "" {
				return out
			}
		}
	case string:
		if strings.Contains(t, " ") && (strings.Contains(t, "aws") || strings.Contains(t, "terraform") || strings.Contains(t, "tofu")) {
			return strings.TrimSpace(t)
		}
	}
	return ""
}

func joinArgs(v any) string {
	switch a := v.(type) {
	case []any:
		out := make([]string, 0, len(a))
		for _, item := range a {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return strings.Join(out, " ")
	case []string:
		return strings.Join(a, " ")
	default:
		return ""
	}
}
