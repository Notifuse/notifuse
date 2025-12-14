package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/Notifuse/notifuse/pkg/tracing"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/google/uuid"
	"go.opencensus.io/trace"
)

type EmailService struct {
	logger           logger.Logger
	authService      domain.AuthService
	secretKey        string
	isDemo           bool
	workspaceRepo    domain.WorkspaceRepository
	templateRepo     domain.TemplateRepository
	templateService  domain.TemplateService
	messageRepo      domain.MessageHistoryRepository
	httpClient       domain.HTTPClient
	webhookEndpoint  string
	apiEndpoint      string
	smtpService      domain.EmailProviderService
	sesService       domain.EmailProviderService
	sparkPostService domain.EmailProviderService
	postmarkService  domain.EmailProviderService
	mailgunService   domain.EmailProviderService
	mailjetService   domain.EmailProviderService
}

// NewEmailService creates a new EmailService instance
func NewEmailService(
	logger logger.Logger,
	authService domain.AuthService,
	secretKey string,
	isDemo bool,
	workspaceRepo domain.WorkspaceRepository,
	templateRepo domain.TemplateRepository,
	templateService domain.TemplateService,
	messageRepo domain.MessageHistoryRepository,
	httpClient domain.HTTPClient,
	webhookEndpoint string,
	apiEndpoint string,
) *EmailService {
	// Initialize provider services
	smtpService := NewSMTPService(logger)
	sesService := NewSESService(authService, logger)
	sparkPostService := NewSparkPostService(httpClient, authService, logger)
	postmarkService := NewPostmarkService(httpClient, authService, logger)
	mailgunService := NewMailgunService(httpClient, authService, logger, webhookEndpoint)
	mailjetService := NewMailjetService(httpClient, authService, logger)

	return &EmailService{
		logger:           logger,
		authService:      authService,
		secretKey:        secretKey,
		isDemo:           isDemo,
		workspaceRepo:    workspaceRepo,
		templateRepo:     templateRepo,
		templateService:  templateService,
		messageRepo:      messageRepo,
		httpClient:       httpClient,
		webhookEndpoint:  webhookEndpoint,
		apiEndpoint:      apiEndpoint,
		smtpService:      smtpService,
		sesService:       sesService,
		sparkPostService: sparkPostService,
		postmarkService:  postmarkService,
		mailgunService:   mailgunService,
		mailjetService:   mailjetService,
	}
}

// CreateSESClient creates a new SES client with the provided credentials
func CreateSESClient(region, accessKey, secretKey string) domain.SESClient {
	sess, _ := session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewStaticCredentials(accessKey, secretKey, ""),
	})
	return ses.New(sess)
}

// TestEmailProvider sends a test email to verify the provider configuration works
func (s *EmailService) TestEmailProvider(ctx context.Context, workspaceID string, provider domain.EmailProvider, to string) error {
	ctx, span := tracing.StartServiceSpan(ctx, "EmailService", "TestEmailProvider")
	defer tracing.EndSpan(span, nil)

	// Authenticate user
	ctx, _, _, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		return err
	}

	// Validate the provider has the required fields
	if len(provider.Senders) == 0 {
		return fmt.Errorf("at least one sender is required for the provider")
	}

	// Use the first sender in the list
	defaultSender := provider.Senders[0]

	// Ensure sender has ID
	if defaultSender.ID == "" {
		defaultSender.ID = uuid.New().String()
		provider.Senders[0] = defaultSender
	}

	// Generate email content
	subject := "Notifuse: Test Email Provider"
	content := "<h1>Notifuse: Test Email Provider</h1><p>This is a test email from Notifuse. Your provider is working!</p>"

	// Send email with the provider details
	messageID := uuid.New().String()

	// Create SendEmailProviderRequest for testing
	request := domain.SendEmailProviderRequest{
		WorkspaceID:   workspaceID,
		IntegrationID: "test-integration", // For testing purposes
		MessageID:     messageID,
		FromAddress:   defaultSender.Email,
		FromName:      defaultSender.Name,
		To:            to,
		Subject:       subject,
		Content:       content,
		Provider:      &provider,
		EmailOptions: domain.EmailOptions{
			ReplyTo: "",
			CC:      nil,
			BCC:     nil,
		},
	}

	err = s.SendEmail(ctx, request, false)

	if err != nil {
		tracing.MarkSpanError(ctx, err)
		return fmt.Errorf("failed to test provider: %w", err)
	}

	return nil
}

// SendEmail sends an email using the specified provider
func (s *EmailService) SendEmail(ctx context.Context, request domain.SendEmailProviderRequest, isMarketing bool) error {
	if s.isDemo {
		return nil
	}

	// Validate the request
	if err := request.Validate(); err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}

	// If fromAddress is not provided, use the first sender's email from the provider
	if request.FromAddress == "" && len(request.Provider.Senders) > 0 {
		request.FromAddress = request.Provider.Senders[0].Email
	}

	// If fromName is not provided, use the first sender's name from the provider
	if request.FromName == "" && len(request.Provider.Senders) > 0 {
		request.FromName = request.Provider.Senders[0].Name
	}

	// Get the appropriate provider service
	providerService, err := s.getProviderService(request.Provider.Kind)
	if err != nil {
		return err
	}

	// Delegate to the provider-specific implementation
	return providerService.SendEmail(ctx, request)
}

