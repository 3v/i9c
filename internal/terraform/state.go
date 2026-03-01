package terraform

import (
	"encoding/json"
	"fmt"
	"os"
)

type StateFile struct {
	Version          int             `json:"version"`
	TerraformVersion string          `json:"terraform_version"`
	Serial           int             `json:"serial"`
	Lineage          string          `json:"lineage"`
	Resources        []StateResource `json:"resources"`
}

type StateResource struct {
	Module    string          `json:"module,omitempty"`
	Mode      string          `json:"mode"`
	Type      string          `json:"type"`
	Name      string          `json:"name"`
	Provider  string          `json:"provider"`
	Instances []StateInstance `json:"instances"`
}

type StateInstance struct {
	SchemaVersion int                    `json:"schema_version"`
	Attributes    map[string]interface{} `json:"attributes"`
	Dependencies  []string               `json:"dependencies"`
}

func ReadStateFile(path string) (*StateFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading state file: %w", err)
	}

	var state StateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}

	return &state, nil
}

func (s *StateFile) ResourceAddresses() []string {
	var addrs []string
	for _, r := range s.Resources {
		prefix := ""
		if r.Module != "" {
			prefix = r.Module + "."
		}
		addr := fmt.Sprintf("%s%s.%s", prefix, r.Type, r.Name)
		addrs = append(addrs, addr)
	}
	return addrs
}
