package codexbridge

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHandleServerRequestApproval(t *testing.T) {
	b := NewAppServerBridge("codex", ".", "gpt-5")
	b.handleLine(`{"id":42,"method":"item/commandExecution/requestApproval","params":{"threadId":"t1","turnId":"u1","command":"aws","args":["sso","login","--profile","dev"]}}`)
	ev := <-b.Events()
	if ev.Type != EventApprovalReq {
		t.Fatalf("expected approval event, got %s", ev.Type)
	}
	if ev.RequestKey == "" {
		t.Fatal("expected request key")
	}
	if !strings.Contains(ev.Text, "aws sso login --profile dev") {
		t.Fatalf("expected command preview in approval text, got %q", ev.Text)
	}
}

func TestHandleServerRequestQuestion(t *testing.T) {
	b := NewAppServerBridge("codex", ".", "gpt-5")
	b.handleLine(`{"id":44,"method":"item/tool/requestUserInput","params":{"threadId":"t1","turnId":"u1","itemId":"x","questions":[{"id":"q1","header":"H","question":"Proceed?"}]}}`)
	ev := <-b.Events()
	if ev.Type != EventQuestionReq {
		t.Fatalf("expected question event, got %s", ev.Type)
	}
	if len(ev.QuestionIDs) != 1 || ev.QuestionIDs[0] != "q1" {
		t.Fatalf("unexpected question ids: %#v", ev.QuestionIDs)
	}
}

func TestHandleNotificationAgentDelta(t *testing.T) {
	b := NewAppServerBridge("codex", ".", "gpt-5")
	b.handleLine(`{"method":"item/agentMessage/delta","params":{"delta":"hello"}}`)
	ev := <-b.Events()
	if ev.Type != EventAgentDelta || ev.Text != "hello" {
		t.Fatalf("unexpected event: %#v", ev)
	}
}

func TestClearByRawID(t *testing.T) {
	b := NewAppServerBridge("codex", ".", "gpt-5")
	key := b.putRequest(json.RawMessage("55"), "item/fileChange/requestApproval", nil)
	b.clearByRawID(json.RawMessage("55"))
	if _, ok := b.takeRequest(key); ok {
		t.Fatal("expected request to be cleared")
	}
}

func TestReconnectErrorIsTransientProgress(t *testing.T) {
	b := NewAppServerBridge("codex", ".", "gpt-5")
	b.stateMu.Lock()
	b.turnID = "turn-1"
	b.stateMu.Unlock()

	b.handleLine(`{"method":"error","params":{"error":{"message":"Reconnecting... 2/5"}}}`)
	ev := <-b.Events()
	if ev.Type != EventInfo {
		t.Fatalf("expected info event for reconnect, got %s", ev.Type)
	}
	if !strings.Contains(ev.Text, "Reconnecting... 2/5") {
		t.Fatalf("unexpected reconnect text: %q", ev.Text)
	}

	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	if b.turnID != "turn-1" {
		t.Fatalf("expected turn to remain active, got %q", b.turnID)
	}
}

func TestItemCompletedEmitsProgress(t *testing.T) {
	b := NewAppServerBridge("codex", ".", "gpt-5")
	b.handleLine(`{"method":"item/completed","params":{"item":{"type":"mcpToolCall"}}}`)
	ev := <-b.Events()
	if ev.Type != EventInfo {
		t.Fatalf("expected info event, got %s", ev.Type)
	}
	if !strings.Contains(ev.Text, "item completed: mcpToolCall") {
		t.Fatalf("unexpected event text: %q", ev.Text)
	}
}
