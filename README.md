# i9c - Infrastructure-as-Code Advisor

A k9s-style terminal UI for monitoring IaC drift, browsing AWS resources across multiple accounts, and generating Terraform/OpenTofu code with AI assistance.

## Features

- **Dashboard** - Overview of infrastructure state: drift count, resource counts, profile status, backend mode, cache health
- **Drift Detection** - Monitors your IaC directory and runs drift plan with the matching tool: `terraform plan` for `.tf` and `tofu plan` for `.tofu`
- **AI Advisor** - Chat-style interface powered by pluggable LLM backends (OpenAI, Claude, Bedrock, Ollama) for infrastructure advice and code generation
- **Codex Interactive Bridge** - Default Advisor path uses `codex app-server` with in-panel approval/question round-trips
- **Resource Browser** - Browse AWS resources across all profiles with filtering by service, profile, and text search
- **Multi-Profile + SSO UX** - Merges profiles from `~/.aws/config` and `~/.aws/credentials`, probes live session state, and provides a dedicated profile picker with in-app SSO login
- **Log Panels** - Segregated system/app/drift/agent logs, all rendered in-app (no stdout corruption)
- **Three-Tier Resource Registry** - Built-in rich providers for core services (EC2, VPC, EKS, S3, IAM, RDS), Cloud Control API for any other resource type, optional AWS Config inventory sweep
- **File Watcher** - Watches `.tf`/`.tofu` files for changes and automatically re-runs drift detection

## Installation

```bash
# Build locally (standalone, no GitHub dependency)
make build
./bin/i9c
```

Or build from source:

```bash
# if you are already in this project directory:
cd i9cdev
go build -o i9c ./cmd/i9c/
./i9c
```

## Prerequisites

- `aws` CLI (required for profile auth, SSO login, and profile/session checks)
- One IaC binary: `terraform` or `tofu`
- `tenv` (recommended fallback; i9c can use it to install missing `terraform`/`tofu`)
- `codex` CLI (optional; if available i9c defaults Advisor to Codex)
- `OPENAI_API_KEY` (used when Codex is unavailable and provider falls back to OpenAI)

Startup checks in i9c validate and log the availability of these dependencies in the `System` logs panel.
If both `.tf` and `.tofu` files exist, drift detection prioritizes OpenTofu (`tofu`).

## Configuration

Create `./.i9c/config.yaml`:

```yaml
iac_dir: ./infrastructure
provider: aws

aws:
  auth: profile                    # "profile" (default) or "key"
  auto_discover: true              # discover all profiles from ~/.aws/config
  exclude_profiles:
    - test-sandbox
  default_profile: default
  profile_probe_timeout_sec: 3
  auto_sso_login_prompt: true
  profile_default_strategy: live-first
  scan_all_profiles_on_startup: true
  region: us-east-1
  sso_browser_command: ""         # optional browser override (mac example: open -na "Google Chrome" --args --profile-directory="Profile 3")
  sso_browser_profile_dir: ""     # optional hint for chromium profile dir

resources:
  auto_discover: true              # use AWS Config to find all resource types
  extra_types:                     # additional CloudFormation types via Cloud Control API
    - AWS::SQS::Queue
    - AWS::SNS::Topic
    - AWS::Route53::HostedZone
    - AWS::ElasticLoadBalancingV2::LoadBalancer
    - AWS::Lambda::Function
  exclude_types:
    - AWS::CloudFormation::Stack

llm:
  provider: codex                  # codex (default) | openai | claude | bedrock | ollama
  model: gpt-5
  api_key_env: OPENAI_API_KEY
  base_url: ""                     # override for ollama (http://localhost:11434)

terraform:
  binary: terraform                # or "tofu" for OpenTofu
  auto_init: true
  drift_check_interval_min: 15
```

## Usage

```bash
# Start with defaults
i9c

# Point to a specific IaC directory
i9c --iac-dir ./terraform/production

# Use a single AWS profile
i9c --aws-profile production --aws-region us-west-2

# Use a custom config file
i9c --config ./my-config.yaml
```

## Keyboard Shortcuts

| Key          | Action                |
|--------------|-----------------------|
| `1-5`        | Switch panels         |
| `tab`        | Next panel            |
| `shift+tab`  | Previous panel        |
| `j/k`        | Navigate up/down      |
| `enter`      | Select / expand       |
| `esc`        | Back / close          |
| `/`          | Filter                |
| `p`          | Open profile picker   |
| `x`          | Cancel active login   |
| `approve/session/decline/cancel` | Reply to Codex approval prompts in Advisor |
| `s`          | Cycle service filter  |
| `i`          | Start typing (Advisor)|
| `?`          | Toggle help           |
| `q`          | Quit                  |

## AWS Authentication

**Profile mode** (default): i9c merges profiles from `~/.aws/config` and `~/.aws/credentials`, probes each profile with `sts:GetCallerIdentity`, and defaults to the first live profile by priority (`AWS_PROFILE`, then `default`, then alphabetical). For expired/no-session SSO profiles, it can run `aws sso login --profile <name>` inside the app.

**Key mode**: Set `aws.auth: key` in config. Reads credentials from environment variables specified in `access_key_env` and `secret_key_env`.

CLI flags `--aws-profile` and `--aws-region` override the config file.

## LLM Providers

| Provider | Config Value | Notes |
|----------|-------------|-------|
| Codex    | `codex`     | Uses local `codex exec` and existing `~/.codex` auth/session |
| OpenAI   | `openai`    | Set `OPENAI_API_KEY` env var |
| Claude   | `claude`    | Set `ANTHROPIC_API_KEY` env var, update `api_key_env` |
| Bedrock  | `bedrock`   | Uses AWS credentials, set `base_url` to region |
| Ollama   | `ollama`    | Local, set `base_url` to `http://localhost:11434` |

## Architecture

```
i9c/
├── cmd/i9c/main.go              # CLI entry point
├── internal/
│   ├── app/                     # Application lifecycle and wiring
│   ├── config/                  # YAML configuration
│   ├── tui/                     # Terminal UI (bubbletea)
│   │   ├── views/               # Dashboard, Drift, Advisor, Resources
│   │   ├── components/          # Reusable TUI components
│   │   └── theme/               # lipgloss styles
│   ├── aws/                     # AWS SDK client management
│   │   └── resources/           # Three-tier resource registry
│   │       └── builtin/         # Rich providers (EC2, VPC, EKS, S3, IAM, RDS)
│   ├── terraform/               # HCL parsing, plan runner, drift model
│   ├── agent/                   # LLM agent orchestration
│   │   ├── providers/           # OpenAI, Claude, Bedrock, Ollama
│   │   └── prompts/             # System prompts for each agent type
│   └── watcher/                 # File system watcher
```

## License

MIT
