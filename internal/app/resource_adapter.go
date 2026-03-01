package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	iaws "i9c/internal/aws"
	"i9c/internal/aws/resources"
)

type ResourceAdapter struct {
	registry      *resources.Registry
	clientManager *iaws.ClientManager

	cachedResources []resources.Resource
	cacheMu         sync.RWMutex
}

func NewResourceAdapter(registry *resources.Registry, clientManager *iaws.ClientManager) *ResourceAdapter {
	return &ResourceAdapter{
		registry:      registry,
		clientManager: clientManager,
	}
}

func (ra *ResourceAdapter) SetCachedResources(res []resources.Resource) {
	ra.cacheMu.Lock()
	defer ra.cacheMu.Unlock()
	ra.cachedResources = res
}

func (ra *ResourceAdapter) ListAllJSON(ctx context.Context, resourceType, profile string) (string, error) {
	ra.cacheMu.RLock()
	cached := ra.cachedResources
	ra.cacheMu.RUnlock()

	var matches []resources.Resource
	for _, r := range cached {
		if resourceType != "" && r.Type != resourceType {
			continue
		}
		if profile != "" && r.Profile != profile {
			continue
		}
		matches = append(matches, r)
	}

	if len(matches) > 0 {
		return marshalResources(matches)
	}

	allRes, err := ra.registry.ListAll(ctx, ra.clientManager)
	if err != nil {
		return "", fmt.Errorf("listing resources: %w", err)
	}

	for _, r := range allRes {
		if resourceType != "" && r.Type != resourceType {
			continue
		}
		if profile != "" && r.Profile != profile {
			continue
		}
		matches = append(matches, r)
	}

	if len(matches) == 0 {
		return fmt.Sprintf("No resources of type %q found.", resourceType), nil
	}

	return marshalResources(matches)
}

func (ra *ResourceAdapter) GetAccountContext(ctx context.Context, profile string) (string, error) {
	clients := ra.clientManager.AllClients()

	type contextResult struct {
		VPCs           []map[string]string `json:"vpcs"`
		Subnets        []map[string]string `json:"subnets"`
		SecurityGroups []map[string]string `json:"security_groups"`
		RouteTables    []map[string]string `json:"route_tables"`
	}

	result := contextResult{}

	var targetConfigs []struct {
		profile string
		cfg     aws.Config
	}

	if profile != "" {
		c, ok := clients[profile]
		if !ok {
			return "", fmt.Errorf("profile %q not found", profile)
		}
		targetConfigs = append(targetConfigs, struct {
			profile string
			cfg     aws.Config
		}{profile, c.Config})
	} else {
		for pname, c := range clients {
			targetConfigs = append(targetConfigs, struct {
				profile string
				cfg     aws.Config
			}{pname, c.Config})
			break // use first profile for default
		}
	}

	for _, tc := range targetConfigs {
		ec2Client := ec2.NewFromConfig(tc.cfg)

		vpcs, err := ec2Client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{})
		if err == nil {
			for _, v := range vpcs.Vpcs {
				vpc := map[string]string{
					"id":      aws.ToString(v.VpcId),
					"cidr":    aws.ToString(v.CidrBlock),
					"state":   string(v.State),
					"profile": tc.profile,
				}
				for _, tag := range v.Tags {
					if aws.ToString(tag.Key) == "Name" {
						vpc["name"] = aws.ToString(tag.Value)
					}
				}
				result.VPCs = append(result.VPCs, vpc)
			}
		}

		subnets, err := ec2Client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{})
		if err == nil {
			for _, s := range subnets.Subnets {
				subnet := map[string]string{
					"id":      aws.ToString(s.SubnetId),
					"vpc_id":  aws.ToString(s.VpcId),
					"cidr":    aws.ToString(s.CidrBlock),
					"az":      aws.ToString(s.AvailabilityZone),
					"profile": tc.profile,
				}
				for _, tag := range s.Tags {
					if aws.ToString(tag.Key) == "Name" {
						subnet["name"] = aws.ToString(tag.Value)
					}
				}
				result.Subnets = append(result.Subnets, subnet)
			}
		}

		sgs, err := ec2Client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{})
		if err == nil {
			for _, sg := range sgs.SecurityGroups {
				sgMap := map[string]string{
					"id":      aws.ToString(sg.GroupId),
					"name":    aws.ToString(sg.GroupName),
					"vpc_id":  aws.ToString(sg.VpcId),
					"profile": tc.profile,
				}
				result.SecurityGroups = append(result.SecurityGroups, sgMap)
			}
		}

		rts, err := ec2Client.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{})
		if err == nil {
			for _, rt := range rts.RouteTables {
				rtMap := map[string]string{
					"id":      aws.ToString(rt.RouteTableId),
					"vpc_id":  aws.ToString(rt.VpcId),
					"profile": tc.profile,
				}
				for _, tag := range rt.Tags {
					if aws.ToString(tag.Key) == "Name" {
						rtMap["name"] = aws.ToString(tag.Value)
					}
				}
				result.RouteTables = append(result.RouteTables, rtMap)
			}
		}
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func marshalResources(resources []resources.Resource) (string, error) {
	type entry struct {
		Profile    string            `json:"profile"`
		Service    string            `json:"service"`
		Type       string            `json:"type"`
		ID         string            `json:"id"`
		Name       string            `json:"name"`
		Region     string            `json:"region"`
		ARN        string            `json:"arn,omitempty"`
		Properties map[string]string `json:"properties,omitempty"`
	}

	var entries []entry
	for _, r := range resources {
		entries = append(entries, entry{
			Profile:    r.Profile,
			Service:    r.Service,
			Type:       r.Type,
			ID:         r.ID,
			Name:       r.Name,
			Region:     r.Region,
			ARN:        r.ARN,
			Properties: r.Properties,
		})
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
