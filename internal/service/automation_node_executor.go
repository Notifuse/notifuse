package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/google/uuid"
)

// NodeExecutionResult contains the outcome of executing a node
type NodeExecutionResult struct {
	NextNodeID  *string                        // Which node to go to next (nil = completed)
	ScheduledAt *time.Time                     // When to process next (nil = now)
	Status      domain.ContactAutomationStatus // New status (active, completed, exited)
	Context     map[string]interface{}         // Updated context
	Output      map[string]interface{}         // Output for node execution log
	Error       error                          // Error if failed
}

// NodeExecutionParams contains all data needed to execute a node
type NodeExecutionParams struct {
	WorkspaceID      string
	Contact          *domain.ContactAutomation
	Node             *domain.AutomationNode
	Automation       *domain.Automation
	ContactData      *domain.Contact        // Full contact data for template rendering
	ExecutionContext map[string]interface{} // Reconstructed context from previous node executions
}

// NodeExecutor executes a specific node type
type NodeExecutor interface {
	Execute(ctx context.Context, params NodeExecutionParams) (*NodeExecutionResult, error)
	NodeType() domain.NodeType
}

// buildNodeOutput creates an output map with node_type included
func buildNodeOutput(nodeType domain.NodeType, data map[string]interface{}) map[string]interface{} {
	if data == nil {
		data = make(map[string]interface{})
	}
	data["node_type"] = string(nodeType)
	return data
}

// TriggerNodeExecutor handles trigger nodes (pass-through to next node)
// Trigger nodes are entry points - the actual trigger logic is handled by the
// database trigger during enrollment. This executor just advances to the next node.
type TriggerNodeExecutor struct{}

// NewTriggerNodeExecutor creates a new trigger node executor
func NewTriggerNodeExecutor() *TriggerNodeExecutor {
	return &TriggerNodeExecutor{}
}

// NodeType returns the node type this executor handles
func (e *TriggerNodeExecutor) NodeType() domain.NodeType {
	return domain.NodeTypeTrigger
}

// Execute passes through to the next node
func (e *TriggerNodeExecutor) Execute(ctx context.Context, params NodeExecutionParams) (*NodeExecutionResult, error) {
	return &NodeExecutionResult{
		NextNodeID: params.Node.NextNodeID,
		Status:     domain.ContactAutomationStatusActive,
		Output:     buildNodeOutput(domain.NodeTypeTrigger, map[string]interface{}{"trigger_type": "timeline"}),
	}, nil
}

// DelayNodeExecutor executes delay nodes
type DelayNodeExecutor struct{}

// NewDelayNodeExecutor creates a new delay node executor
func NewDelayNodeExecutor() *DelayNodeExecutor {
	return &DelayNodeExecutor{}
}

// NodeType returns the node type this executor handles
func (e *DelayNodeExecutor) NodeType() domain.NodeType {
	return domain.NodeTypeDelay
}

// Execute processes a delay node
func (e *DelayNodeExecutor) Execute(ctx context.Context, params NodeExecutionParams) (*NodeExecutionResult, error) {
	// Parse config
	config, err := parseDelayNodeConfig(params.Node.Config)
	if err != nil {
		return nil, fmt.Errorf("invalid delay node config: %w", err)
	}

	// Calculate scheduled time
	var duration time.Duration
	switch config.Unit {
	case "minutes":
		duration = time.Duration(config.Duration) * time.Minute
	case "hours":
		duration = time.Duration(config.Duration) * time.Hour
	case "days":
		duration = time.Duration(config.Duration) * 24 * time.Hour
	default:
		return nil, fmt.Errorf("invalid delay unit: %s", config.Unit)
	}

	scheduledAt := time.Now().UTC().Add(duration)

	return &NodeExecutionResult{
		NextNodeID:  params.Node.NextNodeID,
		ScheduledAt: &scheduledAt,
		Status:      domain.ContactAutomationStatusActive,
		Output: buildNodeOutput(domain.NodeTypeDelay, map[string]interface{}{
			"delay_duration": config.Duration,
			"delay_unit":     config.Unit,
			"delay_until":    scheduledAt,
		}),
	}, nil
}

