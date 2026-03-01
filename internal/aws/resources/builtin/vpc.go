package builtin

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"i9c/internal/aws/resources"
)

type VPCProvider struct{}

func NewVPCProvider() *VPCProvider { return &VPCProvider{} }

func (p *VPCProvider) ServiceName() string { return "VPC" }

func (p *VPCProvider) ResourceTypes() []string {
	return []string{"AWS::EC2::VPC", "AWS::EC2::Subnet", "AWS::EC2::SecurityGroup"}
}

func (p *VPCProvider) List(ctx context.Context, cfg aws.Config) ([]resources.Resource, error) {
	client := ec2.NewFromConfig(cfg)
	var result []resources.Resource

	vpcs, err := p.listVPCs(ctx, client, cfg)
	if err == nil {
		result = append(result, vpcs...)
	}

	subnets, err := p.listSubnets(ctx, client, cfg)
	if err == nil {
		result = append(result, subnets...)
	}

	sgs, err := p.listSecurityGroups(ctx, client, cfg)
	if err == nil {
		result = append(result, sgs...)
	}

	return result, nil
}

func (p *VPCProvider) listVPCs(ctx context.Context, client *ec2.Client, cfg aws.Config) ([]resources.Resource, error) {
	out, err := client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{})
	if err != nil {
		return nil, err
	}

	var result []resources.Resource
	for _, vpc := range out.Vpcs {
		name := tagName(vpc.Tags)
		result = append(result, resources.Resource{
			Service: "VPC",
			Type:    "AWS::EC2::VPC",
			ID:      aws.ToString(vpc.VpcId),
			Name:    name,
			Region:  cfg.Region,
			ARN:     fmt.Sprintf("arn:aws:ec2:%s::vpc/%s", cfg.Region, aws.ToString(vpc.VpcId)),
			Properties: map[string]string{
				"CIDR":      aws.ToString(vpc.CidrBlock),
				"State":     string(vpc.State),
				"IsDefault": fmt.Sprintf("%v", aws.ToBool(vpc.IsDefault)),
			},
		})
	}
	return result, nil
}

func (p *VPCProvider) listSubnets(ctx context.Context, client *ec2.Client, cfg aws.Config) ([]resources.Resource, error) {
	out, err := client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{})
	if err != nil {
		return nil, err
	}

	var result []resources.Resource
	for _, subnet := range out.Subnets {
		name := tagName(subnet.Tags)
		result = append(result, resources.Resource{
			Service: "VPC",
			Type:    "AWS::EC2::Subnet",
			ID:      aws.ToString(subnet.SubnetId),
			Name:    name,
			Region:  cfg.Region,
			Properties: map[string]string{
				"VpcId":  aws.ToString(subnet.VpcId),
				"CIDR":   aws.ToString(subnet.CidrBlock),
				"AZ":     aws.ToString(subnet.AvailabilityZone),
				"Public": fmt.Sprintf("%v", aws.ToBool(subnet.MapPublicIpOnLaunch)),
			},
		})
	}
	return result, nil
}

func (p *VPCProvider) listSecurityGroups(ctx context.Context, client *ec2.Client, cfg aws.Config) ([]resources.Resource, error) {
	out, err := client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{})
	if err != nil {
		return nil, err
	}

	var result []resources.Resource
	for _, sg := range out.SecurityGroups {
		result = append(result, resources.Resource{
			Service: "VPC",
			Type:    "AWS::EC2::SecurityGroup",
			ID:      aws.ToString(sg.GroupId),
			Name:    aws.ToString(sg.GroupName),
			Region:  cfg.Region,
			Properties: map[string]string{
				"VpcId":       aws.ToString(sg.VpcId),
				"Description": aws.ToString(sg.Description),
				"IngressRules": fmt.Sprintf("%d", len(sg.IpPermissions)),
				"EgressRules":  fmt.Sprintf("%d", len(sg.IpPermissionsEgress)),
			},
		})
	}
	return result, nil
}
