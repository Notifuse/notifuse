package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/Notifuse/notifuse/pkg/tracing"
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
	sendGridService  domain.EmailProviderService
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
	// Initialize OAuth2 token service for SMTP OAuth2 support
	oauth2TokenService := NewOAuth2TokenService(logger)

	// Initialize provider services
	smtpService := NewSMTPServiceWithOAuth2(logger, oauth2TokenService)
	sesService := NewSESService(authService, logger)
	sparkPostService := NewSparkPostService(httpClient, authService, logger)
	postmarkService := NewPostmarkService(httpClient, authService, logger)
	mailgunService := NewMailgunService(httpClient, authService, logger, webhookEndpoint)
	mailjetService := NewMailjetService(httpClient, authService, logger)
	sendGridService := NewSendGridService(httpClient, authService, logger)

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
		sendGridService:  sendGridService,
	}
}

// TestEmailProvider sends a test email to verify the provider configuration works
func (s *EmailService) TestEmailProvider(ctx context.Context, workspaceID string, integrationID string, provider domain.EmailProvider, to string) error {
	ctx, span := tracing.StartServiceSpan(ctx, "EmailService", "TestEmailProvider")
	defer tracing.EndSpan(span, nil)

	// Authenticate user
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		return err
	}

	// This sends a real email through the workspace's stored provider credentials,
	// so it is a send and gated like one.
	if !userWorkspace.HasPermission(domain.PermissionResourceTransactional, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceTransactional,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to transactional required",
		)
	}

	// Fill in credentials the client could not send. Workspaces do not serve
	// decrypted credentials, so a client testing a SAVED integration posts blanks
	// — and a test that authenticates with an empty key fails against
	// configuration that works, which accuses the wrong thing.
	//
	// Skipped when no integration is named: the provider is not saved yet, so the
	// client still holds whatever it typed and there is nothing to fill from.
	if integrationID != "" {
		if workspace, wsErr := s.workspaceRepo.GetByID(ctx, workspaceID); wsErr == nil && workspace != nil {
			if stored := workspace.GetIntegrationByID(integrationID); stored != nil {
				hydrateEmailProviderCredentials(&provider, &stored.EmailProvider)
			}
		}
		// A lookup failure is deliberately not fatal: the test then runs with
		// whatever the client sent and fails on its own terms, which is a clearer
		// report than an error about loading a workspace.
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
	case domain.EmailProviderKindSendGrid:
		return s.sendGridService, nil
	default:
		return nil, fmt.Errorf("unsupported provider kind: %s", providerKind)
	}
}

// maxRecordedClickedURLLength bounds the destination URLs persisted in clicked_links
const maxRecordedClickedURLLength = 2048

func (s *EmailService) VisitLink(ctx context.Context, messageID string, workspaceID string, clickedURL string, requestHost string) error {
	// find the message by id
	err := s.messageRepo.SetClicked(ctx, workspaceID, messageID, time.Now(), sanitizeClickedURL(clickedURL, requestHost))
	if err != nil {
		s.logger.Error(err.Error())
		return fmt.Errorf("failed to set clicked: %w", err)
	}

	return nil
}