// parseDelayNodeConfig parses delay node configuration from map
func parseDelayNodeConfig(config map[string]interface{}) (*domain.DelayNodeConfig, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	var c domain.DelayNodeConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := c.Validate(); err != nil {
		return nil, err
	}

	return &c, nil
}

// EmailNodeExecutor executes email nodes
type EmailNodeExecutor struct {
	emailQueueRepo domain.EmailQueueRepository
	templateRepo   domain.TemplateRepository
	workspaceRepo  domain.WorkspaceRepository
	apiEndpoint    string
	logger         logger.Logger
}

// NewEmailNodeExecutor creates a new email node executor
func NewEmailNodeExecutor(
	emailQueueRepo domain.EmailQueueRepository,
	templateRepo domain.TemplateRepository,
	workspaceRepo domain.WorkspaceRepository,
	apiEndpoint string,
	log logger.Logger,
) *EmailNodeExecutor {
	return &EmailNodeExecutor{
		emailQueueRepo: emailQueueRepo,
		templateRepo:   templateRepo,
		workspaceRepo:  workspaceRepo,
		apiEndpoint:    apiEndpoint,
		logger:         log,
	}
}

// NodeType returns the node type this executor handles
func (e *EmailNodeExecutor) NodeType() domain.NodeType {
	return domain.NodeTypeEmail
}

