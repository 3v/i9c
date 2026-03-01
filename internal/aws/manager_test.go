package aws

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"i9c/internal/config"
)

type mockCatalog struct {
	profiles []ProfileInfo
	err      error
}

func (m mockCatalog) DiscoverProfiles(_ []string) ([]ProfileInfo, error) {
	return m.profiles, m.err
}

type mockProbe struct {
	results map[string]ProfileInfo
	errs    map[string]error
}

func (m mockProbe) Probe(_ context.Context, profile, region string, _ time.Duration) (ProfileInfo, error) {
	if r, ok := m.results[profile]; ok {
		if r.Region == "" {
			r.Region = region
		}
		return r, m.errs[profile]
	}
	return ProfileInfo{Name: profile, Region: region, Status: StatusError}, errors.New("missing probe")
}

type mockSSO struct {
	lastProfile string
	err         error
}

func (m *mockSSO) Login(_ context.Context, profile string, _ func(line string)) error {
	m.lastProfile = profile
	return m.err
}

func TestInitializePicksEnvProfileWhenLive(t *testing.T) {
	t.Setenv("AWS_PROFILE", "dev")
	cfg := config.DefaultConfig()
	cfg.AWS.Region = "us-east-1"
	m := NewClientManager(cfg)
	m.catalog = mockCatalog{profiles: []ProfileInfo{
		{Name: "default", Region: "us-east-1"},
		{Name: "dev", Region: "us-east-1"},
		{Name: "zzz", Region: "us-east-1"},
	}}
	m.probe = mockProbe{results: map[string]ProfileInfo{
		"default": {Name: "default", Status: StatusLive, Region: "us-east-1"},
		"dev":     {Name: "dev", Status: StatusLive, Region: "us-east-1"},
		"zzz":     {Name: "zzz", Status: StatusLive, Region: "us-east-1"},
	}}
	m.newFromProfile = func(_ context.Context, _, _ string) (*Client, error) { return nil, nil }

	if err := m.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := m.ActiveProfile(); got != "dev" {
		t.Fatalf("expected active profile dev, got %s", got)
	}
}

func TestInitializeFallsBackToDefaultThenAlpha(t *testing.T) {
	_ = os.Unsetenv("AWS_PROFILE")
	cfg := config.DefaultConfig()
	m := NewClientManager(cfg)
	m.catalog = mockCatalog{profiles: []ProfileInfo{
		{Name: "beta", Region: "us-east-1"},
		{Name: "alpha", Region: "us-east-1"},
	}}
	m.probe = mockProbe{results: map[string]ProfileInfo{
		"beta":  {Name: "beta", Status: StatusLive},
		"alpha": {Name: "alpha", Status: StatusLive},
	}}
	m.newFromProfile = func(_ context.Context, _, _ string) (*Client, error) { return nil, nil }

	if err := m.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := m.ActiveProfile(); got != "alpha" {
		t.Fatalf("expected alphabetical live profile alpha, got %s", got)
	}
}

func TestLoginProfileTransitionsToLive(t *testing.T) {
	cfg := config.DefaultConfig()
	m := NewClientManager(cfg)
	m.profiles["dev"] = ProfileInfo{Name: "dev", Region: "us-east-1", Status: StatusExpired}
	sso := &mockSSO{}
	m.sso = sso
	m.probe = mockProbe{results: map[string]ProfileInfo{
		"dev": {Name: "dev", Region: "us-east-1", Status: StatusLive, AccountID: "123456789012"},
	}}
	m.newFromProfile = func(_ context.Context, _, _ string) (*Client, error) { return nil, nil }

	if err := m.LoginProfile(context.Background(), "dev", nil); err != nil {
		t.Fatal(err)
	}
	if sso.lastProfile != "dev" {
		t.Fatalf("expected login called for dev, got %s", sso.lastProfile)
	}
	if got := m.profiles["dev"].Status; got != StatusLive {
		t.Fatalf("expected live status, got %s", got)
	}
}

func TestInitializeScanTargetsWhenScanAllDisabled(t *testing.T) {
	t.Setenv("AWS_PROFILE", "dev")
	cfg := config.DefaultConfig()
	cfg.AWS.ScanAllProfilesOnStartup = false
	cfg.AWS.DefaultProfile = "ops"

	var probed []string
	m := NewClientManager(cfg)
	m.catalog = mockCatalog{profiles: []ProfileInfo{
		{Name: "dev", Region: "us-east-1"},
		{Name: "ops", Region: "us-east-1"},
		{Name: "other", Region: "us-east-1"},
	}}
	m.probe = mockProbeWithCallback{
		cb: func(name string) { probed = append(probed, name) },
		results: map[string]ProfileInfo{
			"dev": {Name: "dev", Status: StatusLive},
			"ops": {Name: "ops", Status: StatusLive},
		},
	}
	m.newFromProfile = func(_ context.Context, _, _ string) (*Client, error) { return nil, nil }

	if err := m.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, p := range probed {
		got[p] = true
	}
	if !got["dev"] || !got["ops"] {
		t.Fatalf("expected dev and ops probed, got %#v", probed)
	}
	if got["other"] {
		t.Fatalf("expected other not to be probed when scan_all_profiles_on_startup=false")
	}
}

type mockProbeWithCallback struct {
	cb      func(string)
	results map[string]ProfileInfo
}

func (m mockProbeWithCallback) Probe(_ context.Context, profile, region string, _ time.Duration) (ProfileInfo, error) {
	if m.cb != nil {
		m.cb(profile)
	}
	if r, ok := m.results[profile]; ok {
		if r.Region == "" {
			r.Region = region
		}
		return r, nil
	}
	return ProfileInfo{Name: profile, Region: region, Status: StatusError}, errors.New("missing probe")
}
