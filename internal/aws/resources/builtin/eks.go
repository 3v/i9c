package builtin

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"i9c/internal/aws/resources"
)

type EKSProvider struct{}

func NewEKSProvider() *EKSProvider { return &EKSProvider{} }

func (p *EKSProvider) ServiceName() string { return "EKS" }

func (p *EKSProvider) ResourceTypes() []string {
	return []string{"AWS::EKS::Cluster"}
}

func (p *EKSProvider) List(ctx context.Context, cfg aws.Config) ([]resources.Resource, error) {
	client := eks.NewFromConfig(cfg)

	listOut, err := client.ListClusters(ctx, &eks.ListClustersInput{})
	if err != nil {
		return nil, err
	}

	var result []resources.Resource
	for _, name := range listOut.Clusters {
		descOut, err := client.DescribeCluster(ctx, &eks.DescribeClusterInput{
			Name: aws.String(name),
		})
		if err != nil {
			continue
		}
		cluster := descOut.Cluster

		props := map[string]string{
			"Version":  aws.ToString(cluster.Version),
			"Status":   string(cluster.Status),
			"Platform": aws.ToString(cluster.PlatformVersion),
		}
		if cluster.Endpoint != nil {
			props["Endpoint"] = *cluster.Endpoint
		}
		if cluster.ResourcesVpcConfig != nil {
			props["VpcId"] = aws.ToString(cluster.ResourcesVpcConfig.VpcId)
			props["Subnets"] = fmt.Sprintf("%d", len(cluster.ResourcesVpcConfig.SubnetIds))
		}
		if cluster.CreatedAt != nil {
			props["Created"] = cluster.CreatedAt.Format("2006-01-02")
		}

		result = append(result, resources.Resource{
			Service:    "EKS",
			Type:       "AWS::EKS::Cluster",
			ID:         aws.ToString(cluster.Name),
			Name:       aws.ToString(cluster.Name),
			Region:     cfg.Region,
			ARN:        aws.ToString(cluster.Arn),
			Properties: props,
		})
	}

	return result, nil
}

func tagName(tags []ec2types.Tag) string {
	for _, tag := range tags {
		if aws.ToString(tag.Key) == "Name" {
			return aws.ToString(tag.Value)
		}
	}
	return ""
}