// Execute processes an email node by enqueuing to the email queue
func (e *EmailNodeExecutor) Execute(ctx context.Context, params NodeExecutionParams) (*NodeExecutionResult, error) {
	// 0. Validate required parameters
	if params.ContactData == nil {
		return nil, fmt.Errorf("contact data is required for email node")
	}
	if params.Automation == nil {
		return nil, fmt.Errorf("automation is required for email node")
	}

	// 1. Parse config
	config, err := parseEmailNodeConfig(params.Node.Config)
	if err != nil {
		return nil, fmt.Errorf("invalid email node config: %w", err)
	}

	// 2. Get workspace for email provider
	workspace, err := e.workspaceRepo.GetByID(ctx, params.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("workspace not found: %w", err)
	}

	// 3. Get email provider (use marketing email provider)
	emailProvider, integrationID, err := workspace.GetEmailProviderWithIntegrationID(true)
	if err != nil {
		return nil, fmt.Errorf("failed to get email provider: %w", err)
	}
	if emailProvider == nil {
		return nil, fmt.Errorf("no email provider configured for workspace")
	}

	// 4. Get template (version 0 means latest version)
	template, err := e.templateRepo.GetTemplateByID(ctx, params.WorkspaceID, config.TemplateID, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get template: %w", err)
	}

	// 5. Build template data from contact + automation
	templateData := buildAutomationTemplateData(params.ContactData, params.Automation)

	// 6. Generate message ID
	messageID := fmt.Sprintf("%s_%s", params.WorkspaceID, uuid.New().String())

	// 7. Setup tracking settings
	endpoint := e.apiEndpoint
	if workspace.Settings.CustomEndpointURL != nil && *workspace.Settings.CustomEndpointURL != "" {
		endpoint = *workspace.Settings.CustomEndpointURL
	}

	trackingSettings := notifuse_mjml.TrackingSettings{
		Endpoint:       endpoint,
		EnableTracking: workspace.Settings.EmailTrackingEnabled,
		UTMSource:      "automation",
		UTMMedium:      "email",
		UTMCampaign:    params.Automation.Name,
		UTMContent:     config.TemplateID,
		WorkspaceID:    params.WorkspaceID,
		MessageID:      messageID,
	}

	// 8. Compile template
	compiledTemplate, err := notifuse_mjml.CompileTemplate(
		notifuse_mjml.CompileTemplateRequest{
			WorkspaceID:      params.WorkspaceID,
			MessageID:        messageID,
			VisualEditorTree: template.Email.VisualEditorTree,
			TemplateData:     templateData,
			TrackingSettings: trackingSettings,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to compile template: %w", err)
	}
	if !compiledTemplate.Success || compiledTemplate.HTML == nil {
		errMsg := "template compilation failed"
		if compiledTemplate.Error != nil {
			errMsg = compiledTemplate.Error.Message
		}
		return nil, fmt.Errorf("%s", errMsg)
	}
	htmlContent := *compiledTemplate.HTML

	// 9. Process subject line through Liquid templating
	subject, err := notifuse_mjml.ProcessLiquidTemplate(
		template.Email.Subject,
		templateData,
		"email_subject",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to process subject: %w", err)
	}

	// 10. Get sender
	sender := emailProvider.GetSender(template.Email.SenderID)
	if sender == nil {
		return nil, fmt.Errorf("no sender configured for email provider")
	}

	// 11. Create queue entry
	entry := &domain.EmailQueueEntry{
		ID:            uuid.New().String(),
		Status:        domain.EmailQueueStatusPending,
		Priority:      domain.EmailQueuePriorityMarketing,
		SourceType:    domain.EmailQueueSourceAutomation,
		SourceID:      params.Automation.ID,
		IntegrationID: integrationID,
		ProviderKind:  emailProvider.Kind,
		ContactEmail:  params.ContactData.Email,
		MessageID:     messageID,
		TemplateID:    config.TemplateID,
		Payload: domain.EmailQueuePayload{
			FromAddress:        sender.Email,
			FromName:           sender.Name,
			Subject:            subject,
			HTMLContent:        htmlContent,
			RateLimitPerMinute: emailProvider.RateLimitPerMinute,
			EmailOptions: domain.EmailOptions{
				ReplyTo: template.Email.ReplyTo,
			},
		},
		MaxAttempts: 3,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	// 12. Enqueue the email
	if err := e.emailQueueRepo.Enqueue(ctx, params.WorkspaceID, []*domain.EmailQueueEntry{entry}); err != nil {
		return nil, fmt.Errorf("failed to enqueue email: %w", err)
	}

	e.logger.WithFields(map[string]interface{}{
		"workspace_id":  params.WorkspaceID,
		"automation_id": params.Automation.ID,
		"template_id":   config.TemplateID,
		"contact_email": params.ContactData.Email,
		"message_id":    messageID,
	}).Info("Email node executed - email enqueued")

	return &NodeExecutionResult{
		NextNodeID: params.Node.NextNodeID,
		Status:     domain.ContactAutomationStatusActive,
		Output: buildNodeOutput(domain.NodeTypeEmail, map[string]interface{}{
			"template_id": config.TemplateID,
			"message_id":  messageID,
			"to":          params.ContactData.Email,
			"queued":      true,
		}),
	}, nil
}

// parseEmailNodeConfig parses email node configuration from map
func parseEmailNodeConfig(config map[string]interface{}) (*domain.EmailNodeConfig, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	var c domain.EmailNodeConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := c.Validate(); err != nil {
		return nil, err
	}

	return &c, nil
}

// buildAutomationTemplateData creates template data for automation emails
func buildAutomationTemplateData(contact *domain.Contact, automation *domain.Automation) map[string]interface{} {
	data := make(map[string]interface{})

	if contact != nil {
		// Add contact fields
		data["email"] = contact.Email

		// Convert contact to map for proper liquid template rendering
		// This ensures NullableString fields are converted to plain strings
		contactData, err := contact.ToMapOfAny()
		if err != nil {
			// Fallback to empty map if conversion fails
			contactData = domain.MapOfAny{}
		}
		data["contact"] = contactData

		// Add standard contact fields if they exist (for backward compatibility)
		if contact.FirstName != nil && !contact.FirstName.IsNull {
			data["first_name"] = contact.FirstName.String
		}
		if contact.LastName != nil && !contact.LastName.IsNull {
			data["last_name"] = contact.LastName.String
		}
		if contact.FullName != nil && !contact.FullName.IsNull {
			data["full_name"] = contact.FullName.String
		}
		if contact.Country != nil && !contact.Country.IsNull {
			data["country"] = contact.Country.String
		}
	}

	if automation != nil {
		// Add automation context
		data["automation_id"] = automation.ID
		data["automation_name"] = automation.Name
	}

	return data
}

// BranchNodeExecutor executes branch nodes using database queries
type BranchNodeExecutor struct {
	queryBuilder  *QueryBuilder
	workspaceRepo domain.WorkspaceRepository
}

// NewBranchNodeExecutor creates a new branch node executor
func NewBranchNodeExecutor(queryBuilder *QueryBuilder, workspaceRepo domain.WorkspaceRepository) *BranchNodeExecutor {
	return &BranchNodeExecutor{
		queryBuilder:  queryBuilder,
		workspaceRepo: workspaceRepo,
	}
}

// NodeType returns the node type this executor handles
func (e *BranchNodeExecutor) NodeType() domain.NodeType {
	return domain.NodeTypeBranch
}

// Execute processes a branch node
func (e *BranchNodeExecutor) Execute(ctx context.Context, params NodeExecutionParams) (*NodeExecutionResult, error) {
	config, err := parseBranchNodeConfig(params.Node.Config)
	if err != nil {
		return nil, fmt.Errorf("invalid branch node config: %w", err)
	}

	// Get workspace DB connection for query execution
	db, err := e.workspaceRepo.GetConnection(ctx, params.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get db connection: %w", err)
	}

	// Evaluate each path's conditions against contact using database query
	for _, path := range config.Paths {
		if path.Conditions == nil {
			continue
		}

		matches, err := e.evaluateConditionsWithDB(ctx, db, params.ContactData.Email, path.Conditions)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate path %s: %w", path.ID, err)
		}

		if matches {
			nextNodeID := path.NextNodeID
			return &NodeExecutionResult{
				NextNodeID: &nextNodeID,
				Status:     domain.ContactAutomationStatusActive,
				Output: buildNodeOutput(domain.NodeTypeBranch, map[string]interface{}{
					"path_taken": path.ID,
					"path_name":  path.Name,
				}),
			}, nil
		}
	}

	// Fall through to default path
	defaultPath := findDefaultPath(config.Paths, config.DefaultPathID)
	if defaultPath != nil {
		nextNodeID := defaultPath.NextNodeID
		return &NodeExecutionResult{
			NextNodeID: &nextNodeID,
			Status:     domain.ContactAutomationStatusActive,
			Output: buildNodeOutput(domain.NodeTypeBranch, map[string]interface{}{
				"path_taken": "default",
			}),
		}, nil
	}

	// No default path found, complete the automation
	return &NodeExecutionResult{
		NextNodeID: nil,
		Status:     domain.ContactAutomationStatusCompleted,
		Output: buildNodeOutput(domain.NodeTypeBranch, map[string]interface{}{
			"path_taken": "none",
		}),
	}, nil
}

// evaluateConditionsWithDB uses QueryBuilder to check if contact matches conditions
func (e *BranchNodeExecutor) evaluateConditionsWithDB(ctx context.Context, db *sql.DB, email string, conditions *domain.TreeNode) (bool, error) {
	// Build SQL using QueryBuilder (same as segments/triggers)
	sqlStr, args, err := e.queryBuilder.BuildSQL(conditions)
	if err != nil {
		return false, err
	}

	// Wrap in EXISTS with email filter
	// The QueryBuilder returns a SELECT ... FROM contacts ... WHERE ... query
	// We need to add the email filter
	checkSQL := fmt.Sprintf("SELECT EXISTS (%s AND email = $%d)", sqlStr, len(args)+1)
	args = append(args, email)

	var exists bool
	err = db.QueryRowContext(ctx, checkSQL, args...).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("condition query failed: %w", err)
	}

	return exists, nil
}

