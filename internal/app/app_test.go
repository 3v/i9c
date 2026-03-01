package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTryStartDriftRunDebounce(t *testing.T) {
	a := &App{}
	if ok := a.tryStartDriftRun(); !ok {
		t.Fatal("expected first run to start")
	}
	if ok := a.tryStartDriftRun(); ok {
		t.Fatal("expected second run to be debounced")
	}
	a.finishDriftRun()
	if ok := a.tryStartDriftRun(); !ok {
		t.Fatal("expected run to start after finish")
	}
}

func TestMapApprovalDecision(t *testing.T) {
	cases := map[string]string{
		"approve": "accept",
		"session": "acceptForSession",
		"decline": "decline",
		"cancel":  "cancel",
		"blah":    "",
	}
	for in, want := range cases {
		if got := mapApprovalDecision(in); got != want {
			t.Fatalf("input %q: expected %q got %q", in, want, got)
		}
	}
}

func TestNormalizeCodexModel(t *testing.T) {
	if got := normalizeCodexModel(""); got != "gpt-5" {
		t.Fatalf("expected gpt-5 for empty model, got %s", got)
	}
	if got := normalizeCodexModel("gpt-4o"); got != "gpt-5" {
		t.Fatalf("expected gpt-5 for gpt-4o, got %s", got)
	}
	if got := normalizeCodexModel("gpt-5"); got != "gpt-5" {
		t.Fatalf("expected gpt-5 passthrough, got %s", got)
	}
}

func TestIsIACInitialized(t *testing.T) {
	dir := t.TempDir()
	if isIACInitialized(dir) {
		t.Fatal("expected empty dir to be uninitialized")
	}
	if err := os.Mkdir(filepath.Join(dir, ".terraform"), 0o700); err != nil {
		t.Fatalf("mkdir .terraform: %v", err)
	}
	if !isIACInitialized(dir) {
		t.Fatal("expected dir with .terraform to be initialized")
	}
}

func TestDetectIACExtensions(t *testing.T) {
	dir := t.TempDir()
	files := []string{"main.tf", "vars.tf", "network.tofu", "README.md"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}
	tfCount, tofuCount, err := detectIACExtensions(dir)
	if err != nil {
		t.Fatalf("detectIACExtensions: %v", err)
	}
	if tfCount != 2 || tofuCount != 1 {
		t.Fatalf("expected tf=2 tofu=1 got tf=%d tofu=%d", tfCount, tofuCount)
	}
}

func TestCommandAvailable(t *testing.T) {
	if commandAvailable("definitely-not-a-real-command-i9c") {
		t.Fatal("expected fake command to be unavailable")
	}
}
