package domain

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/smithy-go"
)

//go:generate mockgen -destination mocks/mock_sesv2_client.go -package mocks github.com/Notifuse/notifuse/internal/domain SESv2Client

// SESv2Client is the SES API v2 surface Notifuse uses for sending and for tenant management.
// Both live on v2 because tenants exist only there — the v1 API has no tenant operations at all
// and the v1 Go SDK is end-of-support, so it will never gain them. Configuration-set CRUD, SNS
// topics and inbound receipt rules stay on SESWebhookClient (v1); the two coexist deliberately.
type SESv2Client interface {
	// sending
	SendEmail(ctx context.Context, params *sesv2.SendEmailInput, optFns ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error)

	// tenant provisioning
	CreateTenant(ctx context.Context, params *sesv2.CreateTenantInput, optFns ...func(*sesv2.Options)) (*sesv2.CreateTenantOutput, error)
	GetTenant(ctx context.Context, params *sesv2.GetTenantInput, optFns ...func(*sesv2.Options)) (*sesv2.GetTenantOutput, error)
	PutTenantSuppressionAttributes(ctx context.Context, params *sesv2.PutTenantSuppressionAttributesInput, optFns ...func(*sesv2.Options)) (*sesv2.PutTenantSuppressionAttributesOutput, error)
	CreateTenantResourceAssociation(ctx context.Context, params *sesv2.CreateTenantResourceAssociationInput, optFns ...func(*sesv2.Options)) (*sesv2.CreateTenantResourceAssociationOutput, error)

	// identity resolution — v2, NOT the v1 ListIdentities used by inboundRecipients: v1 returns
	// bare names "regardless of verification status", so it cannot answer "can this sender send".
	// v2 IdentityInfo carries VerificationStatus and SendingEnabled. Note the v2 list also
	// includes unverified identities, so callers must filter on those fields.
	ListEmailIdentities(ctx context.Context, params *sesv2.ListEmailIdentitiesInput, optFns ...func(*sesv2.Options)) (*sesv2.ListEmailIdentitiesOutput, error)

	// discovery, verification, teardown
	ListTenants(ctx context.Context, params *sesv2.ListTenantsInput, optFns ...func(*sesv2.Options)) (*sesv2.ListTenantsOutput, error)
	ListTenantResources(ctx context.Context, params *sesv2.ListTenantResourcesInput, optFns ...func(*sesv2.Options)) (*sesv2.ListTenantResourcesOutput, error)
	ListResourceTenants(ctx context.Context, params *sesv2.ListResourceTenantsInput, optFns ...func(*sesv2.Options)) (*sesv2.ListResourceTenantsOutput, error)
	DeleteTenantResourceAssociation(ctx context.Context, params *sesv2.DeleteTenantResourceAssociationInput, optFns ...func(*sesv2.Options)) (*sesv2.DeleteTenantResourceAssociationOutput, error)
	DeleteTenant(ctx context.Context, params *sesv2.DeleteTenantInput, optFns ...func(*sesv2.Options)) (*sesv2.DeleteTenantOutput, error)
}

// ErrSESAccessDenied reports that the AWS credentials lack an optional permission. Every tenant
// feature degrades on it rather than failing: pickers fall back to free text, verification is
// skipped, provisioning reports what it could not do. It is deliberately distinguishable from
// "your credentials are wrong", which the UI must present very differently.
var ErrSESAccessDenied = errors.New("ses: access denied by IAM policy")

// IsSESAccessDenied reports whether err is an AWS authorization failure.
//
// This matches on the error CODE, not on a type: AccessDeniedException is a *common* SES error
// and has no generated Go type in sesv2/types, so errors.As against a typed struct cannot work.
// NotAuthorized is its 401 sibling.
func IsSESAccessDenied(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrSESAccessDenied) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "AccessDeniedException", "AccessDenied", "NotAuthorized":
			return true
		}
	}
	return false
}

