package resources

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
)

type CloudControlProvider struct {
	types []string
}

func NewCloudControlProvider(types []string) *CloudControlProvider {
	return &CloudControlProvider{types: types}
}

func (p *CloudControlProvider) ServiceName() string {
	return "cloudcontrol"
}

func (p *CloudControlProvider) ResourceTypes() []string {
	return p.types
}

func (p *CloudControlProvider) List(ctx context.Context, cfg aws.Config) ([]Resource, error) {
	client := cloudcontrol.NewFromConfig(cfg)
	var allResources []Resource

	for _, typeName := range p.types {
		resources, err := p.listType(ctx, client, cfg, typeName)
		if err != nil {
			continue
		}
		allResources = append(allResources, resources...)
	}

	return allResources, nil
}

func (p *CloudControlProvider) listType(ctx context.Context, client *cloudcontrol.Client, cfg aws.Config, typeName string) ([]Resource, error) {
	input := &cloudcontrol.ListResourcesInput{
		TypeName: aws.String(typeName),
	}

	var resources []Resource
	paginator := cloudcontrol.NewListResourcesPaginator(client, input)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, desc := range page.ResourceDescriptions {
			res := Resource{
				Service: extractService(typeName),
				Type:    typeName,
				Region:  cfg.Region,
			}

			if desc.Identifier != nil {
				res.ID = *desc.Identifier
			}

			if desc.Properties != nil {
				props := make(map[string]interface{})
				if err := json.Unmarshal([]byte(*desc.Properties), &props); err == nil {
					res.Properties = flattenProperties(props)
					if name, ok := props["Name"]; ok {
						res.Name = stringVal(name)
					}
					if arn, ok := props["Arn"]; ok {
						res.ARN = stringVal(arn)
					}
				}
			}

			resources = append(resources, res)
		}
	}

	return resources, nil
}

func extractService(typeName string) string {
	parts := strings.Split(typeName, "::")
	if len(parts) >= 2 {
		return parts[1]
	}
	return typeName
}

func flattenProperties(props map[string]interface{}) map[string]string {
	result := make(map[string]string)
	for k, v := range props {
		switch val := v.(type) {
		case string:
			result[k] = val
		default:
			b, _ := json.Marshal(val)
			result[k] = string(b)
		}
	}
	return result
}

func stringVal(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}
