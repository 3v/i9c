package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"i9c/internal/app"
	"i9c/internal/config"
)

var version = "dev"

var (
	cfgFile    string
	awsProfile string
	awsRegion  string
	iacDir     string
)

func main() {
	defaultCfgPath := filepath.Join(defaultRoot(), "config.yaml")
	rootCmd := &cobra.Command{
		Use:   "i9c",
		Short: "Infrastructure-as-Code Advisor",
		Long:  "i9c is a terminal UI for monitoring IaC drift, browsing AWS resources, and generating Terraform/OpenTofu code with AI assistance.",
		RunE:  run,
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", fmt.Sprintf("config file (default %s)", defaultCfgPath))
	rootCmd.PersistentFlags().StringVar(&awsProfile, "aws-profile", "", "narrow to a single AWS profile")
	rootCmd.PersistentFlags().StringVar(&awsRegion, "aws-region", "", "override AWS region for all profiles")
	rootCmd.PersistentFlags().StringVar(&iacDir, "iac-dir", "", "path to IaC directory to monitor")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func defaultRoot() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".i9c"
	}
	return filepath.Join(home, ".i9c")
}

func run(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if awsProfile != "" {
		cfg.AWS.AutoDiscover = false
		cfg.AWS.DefaultProfile = awsProfile
	}
	if awsRegion != "" {
		cfg.AWS.Region = awsRegion
	}
	if iacDir != "" {
		cfg.IACDir = iacDir
	}

	return app.Run(cfg)
}