// parseBranchNodeConfig parses branch node configuration from map
func parseBranchNodeConfig(config map[string]interface{}) (*domain.BranchNodeConfig, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	var c domain.BranchNodeConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &c, nil
}

// findDefaultPath finds the default path in a list of branch paths
func findDefaultPath(paths []domain.BranchPath, defaultPathID string) *domain.BranchPath {
	for i := range paths {
		if paths[i].ID == defaultPathID {
			return &paths[i]
		}
	}
	// Return first path if no default found
	if len(paths) > 0 {
		return &paths[0]
	}
	return nil
}

// FilterNodeExecutor executes filter nodes using database queries
type FilterNodeExecutor struct {
	queryBuilder  *QueryBuilder
	workspaceRepo domain.WorkspaceRepository
}

// NewFilterNodeExecutor creates a new filter node executor
func NewFilterNodeExecutor(queryBuilder *QueryBuilder, workspaceRepo domain.WorkspaceRepository) *FilterNodeExecutor {
	return &FilterNodeExecutor{
		queryBuilder:  queryBuilder,
		workspaceRepo: workspaceRepo,
	}
}

// NodeType returns the node type this executor handles
func (e *FilterNodeExecutor) NodeType() domain.NodeType {
	return domain.NodeTypeFilter
}

