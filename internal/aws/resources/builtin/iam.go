package builtin

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"i9c/internal/aws/resources"
)

type IAMProvider struct{}

func NewIAMProvider() *IAMProvider { return &IAMProvider{} }

func (p *IAMProvider) ServiceName() string { return "IAM" }

func (p *IAMProvider) ResourceTypes() []string {
	return []string{"AWS::IAM::Role", "AWS::IAM::User", "AWS::IAM::Policy"}
}

func (p *IAMProvider) List(ctx context.Context, cfg aws.Config) ([]resources.Resource, error) {
	client := iam.NewFromConfig(cfg)
	var result []resources.Resource

	roles, err := p.listRoles(ctx, client)
	if err == nil {
		result = append(result, roles...)
	}

	users, err := p.listUsers(ctx, client)
	if err == nil {
		result = append(result, users...)
	}

	return result, nil
}

func (p *IAMProvider) listRoles(ctx context.Context, client *iam.Client) ([]resources.Resource, error) {
	input := &iam.ListRolesInput{}
	var result []resources.Resource

	paginator := iam.NewListRolesPaginator(client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, role := range page.Roles {
			props := map[string]string{
				"Path": aws.ToString(role.Path),
			}
			if role.CreateDate != nil {
				props["Created"] = role.CreateDate.Format("2006-01-02")
			}
			if role.MaxSessionDuration != nil {
				props["MaxSession"] = fmt.Sprintf("%ds", *role.MaxSessionDuration)
			}

			result = append(result, resources.Resource{
				Service:    "IAM",
				Type:       "AWS::IAM::Role",
				ID:         aws.ToString(role.RoleId),
				Name:       aws.ToString(role.RoleName),
				Region:     "global",
				ARN:        aws.ToString(role.Arn),
				Properties: props,
			})
		}
	}
	return result, nil
}

func (p *IAMProvider) listUsers(ctx context.Context, client *iam.Client) ([]resources.Resource, error) {
	input := &iam.ListUsersInput{}
	var result []resources.Resource

	paginator := iam.NewListUsersPaginator(client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, user := range page.Users {
			props := map[string]string{
				"Path": aws.ToString(user.Path),
			}
			if user.CreateDate != nil {
				props["Created"] = user.CreateDate.Format("2006-01-02")
			}
			if user.PasswordLastUsed != nil {
				props["LastLogin"] = user.PasswordLastUsed.Format("2006-01-02")
			}

			result = append(result, resources.Resource{
				Service:    "IAM",
				Type:       "AWS::IAM::User",
				ID:         aws.ToString(user.UserId),
				Name:       aws.ToString(user.UserName),
				Region:     "global",
				ARN:        aws.ToString(user.Arn),
				Properties: props,
			})
		}
	}
	return result, nil
}
