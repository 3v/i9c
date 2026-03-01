package builtin

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"i9c/internal/aws/resources"
)

type RDSProvider struct{}

func NewRDSProvider() *RDSProvider { return &RDSProvider{} }

func (p *RDSProvider) ServiceName() string { return "RDS" }

func (p *RDSProvider) ResourceTypes() []string {
	return []string{"AWS::RDS::DBInstance", "AWS::RDS::DBCluster"}
}

func (p *RDSProvider) List(ctx context.Context, cfg aws.Config) ([]resources.Resource, error) {
	client := rds.NewFromConfig(cfg)
	var result []resources.Resource

	instances, err := p.listInstances(ctx, client, cfg)
	if err == nil {
		result = append(result, instances...)
	}

	clusters, err := p.listClusters(ctx, client, cfg)
	if err == nil {
		result = append(result, clusters...)
	}

	return result, nil
}

func (p *RDSProvider) listInstances(ctx context.Context, client *rds.Client, cfg aws.Config) ([]resources.Resource, error) {
	input := &rds.DescribeDBInstancesInput{}
	var result []resources.Resource

	paginator := rds.NewDescribeDBInstancesPaginator(client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, db := range page.DBInstances {
			props := map[string]string{
				"Engine":        aws.ToString(db.Engine),
				"EngineVersion": aws.ToString(db.EngineVersion),
				"Class":         aws.ToString(db.DBInstanceClass),
				"Status":        aws.ToString(db.DBInstanceStatus),
				"MultiAZ":       fmt.Sprintf("%v", aws.ToBool(db.MultiAZ)),
				"Encrypted":     fmt.Sprintf("%v", aws.ToBool(db.StorageEncrypted)),
			}
			if db.Endpoint != nil {
				props["Endpoint"] = aws.ToString(db.Endpoint.Address)
				props["Port"] = fmt.Sprintf("%d", db.Endpoint.Port)
			}

			result = append(result, resources.Resource{
				Service:    "RDS",
				Type:       "AWS::RDS::DBInstance",
				ID:         aws.ToString(db.DBInstanceIdentifier),
				Name:       aws.ToString(db.DBInstanceIdentifier),
				Region:     cfg.Region,
				ARN:        aws.ToString(db.DBInstanceArn),
				Properties: props,
			})
		}
	}
	return result, nil
}

func (p *RDSProvider) listClusters(ctx context.Context, client *rds.Client, cfg aws.Config) ([]resources.Resource, error) {
	input := &rds.DescribeDBClustersInput{}
	var result []resources.Resource

	paginator := rds.NewDescribeDBClustersPaginator(client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, cluster := range page.DBClusters {
			props := map[string]string{
				"Engine":        aws.ToString(cluster.Engine),
				"EngineVersion": aws.ToString(cluster.EngineVersion),
				"Status":        aws.ToString(cluster.Status),
				"Members":       fmt.Sprintf("%d", len(cluster.DBClusterMembers)),
				"Encrypted":     fmt.Sprintf("%v", aws.ToBool(cluster.StorageEncrypted)),
			}
			if cluster.Endpoint != nil {
				props["Endpoint"] = *cluster.Endpoint
			}

			result = append(result, resources.Resource{
				Service:    "RDS",
				Type:       "AWS::RDS::DBCluster",
				ID:         aws.ToString(cluster.DBClusterIdentifier),
				Name:       aws.ToString(cluster.DBClusterIdentifier),
				Region:     cfg.Region,
				ARN:        aws.ToString(cluster.DBClusterArn),
				Properties: props,
			})
		}
	}
	return result, nil
}
