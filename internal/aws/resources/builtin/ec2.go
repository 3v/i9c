package builtin

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"i9c/internal/aws/resources"
)

type EC2Provider struct{}

func NewEC2Provider() *EC2Provider { return &EC2Provider{} }

func (p *EC2Provider) ServiceName() string { return "EC2" }

func (p *EC2Provider) ResourceTypes() []string {
	return []string{"AWS::EC2::Instance"}
}

func (p *EC2Provider) List(ctx context.Context, cfg aws.Config) ([]resources.Resource, error) {
	client := ec2.NewFromConfig(cfg)

	input := &ec2.DescribeInstancesInput{}
	var result []resources.Resource

	paginator := ec2.NewDescribeInstancesPaginator(client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, reservation := range page.Reservations {
			for _, inst := range reservation.Instances {
				name := ""
				for _, tag := range inst.Tags {
					if aws.ToString(tag.Key) == "Name" {
						name = aws.ToString(tag.Value)
						break
					}
				}

				props := map[string]string{
					"State":        string(inst.State.Name),
					"InstanceType": string(inst.InstanceType),
					"AZ":           aws.ToString(inst.Placement.AvailabilityZone),
					"PrivateIP":    aws.ToString(inst.PrivateIpAddress),
					"PublicIP":     aws.ToString(inst.PublicIpAddress),
					"LaunchTime":   "",
				}
				if inst.LaunchTime != nil {
					props["LaunchTime"] = inst.LaunchTime.Format("2006-01-02 15:04:05")
				}

				result = append(result, resources.Resource{
					Service:    "EC2",
					Type:       "AWS::EC2::Instance",
					ID:         aws.ToString(inst.InstanceId),
					Name:       name,
					Region:     cfg.Region,
					ARN:        fmt.Sprintf("arn:aws:ec2:%s::instance/%s", cfg.Region, aws.ToString(inst.InstanceId)),
					Properties: props,
				})
			}
		}
	}

	return result, nil
}
