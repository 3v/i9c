package resources

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/configservice"
	"github.com/aws/aws-sdk-go-v2/service/configservice/types"
)

type ConfigServiceProvider struct{}

func NewConfigServiceProvider() *ConfigServiceProvider {
	return &ConfigServiceProvider{}
}

func (p *ConfigServiceProvider) DiscoverResourceTypes(ctx context.Context, cfg aws.Config) ([]string, error) {
	client := configservice.NewFromConfig(cfg)

	input := &configservice.GetDiscoveredResourceCountsInput{
		Limit: 100,
	}

	resp, err := client.GetDiscoveredResourceCounts(ctx, input)
	if err != nil {
		return nil, err
	}

	var resourceTypes []string
	for _, count := range resp.ResourceCounts {
		if count.ResourceType != "" {
			resourceTypes = append(resourceTypes, string(count.ResourceType))
		}
	}

	return resourceTypes, nil
}

func (p *ConfigServiceProvider) ListResources(ctx context.Context, cfg aws.Config, resourceType string) ([]Resource, error) {
	client := configservice.NewFromConfig(cfg)

	input := &configservice.ListDiscoveredResourcesInput{
		ResourceType: types.ResourceType(resourceType),
		Limit:        100,
	}

	resp, err := client.ListDiscoveredResources(ctx, input)
	if err != nil {
		return nil, err
	}

	var resources []Resource
	for _, item := range resp.ResourceIdentifiers {
		res := Resource{
			Service: extractServiceFromConfigType(resourceType),
			Type:    resourceType,
			Region:  cfg.Region,
		}
		if item.ResourceId != nil {
			res.ID = *item.ResourceId
		}
		if item.ResourceName != nil {
			res.Name = *item.ResourceName
		}
		resources = append(resources, res)
	}

	return resources, nil
}

func cfTypeToCloudControlType(configType string) string {
	return configType
}

func extractServiceFromConfigType(resourceType string) string {
	return extractService(resourceType)
}