// Execute processes a filter node
func (e *FilterNodeExecutor) Execute(ctx context.Context, params NodeExecutionParams) (*NodeExecutionResult, error) {
	config, err := parseFilterNodeConfig(params.Node.Config)
	if err != nil {
		return nil, fmt.Errorf("invalid filter node config: %w", err)
	}

	// Get workspace DB connection
	db, err := e.workspaceRepo.GetConnection(ctx, params.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get db connection: %w", err)
	}

	// Evaluate conditions using database query
	matches, err := e.evaluateConditionsWithDB(ctx, db, params.ContactData.Email, config.Conditions)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate filter: %w", err)
	}

	if matches {
		// Filter passed - continue to next node (or complete if empty)
		var nextNodeID *string
		if config.ContinueNodeID != "" {
			nextNodeID = &config.ContinueNodeID
		}
		status := domain.ContactAutomationStatusActive
		if nextNodeID == nil {
			status = domain.ContactAutomationStatusCompleted
		}
		return &NodeExecutionResult{
			NextNodeID: nextNodeID,
			Status:     status,
			Output:     buildNodeOutput(domain.NodeTypeFilter, map[string]interface{}{"filter_passed": true}),
		}, nil
	}

	// Filter failed - go to rejection path (or complete if empty)
	var nextNodeID *string
	if config.ExitNodeID != "" {
		nextNodeID = &config.ExitNodeID
	}
	status := domain.ContactAutomationStatusActive
	if nextNodeID == nil {
		status = domain.ContactAutomationStatusCompleted
	}
	return &NodeExecutionResult{
		NextNodeID: nextNodeID,
		Status:     status,
		Output:     buildNodeOutput(domain.NodeTypeFilter, map[string]interface{}{"filter_passed": false}),
	}, nil
}

// evaluateConditionsWithDB uses QueryBuilder to check if contact matches conditions
func (e *FilterNodeExecutor) evaluateConditionsWithDB(ctx context.Context, db *sql.DB, email string, conditions *domain.TreeNode) (bool, error) {
	sqlStr, args, err := e.queryBuilder.BuildSQL(conditions)
	if err != nil {
		return false, err
	}

	checkSQL := fmt.Sprintf("SELECT EXISTS (%s AND email = $%d)", sqlStr, len(args)+1)
	args = append(args, email)

	var exists bool
	err = db.QueryRowContext(ctx, checkSQL, args...).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("condition query failed: %w", err)
	}

	return exists, nil
}

// parseFilterNodeConfig parses filter node configuration from map
func parseFilterNodeConfig(config map[string]interface{}) (*domain.FilterNodeConfig, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	var c domain.FilterNodeConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &c, nil
}

// AddToListNodeExecutor executes add-to-list nodes
type AddToListNodeExecutor struct {
	contactListRepo domain.ContactListRepository
}

// NewAddToListNodeExecutor creates a new add-to-list node executor
func NewAddToListNodeExecutor(contactListRepo domain.ContactListRepository) *AddToListNodeExecutor {
	return &AddToListNodeExecutor{
		contactListRepo: contactListRepo,
	}
}

// NodeType returns the node type this executor handles
func (e *AddToListNodeExecutor) NodeType() domain.NodeType {
	return domain.NodeTypeAddToList
}

