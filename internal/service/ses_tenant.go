package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	sesv2types "github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

// sesTenantPageSize bounds each discovery page. An account may hold up to 10,000 tenants by
// default and a picker only needs a head, so listing stops after sesTenantPageCap pages.
const (
	sesTenantPageSize = 100
	sesTenantPageCap  = 5
)

// ManagedTenantName is the tenant Notifuse provisions for an integration. It is derived, so the
// same integration always converges on the same tenant however many times provisioning runs.
func ManagedTenantName(integrationID string) string {
	return fmt.Sprintf("notifuse-%s", integrationID)
}

// EnsureTenantIsolation converges an integration's AWS state on "isolated": a tenant of its own,
// with its own suppression list, associated with the configuration set we send through and with
// every verified identity its senders resolve to.
//
// It is idempotent and resumable, and it reports partial success rather than failing: an operator
// whose IAM allows creating a tenant but not associating resources needs to see both halves. The
// caller persists result.TenantName LAST, so a failure anywhere leaves "AWS has resources Notifuse
// isn't using yet" — billable, visible and retryable — rather than "Notifuse routes sends through
// a tenant that isn't ready", which would be a total outage for that integration.
func (s *SESService) EnsureTenantIsolation(
	ctx context.Context,
	config domain.AmazonSESSettings,
	integrationID string,
	senders []domain.EmailSender,
) (*domain.SESTenantProvisionResult, error) {
	if config.AccessKey == "" || config.SecretKey == "" {
		return nil, ErrInvalidAWSCredentials
	}

	client := s.sesV2ClientFactory(config)
	tenantName := ManagedTenantName(integrationID)
	result := &domain.SESTenantProvisionResult{TenantName: tenantName}

	tenantARN, err := s.ensureTenant(ctx, client, tenantName, result)
	if err != nil {
		return result, err
	}
	if tenantARN == "" {
		// Denied before anything existed; the result already says which permission is missing.
		return result, nil
	}

	// The configuration set must exist before it can be associated, and it must be the one the
	// send path will actually use.
	configSetName, managed := configurationSetFor(&config, integrationID)
	if managed {
		if err := s.setupConfigurationSet(ctx, config, configSetName); err != nil {
			s.logger.WithField("config_set_name", configSetName).
				Error("Failed to ensure SES configuration set for tenant: " + err.Error())
			return result, fmt.Errorf("failed to ensure configuration set: %w", err)
		}
		s.invalidateConfigurationSetCache(integrationID, config.Region)
	}

	configSetARN := domain.SESResourceARN(tenantARN, "configuration-set", configSetName)
	before := len(result.Associated)
	s.associate(ctx, client, tenantName, configSetARN, result)
	result.ConfigurationSetAssociated = len(result.Associated) > before

	s.associateIdentities(ctx, client, tenantName, tenantARN, senders, result)

	return result, nil
}

// ensureTenant creates the tenant with tenant-scoped suppression, or converges an existing one.
// Returns the tenant ARN, which is where every other resource ARN is derived from — no STS call
// and no account ID guessing.
func (s *SESService) ensureTenant(
	ctx context.Context,
	client domain.SESv2Client,
	tenantName string,
	result *domain.SESTenantProvisionResult,
) (string, error) {
	out, err := client.CreateTenant(ctx, &sesv2.CreateTenantInput{
		TenantName: awsv2.String(tenantName),
		SuppressionAttributes: &sesv2types.TenantSuppressionAttributes{
			// Without this the tenant silently inherits ACCOUNT scope and shares the
			// account-level suppression list — reputation isolation without suppression
			// isolation, which is half of what the feature exists for.
			SuppressionScope: sesv2types.SuppressionListScopeTenant,
			SuppressedReasons: []sesv2types.SuppressionListReason{
				sesv2types.SuppressionListReasonBounce,
				sesv2types.SuppressionListReasonComplaint,
			},
		},
	})

	switch {
	case err == nil:
		result.Created = true
		result.SuppressionScoped = true
		return awsv2.ToString(out.TenantArn), nil

	case domain.IsSESAccessDenied(err):
		s.recordMissingPermission(result, "ses:CreateTenant",
			fmt.Sprintf("aws sesv2 create-tenant --tenant-name %s --suppression-attributes SuppressionScope=TENANT,SuppressedReasons=BOUNCE,COMPLAINT", tenantName))
		return "", nil

	case isAlreadyExists(err):
		// Someone (a previous run, or the operator by hand) already made it. Converge rather
		// than assume: a hand-made tenant defaults to the shared account suppression list.
		return s.convergeExistingTenant(ctx, client, tenantName, result)

	default:
		return "", wrapSESError(err, "create SES tenant")
	}
}

