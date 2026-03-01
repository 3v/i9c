package aws

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestBuildSSOLoginCmd(t *testing.T) {
	got := buildSSOLoginCmd("dev")
	want := []string{"aws", "sso", "login", "--profile", "dev"}
	if len(got) != len(want) {
		t.Fatalf("unexpected command length: %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %q at index %d, got %q", want[i], i, got[i])
		}
	}
}

func TestCLISSOLoginRunnerStreamsOutput(t *testing.T) {
	orig := commandContext
	t.Cleanup(func() { commandContext = orig })
	commandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "echo out; echo err 1>&2")
	}

	var lines []string
	r := &CLISSOLoginRunner{}
	if err := r.Login(context.Background(), "dev", func(line string) { lines = append(lines, line) }); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "[stdout] out") {
		t.Fatalf("missing stdout line: %s", joined)
	}
	if !strings.Contains(joined, "[stderr] err") {
		t.Fatalf("missing stderr line: %s", joined)
	}
	if !strings.Contains(joined, "SSO login completed") {
		t.Fatalf("missing completion line: %s", joined)
	}
}

func TestConfiguredSSOLoginRunnerBrowserEnv(t *testing.T) {
	r := &ConfiguredSSOLoginRunner{
		BrowserCommand:    "open -na 'Google Chrome' --args --profile-directory=Profile 3",
		BrowserProfileDir: "Profile 3",
	}
	env := strings.Join(r.browserEnv(), "\n")
	if !strings.Contains(env, "BROWSER=open -na 'Google Chrome' --args --profile-directory=Profile 3") {
		t.Fatalf("expected browser env to include BROWSER, got %s", env)
	}
}
