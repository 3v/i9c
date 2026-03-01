package state

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type SessionState struct {
	LastProfile string `yaml:"last_profile"`
	LastIACDir  string `yaml:"last_iac_dir"`
}

type ProfileSelectionStore interface {
	Load() (*SessionState, error)
	Save(state *SessionState) error
}

type YAMLProfileSelectionStore struct {
	path string
}

func NewYAMLProfileSelectionStore(root string) *YAMLProfileSelectionStore {
	return &YAMLProfileSelectionStore{path: filepath.Join(root, "state", "session.yaml")}
}

func (s *YAMLProfileSelectionStore) Load() (*SessionState, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &SessionState{}, nil
		}
		return nil, err
	}
	var st SessionState
	if err := yaml.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *YAMLProfileSelectionStore) Save(state *SessionState) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}
