package aws

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type AuthType string

const (
	AuthTypeSSO     AuthType = "SSO"
	AuthTypeStatic  AuthType = "STATIC"
	AuthTypeUnknown AuthType = "UNKNOWN"
)

type SessionStatus string

const (
	StatusLive      SessionStatus = "LIVE"
	StatusExpired   SessionStatus = "EXPIRED"
	StatusNoSession SessionStatus = "NO_SESSION"
	StatusDenied    SessionStatus = "DENIED"
	StatusError     SessionStatus = "ERROR"
	StatusUnknown   SessionStatus = "UNKNOWN"
)

type ProfileInfo struct {
	Name       string
	Region     string
	AuthType   AuthType
	Status     SessionStatus
	AccountID  string
	SessionRef string
	Error      string
}

type ProfileCatalogProvider interface {
	DiscoverProfiles(exclude []string) ([]ProfileInfo, error)
}

type FileProfileCatalogProvider struct{}

func (p *FileProfileCatalogProvider) DiscoverProfiles(exclude []string) ([]ProfileInfo, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(home, ".aws", "config")
	credPath := filepath.Join(home, ".aws", "credentials")

	cfgProfiles, err := parseConfigProfiles(configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	credProfiles, err := parseCredentialProfiles(credPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	excluded := map[string]bool{}
	for _, e := range exclude {
		excluded[e] = true
	}

	merged := map[string]ProfileInfo{}
	for name, p := range cfgProfiles {
		if excluded[name] {
			continue
		}
		merged[name] = p
	}
	for name, p := range credProfiles {
		if excluded[name] {
			continue
		}
		if existing, ok := merged[name]; ok {
			if existing.Region == "" {
				existing.Region = p.Region
			}
			if existing.AuthType == AuthTypeUnknown {
				existing.AuthType = p.AuthType
			}
			merged[name] = existing
			continue
		}
		merged[name] = p
	}

	if len(merged) == 0 {
		merged["default"] = ProfileInfo{Name: "default", AuthType: AuthTypeUnknown, Status: StatusUnknown}
	}

	names := make([]string, 0, len(merged))
	for n := range merged {
		names = append(names, n)
	}
	sort.Strings(names)

	out := make([]ProfileInfo, 0, len(names))
	for _, n := range names {
		pp := merged[n]
		pp.Status = StatusUnknown
		out = append(out, pp)
	}
	return out, nil
}

func parseConfigProfiles(path string) (map[string]ProfileInfo, error) {
	out := map[string]ProfileInfo{}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var current *ProfileInfo
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if current != nil {
				out[current.Name] = *current
			}
			section := strings.Trim(strings.Trim(line, "[]"), " ")
			name := section
			if strings.HasPrefix(section, "profile ") {
				name = strings.TrimSpace(strings.TrimPrefix(section, "profile "))
			}
			current = &ProfileInfo{Name: name, AuthType: AuthTypeUnknown}
			continue
		}
		if current == nil {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		switch key {
		case "region":
			current.Region = value
		case "sso_start_url", "sso_session", "sso_account_id", "sso_role_name":
			current.AuthType = AuthTypeSSO
			if key == "sso_session" {
				current.SessionRef = value
			}
		}
	}
	if current != nil {
		out[current.Name] = *current
	}
	return out, scanner.Err()
}

func parseCredentialProfiles(path string) (map[string]ProfileInfo, error) {
	out := map[string]ProfileInfo{}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var current *ProfileInfo
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if current != nil {
				out[current.Name] = *current
			}
			section := strings.Trim(strings.Trim(line, "[]"), " ")
			current = &ProfileInfo{Name: section, AuthType: AuthTypeUnknown}
			continue
		}
		if current == nil {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		switch key {
		case "aws_access_key_id", "aws_secret_access_key":
			current.AuthType = AuthTypeStatic
		}
	}
	if current != nil {
		out[current.Name] = *current
	}
	return out, scanner.Err()
}