// getProviderService returns the appropriate email provider service based on provider kind
func (s *EmailService) getProviderService(providerKind domain.EmailProviderKind) (domain.EmailProviderService, error) {
	switch providerKind {
	case domain.EmailProviderKindSMTP:
		return s.smtpService, nil
	case domain.EmailProviderKindSES:
		return s.sesService, nil
	case domain.EmailProviderKindSparkPost:
		return s.sparkPostService, nil
	case domain.EmailProviderKindPostmark:
		return s.postmarkService, nil
	case domain.EmailProviderKindMailgun:
		return s.mailgunService, nil
	case domain.EmailProviderKindMailjet:
		return s.mailjetService, nil
	default:
		return nil, fmt.Errorf("unsupported provider kind: %s", providerKind)
	}
}

func (s *EmailService) VisitLink(ctx context.Context, messageID string, workspaceID string) error {
	// find the message by id
	err := s.messageRepo.SetClicked(ctx, workspaceID, messageID, time.Now())
	if err != nil {
		s.logger.Error(err.Error())
		return fmt.Errorf("failed to set clicked: %w", err)
	}

	return nil
}

func (s *EmailService) OpenEmail(ctx context.Context, messageID string, workspaceID string) error {
	// find the message by id
	err := s.messageRepo.SetOpened(ctx, workspaceID, messageID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to update message opened: %w", err)
	}
	return nil
}