func (s *SESService) convergeExistingTenant(
	ctx context.Context,
	client domain.SESv2Client,
	tenantName string,
	result *domain.SESTenantProvisionResult,
) (string, error) {
	var tenantARN string

	got, err := client.GetTenant(ctx, &sesv2.GetTenantInput{TenantName: awsv2.String(tenantName)})
	switch {
	case err == nil && got.Tenant != nil:
		tenantARN = awsv2.ToString(got.Tenant.TenantArn)
		result.SendingStatus = string(got.Tenant.SendingStatus)
		if got.Tenant.SuppressionAttributes != nil {
			result.SuppressionScoped = got.Tenant.SuppressionAttributes.SuppressionScope == sesv2types.SuppressionListScopeTenant
		}
	case domain.IsSESAccessDenied(err):
		s.recordMissingPermission(result, "ses:GetTenant",
			fmt.Sprintf("aws sesv2 get-tenant --tenant-name %s", tenantName))
		return "", nil
	case err != nil:
		return "", wrapSESError(err, "read SES tenant")
	}

	if result.SuppressionScoped {
		return tenantARN, nil
	}

	_, err = client.PutTenantSuppressionAttributes(ctx, &sesv2.PutTenantSuppressionAttributesInput{
		TenantName:       awsv2.String(tenantName),
		SuppressionScope: sesv2types.SuppressionListScopeTenant,
		SuppressedReasons: []sesv2types.SuppressionListReason{
			sesv2types.SuppressionListReasonBounce,
			sesv2types.SuppressionListReasonComplaint,
		},
	})
	switch {
	case err == nil:
		result.SuppressionScoped = true
	case domain.IsSESAccessDenied(err):
		s.recordMissingPermission(result, "ses:PutTenantSuppressionAttributes",
			fmt.Sprintf("aws sesv2 put-tenant-suppression-attributes --tenant-name %s --suppression-scope TENANT --suppressed-reasons BOUNCE COMPLAINT", tenantName))
	default:
		return tenantARN, wrapSESError(err, "set SES tenant suppression scope")
	}

	return tenantARN, nil
}

// associate attaches one resource to the tenant. An association that already exists is success:
// provisioning must converge when it runs twice, or concurrently.
func (s *SESService) associate(
	ctx context.Context,
	client domain.SESv2Client,
	tenantName, resourceARN string,
	result *domain.SESTenantProvisionResult,
) {
	if resourceARN == "" {
		return
	}

	_, err := client.CreateTenantResourceAssociation(ctx, &sesv2.CreateTenantResourceAssociationInput{
		TenantName:  awsv2.String(tenantName),
		ResourceArn: awsv2.String(resourceARN),
	})

	switch {
	case err == nil, isAlreadyExists(err):
		result.Associated = append(result.Associated, resourceARN)
	case domain.IsSESAccessDenied(err):
		s.recordMissingPermission(result, "ses:CreateTenantResourceAssociation",
			fmt.Sprintf("aws sesv2 create-tenant-resource-association --tenant-name %s --resource-arn %s", tenantName, resourceARN))
	default:
		s.logger.WithField("resource_arn", resourceARN).
			Error("Failed to associate resource with SES tenant: " + err.Error())
	}
}

// associateIdentities resolves which verified identity each sender actually sends under and
// associates every match. Resolution enumerates the account's identities rather than guessing:
// SES may match an exact address, its domain, or a parent domain, and associating all matches
// removes any need to predict which one it picks. Associations are free and idempotent.
func (s *SESService) associateIdentities(
	ctx context.Context,
	client domain.SESv2Client,
	tenantName, tenantARN string,
	senders []domain.EmailSender,
	result *domain.SESTenantProvisionResult,
) {
	if len(senders) == 0 {
		return
	}

	usable, err := s.listUsableIdentities(ctx, client)
	if err != nil {
		if domain.IsSESAccessDenied(err) {
			s.recordMissingPermission(result, "ses:ListEmailIdentities",
				"aws sesv2 list-email-identities")
			return
		}
		s.logger.Error("Failed to list SES identities: " + err.Error())
		return
	}

	seen := map[string]bool{}
	for _, sender := range senders {
		matches := identityCoverage(sender.Email, usable)
		if len(matches) == 0 {
			// This send would fail with or without tenants, so surfacing it is free.
			result.UnverifiedSenders = append(result.UnverifiedSenders, sender.Email)
			continue
		}
		for _, identity := range matches {
			if seen[identity] {
				continue
			}
			seen[identity] = true
			s.associate(ctx, client, tenantName, domain.SESResourceARN(tenantARN, "identity", identity), result)
		}
	}
}

