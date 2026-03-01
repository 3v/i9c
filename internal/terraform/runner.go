package terraform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Runner struct {
	binary  string
	workDir string
}

func NewRunner(binary, workDir string) *Runner {
	return &Runner{
		binary:  binary,
		workDir: filepath.Clean(workDir),
	}
}

func (r *Runner) SetWorkDir(dir string) {
	r.workDir = filepath.Clean(dir)
}

func (r *Runner) SetBinary(binary string) {
	r.binary = binary
}

func (r *Runner) Binary() string {
	return r.binary
}

func DetectBinary(workDir string) string {
	entries, err := os.ReadDir(filepath.Clean(workDir))
	if err != nil {
		return ""
	}

	hasTF := false
	hasTofu := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		switch ext {
		case ".tofu":
			hasTofu = true
		case ".tf":
			hasTF = true
		}
	}

	if hasTofu {
		return "tofu"
	}
	if hasTF {
		return "terraform"
	}
	return ""
}

func (r *Runner) Init(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, r.binary, "init", "-input=false")
	cmd.Dir = r.workDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("terraform init: %s: %w", stderr.String(), err)
	}
	return nil
}

func (r *Runner) Plan(ctx context.Context) (*DriftResult, error) {
	cmd := exec.CommandContext(ctx, r.binary, "plan", "-json", "-detailed-exitcode", "-input=false", "-no-color")
	cmd.Dir = r.workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		return &DriftResult{Error: fmt.Sprintf("terraform plan: %s: %v", stderr.String(), err)}, nil
	}

	result := &DriftResult{
		RawPlan:    stdout.Bytes(),
		HasChanges: exitCode == 2,
	}

	if exitCode == 1 {
		result.Error = fmt.Sprintf("terraform plan failed: %s", stderr.String())
		return result, nil
	}

	changes, err := parsePlanJSON(stdout.Bytes())
	if err != nil {
		result.Error = fmt.Sprintf("parsing plan output: %v", err)
		return result, nil
	}
	result.Changes = changes

	return result, nil
}

func (r *Runner) Show(ctx context.Context, planFile string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, r.binary, "show", "-json", planFile)
	cmd.Dir = r.workDir
	return cmd.Output()
}

func (r *Runner) RunReadOnly(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
	if err := ValidateReadOnlySubcommand(subcommand); err != nil {
		return nil, err
	}
	cmdArgs := append([]string{subcommand}, args...)
	cmd := exec.CommandContext(ctx, r.binary, cmdArgs...)
	cmd.Dir = r.workDir
	return cmd.CombinedOutput()
}

func parsePlanJSON(data []byte) ([]ResourceChange, error) {
	var changes []ResourceChange

	lines := bytes.Split(data, []byte("\n"))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		var msg planMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		if msg.Type != "resource_drift" && msg.Type != "planned_change" {
			continue
		}

		if msg.Change == nil {
			continue
		}

		action := mapAction(msg.Change.Action)
		if !action.IsDrift() {
			continue
		}

		rc := ResourceChange{
			Address: msg.Change.Resource.Addr,
			Type:    msg.Change.Resource.ResourceType,
			Name:    msg.Change.Resource.ResourceName,
			Action:  action,
		}

		if msg.Change.Before != nil {
			rc.Before = make(map[string]interface{})
			json.Unmarshal(msg.Change.Before, &rc.Before)
		}
		if msg.Change.After != nil {
			rc.After = make(map[string]interface{})
			json.Unmarshal(msg.Change.After, &rc.After)
		}

		changes = append(changes, rc)
	}

	return changes, nil
}

type planMessage struct {
	Type   string      `json:"type"`
	Change *planChange `json:"change,omitempty"`
}

type planChange struct {
	Resource planResource    `json:"resource"`
	Action   json.RawMessage `json:"action"`
	Before   json.RawMessage `json:"before,omitempty"`
	After    json.RawMessage `json:"after,omitempty"`
}

type planResource struct {
	Addr         string `json:"addr"`
	ResourceType string `json:"resource_type"`
	ResourceName string `json:"resource_name"`
}

func mapAction(raw json.RawMessage) Action {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return Action(s)
	}

	var actions []string
	if json.Unmarshal(raw, &actions) == nil {
		switch {
		case contains(actions, "create") && contains(actions, "delete"):
			return ActionReplace
		case contains(actions, "create"):
			return ActionCreate
		case contains(actions, "delete"):
			return ActionDelete
		case contains(actions, "update"):
			return ActionUpdate
		}
	}

	return ActionNoOp
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
