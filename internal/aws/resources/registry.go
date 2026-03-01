package resources

import (
	"context"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	iaws "i9c/internal/aws"
	"i9c/internal/config"
)

type Registry struct {
	cfg            *config.Config
	builtinProvs   map[string]ResourceProvider
	cloudControl   *CloudControlProvider
	configService  *ConfigServiceProvider
	excludeTypes   map[string]bool
}

func NewRegistry(cfg *config.Config) *Registry {
	excludeTypes := make(map[string]bool)
	for _, t := range cfg.Resources.ExcludeTypes {
		excludeTypes[t] = true
	}

	r := &Registry{
		cfg:          cfg,
		builtinProvs: make(map[string]ResourceProvider),
		cloudControl: NewCloudControlProvider(cfg.Resources.ExtraTypes),
		excludeTypes: excludeTypes,
	}

	if cfg.Resources.AutoDiscover {
		r.configService = NewConfigServiceProvider()
	}

	return r
}

func (r *Registry) RegisterBuiltin(p ResourceProvider) {
	for _, t := range p.ResourceTypes() {
		r.builtinProvs[t] = p
	}
}

func (r *Registry) ListAll(ctx context.Context, manager *iaws.ClientManager) ([]Resource, error) {
	clients := manager.AllClients()
	var allResources []Resource
	var mu sync.Mutex
	var wg sync.WaitGroup

	for profile, client := range clients {
		wg.Add(1)
		go func(profile string, cfg aws.Config) {
			defer wg.Done()

			resources := r.listForClient(ctx, profile, cfg)
			mu.Lock()
			allResources = append(allResources, resources...)
			mu.Unlock()
		}(profile, client.Config)
	}

	wg.Wait()
	return allResources, nil
}

func (r *Registry) listForClient(ctx context.Context, profile string, cfg aws.Config) []Resource {
	var resources []Resource
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, provider := range r.builtinProvs {
		wg.Add(1)
		go func(p ResourceProvider) {
			defer wg.Done()
			items, err := p.List(ctx, cfg)
			if err != nil {
				return
			}
			mu.Lock()
			for i := range items {
				items[i].Profile = profile
			}
			resources = append(resources, items...)
			mu.Unlock()
		}(provider)
	}

	if r.cloudControl != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			items, err := r.cloudControl.List(ctx, cfg)
			if err != nil {
				return
			}
			mu.Lock()
			for i := range items {
				items[i].Profile = profile
			}
			resources = append(resources, items...)
			mu.Unlock()
		}()
	}

	wg.Wait()

	var filtered []Resource
	for _, res := range resources {
		if !r.excludeTypes[res.Type] {
			filtered = append(filtered, res)
		}
	}
	return filtered
}
