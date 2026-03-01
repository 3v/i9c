package aws

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
)

var commandContext = exec.CommandContext

type SSOLoginRunner interface {
	Login(ctx context.Context, profile string, sink func(line string)) error
}

type CLISSOLoginRunner struct{}

func (r *CLISSOLoginRunner) Login(ctx context.Context, profile string, sink func(line string)) error {
	args := buildSSOLoginCmd(profile)
	cmd := commandContext(ctx, args[0], args[1:]...)
	cmd.Env = append(os.Environ(), r.browserEnv()...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	stream := func(prefix string, r io.Reader) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			if sink != nil {
				sink(fmt.Sprintf("%s %s", prefix, sc.Text()))
			}
		}
	}
	wg.Add(2)
	go stream("[stdout]", stdout)
	go stream("[stderr]", stderr)
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		return err
	}
	if sink != nil {
		sink("SSO login completed")
	}
	return nil
}

func buildSSOLoginCmd(profile string) []string {
	return []string{"aws", "sso", "login", "--profile", profile}
}

func (r *CLISSOLoginRunner) browserEnv() []string { return nil }

type ConfiguredSSOLoginRunner struct {
	BrowserCommand    string
	BrowserProfileDir string
}

func (r *ConfiguredSSOLoginRunner) Login(ctx context.Context, profile string, sink func(line string)) error {
	args := buildSSOLoginCmd(profile)
	cmd := commandContext(ctx, args[0], args[1:]...)
	cmd.Env = append(os.Environ(), r.browserEnv()...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	stream := func(prefix string, reader io.Reader) {
		defer wg.Done()
		sc := bufio.NewScanner(reader)
		for sc.Scan() {
			if sink != nil {
				sink(fmt.Sprintf("%s %s", prefix, sc.Text()))
			}
		}
	}
	wg.Add(2)
	go stream("[stdout]", stdout)
	go stream("[stderr]", stderr)
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		return err
	}
	if sink != nil {
		sink("SSO login completed")
	}
	return nil
}

func (r *ConfiguredSSOLoginRunner) browserEnv() []string {
	if r.BrowserCommand == "" && r.BrowserProfileDir == "" {
		return nil
	}
	out := []string{}
	if r.BrowserCommand != "" {
		out = append(out, "BROWSER="+r.BrowserCommand)
	}
	if r.BrowserProfileDir != "" && runtime.GOOS == "darwin" {
		// Works with Chromium-based browsers when BROWSER points to a wrapper script.
		out = append(out, "AWS_SSO_CHROME_PROFILE_DIR="+r.BrowserProfileDir)
	}
	return out
}
