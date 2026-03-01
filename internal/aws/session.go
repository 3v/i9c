package aws

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type SessionProbe interface {
	Probe(ctx context.Context, profile, region string, timeout time.Duration) (ProfileInfo, error)
}

type STSSessionProbe struct{}

func (p *STSSessionProbe) Probe(ctx context.Context, profile, region string, timeout time.Duration) (ProfileInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	opts := []func(*awsconfig.LoadOptions) error{awsconfig.WithSharedConfigProfile(profile)}
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		status := classifySessionErr(err)
		return ProfileInfo{Name: profile, Region: region, Status: status, Error: err.Error()}, err
	}

	client := sts.NewFromConfig(cfg)
	out, err := client.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		status := classifySessionErr(err)
		return ProfileInfo{Name: profile, Region: cfg.Region, Status: status, Error: err.Error()}, err
	}

	return ProfileInfo{
		Name:      profile,
		Region:    cfg.Region,
		Status:    StatusLive,
		AccountID: aws.ToString(out.Account),
	}, nil
}

func classifySessionErr(err error) SessionStatus {
	if err == nil {
		return StatusLive
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "token has expired"), strings.Contains(msg, "expiredtoken"), strings.Contains(msg, "expiration"):
		return StatusExpired
	case strings.Contains(msg, "sso"), strings.Contains(msg, "login"), strings.Contains(msg, "no credentials"):
		return StatusNoSession
	case strings.Contains(msg, "accessdenied"), strings.Contains(msg, "not authorized"), errors.Is(err, context.DeadlineExceeded):
		return StatusDenied
	default:
		return StatusError
	}
}
