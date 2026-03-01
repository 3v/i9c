package aws

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	iconfig "i9c/internal/config"
)

type Client struct {
	Profile string
	Region  string
	Config  aws.Config
}

func NewClientFromProfile(ctx context.Context, profile, region string) (*Client, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithSharedConfigProfile(profile),
	}
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}

	return &Client{
		Profile: profile,
		Region:  cfg.Region,
		Config:  cfg,
	}, nil
}

func NewClientFromKeys(ctx context.Context, icfg *iconfig.AWSConfig) (*Client, error) {
	accessKey := os.Getenv(icfg.AccessKeyEnv)
	secretKey := os.Getenv(icfg.SecretKeyEnv)
	sessionToken := os.Getenv(icfg.SessionTokenEnv)

	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKey, secretKey, sessionToken),
		),
	}
	if icfg.Region != "" {
		opts = append(opts, awsconfig.WithRegion(icfg.Region))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}

	return &Client{
		Profile: "keys",
		Region:  cfg.Region,
		Config:  cfg,
	}, nil
}