// listUsableIdentities returns the identities that can actually send. ListEmailIdentities
// includes unverified ones, so presence in the list means nothing on its own.
func (s *SESService) listUsableIdentities(ctx context.Context, client domain.SESv2Client) (map[string]bool, error) {
	usable := map[string]bool{}
	var nextToken *string

	for page := 0; page < sesTenantPageCap; page++ {
		out, err := client.ListEmailIdentities(ctx, &sesv2.ListEmailIdentitiesInput{
			PageSize:  awsv2.Int32(sesTenantPageSize),
			NextToken: nextToken,
		})
		if err != nil {
			return nil, err
		}

		for _, identity := range out.EmailIdentities {
			name := awsv2.ToString(identity.IdentityName)
			if name == "" {
				continue
			}
			if identity.SendingEnabled && identity.VerificationStatus == sesv2types.VerificationStatusSuccess {
				usable[strings.ToLower(name)] = true
			}
		}

		if out.NextToken == nil || *out.NextToken == "" {
			break
		}
		nextToken = out.NextToken
	}

	return usable, nil
}

// identityCoverage returns every verified identity that covers a sender address, in SES's own
// resolution order: the exact address, then its domain, then each parent domain.
func identityCoverage(email string, usable map[string]bool) []string {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return nil
	}

	var matches []string
	lower := strings.ToLower(email)
	if usable[lower] {
		matches = append(matches, lower)
	}

	domainPart := lower[at+1:]
	for domainPart != "" {
		if usable[domainPart] {
			matches = append(matches, domainPart)
		}
		dot := strings.Index(domainPart, ".")
		if dot < 0 {
			break
		}
		domainPart = domainPart[dot+1:]
	}

	return matches
}

// VerifyTenantAssociation answers the question that actually decides whether a tenant send
// succeeds. "The configuration set exists" is a different question — which is why registering
// webhooks and then naming a tenant still breaks until the association is made.
func (s *SESService) VerifyTenantAssociation(
	ctx context.Context,
	config domain.AmazonSESSettings,
	tenantName, configSetName string,
) (*domain.SESTenantVerification, error) {
	if tenantName == "" {
		return nil, fmt.Errorf("tenant name is required")
	}

	client := s.sesV2ClientFactory(config)
	verification := &domain.SESTenantVerification{
		TenantName:           tenantName,
		ConfigurationSetName: configSetName,
	}

	got, err := client.GetTenant(ctx, &sesv2.GetTenantInput{TenantName: awsv2.String(tenantName)})
	switch {
	case domain.IsSESAccessDenied(err):
		return nil, domain.ErrSESAccessDenied
	case isNotFound(err):
		return verification, nil // Exists stays false: a clearer answer than "not associated"
	case err != nil:
		return nil, wrapSESError(err, "read SES tenant")
	}

	verification.Exists = true
	if got.Tenant != nil {
		verification.SendingStatus = string(got.Tenant.SendingStatus)
		if got.Tenant.SuppressionAttributes != nil {
			verification.SuppressionScope = string(got.Tenant.SuppressionAttributes.SuppressionScope)
		}
	}

	if configSetName == "" {
		return verification, nil
	}

	associated, err := s.configurationSetAssociated(ctx, client, tenantName, configSetName)
	if err != nil {
		if domain.IsSESAccessDenied(err) {
			return nil, domain.ErrSESAccessDenied
		}
		return nil, wrapSESError(err, "list SES tenant resources")
	}

	verification.ConfigurationSetAssociated = associated
	if !associated {
		verification.FixCommand = fmt.Sprintf(
			"aws sesv2 create-tenant-resource-association --tenant-name %s --resource-arn <configuration-set-arn:%s>",
			tenantName, configSetName)
	}

	return verification, nil
}

