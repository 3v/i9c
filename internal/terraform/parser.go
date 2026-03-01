package terraform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

type HCLFile struct {
	Path      string
	Resources []HCLResource
	Variables []HCLVariable
	Outputs   []HCLOutput
}

type HCLResource struct {
	Type   string
	Name   string
	Labels []string
}

type HCLVariable struct {
	Name string
}

type HCLOutput struct {
	Name string
}

func ParseHCLDir(dir string) ([]HCLFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	var files []HCLFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".tf" && ext != ".tofu" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		hf, err := ParseHCLFile(path)
		if err != nil {
			continue
		}
		files = append(files, *hf)
	}

	return files, nil
}

func ParseHCLFile(path string) (*HCLFile, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	file, diags := hclsyntax.ParseConfig(src, path, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("parsing %s: %s", path, diags.Error())
	}

	hf := &HCLFile{Path: path}

	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return hf, nil
	}

	for _, block := range body.Blocks {
		switch block.Type {
		case "resource":
			if len(block.Labels) >= 2 {
				hf.Resources = append(hf.Resources, HCLResource{
					Type:   block.Labels[0],
					Name:   block.Labels[1],
					Labels: block.Labels,
				})
			}
		case "variable":
			if len(block.Labels) >= 1 {
				hf.Variables = append(hf.Variables, HCLVariable{Name: block.Labels[0]})
			}
		case "output":
			if len(block.Labels) >= 1 {
				hf.Outputs = append(hf.Outputs, HCLOutput{Name: block.Labels[0]})
			}
		}
	}

	return hf, nil
}