// sanitizeClickedURL decides whether a clicked destination URL may be recorded
// per-link; it returns "" (aggregate-only) when the URL is not a plausible
// http(s) destination or when it points back at the host serving the click
// redirect: all links of one email (tracking redirect, unsubscribe,
// notification center) are built from the same send-time endpoint, and the
// unsubscribe/notification-center ones embed the recipient's raw email
// address, which must not be persisted in clicked_links keys. Comparing
// against the request host identifies that send-time endpoint without a
// workspace lookup on the hot click path, and stays correct when the
// workspace endpoint is later reconfigured.
//
// It also strips the web analytics identity token from the URLs it does keep.
// A recorded URL becomes a JSONB *key* in clicked_links and per-link reporting
// aggregates those keys across a broadcast, but the token is minted per
// recipient over a fresh random nonce: left in place it would turn one row per
// link into one row per recipient, and persist a bearer identity credential in
// the workspace database — the same two hazards the request-host check avoids
// for the unsubscribe links.
func sanitizeClickedURL(clickedURL string, requestHost string) string {
	if clickedURL == "" {
		return ""
	}

	parsed, err := url.Parse(clickedURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}

	if requestHost != "" && strings.EqualFold(parsed.Host, requestHost) {
		return ""
	}

	// The parameter name has to be the one notifuse_mjml WRITES onto a tracked
	// link, not a copy of the literal: a second definition drifting from the
	// first would silently stop matching, and the per-recipient credential the
	// strip exists to remove would become a clicked_links JSONB key.
	recorded := stripQueryParam(clickedURL, domain.WebIdentifyQueryParam)

	// The cap is measured on what we are about to persist, not on what arrived:
	// it exists to bound the clicked_links key, and the token adds ~150
	// characters that never reach that key. Checking the raw URL first would
	// drop an otherwise recordable click for the length of a parameter this
	// function has just removed.
	if len(recorded) > maxRecordedClickedURLLength {
		return ""
	}

	return recorded
}

