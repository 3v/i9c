package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	IACDir    string          `yaml:"iac_dir"`
	Provider  string          `yaml:"provider"`
	Paths     PathsConfig     `yaml:"paths"`
	AWS       AWSConfig       `yaml:"aws"`
	Resources ResourcesConfig `yaml:"resources"`
	LLM       LLMConfig       `yaml:"llm"`
	Terraform TerraformConfig `yaml:"terraform"`
}

type PathsConfig struct {
	Root string `yaml:"root"`
}

type AWSConfig struct {
	Auth                     string   `yaml:"auth"`
	AutoDiscover             bool     `yaml:"auto_discover"`
	ExcludeProfiles          []string `yaml:"exclude_profiles"`
	DefaultProfile           string   `yaml:"default_profile"`
	ProfileProbeTimeoutSec   int      `yaml:"profile_probe_timeout_sec"`
	AutoSSOLoginPrompt       bool     `yaml:"auto_sso_login_prompt"`
	ProfileDefaultStrategy   string   `yaml:"profile_default_strategy"`
	ScanAllProfilesOnStartup bool     `yaml:"scan_all_profiles_on_startup"`
	AccessKeyEnv             string   `yaml:"access_key_env"`
	SecretKeyEnv             string   `yaml:"secret_key_env"`
	SessionTokenEnv          string   `yaml:"session_token_env"`
	Region                   string   `yaml:"region"`
	SSOBrowserCommand        string   `yaml:"sso_browser_command"`
	SSOBrowserProfileDir     string   `yaml:"sso_browser_profile_dir"`
}

type ResourcesConfig struct {
	AutoDiscover bool     `yaml:"auto_discover"`
	ExtraTypes   []string `yaml:"extra_types"`
	ExcludeTypes []string `yaml:"exclude_types"`
}

type LLMConfig struct {
	Provider  string `yaml:"provider"`
	Model     string `yaml:"model"`
	APIKey    string `yaml:"api_key,omitempty"`
	APIKeyEnv string `yaml:"api_key_env,omitempty"`
	BaseURL   string `yaml:"base_url,omitempty"`
}

func (c *LLMConfig) ResolveAPIKey() string {
	if c.APIKey != "" {
		return c.APIKey
	}
	if c.APIKeyEnv != "" {
		return os.Getenv(c.APIKeyEnv)
	}
	return ""
}

type TerraformConfig struct {
	Binary                string `yaml:"binary"`
	Version               string `yaml:"version"`
	AutoInit              bool   `yaml:"auto_init"`
	DriftCheckIntervalMin int    `yaml:"drift_check_interval_min"`
}

func DefaultConfig() *Config {
	return &Config{
		IACDir:   "",
		Provider: "aws",
		Paths: PathsConfig{
			Root: ".i9c",
		},
		AWS: AWSConfig{
			Auth:                     "profile",
			AutoDiscover:             true,
			DefaultProfile:           "default",
			ProfileProbeTimeoutSec:   3,
			AutoSSOLoginPrompt:       true,
			ProfileDefaultStrategy:   "live-first",
			ScanAllProfilesOnStartup: true,
			AccessKeyEnv:             "AWS_ACCESS_KEY_ID",
			SecretKeyEnv:             "AWS_SECRET_ACCESS_KEY",
			SessionTokenEnv:          "AWS_SESSION_TOKEN",
			Region:                   "us-east-1",
		},
		Resources: ResourcesConfig{
			AutoDiscover: true,
		},
		LLM: LLMConfig{
			Provider:  "codex",
			Model:     "gpt-5",
			APIKeyEnv: "OPENAI_API_KEY",
		},
		Terraform: TerraformConfig{
			Binary:                "tofu",
			Version:               "latest",
			AutoInit:              true,
			DriftCheckIntervalMin: 15,
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path == "" {
		path = filepath.Join(cfg.Paths.Root, "config.yaml")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

func (c *Config) Save(path string) error {
	if path == "" {
		path = filepath.Join(c.Paths.Root, "config.yaml")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}

func (c *Config) Validate() error {
	if c.AWS.Auth != "profile" && c.AWS.Auth != "key" {
		return fmt.Errorf("aws.auth must be 'profile' or 'key', got %q", c.AWS.Auth)
	}
	if c.AWS.ProfileProbeTimeoutSec < 1 {
		return fmt.Errorf("aws.profile_probe_timeout_sec must be >= 1")
	}
	if c.AWS.ProfileDefaultStrategy != "live-first" {
		return fmt.Errorf("aws.profile_default_strategy must be 'live-first'")
	}
	if c.LLM.Provider != "" {
		validProviders := map[string]bool{"codex": true, "openai": true, "claude": true, "bedrock": true, "ollama": true}
		if !validProviders[c.LLM.Provider] {
			return fmt.Errorf("llm.provider must be one of codex, openai, claude, bedrock, ollama; got %q", c.LLM.Provider)
		}
	}
	if c.Terraform.Binary != "terraform" && c.Terraform.Binary != "tofu" {
		return fmt.Errorf("terraform.binary must be 'terraform' or 'tofu', got %q", c.Terraform.Binary)
	}
	if c.Terraform.DriftCheckIntervalMin < 1 {
		return fmt.Errorf("terraform.drift_check_interval_min must be >= 1")
	}
	return nil
}

func (c *Config) EnsureLocalDirs() error {
	root := c.Paths.Root
	if root == "" {
		root = ".i9c"
		c.Paths.Root = root
	}
	for _, dir := range []string{
		root,
		filepath.Join(root, "state"),
		filepath.Join(root, "logs"),
		filepath.Join(root, "cache"),
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}