// Execute processes an add-to-list node
func (e *AddToListNodeExecutor) Execute(ctx context.Context, params NodeExecutionParams) (*NodeExecutionResult, error) {
	config, err := parseAddToListNodeConfig(params.Node.Config)
	if err != nil {
		return nil, fmt.Errorf("invalid add-to-list node config: %w", err)
	}

	// Add contact to list
	now := time.Now().UTC()
	contactList := &domain.ContactList{
		Email:     params.Contact.ContactEmail,
		ListID:    config.ListID,
		Status:    domain.ContactListStatus(config.Status),
		CreatedAt: now,
		UpdatedAt: now,
	}

	err = e.contactListRepo.AddContactToList(ctx, params.WorkspaceID, contactList)
	if err != nil {
		// Log but don't fail - contact might already be in list
		return &NodeExecutionResult{
			NextNodeID: params.Node.NextNodeID,
			Status:     domain.ContactAutomationStatusActive,
			Output: buildNodeOutput(domain.NodeTypeAddToList, map[string]interface{}{
				"list_id": config.ListID,
				"status":  config.Status,
				"error":   err.Error(),
			}),
		}, nil
	}

	return &NodeExecutionResult{
		NextNodeID: params.Node.NextNodeID,
		Status:     domain.ContactAutomationStatusActive,
		Output: buildNodeOutput(domain.NodeTypeAddToList, map[string]interface{}{
			"list_id": config.ListID,
			"status":  config.Status,
		}),
	}, nil
}

// parseAddToListNodeConfig parses add-to-list node configuration from map
func parseAddToListNodeConfig(config map[string]interface{}) (*domain.AddToListNodeConfig, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	var c domain.AddToListNodeConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := c.Validate(); err != nil {
		return nil, err
	}

	return &c, nil
}

// RemoveFromListNodeExecutor executes remove-from-list nodes
type RemoveFromListNodeExecutor struct {
	contactListRepo domain.ContactListRepository
}

// NewRemoveFromListNodeExecutor creates a new remove-from-list node executor
func NewRemoveFromListNodeExecutor(contactListRepo domain.ContactListRepository) *RemoveFromListNodeExecutor {
	return &RemoveFromListNodeExecutor{
		contactListRepo: contactListRepo,
	}
}

// NodeType returns the node type this executor handles
func (e *RemoveFromListNodeExecutor) NodeType() domain.NodeType {
	return domain.NodeTypeRemoveFromList
}

// Execute processes a remove-from-list node
func (e *RemoveFromListNodeExecutor) Execute(ctx context.Context, params NodeExecutionParams) (*NodeExecutionResult, error) {
	config, err := parseRemoveFromListNodeConfig(params.Node.Config)
	if err != nil {
		return nil, fmt.Errorf("invalid remove-from-list node config: %w", err)
	}

	// Remove contact from list
	err = e.contactListRepo.RemoveContactFromList(ctx, params.WorkspaceID, params.Contact.ContactEmail, config.ListID)
	if err != nil {
		// Log but don't fail - contact might not be in list
		return &NodeExecutionResult{
			NextNodeID: params.Node.NextNodeID,
			Status:     domain.ContactAutomationStatusActive,
			Output: buildNodeOutput(domain.NodeTypeRemoveFromList, map[string]interface{}{
				"list_id": config.ListID,
				"error":   err.Error(),
			}),
		}, nil
	}

	return &NodeExecutionResult{
		NextNodeID: params.Node.NextNodeID,
		Status:     domain.ContactAutomationStatusActive,
		Output: buildNodeOutput(domain.NodeTypeRemoveFromList, map[string]interface{}{
			"list_id": config.ListID,
		}),
	}, nil
}

// parseRemoveFromListNodeConfig parses remove-from-list node configuration from map
func parseRemoveFromListNodeConfig(config map[string]interface{}) (*domain.RemoveFromListNodeConfig, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	var c domain.RemoveFromListNodeConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := c.Validate(); err != nil {
		return nil, err
	}

	return &c, nil
}

// ListStatusBranchNodeExecutor executes list status branch nodes
type ListStatusBranchNodeExecutor struct {
	contactListRepo domain.ContactListRepository
}

// NewListStatusBranchNodeExecutor creates a new list status branch node executor
func NewListStatusBranchNodeExecutor(contactListRepo domain.ContactListRepository) *ListStatusBranchNodeExecutor {
	return &ListStatusBranchNodeExecutor{
		contactListRepo: contactListRepo,
	}
}