// stripQueryParam removes every occurrence of one query parameter from a URL,
// working on the raw string rather than re-serialising a parsed url.URL. A
// template link can reach here still carrying an unrendered Liquid placeholder,
// and url.URL.String() would percent-escape its braces and spaces — recording a
// link under a key that no longer matches the one the template ships. Raw
// surgery also keeps parameter order, the fragment, userinfo and the port
// exactly as they were, and leaves URLs without the parameter byte-identical,
// so keys recorded before this existed keep aggregating with new ones.
func stripQueryParam(rawURL string, name string) string {
	// The query is what sits between the first '?' and the fragment. Anything
	// after '#' stays untouched: a token there is not what the SDK reads, and
	// the fragment never reaches the customer's server anyway.
	head, fragment := rawURL, ""
	if hash := strings.IndexByte(rawURL, '#'); hash >= 0 {
		head, fragment = rawURL[:hash], rawURL[hash:]
	}

	mark := strings.IndexByte(head, '?')
	if mark < 0 {
		return rawURL
	}

	base, query := head[:mark], head[mark+1:]
	pairs := strings.Split(query, "&")
	kept := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		key := pair
		if eq := strings.IndexByte(pair, '='); eq >= 0 {
			key = pair[:eq]
		}
		if key == name {
			continue
		}
		kept = append(kept, pair)
	}

	if len(kept) == len(pairs) {
		return rawURL
	}
	if len(kept) == 0 {
		// The '?' goes with the last parameter: a link that carried nothing but
		// the token must record as the bare destination, not as "…/product?".
		return base + fragment
	}

	return base + "?" + strings.Join(kept, "&") + fragment
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

	// Resolve language variant based on contact's language
	contactLang := ""
	if request.Contact != nil && request.Contact.Language != nil && !request.Contact.Language.IsNull {
		contactLang = request.Contact.Language.String
	}

	// Get workspace to check for custom endpoint URL and default language
	workspace, err := s.workspaceRepo.GetByID(ctx, request.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace: %w", err)
	}

	emailContent := template.ResolveEmailContent(contactLang, workspace.Settings.DefaultLanguage)

	// Find the emailSender
	emailSender := request.EmailProvider.GetSender(emailContent.SenderID)

	if emailSender == nil {
		return fmt.Errorf("sender not found: %s", emailContent.SenderID)
	}

	span.AddAttributes(
		trace.StringAttribute("template.subject", emailContent.Subject),
		trace.StringAttribute("template.from_email", emailSender.Email),
	)

	// set utm_content to the template id if not set
	if request.TrackingSettings.UTMContent == "" {
		request.TrackingSettings.UTMContent = template.ID
	}

	// Resolve the tracking/base endpoint: custom endpoint if set, else the API endpoint.
	endpoint := workspace.Settings.ResolveEndpoint(s.apiEndpoint)

	// Mint the web analytics identity for THIS recipient, so the rebuild below
	// can carry it as one more field: what is not listed in that literal is what
	// gets silently dropped.
	//
	// The gate is the feature being on AND at least one declared destination.
	// The emptiness check is deliberately not delegated: an empty allowlist means
	// "accept beats from any host" to the tracker, and carrying that reading over
	// here would append a contact's identity to every link in the email, whoever
	// it points at. The per-link host match runs later, in TrackLinks, against
	// these same hosts.
	//
	// The third condition is who receives the message. The token names exactly
	// one contact, every tracked link in the body carries that same token, and
	// this send delivers the body to To plus every CC and BCC address — which
	// arrive verbatim from the transactional.send request (and from the SMTP
	// bridge's own headers, see smtp_bridge_handler.go). So a notification
	// configured with a CC would hand each of those inboxes a working, week-long
	// bearer identity for the actual recipient: whoever clicks from there is
	// recorded as that contact, and with the contact bridge on, their web goals
	// become that contact's timeline entries, segment recomputations and
	// automation enrolments. Nothing downstream can tell those clicks apart, so
	// the only safe reading is that a message with additional recipients cannot
	// promise it reaches only the contact it identifies. No token at all costs
	// that send its visit attribution, which is the cheaper failure.
	singleRecipient := len(request.EmailOptions.CC) == 0 && len(request.EmailOptions.BCC) == 0

	// The last condition is the per-notification opt-out. TrackLinks returns the
	// HTML untouched on TrackingModeDisabled — no redirect, no UTM and no nf_id —
	// so a token minted for such a send is a live credential, valid for
	// domain.WebIdentifyTokenTTL, that no link in the email can ever carry. The
	// Supabase auth notifications are all configured that way, so without this
	// term every signup confirmation, magic link and recovery mail on a workspace
	// running web analytics mints one and throws it away.
	//
	// Only the explicit opt-out counts here: an absent mode and "inherit" are the
	// same state, and EnableTracking being false is not one — a workspace can run
	// web analytics with email click tracking off and still need the recipient
	// identified on landing, which is why TrackLinks treats an identity token as
	// its own reason to rewrite the links.
	trackingOptedOut := request.TrackingSettings.TrackingMode == notifuse_mjml.TrackingModeDisabled

	var identifyToken string
	var identifyAllowedHosts []string
	if wa := workspace.Settings.WebAnalytics; singleRecipient && !trackingOptedOut && wa.CanIdentifyFromEmailLinks() {
		token, err := domain.BuildWebIdentifyToken(
			request.Contact.Email,
			workspace.Settings.SecretKey,
			domain.WebIdentifyTokenTTL,
			time.Now().UTC(),
		)
		if err != nil {
			// Identity is an analytics enrichment on top of the send; the send is
			// the legitimate work. A token that cannot be built costs the visit its
			// attribution, never the email.
			s.logger.WithFields(map[string]interface{}{
				"error":      err.Error(),
				"workspace":  request.WorkspaceID,
				"message_id": request.MessageID,
			}).Error("Failed to mint the web analytics identity token, sending without it")
		} else {
			identifyToken = token
			identifyAllowedHosts = wa.AllowedDomains
		}
	}

	trackingSettings := notifuse_mjml.TrackingSettings{
		Endpoint:       endpoint,
		EnableTracking: request.TrackingSettings.EnableTracking,
		// TrackingMode must survive this rebuild: it carries the per-notification
		// full veto that TrackLinks enforces (no redirect, no pixel, no UTM) —
		// without it, the UTMContent default above would still rewrite opted-out
		// single-use auth URLs.
		TrackingMode: request.TrackingSettings.TrackingMode,
		UTMSource:    request.TrackingSettings.UTMSource,
		UTMMedium:    request.TrackingSettings.UTMMedium,
		UTMCampaign:  request.TrackingSettings.UTMCampaign,
		UTMContent:   request.TrackingSettings.UTMContent,
		UTMTerm:      request.TrackingSettings.UTMTerm,
		WorkspaceID:  request.WorkspaceID,
		MessageID:    request.MessageID,
		// Request-scoped, and both json:"-": the credential reaches the compiler
		// without ever reaching a stored tracking_settings row.
		IdentifyToken:        identifyToken,
		IdentifyAllowedHosts: identifyAllowedHosts,
	}

	compileTemplateRequest := domain.CompileTemplateRequest{
		WorkspaceID:      request.WorkspaceID,
		MessageID:        request.MessageID,
		TemplateData:     request.MessageData.Data,
		TrackingSettings: trackingSettings,
	}
	// Wires the resolved variant's tree/source + its inbox-preview override;
	// an explicit per-send override from EmailOptions still wins.
	emailContent.ApplyToCompileRequest(&compileTemplateRequest, request.EmailOptions.SubjectPreview)

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
		emailContent.Subject,
		request.MessageData.Data,
		"email_subject",
	)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"error":       err.Error(),
			"message_id":  request.MessageID,
			"template_id": request.TemplateConfig.TemplateID,
			"subject":     emailContent.Subject,
		}).Error("Failed to process subject line with Liquid templating")
		tracing.MarkSpanError(ctx, err)
		return fmt.Errorf("failed to process subject with Liquid: %w", err)
	}

	// Allow override of subject via email options
	if request.EmailOptions.Subject != nil && *request.EmailOptions.Subject != "" {
		overrideSubject, err := notifuse_mjml.ProcessLiquidTemplate(
			*request.EmailOptions.Subject,
			request.MessageData.Data,
			"email_subject_override",
		)
		if err != nil {
			s.logger.WithFields(map[string]interface{}{
				"error":       err.Error(),
				"message_id":  request.MessageID,
				"template_id": request.TemplateConfig.TemplateID,
				"subject":     *request.EmailOptions.Subject,
			}).Error("Failed to process subject override with Liquid templating")
			tracing.MarkSpanError(ctx, err)
			return fmt.Errorf("failed to process subject override with Liquid: %w", err)
		}
		s.logger.WithFields(map[string]interface{}{
			"message_id":       request.MessageID,
			"default_subject":  subject,
			"override_subject": overrideSubject,
		}).Debug("Using subject override")
		subject = overrideSubject
	}

	htmlContent := *compiledTemplate.HTML

	now := time.Now().UTC()

	// Convert email options to channel options for storage
	channelOptions := request.EmailOptions.ToChannelOptions()

	// Create message history record
	messageHistory := &domain.MessageHistory{
		ID:                          request.MessageID,
		ExternalID:                  request.ExternalID,
		ContactEmail:                request.Contact.Email,
		AutomationID:                request.AutomationID,
		TransactionalNotificationID: request.TransactionalNotificationID,
		TemplateID:                  request.TemplateConfig.TemplateID,
		Channel:                     "email",
		MessageData:                 request.MessageData,
		ChannelOptions:              channelOptions,
		SentAt:                      now,
		CreatedAt:                   now,
		UpdatedAt:                   now,
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
	if emailContent.ReplyTo != "" {
		request.EmailOptions.ReplyTo = emailContent.ReplyTo
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
		// Record the failure with a targeted status write, not a whole-row update.
		// Create() encrypted message_data on the way in and left this struct
		// holding the plaintext template data, so writing the row back from it
		// would store the blob in clear — and would restore every other column
		// from a copy taken before the send. The queue worker's upsert leaves the
		// same columns alone on a retry for the same reason.
		errorMsg := err.Error()
		updateErr := s.messageRepo.SetStatusesIfNotSet(ctx, request.WorkspaceID, []domain.MessageEventUpdate{{
			ID:         request.MessageID,
			Event:      domain.MessageEventFailed,
			Timestamp:  now,
			StatusInfo: &errorMsg,
		}})
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
