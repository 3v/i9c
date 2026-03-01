package mcp

import "context"

type DiscoveryBackend interface {
	Name() string
	Discover(ctx context.Context, query string) (string, error)
}

type Manager struct {
	Primary  DiscoveryBackend
	Fallback DiscoveryBackend
}

func (m *Manager) Discover(ctx context.Context, query string) (backendName, payload string, err error) {
	if m.Primary != nil {
		payload, err = m.Primary.Discover(ctx, query)
		if err == nil {
			return m.Primary.Name(), payload, nil
		}
	}
	if m.Fallback != nil {
		payload, err = m.Fallback.Discover(ctx, query)
		if err == nil {
			return m.Fallback.Name(), payload, nil
		}
	}
	return "", "", err
}