// NodeType returns the node type this executor handles
func (e *ListStatusBranchNodeExecutor) NodeType() domain.NodeType {
	return domain.NodeTypeListStatusBranch
}

// Execute processes a list status branch node
func (e *ListStatusBranchNodeExecutor) Execute(ctx context.Context, params NodeExecutionParams) (*NodeExecutionResult, error) {
	config, err := parseListStatusBranchNodeConfig(params.Node.Config)
	if err != nil {
		return nil, fmt.Errorf("invalid list_status_branch node config: %w", err)
	}

	// Query contact's status in the specified list
	contactList, err := e.contactListRepo.GetContactListByIDs(
		ctx,
		params.WorkspaceID,
		params.Contact.ContactEmail,
		config.ListID,
	)

	var nextNodeID string
	var branchTaken string
	var contactStatus string

	if err != nil {
		// Check if error is "not found" - means contact is not in list
		if _, ok := err.(*domain.ErrContactListNotFound); ok {
			nextNodeID = config.NotInListNodeID
			branchTaken = "not_in_list"
			contactStatus = "not_found"
		} else {
			// Actual error - propagate it
			return nil, fmt.Errorf("failed to check contact list status: %w", err)
		}
	} else {
		// Contact found in list - check status
		contactStatus = string(contactList.Status)
		if contactList.Status == domain.ContactListStatusActive {
			nextNodeID = config.ActiveNodeID
			branchTaken = "active"
		} else {
			// Any non-active status: pending, unsubscribed, bounced, complained
			nextNodeID = config.NonActiveNodeID
			branchTaken = "non_active"
		}
	}

	// Handle case where branch has no target (terminal)
	var nextNodePtr *string
	if nextNodeID != "" {
		nextNodePtr = &nextNodeID
	}

	status := domain.ContactAutomationStatusActive
	if nextNodePtr == nil {
		status = domain.ContactAutomationStatusCompleted
	}

	return &NodeExecutionResult{
		NextNodeID: nextNodePtr,
		Status:     status,
		Output: buildNodeOutput(domain.NodeTypeListStatusBranch, map[string]interface{}{
			"list_id":        config.ListID,
			"branch_taken":   branchTaken,
			"contact_status": contactStatus,
		}),
	}, nil
}

// parseListStatusBranchNodeConfig parses list status branch node configuration from map
func parseListStatusBranchNodeConfig(config map[string]interface{}) (*domain.ListStatusBranchNodeConfig, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	var c domain.ListStatusBranchNodeConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := c.Validate(); err != nil {
		return nil, err
	}

	return &c, nil
}

// ABTestNodeExecutor executes A/B test nodes
type ABTestNodeExecutor struct{}

// NewABTestNodeExecutor creates a new A/B test node executor
func NewABTestNodeExecutor() *ABTestNodeExecutor {
	return &ABTestNodeExecutor{}
}

// NodeType returns the node type this executor handles
func (e *ABTestNodeExecutor) NodeType() domain.NodeType {
	return domain.NodeTypeABTest
}

// Execute processes an A/B test node using deterministic variant selection
func (e *ABTestNodeExecutor) Execute(ctx context.Context, params NodeExecutionParams) (*NodeExecutionResult, error) {
	config, err := parseABTestNodeConfig(params.Node.Config)
	if err != nil {
		return nil, fmt.Errorf("invalid ab_test node config: %w", err)
	}

	// Select variant deterministically based on email + nodeID
	variant := e.selectVariantDeterministic(
		params.Contact.ContactEmail,
		params.Node.ID,
		config.Variants,
	)

	return &NodeExecutionResult{
		NextNodeID: &variant.NextNodeID,
		Status:     domain.ContactAutomationStatusActive,
		Output: buildNodeOutput(domain.NodeTypeABTest, map[string]interface{}{
			"variant_id":   variant.ID,
			"variant_name": variant.Name,
		}),
	}, nil
}

// selectVariantDeterministic selects a variant using deterministic hashing
// Same email + nodeID will always result in the same variant
func (e *ABTestNodeExecutor) selectVariantDeterministic(email, nodeID string, variants []domain.ABTestVariant) domain.ABTestVariant {
	// Use FNV-32a hash for deterministic selection
	h := fnv32a(email + nodeID)
	roll := int(h % 100)

	cumulative := 0
	for _, v := range variants {
		cumulative += v.Weight
		if roll < cumulative {
			return v
		}
	}
	// Fallback to last variant if weights don't sum to 100
	return variants[len(variants)-1]
}

