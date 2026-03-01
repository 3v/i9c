package mcp

import (
	"context"
	"errors"
	"testing"
)

type mockBackend struct {
	name    string
	payload string
	err     error
}

func (m mockBackend) Name() string { return m.name }
func (m mockBackend) Discover(_ context.Context, _ string) (string, error) {
	return m.payload, m.err
}

func TestDiscoverFallsBackToSecondary(t *testing.T) {
	m := &Manager{
		Primary:  mockBackend{name: "managed", err: errors.New("down")},
		Fallback: mockBackend{name: "local", payload: "ok"},
	}
	backend, payload, err := m.Discover(context.Background(), "ec2")
	if err != nil {
		t.Fatal(err)
	}
	if backend != "local" || payload != "ok" {
		t.Fatalf("unexpected fallback result: backend=%s payload=%s", backend, payload)
	}
}
