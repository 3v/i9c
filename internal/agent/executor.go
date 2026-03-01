package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ResourceLister interface {
	ListAllJSON(ctx context.Context, resourceType, profile string) (string, error)
	GetAccountContext(ctx context.Context, profile string) (string, error)
}

type ToolExecutor struct {
	ResourceLister ResourceLister
	IACDir         string
	IACBinary      string
	DriftData      func() string
}

func (e *ToolExecutor) Execute(ctx context.Context, name, arguments string) (string, error) {
	var args map[string]interface{}
	if arguments != "" {
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("parsing tool arguments: %w", err)
		}
	}

	switch name {
	case "list_resources":
		return e.listResources(ctx, args)
	case "describe_resource":
		return e.describeResource(ctx, args)
	case "get_account_context":
		return e.getAccountContext(ctx, args)
	case "get_drift":
		return e.getDrift()
	case "read_hcl_file":
		return e.readHCLFile(args)
	case "list_hcl_resources":
		return e.listHCLResources()
	case "generate_hcl":
		return e.generateHCL(args)
	case "is_folder_empty":
		return e.isFolderEmpty()
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

func (e *ToolExecutor) listResources(ctx context.Context, args map[string]interface{}) (string, error) {
	resourceType, _ := args["resource_type"].(string)
	profile, _ := args["profile"].(string)
	if resourceType == "" {
		return "", fmt.Errorf("resource_type is required")
	}
	return e.ResourceLister.ListAllJSON(ctx, resourceType, profile)
}

func (e *ToolExecutor) describeResource(ctx context.Context, args map[string]interface{}) (string, error) {
	resourceType, _ := args["resource_type"].(string)
	profile, _ := args["profile"].(string)
	result, err := e.ResourceLister.ListAllJSON(ctx, resourceType, profile)
	if err != nil {
		return "", err
	}
	resourceID, _ := args["resource_id"].(string)
	if resourceID != "" {
		return fmt.Sprintf("Resources matching %q:\n%s", resourceID, result), nil
	}
	return result, nil
}

func (e *ToolExecutor) getAccountContext(ctx context.Context, args map[string]interface{}) (string, error) {
	profile, _ := args["profile"].(string)
	return e.ResourceLister.GetAccountContext(ctx, profile)
}

func (e *ToolExecutor) getDrift() (string, error) {
	if e.DriftData != nil {
		data := e.DriftData()
		if data != "" {
			return data, nil
		}
	}
	return "No drift data available. The IaC folder may be empty or terraform plan has not been run.", nil
}

func (e *ToolExecutor) readHCLFile(args map[string]interface{}) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	fullPath := filepath.Join(e.IACDir, filepath.Clean(path))
	if !strings.HasPrefix(fullPath, filepath.Clean(e.IACDir)) {
		return "", fmt.Errorf("path escapes IaC directory")
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("reading file: %w", err)
	}
	return string(data), nil
}

func (e *ToolExecutor) listHCLResources() (string, error) {
	dir := filepath.Clean(e.IACDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "IaC directory does not exist or is not accessible.", nil
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".tf") || strings.HasSuffix(entry.Name(), ".tofu")) {
			files = append(files, entry.Name())
		}
	}

	if len(files) == 0 {
		return "No .tf or .tofu files found in the IaC directory.", nil
	}

	var result strings.Builder
	fmt.Fprintf(&result, "Found %d IaC files:\n", len(files))
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			continue
		}
		fmt.Fprintf(&result, "\n### %s\n", f)
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "resource ") || strings.HasPrefix(trimmed, "data ") ||
				strings.HasPrefix(trimmed, "module ") || strings.HasPrefix(trimmed, "variable ") ||
				strings.HasPrefix(trimmed, "output ") || strings.HasPrefix(trimmed, "locals ") {
				fmt.Fprintf(&result, "  %s\n", trimmed)
			}
		}
	}

	return result.String(), nil
}

func (e *ToolExecutor) generateHCL(args map[string]interface{}) (string, error) {
	filename, _ := args["filename"].(string)
	content, _ := args["content"].(string)
	if filename == "" || content == "" {
		return "", fmt.Errorf("filename and content are required")
	}

	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return "", fmt.Errorf("filename must not contain path separators")
	}

	if e.IACBinary == "tofu" && strings.HasSuffix(filename, ".tf") {
		filename = strings.TrimSuffix(filename, ".tf") + ".tofu"
	} else if e.IACBinary == "terraform" && strings.HasSuffix(filename, ".tofu") {
		filename = strings.TrimSuffix(filename, ".tofu") + ".tf"
	}

	dir := filepath.Clean(e.IACDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("creating IaC directory: %w", err)
	}

	fullPath := filepath.Join(dir, filename)
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("writing file: %w", err)
	}

	return fmt.Sprintf("Successfully wrote %s (%d bytes)", fullPath, len(content)), nil
}

func (e *ToolExecutor) isFolderEmpty() (string, error) {
	dir := filepath.Clean(e.IACDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return `{"empty": true, "reason": "directory does not exist or is not accessible"}`, nil
	}

	for _, entry := range entries {
		if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".tf") || strings.HasSuffix(entry.Name(), ".tofu")) {
			return `{"empty": false}`, nil
		}
	}

	return `{"empty": true, "reason": "no .tf or .tofu files found"}`, nil
}