func (s *SESService) configurationSetAssociated(
	ctx context.Context,
	client domain.SESv2Client,
	tenantName, configSetName string,
) (bool, error) {
	var nextToken *string

	for page := 0; page < sesTenantPageCap; page++ {
		out, err := client.ListTenantResources(ctx, &sesv2.ListTenantResourcesInput{
			TenantName: awsv2.String(tenantName),
			Filter:     map[string]string{"RESOURCE_TYPE": string(sesv2types.ResourceTypeConfigurationSet)},
			PageSize:   awsv2.Int32(sesTenantPageSize),
			NextToken:  nextToken,
		})
		if err != nil {
			return false, err
		}

		for _, resource := range out.TenantResources {
			// Compare the parsed resource NAME. A suffix match would consider
			// "prod-notifuse-abc" a match for "notifuse-abc".
			if domain.SESResourceNameFromARN(awsv2.ToString(resource.ResourceArn)) == configSetName {
				return true, nil
			}
		}

		if out.NextToken == nil || *out.NextToken == "" {
			break
		}
		nextToken = out.NextToken
	}

	return false, nil
}

// ListSESTenants returns the tenants in the settings' region for the console picker.
func (s *SESService) ListSESTenants(ctx context.Context, config domain.AmazonSESSettings) ([]domain.SESTenant, bool, error) {
	client := s.sesV2ClientFactory(config)

	var (
		tenants   []domain.SESTenant
		nextToken *string
		hasMore   bool
	)

	for page := 0; page < sesTenantPageCap; page++ {
		out, err := client.ListTenants(ctx, &sesv2.ListTenantsInput{
			PageSize:  awsv2.Int32(sesTenantPageSize),
			NextToken: nextToken,
		})
		if err != nil {
			if domain.IsSESAccessDenied(err) {
				return nil, false, domain.ErrSESAccessDenied
			}
			return nil, false, wrapSESError(err, "list SES tenants")
		}

		for _, tenant := range out.Tenants {
			entry := domain.SESTenant{
				Name: awsv2.ToString(tenant.TenantName),
				ID:   awsv2.ToString(tenant.TenantId),
				ARN:  awsv2.ToString(tenant.TenantArn),
			}
			if tenant.CreatedTimestamp != nil {
				entry.CreatedAt = *tenant.CreatedTimestamp
			}
			tenants = append(tenants, entry)
		}

		if out.NextToken == nil || *out.NextToken == "" {
			return tenants, false, nil
		}
		nextToken = out.NextToken
		hasMore = true
	}

	return tenants, hasMore, nil
}

func (s *SESService) recordMissingPermission(result *domain.SESTenantProvisionResult, action, fixCommand string) {
	for _, existing := range result.MissingPermissions {
		if existing == action {
			return
		}
	}
	result.MissingPermissions = append(result.MissingPermissions, action)
	result.FixCommands = append(result.FixCommands, fixCommand)
}

func isAlreadyExists(err error) bool {
	var alreadyExists *sesv2types.AlreadyExistsException
	return errors.As(err, &alreadyExists)
}

func isNotFound(err error) bool {
	var notFound *sesv2types.NotFoundException
	return errors.As(err, &notFound)
}

// DeleteSendingResources removes everything Notifuse created for this integration, in the only
// order AWS permits: a configuration set or identity that is associated with a tenant cannot be
// deleted, so associations come off first.
//
// This runs on integration deletion, not on webhook unregistration. Unregistering webhooks must
// leave the configuration set in place because sends still resolve it — deleting it there would
// make every subsequent send fail. The cost of that split is that the tenant, which AWS bills
// monthly, only disappears here.
//
// Every step is best-effort: a half-removed AWS account must never block deleting an integration
// from Notifuse.
func (s *SESService) DeleteSendingResources(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	providerConfig *domain.EmailProvider,
) error {
	if providerConfig == nil || providerConfig.SES == nil ||
		providerConfig.SES.AccessKey == "" || providerConfig.SES.SecretKey == "" {
		return ErrInvalidSESConfig
	}

	config := *providerConfig.SES
	configSetName, configSetManaged := configurationSetFor(&config, integrationID)
	// Not ResolveTenant: a tenant that isolation was switched off for still exists, still holds
	// the association that blocks deleting the configuration set, and still bills.
	tenantName := config.KnownTenant()
	managedTenant := config.OwnsManagedTenant()

	client := s.sesV2ClientFactory(config)

	// 1. Dissociate, so the delete below is permitted at all. Identities are dissociated only
	//    from a tenant we own: an operator's tenant may share them with other integrations.
	if tenantName != "" {
		if arn := s.tenantARN(ctx, client, tenantName); arn != "" {
			s.dissociate(ctx, client, tenantName, domain.SESResourceARN(arn, "configuration-set", configSetName))

			if managedTenant {
				for _, identity := range s.associatedIdentities(ctx, client, tenantName) {
					s.dissociate(ctx, client, tenantName, identity)
				}
			}
		}
	}

	// 2. Delete the configuration set, but only if it is ours.
	if configSetManaged {
		if err := s.DeleteConfigurationSet(ctx, config, configSetName); err != nil {
			s.logger.WithField("config_set_name", configSetName).
				Error("Failed to delete SES configuration set during integration deletion: " + err.Error())
		}
		s.invalidateConfigurationSetCache(integrationID, config.Region)
	}

	// 3. Delete the tenant, but only one we created. An operator's own tenant is not ours to
	//    remove, and deleting it would destroy their reputation history and suppression list.
	if managedTenant {
		if _, err := client.DeleteTenant(ctx, &sesv2.DeleteTenantInput{
			TenantName: awsv2.String(config.ManagedTenantName),
		}); err != nil && !isNotFound(err) {
			s.logger.WithField("tenant", config.ManagedTenantName).
				Error("Failed to delete SES tenant during integration deletion; it will keep billing until removed manually: " + err.Error())
		}
	}

	return nil
}

