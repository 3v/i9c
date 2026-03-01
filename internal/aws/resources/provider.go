package resources

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
)

type Resource struct {
	Profile    string
	Service    string
	Type       string
	ID         string
	Name       string
	Region     string
	ARN        string
	Properties map[string]string
}

type ResourceProvider interface {
	ServiceName() string
	ResourceTypes() []string
	List(ctx context.Context, cfg aws.Config) ([]Resource, error)
}
