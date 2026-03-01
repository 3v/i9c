package aws

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseConfigProfiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	content := `[default]
region = us-west-2
sso_session = corp

[profile dev]
region = us-east-1
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	profiles, err := parseConfigProfiles(path)
	if err != nil {
		t.Fatal(err)
	}
	if profiles["default"].AuthType != AuthTypeSSO {
		t.Fatalf("expected default auth type SSO, got %s", profiles["default"].AuthType)
	}
	if profiles["dev"].Region != "us-east-1" {
		t.Fatalf("expected dev region us-east-1, got %s", profiles["dev"].Region)
	}
}

func TestParseCredentialProfiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	content := `[default]
aws_access_key_id = AKIA...
aws_secret_access_key = SECRET
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	profiles, err := parseCredentialProfiles(path)
	if err != nil {
		t.Fatal(err)
	}
	if profiles["default"].AuthType != AuthTypeStatic {
		t.Fatalf("expected static auth type, got %s", profiles["default"].AuthType)
	}
}