// tenantARN returns the tenant's ARN, which every resource ARN is derived from. Empty when the
// tenant is gone or unreadable — in which case there is nothing to dissociate.
func (s *SESService) tenantARN(ctx context.Context, client domain.SESv2Client, tenantName string) string {
	out, err := client.GetTenant(ctx, &sesv2.GetTenantInput{TenantName: awsv2.String(tenantName)})
	if err != nil || out.Tenant == nil {
		return ""
	}
	return awsv2.ToString(out.Tenant.TenantArn)
}

// associatedIdentities lists the identity ARNs attached to a tenant.
func (s *SESService) associatedIdentities(ctx context.Context, client domain.SESv2Client, tenantName string) []string {
	var (
		identities []string
		nextToken  *string
	)

	for page := 0; page < sesTenantPageCap; page++ {
		out, err := client.ListTenantResources(ctx, &sesv2.ListTenantResourcesInput{
			TenantName: awsv2.String(tenantName),
			Filter:     map[string]string{"RESOURCE_TYPE": string(sesv2types.ResourceTypeEmailIdentity)},
			PageSize:   awsv2.Int32(sesTenantPageSize),
			NextToken:  nextToken,
		})
		if err != nil {
			return identities
		}

		for _, resource := range out.TenantResources {
			if arn := awsv2.ToString(resource.ResourceArn); arn != "" {
				identities = append(identities, arn)
			}
		}

		if out.NextToken == nil || *out.NextToken == "" {
			break
		}
		nextToken = out.NextToken
	}

	return identities
}

func (s *SESService) dissociate(ctx context.Context, client domain.SESv2Client, tenantName, resourceARN string) {
	if resourceARN == "" {
		return
	}
	if _, err := client.DeleteTenantResourceAssociation(ctx, &sesv2.DeleteTenantResourceAssociationInput{
		TenantName:  awsv2.String(tenantName),
		ResourceArn: awsv2.String(resourceARN),
	}); err != nil && !isNotFound(err) {
		s.logger.WithField("resource_arn", resourceARN).
			Error("Failed to dissociate resource from SES tenant: " + err.Error())
	}
}

// AssociateExistingTenant re-attaches the configuration set and sender identities to a tenant
// that already exists. It never creates anything, so it is safe to run implicitly — which
// matters because re-registering webhooks recreates the configuration set as a NEW resource,
// silently dropping its tenant association and rejecting every send until it is restored.
func (s *SESService) AssociateExistingTenant(
	ctx context.Context,
	config domain.AmazonSESSettings,
	integrationID string,
	senders []domain.EmailSender,
) (*domain.SESTenantProvisionResult, error) {
	tenantName := config.ResolveTenant()
	if tenantName == "" {
		return nil, nil
	}

	client := s.sesV2ClientFactory(config)
	result := &domain.SESTenantProvisionResult{TenantName: tenantName}

	tenantARN := s.tenantARN(ctx, client, tenantName)
	if tenantARN == "" {
		return result, nil
	}

	configSetName, _ := configurationSetFor(&config, integrationID)
	before := len(result.Associated)
	s.associate(ctx, client, tenantName, domain.SESResourceARN(tenantARN, "configuration-set", configSetName), result)
	result.ConfigurationSetAssociated = len(result.Associated) > before

	s.associateIdentities(ctx, client, tenantName, tenantARN, senders, result)

	return result, nil
}