// SESTenant is a tenant as listed by the SES v2 API (from TenantInfo).
type SESTenant struct {
	Name      string    `json:"name"`
	ID        string    `json:"id,omitempty"`
	ARN       string    `json:"arn,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// SESTenantProvisionResult describes what EnsureTenantIsolation actually achieved. It is a
// partial result by design: an operator whose IAM allows creating a tenant but not associating
// resources needs to see both halves, not a bare error.
type SESTenantProvisionResult struct {
	TenantName string `json:"tenant_name"`
	// Created distinguishes "we made it" from "it already existed".
	Created bool `json:"created"`
	// SuppressionScoped is the point of the feature: false means bounces still land on the
	// shared account-level suppression list.
	SuppressionScoped bool `json:"suppression_scoped"`
	// Associated lists the resource ARNs now attached to the tenant.
	Associated []string `json:"associated,omitempty"`
	// ConfigurationSetAssociated is the one fact that decides whether a tenant send succeeds.
	// SES rejects a send whose configuration set is not associated with the named tenant, so
	// the tenant must not be recorded on the integration until this is true — otherwise a
	// partial provisioning failure turns into a total outage on the next send.
	ConfigurationSetAssociated bool `json:"configuration_set_associated"`
	// UnverifiedSenders lists sender addresses no usable verified identity covers. Those sends
	// would fail with or without tenants, so surfacing them is a free diagnostic.
	UnverifiedSenders []string `json:"unverified_senders,omitempty"`
	// MissingPermissions names the IAM actions that were denied, e.g. "ses:CreateTenant".
	MissingPermissions []string `json:"missing_permissions,omitempty"`
	// FixCommands holds copy-pasteable aws-cli equivalents for whatever could not be done.
	FixCommands []string `json:"fix_commands,omitempty"`
	// ProvisionedButUnsaved is the one state the UI must never render as "not provisioned":
	// AWS has the tenant (and is billing for it) but the settings write-back failed. The retry
	// is safe because every step converges.
	ProvisionedButUnsaved bool `json:"provisioned_but_unsaved,omitempty"`
	// SendingStatus mirrors the tenant's SES sending status (ENABLED/REINSTATED/DISABLED).
	// DISABLED means SES paused this tenant — the exact event isolation exists to contain.
	SendingStatus string `json:"sending_status,omitempty"`
}

// SESTenantVerification reports whether a tenant is actually usable for sending.
type SESTenantVerification struct {
	TenantName string `json:"tenant_name"`
	// Exists is false when the named tenant is not in the account/region at all.
	Exists bool `json:"exists"`
	// ConfigurationSetAssociated answers the question that decides whether sends succeed.
	ConfigurationSetAssociated bool   `json:"configuration_set_associated"`
	ConfigurationSetName       string `json:"configuration_set_name,omitempty"`
	// SuppressionScope is "TENANT" or "ACCOUNT"; ACCOUNT means the operator has reputation
	// isolation but not suppression isolation, which is the half-configuration AWS defaults to.
	SuppressionScope string `json:"suppression_scope,omitempty"`
	SendingStatus    string `json:"sending_status,omitempty"`
	// FixCommand is the aws-cli line that repairs a missing association.
	FixCommand string `json:"fix_command,omitempty"`
}

// sesRegions is the set of AWS regions the console offers for SES. The region is validated
// against it before any client is built: the value otherwise flows straight into the AWS
// endpoint host.
var sesRegions = map[string]struct{}{
	"us-east-2": {}, "us-east-1": {}, "us-west-1": {}, "us-west-2": {},
	"af-south-1": {}, "ap-south-2": {}, "ap-southeast-3": {}, "ap-southeast-5": {},
	"ap-south-1": {}, "ap-northeast-3": {}, "ap-northeast-2": {}, "ap-southeast-1": {},
	"ap-southeast-2": {}, "ap-northeast-1": {}, "ca-central-1": {}, "ca-west-1": {},
	"eu-central-1": {}, "eu-central-2": {}, "eu-west-1": {}, "eu-west-2": {},
	"eu-south-1": {}, "eu-west-3": {}, "eu-north-1": {}, "il-central-1": {},
	"me-south-1": {}, "me-central-1": {}, "sa-east-1": {},
	"us-gov-east-1": {}, "us-gov-west-1": {},
}

// IsValidSESRegion reports whether region is one of the SES regions Notifuse supports.
func IsValidSESRegion(region string) bool {
	_, ok := sesRegions[region]
	return ok
}

// SESResourceARN builds the ARN of an SES resource from a known-good tenant ARN, which is where
// the account ID comes from. Using an ARN we were handed avoids an STS GetCallerIdentity call
// and the dependency it would drag in.
//
// resourceType is the ARN path segment, e.g. "configuration-set" or "identity".
func SESResourceARN(tenantARN, resourceType, name string) string {
	// arn:aws:ses:<region>:<account>:tenant/<name>/<id>
	parts := strings.SplitN(tenantARN, ":", 6)
	if len(parts) < 6 {
		return ""
	}
	return strings.Join([]string{parts[0], parts[1], parts[2], parts[3], parts[4], resourceType + "/" + name}, ":")
}

// SESResourceNameFromARN returns the resource name from an SES ARN — the segment after the
// resource type. Comparing parsed names is the only safe way to answer "is this configuration
// set associated": a suffix match would consider "prod-notifuse-abc" a match for "notifuse-abc".
func SESResourceNameFromARN(resourceARN string) string {
	parts := strings.SplitN(resourceARN, ":", 6)
	if len(parts) < 6 {
		return ""
	}
	resource := parts[5]
	idx := strings.Index(resource, "/")
	if idx < 0 {
		return ""
	}
	return resource[idx+1:]
}
