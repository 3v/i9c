package builtin

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"i9c/internal/aws/resources"
)

type S3Provider struct{}

func NewS3Provider() *S3Provider { return &S3Provider{} }

func (p *S3Provider) ServiceName() string { return "S3" }

func (p *S3Provider) ResourceTypes() []string {
	return []string{"AWS::S3::Bucket"}
}

func (p *S3Provider) List(ctx context.Context, cfg aws.Config) ([]resources.Resource, error) {
	client := s3.NewFromConfig(cfg)
	out, err := client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, err
	}

	var result []resources.Resource
	for _, bucket := range out.Buckets {
		name := aws.ToString(bucket.Name)
		props := map[string]string{}
		if bucket.CreationDate != nil {
			props["Created"] = bucket.CreationDate.Format("2006-01-02")
		}

		result = append(result, resources.Resource{
			Service:    "S3",
			Type:       "AWS::S3::Bucket",
			ID:         name,
			Name:       name,
			Region:     cfg.Region,
			ARN:        fmt.Sprintf("arn:aws:s3:::%s", name),
			Properties: props,
		})
	}

	return result, nil
}
