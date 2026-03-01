package iac

import (
	"fmt"
	"os/exec"
	"strings"
)

func EnsureBinary(binary, version string) (string, error) {
	path, err := exec.LookPath(binary)
	if err == nil {
		return path, nil
	}

	tenvPath, tenvErr := exec.LookPath("tenv")
	if tenvErr != nil {
		return "", fmt.Errorf("%s not found in PATH and tenv is not installed. "+
			"Install %s directly or install tenv (https://github.com/tofuutils/tenv) to manage versions", binary, binary)
	}

	tenvBinary := tenvBinaryName(binary)
	installArgs := []string{tenvBinary, "install"}
	if version != "" && version != "latest" {
		installArgs = append(installArgs, version)
	}

	cmd := exec.Command(tenvPath, installArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tenv install %s failed: %s: %w", binary, string(output), err)
	}

	path, err = exec.LookPath(binary)
	if err != nil {
		return "", fmt.Errorf("%s still not found after tenv install. Ensure tenv shims are in PATH.", binary)
	}

	return path, nil
}

func InstalledVersion(binary string) (string, error) {
	path, err := exec.LookPath(binary)
	if err != nil {
		return "", fmt.Errorf("%s not found: %w", binary, err)
	}

	cmd := exec.Command(path, "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		cmd = exec.Command(path, "--version")
		output, err = cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("getting version: %w", err)
		}
	}

	return strings.TrimSpace(string(output)), nil
}

func tenvBinaryName(binary string) string {
	switch binary {
	case "terraform":
		return "tf"
	case "tofu":
		return "tofu"
	default:
		return binary
	}
}