// SendEmailForTemplate handles sending through the email channel
func (s *EmailService) SendEmailForTemplate(ctx context.Context, request domain.SendEmailRequest) error {
	ctx, span := tracing.StartServiceSpan(ctx, "EmailService", "SendEmailForTemplate")
	defer span.End()

	// Validate request
	if err := request.Validate(); err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}

	span.AddAttributes(
		trace.StringAttribute("workspace", request.WorkspaceID),
		trace.StringAttribute("message_id", request.MessageID),
		trace.StringAttribute("contact.email", request.Contact.Email),
		trace.StringAttribute("template_id", request.TemplateConfig.TemplateID),
	)

	s.logger.WithFields(map[string]interface{}{
		"workspace":   request.WorkspaceID,
		"message_id":  request.MessageID,
		"contact":     request.Contact.Email,
		"template_id": request.TemplateConfig.TemplateID,
	}).Debug("Preparing to send email notification")

	// Get the template (mark as system call to bypass authentication)
	systemCtx := context.WithValue(ctx, domain.SystemCallKey, true)
	template, err := s.templateService.GetTemplateByID(systemCtx, request.WorkspaceID, request.TemplateConfig.TemplateID, int64(0))
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"error":       err.Error(),
			"template_id": request.TemplateConfig.TemplateID,
		}).Error("Failed to get template")

		tracing.MarkSpanError(ctx, err)
		return fmt.Errorf("failed to get template: %w", err)
	}

	// Find the emailSender
	emailSender := request.EmailProvider.GetSender(template.Email.SenderID)

	if emailSender == nil {
		return fmt.Errorf("sender not found: %s", template.Email.SenderID)
	}

	span.AddAttributes(
		trace.StringAttribute("template.subject", template.Email.Subject),
		trace.StringAttribute("template.from_email", emailSender.Email),
	)

	// set utm_content to the template id if not set
	if request.TrackingSettings.UTMContent == "" {
		request.TrackingSettings.UTMContent = template.ID
	}

	// Get workspace to check for custom endpoint URL
	workspace, err := s.workspaceRepo.GetByID(ctx, request.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace: %w", err)
	}

	// Use workspace CustomEndpointURL if provided, otherwise use the default API endpoint
	endpoint := s.apiEndpoint
	if workspace.Settings.CustomEndpointURL != nil && *workspace.Settings.CustomEndpointURL != "" {
		endpoint = *workspace.Settings.CustomEndpointURL
	}

	trackingSettings := notifuse_mjml.TrackingSettings{
		Endpoint:       endpoint,
		EnableTracking: request.TrackingSettings.EnableTracking,
		UTMSource:      request.TrackingSettings.UTMSource,
		UTMMedium:      request.TrackingSettings.UTMMedium,
		UTMCampaign:    request.TrackingSettings.UTMCampaign,
		UTMContent:     request.TrackingSettings.UTMContent,
		UTMTerm:        request.TrackingSettings.UTMTerm,
		WorkspaceID:    request.WorkspaceID,
		MessageID:      request.MessageID,
	}

	compileTemplateRequest := domain.CompileTemplateRequest{
		WorkspaceID:      request.WorkspaceID,
		MessageID:        request.MessageID,
		VisualEditorTree: template.Email.VisualEditorTree,
		TemplateData:     request.MessageData.Data,
		TrackingSettings: trackingSettings,
	}

	// Compile the template with the message data (use system context to bypass authentication)
	compiledTemplate, err := s.templateService.CompileTemplate(systemCtx, compileTemplateRequest)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"error":       err.Error(),
			"template_id": request.TemplateConfig.TemplateID,
		}).Error("Failed to compile template")

		tracing.MarkSpanError(ctx, err)
		return fmt.Errorf("failed to compile template: %w", err)
	}

	tracing.AddAttribute(ctx, "template.compilation_success", compiledTemplate.Success)

	if !compiledTemplate.Success || compiledTemplate.HTML == nil {
		errMsg := "Unknown error"
		if compiledTemplate.Error != nil {
			errMsg = compiledTemplate.Error.Message
		}
		s.logger.WithField("error", errMsg).Error("Template compilation failed")

		err := fmt.Errorf("template compilation failed: %s", errMsg)
		tracing.MarkSpanError(ctx, err)
		return err
	}

	// Get necessary email information from the template
	fromEmail := emailSender.Email
	fromName := emailSender.Name

	// Allow override of from name via email options
	if request.EmailOptions.FromName != nil && *request.EmailOptions.FromName != "" {
		s.logger.WithFields(map[string]interface{}{
			"message_id":         request.MessageID,
			"default_from_name":  emailSender.Name,
			"override_from_name": *request.EmailOptions.FromName,
		}).Debug("Using from_name override")
		fromName = *request.EmailOptions.FromName
	}

	// Process subject line through Liquid templating if it contains Liquid tags
	subject, err := notifuse_mjml.ProcessLiquidTemplate(
		template.Email.Subject,
		request.MessageData.Data,
		"email_subject",
	)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"error":       err.Error(),
			"message_id":  request.MessageID,
			"template_id": request.TemplateConfig.TemplateID,
			"subject":     template.Email.Subject,
		}).Error("Failed to process subject line with Liquid templating")
		tracing.MarkSpanError(ctx, err)
		return fmt.Errorf("failed to process subject with Liquid: %w", err)
	}

	htmlContent := *compiledTemplate.HTML
	now := time.Now().UTC()

	// Convert email options to channel options for storage
	channelOptions := request.EmailOptions.ToChannelOptions()

	// Create message history record
	messageHistory := &domain.MessageHistory{
		ID:             request.MessageID,
		ExternalID:     request.ExternalID,
		ContactEmail:   request.Contact.Email,
		AutomationID:   request.AutomationID,
		TemplateID:     request.TemplateConfig.TemplateID,
		Channel:        "email",
		MessageData:    request.MessageData,
		ChannelOptions: channelOptions,
		SentAt:         now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Save to message history
	if err := s.messageRepo.Create(ctx, request.WorkspaceID, workspace.Settings.SecretKey, messageHistory); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"error":      err.Error(),
			"message_id": request.MessageID,
		}).Error("Failed to create message history")

		tracing.MarkSpanError(ctx, err)
		return fmt.Errorf("failed to create message history: %w", err)
	}

	tracing.AddAttribute(ctx, "message_history.created", true)

	// Send the email using the email service
	s.logger.WithFields(map[string]interface{}{
		"to":         request.Contact.Email,
		"from":       fromEmail,
		"subject":    subject,
		"message_id": request.MessageID,
	}).Debug("Sending email")

	tracing.AddAttribute(ctx, "email.sending", true)

	// optional override for reply to
	if template.Email.ReplyTo != "" {
		request.EmailOptions.ReplyTo = template.Email.ReplyTo
	}

	// Create SendEmailProviderRequest
	providerRequest := domain.SendEmailProviderRequest{
		WorkspaceID:   request.WorkspaceID,
		IntegrationID: request.IntegrationID,
		MessageID:     request.MessageID,
		FromAddress:   fromEmail,
		FromName:      fromName,
		To:            request.Contact.Email,
		Subject:       subject,
		Content:       htmlContent,
		Provider:      request.EmailProvider,
		EmailOptions:  request.EmailOptions,
	}

	err = s.SendEmail(ctx, providerRequest, false)

	if err != nil {
		// Update message history with error status
		messageHistory.FailedAt = &now
		messageHistory.UpdatedAt = now
		errorMsg := err.Error()
		messageHistory.StatusInfo = &errorMsg

		// Attempt to update the message history record
		updateErr := s.messageRepo.Update(ctx, request.WorkspaceID, messageHistory)
		if updateErr != nil {
			s.logger.WithFields(map[string]interface{}{
				"error":      updateErr.Error(),
				"message_id": request.MessageID,
			}).Error("Failed to update message history with error status")

			tracing.AddAttribute(ctx, "message_history.update_error", updateErr.Error())
		}

		s.logger.WithFields(map[string]interface{}{
			"error":      err.Error(),
			"message_id": request.MessageID,
			"to":         request.Contact.Email,
		}).Error("Failed to send email")

		tracing.MarkSpanError(ctx, err)
		tracing.AddAttribute(ctx, "email.error", err.Error())
		return fmt.Errorf("failed to send email: %w", err)
	}

	s.logger.WithFields(map[string]interface{}{
		"message_id": request.MessageID,
		"to":         request.Contact.Email,
	}).Info("Email sent successfully")

	tracing.AddAttribute(ctx, "email.sent", true)
	return nil
}