// fnv32a computes FNV-1a 32-bit hash
func fnv32a(s string) uint32 {
	const prime32 = 16777619
	const offset32 = 2166136261

	h := uint32(offset32)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= prime32
	}
	return h
}

// parseABTestNodeConfig parses A/B test node configuration from map
func parseABTestNodeConfig(config map[string]interface{}) (*domain.ABTestNodeConfig, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	var c domain.ABTestNodeConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := c.Validate(); err != nil {
		return nil, err
	}

	return &c, nil
}

// WebhookNodeExecutor executes webhook nodes
type WebhookNodeExecutor struct {
	httpClient *http.Client
	logger     logger.Logger
}

// NewWebhookNodeExecutor creates a new webhook node executor
func NewWebhookNodeExecutor(log logger.Logger) *WebhookNodeExecutor {
	return &WebhookNodeExecutor{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		logger:     log,
	}
}

// NodeType returns the node type this executor handles
func (e *WebhookNodeExecutor) NodeType() domain.NodeType {
	return domain.NodeTypeWebhook
}

// Execute processes a webhook node
func (e *WebhookNodeExecutor) Execute(ctx context.Context, params NodeExecutionParams) (*NodeExecutionResult, error) {
	// 1. Parse config
	config, err := parseWebhookNodeConfig(params.Node.Config)
	if err != nil {
		return nil, fmt.Errorf("invalid webhook node config: %w", err)
	}

	// 2. Build payload with contact data
	payload := buildWebhookPayload(params.ContactData, params.Automation, params.Node.ID)

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	// 3. Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.URL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create webhook request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if config.Secret != nil && *config.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+*config.Secret)
	}

	// 4. Make HTTP POST request
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body (limit to 10KB)
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read webhook response: %w", err)
	}

	// 5. Handle response status
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		// 4xx - client error, fail immediately (won't be fixed by retry)
		return nil, fmt.Errorf("webhook returned client error: %d %s", resp.StatusCode, string(bodyBytes))
	}
	if resp.StatusCode >= 500 {
		// 5xx - server error, return error to trigger retry via existing backoff
		return nil, fmt.Errorf("webhook returned server error: %d %s", resp.StatusCode, string(bodyBytes))
	}

	// 6. Parse JSON response for context storage
	var responseData map[string]interface{}
	if len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, &responseData); err != nil {
			// If response isn't valid JSON, store as raw string
			responseData = map[string]interface{}{
				"raw": string(bodyBytes),
			}
		}
	}

	e.logger.WithFields(map[string]interface{}{
		"workspace_id":  params.WorkspaceID,
		"automation_id": params.Automation.ID,
		"url":           config.URL,
		"status_code":   resp.StatusCode,
	}).Info("Webhook node executed successfully")

	return &NodeExecutionResult{
		NextNodeID: params.Node.NextNodeID,
		Status:     domain.ContactAutomationStatusActive,
		Output: buildNodeOutput(domain.NodeTypeWebhook, map[string]interface{}{
			"url":         config.URL,
			"status_code": resp.StatusCode,
			"response":    responseData,
		}),
	}, nil
}

// buildWebhookPayload creates the payload for webhook requests
func buildWebhookPayload(contact *domain.Contact, automation *domain.Automation, nodeID string) map[string]interface{} {
	payload := map[string]interface{}{
		"node_id":   nodeID,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	if contact != nil {
		payload["email"] = contact.Email
		payload["contact"] = contact
	}

	if automation != nil {
		payload["automation_id"] = automation.ID
		payload["automation_name"] = automation.Name
	}

	return payload
}

// parseWebhookNodeConfig parses webhook node configuration from map
func parseWebhookNodeConfig(config map[string]interface{}) (*domain.WebhookNodeConfig, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	var c domain.WebhookNodeConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := c.Validate(); err != nil {
		return nil, err
	}

	return &c, nil
}
