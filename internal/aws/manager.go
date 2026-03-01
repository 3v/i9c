package aws

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"i9c/internal/config"
)

type ClientManager struct {
	cfg            *config.Config
	catalog        ProfileCatalogProvider
	probe          SessionProbe
	sso            SSOLoginRunner
	newFromProfile func(context.Context, string, string) (*Client, error)
	newFromKeys    func(context.Context, *config.AWSConfig) (*Client, error)

	clients       map[string]*Client
	profiles      map[string]ProfileInfo
	activeProfile string

	mu sync.RWMutex
}

func NewClientManager(cfg *config.Config) *ClientManager {
	return &ClientManager{
		cfg:     cfg,
		catalog: &FileProfileCatalogProvider{},
		probe:   &STSSessionProbe{},
		sso: &ConfiguredSSOLoginRunner{
			BrowserCommand:    cfg.AWS.SSOBrowserCommand,
			BrowserProfileDir: cfg.AWS.SSOBrowserProfileDir,
		},
		newFromProfile: NewClientFromProfile,
		newFromKeys:    NewClientFromKeys,
		clients:        make(map[string]*Client),
		profiles:       make(map[string]ProfileInfo),
	}
}

func (m *ClientManager) SetCatalogProvider(p ProfileCatalogProvider) { m.catalog = p }
func (m *ClientManager) SetSessionProbe(p SessionProbe)              { m.probe = p }
func (m *ClientManager) SetSSOLoginRunner(r SSOLoginRunner)          { m.sso = r }

func (m *ClientManager) Initialize(ctx context.Context) error {
	if m.cfg.AWS.Auth == "key" {
		client, err := m.newFromKeys(ctx, &m.cfg.AWS)
		if err != nil {
			return fmt.Errorf("creating AWS client from keys: %w", err)
		}
		m.mu.Lock()
		m.clients["keys"] = client
		m.activeProfile = "keys"
		m.profiles["keys"] = ProfileInfo{Name: "keys", Status: StatusLive, AuthType: AuthTypeStatic}
		m.mu.Unlock()
		return nil
	}

	profiles, err := m.catalog.DiscoverProfiles(m.cfg.AWS.ExcludeProfiles)
	if err != nil {
		return fmt.Errorf("discovering AWS profiles: %w", err)
	}

	timeout := time.Duration(m.cfg.AWS.ProfileProbeTimeoutSec) * time.Second
	probeTargets := make(map[string]bool, len(profiles))
	if m.cfg.AWS.ScanAllProfilesOnStartup {
		for _, p := range profiles {
			probeTargets[p.Name] = true
		}
	} else {
		if envProfile := os.Getenv("AWS_PROFILE"); envProfile != "" {
			probeTargets[envProfile] = true
		}
		if m.cfg.AWS.DefaultProfile != "" {
			probeTargets[m.cfg.AWS.DefaultProfile] = true
		}
		probeTargets["default"] = true
	}

	for _, p := range profiles {
		if !probeTargets[p.Name] {
			m.mu.Lock()
			m.profiles[p.Name] = p
			m.mu.Unlock()
			continue
		}
		region := p.Region
		if region == "" {
			region = m.cfg.AWS.Region
		}
		live, probeErr := m.probe.Probe(ctx, p.Name, region, timeout)
		p.Status = live.Status
		p.AccountID = live.AccountID
		p.Region = live.Region
		if probeErr != nil {
			p.Error = probeErr.Error()
		}

		m.mu.Lock()
		m.profiles[p.Name] = p
		m.mu.Unlock()
		if p.Status != StatusLive {
			continue
		}
		client, err := m.newFromProfile(ctx, p.Name, p.Region)
		if err != nil {
			continue
		}
		m.mu.Lock()
		m.clients[p.Name] = client
		m.mu.Unlock()
	}

	m.mu.Lock()
	m.activeProfile = m.pickDefaultProfileLocked()
	m.mu.Unlock()

	return nil
}

func (m *ClientManager) pickDefaultProfileLocked() string {
	candidates := []string{}
	for name, p := range m.profiles {
		if p.Status == StatusLive {
			candidates = append(candidates, name)
		}
	}
	sort.Strings(candidates)
	if len(candidates) == 0 {
		return ""
	}
	envProfile := os.Getenv("AWS_PROFILE")
	if envProfile != "" {
		if p, ok := m.profiles[envProfile]; ok && p.Status == StatusLive {
			return envProfile
		}
	}
	if p, ok := m.profiles["default"]; ok && p.Status == StatusLive {
		return "default"
	}
	return candidates[0]
}

func (m *ClientManager) ActiveProfile() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeProfile
}

func (m *ClientManager) Profiles() []ProfileInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.profiles))
	for n := range m.profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]ProfileInfo, 0, len(names))
	for _, n := range names {
		p := m.profiles[n]
		out = append(out, p)
	}
	return out
}

func (m *ClientManager) SelectProfile(profile string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.profiles[profile]
	if !ok {
		return fmt.Errorf("profile %q not found", profile)
	}
	if p.Status != StatusLive {
		return fmt.Errorf("profile %q is not live (status=%s)", profile, p.Status)
	}
	m.activeProfile = profile
	return nil
}

func (m *ClientManager) LoginProfile(ctx context.Context, profile string, sink func(line string)) error {
	if m.sso == nil {
		return fmt.Errorf("no SSO login runner configured")
	}
	if err := m.sso.Login(ctx, profile, sink); err != nil {
		return err
	}

	region := m.cfg.AWS.Region
	if p, ok := m.profiles[profile]; ok && p.Region != "" {
		region = p.Region
	}
	timeout := time.Duration(m.cfg.AWS.ProfileProbeTimeoutSec) * time.Second
	live, probeErr := m.probe.Probe(ctx, profile, region, timeout)
	m.mu.Lock()
	p := m.profiles[profile]
	p.Status = live.Status
	p.AccountID = live.AccountID
	p.Region = live.Region
	if probeErr != nil {
		p.Error = probeErr.Error()
	} else {
		p.Error = ""
	}
	m.profiles[profile] = p
	m.mu.Unlock()

	if probeErr != nil || live.Status != StatusLive {
		return fmt.Errorf("profile %q still not live after login", profile)
	}

	client, err := m.newFromProfile(ctx, profile, p.Region)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.clients[profile] = client
	if m.activeProfile == "" {
		m.activeProfile = profile
	}
	m.mu.Unlock()
	return nil
}

func (m *ClientManager) GetClient(profile string) (*Client, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.clients[profile]
	return c, ok
}

func (m *ClientManager) AllClients() map[string]*Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]*Client, len(m.clients))
	for k, v := range m.clients {
		result[k] = v
	}
	return result
}

func (m *ClientManager) ProfileNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.clients))
	for k := range m.clients {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
