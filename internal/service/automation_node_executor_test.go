package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupMockLoggerForNodeExecutor sets up a mock logger for tests
func setupMockLoggerForNodeExecutor(ctrl *gomock.Controller) *pkgmocks.MockLogger {
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	return mockLogger
}

// createTestWorkspaceWithEmailProvider creates a test workspace with email provider configured
func createTestWorkspaceWithEmailProvider() *domain.Workspace {
	integrationID := "integration123"
	return &domain.Workspace{
		ID:   "ws1",
		Name: "Test Workspace",
		Settings: domain.WorkspaceSettings{
			EmailTrackingEnabled:     true,
			MarketingEmailProviderID: integrationID,
		},
		Integrations: []domain.Integration{
			{
				ID:   integrationID,
				Name: "Test Email Provider",
				Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind:               domain.EmailProviderKindSMTP,
					RateLimitPerMinute: 60,
					Senders: []domain.EmailSender{
						{
							ID:        "sender1",
							Email:     "sender@example.com",
							Name:      "Test Sender",
							IsDefault: true,
						},
					},
					SMTP: &domain.SMTPSettings{
						Host:     "smtp.example.com",
						Port:     587,
						Username: "user",
						Password: "pass",
					},
				},
			},
		},
	}
}

func TestDelayNodeExecutor_Execute(t *testing.T) {
	executor := NewDelayNodeExecutor()

	t.Run("valid delay in minutes", func(t *testing.T) {
		params := NodeExecutionParams{
			WorkspaceID: "ws1",
			Node: &domain.AutomationNode{
				ID:         "node1",
				Type:       domain.NodeTypeDelay,
				NextNodeID: strPtr("node2"),
				Config: map[string]interface{}{
					"duration": 30,
					"unit":     "minutes",
				},
			},
			Contact: &domain.ContactAutomation{
				ID:           "ca1",
				ContactEmail: "test@example.com",
			},
		}

		result, err := executor.Execute(context.Background(), params)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.NotNil(t, result.NextNodeID)
		assert.Equal(t, "node2", *result.NextNodeID)
		assert.Equal(t, domain.ContactAutomationStatusActive, result.Status)
		assert.NotNil(t, result.ScheduledAt)

		// Check scheduled time is approximately 30 minutes from now
		expectedTime := time.Now().UTC().Add(30 * time.Minute)
		assert.WithinDuration(t, expectedTime, *result.ScheduledAt, time.Minute)

		// Verify node_type is included in Output
		assert.Equal(t, "delay", result.Output["node_type"])
		assert.Equal(t, 30, result.Output["delay_duration"])
		assert.Equal(t, "minutes", result.Output["delay_unit"])
	})

	t.Run("valid delay in hours", func(t *testing.T) {
		params := NodeExecutionParams{
			WorkspaceID: "ws1",
			Node: &domain.AutomationNode{
				ID:         "node1",
				Type:       domain.NodeTypeDelay,
				NextNodeID: strPtr("node2"),
				Config: map[string]interface{}{
					"duration": 2,
					"unit":     "hours",
				},
			},
		}

		result, err := executor.Execute(context.Background(), params)
		require.NoError(t, err)
		require.NotNil(t, result)

		expectedTime := time.Now().UTC().Add(2 * time.Hour)
		assert.WithinDuration(t, expectedTime, *result.ScheduledAt, time.Minute)
	})

	t.Run("valid delay in days", func(t *testing.T) {
		params := NodeExecutionParams{
			WorkspaceID: "ws1",
			Node: &domain.AutomationNode{
				ID:         "node1",
				Type:       domain.NodeTypeDelay,
				NextNodeID: strPtr("node2"),
				Config: map[string]interface{}{
					"duration": 1,
					"unit":     "days",
				},
			},
		}

		result, err := executor.Execute(context.Background(), params)
		require.NoError(t, err)
		require.NotNil(t, result)

		expectedTime := time.Now().UTC().Add(24 * time.Hour)
		assert.WithinDuration(t, expectedTime, *result.ScheduledAt, time.Minute)
	})

	t.Run("invalid unit", func(t *testing.T) {
		params := NodeExecutionParams{
			WorkspaceID: "ws1",
			Node: &domain.AutomationNode{
				ID:   "node1",
				Type: domain.NodeTypeDelay,
				Config: map[string]interface{}{
					"duration": 1,
					"unit":     "invalid",
				},
			},
		}

		result, err := executor.Execute(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "invalid")
	})

	t.Run("invalid duration", func(t *testing.T) {
		params := NodeExecutionParams{
			WorkspaceID: "ws1",
			Node: &domain.AutomationNode{
				ID:   "node1",
				Type: domain.NodeTypeDelay,
				Config: map[string]interface{}{
					"duration": 0,
					"unit":     "minutes",
				},
			},
		}

		result, err := executor.Execute(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestDelayNodeExecutor_NodeType(t *testing.T) {
	executor := NewDelayNodeExecutor()
	assert.Equal(t, domain.NodeTypeDelay, executor.NodeType())
}

func TestParseDelayNodeConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := map[string]interface{}{
			"duration": 30,
			"unit":     "minutes",
		}

		c, err := parseDelayNodeConfig(config)
		require.NoError(t, err)
		assert.Equal(t, 30, c.Duration)
		assert.Equal(t, "minutes", c.Unit)
	})

	t.Run("invalid config - missing duration", func(t *testing.T) {
		config := map[string]interface{}{
			"unit": "minutes",
		}

		_, err := parseDelayNodeConfig(config)
		assert.Error(t, err)
	})

	t.Run("invalid config - invalid unit", func(t *testing.T) {
		config := map[string]interface{}{
			"duration": 30,
			"unit":     "invalid",
		}

		_, err := parseDelayNodeConfig(config)
		assert.Error(t, err)
	})
}

func TestParseEmailNodeConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := map[string]interface{}{
			"template_id": "tpl123",
		}

		c, err := parseEmailNodeConfig(config)
		require.NoError(t, err)
		assert.Equal(t, "tpl123", c.TemplateID)
	})

	t.Run("invalid config - missing template_id", func(t *testing.T) {
		config := map[string]interface{}{}

		_, err := parseEmailNodeConfig(config)
		assert.Error(t, err)
	})
}

func TestParseBranchNodeConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := map[string]interface{}{
			"paths": []interface{}{
				map[string]interface{}{
					"id":           "p1",
					"name":         "VIP",
					"next_node_id": "node_vip",
				},
				map[string]interface{}{
					"id":           "p2",
					"name":         "Regular",
					"next_node_id": "node_regular",
				},
			},
			"default_path_id": "p2",
		}

		c, err := parseBranchNodeConfig(config)
		require.NoError(t, err)
		assert.Len(t, c.Paths, 2)
		assert.Equal(t, "p2", c.DefaultPathID)
	})
}

func TestParseFilterNodeConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := map[string]interface{}{
			"continue_node_id": "node_continue",
			"exit_node_id":     "node_exit",
		}

		c, err := parseFilterNodeConfig(config)
		require.NoError(t, err)
		assert.Equal(t, "node_continue", c.ContinueNodeID)
		assert.Equal(t, "node_exit", c.ExitNodeID)
	})
}

func TestParseAddToListNodeConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := map[string]interface{}{
			"list_id": "list123",
			"status":  "subscribed",
		}

		c, err := parseAddToListNodeConfig(config)
		require.NoError(t, err)
		assert.Equal(t, "list123", c.ListID)
		assert.Equal(t, "subscribed", c.Status)
	})

	t.Run("invalid config - missing list_id", func(t *testing.T) {
		config := map[string]interface{}{
			"status": "subscribed",
		}

		_, err := parseAddToListNodeConfig(config)
		assert.Error(t, err)
	})

	t.Run("invalid config - invalid status", func(t *testing.T) {
		config := map[string]interface{}{
			"list_id": "list123",
			"status":  "invalid",
		}

		_, err := parseAddToListNodeConfig(config)
		assert.Error(t, err)
	})
}

func TestParseRemoveFromListNodeConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := map[string]interface{}{
			"list_id": "list123",
		}

		c, err := parseRemoveFromListNodeConfig(config)
		require.NoError(t, err)
		assert.Equal(t, "list123", c.ListID)
	})

	t.Run("invalid config - missing list_id", func(t *testing.T) {
		config := map[string]interface{}{}

		_, err := parseRemoveFromListNodeConfig(config)
		assert.Error(t, err)
	})
}

func TestFindDefaultPath(t *testing.T) {
	paths := []domain.BranchPath{
		{ID: "p1", Name: "Path 1", NextNodeID: "node1"},
		{ID: "p2", Name: "Path 2", NextNodeID: "node2"},
		{ID: "p3", Name: "Path 3", NextNodeID: "node3"},
	}

	t.Run("finds matching path", func(t *testing.T) {
		result := findDefaultPath(paths, "p2")
		require.NotNil(t, result)
		assert.Equal(t, "p2", result.ID)
		assert.Equal(t, "node2", result.NextNodeID)
	})

	t.Run("returns first path if default not found", func(t *testing.T) {
		result := findDefaultPath(paths, "nonexistent")
		require.NotNil(t, result)
		assert.Equal(t, "p1", result.ID)
	})

	t.Run("returns nil for empty paths", func(t *testing.T) {
		result := findDefaultPath([]domain.BranchPath{}, "p1")
		assert.Nil(t, result)
	})
}

func TestBuildAutomationTemplateData(t *testing.T) {
	t.Run("builds data from contact", func(t *testing.T) {
		firstName := &domain.NullableString{String: "John", IsNull: false}
		lastName := &domain.NullableString{String: "Doe", IsNull: false}

		contact := &domain.Contact{
			Email:     "john@example.com",
			FirstName: firstName,
			LastName:  lastName,
		}

		automation := &domain.Automation{
			ID:   "auto123",
			Name: "Test Automation",
		}

		data := buildAutomationTemplateData(contact, automation)

		assert.Equal(t, "john@example.com", data["email"])
		assert.Equal(t, "John", data["first_name"])
		assert.Equal(t, "Doe", data["last_name"])
		assert.Equal(t, "auto123", data["automation_id"])
		assert.Equal(t, "Test Automation", data["automation_name"])
	})

	t.Run("handles nil contact", func(t *testing.T) {
		automation := &domain.Automation{
			ID:   "auto123",
			Name: "Test Automation",
		}

		data := buildAutomationTemplateData(nil, automation)

		assert.NotContains(t, data, "email")
		assert.Equal(t, "auto123", data["automation_id"])
	})

	t.Run("handles nil automation", func(t *testing.T) {
		contact := &domain.Contact{
			Email: "john@example.com",
		}

		data := buildAutomationTemplateData(contact, nil)

		assert.Equal(t, "john@example.com", data["email"])
		assert.NotContains(t, data, "automation_id")
	})

	t.Run("handles null nullable fields", func(t *testing.T) {
		firstName := &domain.NullableString{String: "", IsNull: true}

		contact := &domain.Contact{
			Email:     "john@example.com",
			FirstName: firstName,
		}

		data := buildAutomationTemplateData(contact, nil)

		assert.Equal(t, "john@example.com", data["email"])
		assert.NotContains(t, data, "first_name")
	})
}

func TestEmailNodeExecutor_Execute_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := setupMockLoggerForNodeExecutor(ctrl)

	executor := NewEmailNodeExecutor(mockEmailService, mockWorkspaceRepo, "https://api.example.com", mockLogger)

	workspace := createTestWorkspaceWithEmailProvider()

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), "ws1").
		Return(workspace, nil)

	mockEmailService.EXPECT().
		SendEmailForTemplate(gomock.Any(), gomock.Any()).
		Return(nil)

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:         "email_node1",
			Type:       domain.NodeTypeEmail,
			NextNodeID: strPtr("next_node"),
			Config: map[string]interface{}{
				"template_id": "tpl123",
			},
		},
		Contact: &domain.ContactAutomation{
			ID:           "ca1",
			ContactEmail: "recipient@example.com",
		},
		ContactData: &domain.Contact{
			Email: "recipient@example.com",
		},
		Automation: &domain.Automation{
			ID:   "auto1",
			Name: "Test Automation",
		},
	}

	result, err := executor.Execute(context.Background(), params)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "next_node", *result.NextNodeID)
	assert.Equal(t, domain.ContactAutomationStatusActive, result.Status)
	assert.Equal(t, "email", result.Output["node_type"])
	assert.Equal(t, "tpl123", result.Output["template_id"])
	assert.Equal(t, "recipient@example.com", result.Output["to"])
	assert.NotEmpty(t, result.Output["message_id"])
}

func TestEmailNodeExecutor_Execute_NilContactData(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := setupMockLoggerForNodeExecutor(ctrl)

	executor := NewEmailNodeExecutor(mockEmailService, mockWorkspaceRepo, "https://api.example.com", mockLogger)

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:   "email_node1",
			Type: domain.NodeTypeEmail,
			Config: map[string]interface{}{
				"template_id": "tpl123",
			},
		},
		Contact: &domain.ContactAutomation{
			ID:           "ca1",
			ContactEmail: "recipient@example.com",
		},
		ContactData: nil, // Missing contact data
		Automation: &domain.Automation{
			ID:   "auto1",
			Name: "Test Automation",
		},
	}

	result, err := executor.Execute(context.Background(), params)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "contact data is required for email node")
}

func TestEmailNodeExecutor_Execute_NilAutomation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := setupMockLoggerForNodeExecutor(ctrl)

	executor := NewEmailNodeExecutor(mockEmailService, mockWorkspaceRepo, "https://api.example.com", mockLogger)

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:   "email_node1",
			Type: domain.NodeTypeEmail,
			Config: map[string]interface{}{
				"template_id": "tpl123",
			},
		},
		Contact: &domain.ContactAutomation{
			ID:           "ca1",
			ContactEmail: "recipient@example.com",
		},
		ContactData: &domain.Contact{
			Email: "recipient@example.com",
		},
		Automation: nil, // Missing automation
	}

	result, err := executor.Execute(context.Background(), params)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "automation is required for email node")
}

func TestEmailNodeExecutor_Execute_InvalidConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := setupMockLoggerForNodeExecutor(ctrl)

	executor := NewEmailNodeExecutor(mockEmailService, mockWorkspaceRepo, "https://api.example.com", mockLogger)

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:     "email_node1",
			Type:   domain.NodeTypeEmail,
			Config: map[string]interface{}{
				// Missing template_id
			},
		},
		Contact: &domain.ContactAutomation{
			ID:           "ca1",
			ContactEmail: "recipient@example.com",
		},
		ContactData: &domain.Contact{
			Email: "recipient@example.com",
		},
		Automation: &domain.Automation{
			ID:   "auto1",
			Name: "Test Automation",
		},
	}

	result, err := executor.Execute(context.Background(), params)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid email node config")
}

func TestEmailNodeExecutor_Execute_WorkspaceNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := setupMockLoggerForNodeExecutor(ctrl)

	executor := NewEmailNodeExecutor(mockEmailService, mockWorkspaceRepo, "https://api.example.com", mockLogger)

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), "ws1").
		Return(nil, errors.New("workspace not found"))

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:   "email_node1",
			Type: domain.NodeTypeEmail,
			Config: map[string]interface{}{
				"template_id": "tpl123",
			},
		},
		Contact: &domain.ContactAutomation{
			ID:           "ca1",
			ContactEmail: "recipient@example.com",
		},
		ContactData: &domain.Contact{
			Email: "recipient@example.com",
		},
		Automation: &domain.Automation{
			ID:   "auto1",
			Name: "Test Automation",
		},
	}

	result, err := executor.Execute(context.Background(), params)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "workspace not found")
}

func TestEmailNodeExecutor_Execute_NoEmailProvider(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := setupMockLoggerForNodeExecutor(ctrl)

	executor := NewEmailNodeExecutor(mockEmailService, mockWorkspaceRepo, "https://api.example.com", mockLogger)

	// Workspace without email provider
	workspace := &domain.Workspace{
		ID:           "ws1",
		Name:         "Test Workspace",
		Integrations: []domain.Integration{},
	}

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), "ws1").
		Return(workspace, nil)

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:   "email_node1",
			Type: domain.NodeTypeEmail,
			Config: map[string]interface{}{
				"template_id": "tpl123",
			},
		},
		Contact: &domain.ContactAutomation{
			ID:           "ca1",
			ContactEmail: "recipient@example.com",
		},
		ContactData: &domain.Contact{
			Email: "recipient@example.com",
		},
		Automation: &domain.Automation{
			ID:   "auto1",
			Name: "Test Automation",
		},
	}

	result, err := executor.Execute(context.Background(), params)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no email provider configured")
}

func TestEmailNodeExecutor_Execute_SendFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := setupMockLoggerForNodeExecutor(ctrl)

	executor := NewEmailNodeExecutor(mockEmailService, mockWorkspaceRepo, "https://api.example.com", mockLogger)

	workspace := createTestWorkspaceWithEmailProvider()

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), "ws1").
		Return(workspace, nil)

	mockEmailService.EXPECT().
		SendEmailForTemplate(gomock.Any(), gomock.Any()).
		Return(errors.New("email provider error"))

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:   "email_node1",
			Type: domain.NodeTypeEmail,
			Config: map[string]interface{}{
				"template_id": "tpl123",
			},
		},
		Contact: &domain.ContactAutomation{
			ID:           "ca1",
			ContactEmail: "recipient@example.com",
		},
		ContactData: &domain.Contact{
			Email: "recipient@example.com",
		},
		Automation: &domain.Automation{
			ID:   "auto1",
			Name: "Test Automation",
		},
	}

	result, err := executor.Execute(context.Background(), params)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to send email")
}

func TestEmailNodeExecutor_Execute_WithCustomEndpoint(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := setupMockLoggerForNodeExecutor(ctrl)

	executor := NewEmailNodeExecutor(mockEmailService, mockWorkspaceRepo, "https://api.example.com", mockLogger)

	customEndpoint := "https://custom.endpoint.com"
	workspace := createTestWorkspaceWithEmailProvider()
	workspace.Settings.CustomEndpointURL = &customEndpoint

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), "ws1").
		Return(workspace, nil)

	// Capture the request to verify custom endpoint is used
	mockEmailService.EXPECT().
		SendEmailForTemplate(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req domain.SendEmailRequest) error {
			assert.Equal(t, customEndpoint, req.TrackingSettings.Endpoint)
			return nil
		})

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:         "email_node1",
			Type:       domain.NodeTypeEmail,
			NextNodeID: strPtr("next_node"),
			Config: map[string]interface{}{
				"template_id": "tpl123",
			},
		},
		Contact: &domain.ContactAutomation{
			ID:           "ca1",
			ContactEmail: "recipient@example.com",
		},
		ContactData: &domain.Contact{
			Email: "recipient@example.com",
		},
		Automation: &domain.Automation{
			ID:   "auto1",
			Name: "Test Automation",
		},
	}

	result, err := executor.Execute(context.Background(), params)
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestEmailNodeExecutor_NodeType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := setupMockLoggerForNodeExecutor(ctrl)

	executor := NewEmailNodeExecutor(mockEmailService, mockWorkspaceRepo, "https://api.example.com", mockLogger)
	assert.Equal(t, domain.NodeTypeEmail, executor.NodeType())
}

// buildSimpleCondition creates a simple TreeNode condition for testing
func buildSimpleCondition() *domain.TreeNode {
	return &domain.TreeNode{
		Kind: "leaf",
		Leaf: &domain.TreeNodeLeaf{
			Source: "contacts",
			Contact: &domain.ContactCondition{
				Filters: []*domain.DimensionFilter{
					{
						FieldName:    "email",
						FieldType:    "string",
						Operator:     "equals",
						StringValues: []string{"test@example.com"},
					},
				},
			},
		},
	}
}

func TestBranchNodeExecutor_Execute_FirstPathMatches(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock DB
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Mock workspace repo
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), "ws1").
		Return(db, nil)

	// Mock SQL query - first path matches
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	executor := NewBranchNodeExecutor(NewQueryBuilder(), mockWorkspaceRepo)

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:   "branch1",
			Type: domain.NodeTypeBranch,
			Config: map[string]interface{}{
				"paths": []interface{}{
					map[string]interface{}{
						"id":           "p1",
						"name":         "VIP Path",
						"next_node_id": "vip_node",
						"conditions":   buildSimpleConditionMap(),
					},
					map[string]interface{}{
						"id":           "p2",
						"name":         "Regular Path",
						"next_node_id": "regular_node",
					},
				},
				"default_path_id": "p2",
			},
		},
		ContactData: &domain.Contact{Email: "test@example.com"},
	}

	result, err := executor.Execute(context.Background(), params)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "vip_node", *result.NextNodeID)
	assert.Equal(t, "branch", result.Output["node_type"])
	assert.Equal(t, "p1", result.Output["path_taken"])
	assert.Equal(t, domain.ContactAutomationStatusActive, result.Status)
}

// buildSimpleConditionMap creates a simple condition map for testing branch/filter nodes
// This matches the actual TreeNode schema used by the codebase
func buildSimpleConditionMap() map[string]interface{} {
	return map[string]interface{}{
		"kind": "leaf",
		"leaf": map[string]interface{}{
			"source": "contacts",
			"contact": map[string]interface{}{
				"filters": []interface{}{
					map[string]interface{}{
						"field_name":    "email",
						"field_type":    "string",
						"operator":      "equals",
						"string_values": []interface{}{"test@example.com"},
					},
				},
			},
		},
	}
}

func TestBranchNodeExecutor_Execute_SecondPathMatches(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	// GetConnection is called once at the start
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), "ws1").
		Return(db, nil)

	// First path doesn't match
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	// Second path matches
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	executor := NewBranchNodeExecutor(NewQueryBuilder(), mockWorkspaceRepo)

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:   "branch1",
			Type: domain.NodeTypeBranch,
			Config: map[string]interface{}{
				"paths": []interface{}{
					map[string]interface{}{
						"id":           "p1",
						"name":         "VIP Path",
						"next_node_id": "vip_node",
						"conditions":   buildSimpleConditionMap(),
					},
					map[string]interface{}{
						"id":           "p2",
						"name":         "Regular Path",
						"next_node_id": "regular_node",
						"conditions":   buildSimpleConditionMap(),
					},
				},
				"default_path_id": "p1",
			},
		},
		ContactData: &domain.Contact{Email: "other@example.com"},
	}

	result, err := executor.Execute(context.Background(), params)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "regular_node", *result.NextNodeID)
	assert.Equal(t, "p2", result.Output["path_taken"])
}

func TestBranchNodeExecutor_Execute_DefaultPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), "ws1").
		Return(db, nil)

	// Path doesn't match
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	executor := NewBranchNodeExecutor(NewQueryBuilder(), mockWorkspaceRepo)

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:   "branch1",
			Type: domain.NodeTypeBranch,
			Config: map[string]interface{}{
				"paths": []interface{}{
					map[string]interface{}{
						"id":           "p1",
						"name":         "VIP Path",
						"next_node_id": "vip_node",
						"conditions":   buildSimpleConditionMap(),
					},
					map[string]interface{}{
						"id":           "p2",
						"name":         "Default Path",
						"next_node_id": "default_node",
					},
				},
				"default_path_id": "p2",
			},
		},
		ContactData: &domain.Contact{Email: "other@example.com"},
	}

	result, err := executor.Execute(context.Background(), params)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "default_node", *result.NextNodeID)
	assert.Equal(t, "default", result.Output["path_taken"])
}

func TestBranchNodeExecutor_Execute_NilConditionsSkipped(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	// GetConnection is still called even when no conditions need evaluation
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), "ws1").
		Return(db, nil)

	executor := NewBranchNodeExecutor(NewQueryBuilder(), mockWorkspaceRepo)

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:   "branch1",
			Type: domain.NodeTypeBranch,
			Config: map[string]interface{}{
				"paths": []interface{}{
					map[string]interface{}{
						"id":           "p1",
						"name":         "Path without conditions",
						"next_node_id": "some_node",
						// No conditions - should be skipped
					},
					map[string]interface{}{
						"id":           "p2",
						"name":         "Default Path",
						"next_node_id": "default_node",
					},
				},
				"default_path_id": "p2",
			},
		},
		ContactData: &domain.Contact{Email: "test@example.com"},
	}

	result, err := executor.Execute(context.Background(), params)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should fall through to default
	assert.Equal(t, "default_node", *result.NextNodeID)
	assert.Equal(t, "default", result.Output["path_taken"])
}

func TestBranchNodeExecutor_Execute_DBConnectionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), "ws1").
		Return(nil, errors.New("database connection error"))

	executor := NewBranchNodeExecutor(NewQueryBuilder(), mockWorkspaceRepo)

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:   "branch1",
			Type: domain.NodeTypeBranch,
			Config: map[string]interface{}{
				"paths": []interface{}{
					map[string]interface{}{
						"id":           "p1",
						"name":         "VIP Path",
						"next_node_id": "vip_node",
						"conditions":   buildSimpleConditionMap(),
					},
				},
				"default_path_id": "p1",
			},
		},
		ContactData: &domain.Contact{Email: "test@example.com"},
	}

	result, err := executor.Execute(context.Background(), params)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to get db connection")
}

func TestBranchNodeExecutor_Execute_QueryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), "ws1").
		Return(db, nil)

	// Query fails
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnError(sql.ErrConnDone)

	executor := NewBranchNodeExecutor(NewQueryBuilder(), mockWorkspaceRepo)

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:   "branch1",
			Type: domain.NodeTypeBranch,
			Config: map[string]interface{}{
				"paths": []interface{}{
					map[string]interface{}{
						"id":           "p1",
						"name":         "VIP Path",
						"next_node_id": "vip_node",
						"conditions":   buildSimpleConditionMap(),
					},
				},
				"default_path_id": "p1",
			},
		},
		ContactData: &domain.Contact{Email: "test@example.com"},
	}

	result, err := executor.Execute(context.Background(), params)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to evaluate path")
}

func TestBranchNodeExecutor_Execute_NoPathsCompletes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	// GetConnection is always called
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), "ws1").
		Return(db, nil)

	executor := NewBranchNodeExecutor(NewQueryBuilder(), mockWorkspaceRepo)

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:   "branch1",
			Type: domain.NodeTypeBranch,
			Config: map[string]interface{}{
				"paths":           []interface{}{},
				"default_path_id": "nonexistent",
			},
		},
		ContactData: &domain.Contact{Email: "test@example.com"},
	}

	result, err := executor.Execute(context.Background(), params)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Nil(t, result.NextNodeID)
	assert.Equal(t, domain.ContactAutomationStatusCompleted, result.Status)
	assert.Equal(t, "none", result.Output["path_taken"])
}

func TestBranchNodeExecutor_NodeType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	executor := NewBranchNodeExecutor(NewQueryBuilder(), mockWorkspaceRepo)
	assert.Equal(t, domain.NodeTypeBranch, executor.NodeType())
}

func TestFilterNodeExecutor_Execute_PassesFilter(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), "ws1").
		Return(db, nil)

	// Contact matches filter
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	executor := NewFilterNodeExecutor(NewQueryBuilder(), mockWorkspaceRepo)

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:   "filter1",
			Type: domain.NodeTypeFilter,
			Config: map[string]interface{}{
				"continue_node_id": "continue_node",
				"exit_node_id":     "exit_node",
				"conditions":       buildSimpleConditionMap(),
			},
		},
		ContactData: &domain.Contact{Email: "test@example.com"},
	}

	result, err := executor.Execute(context.Background(), params)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "continue_node", *result.NextNodeID)
	assert.Equal(t, "filter", result.Output["node_type"])
	assert.Equal(t, true, result.Output["filter_passed"])
	assert.Equal(t, domain.ContactAutomationStatusActive, result.Status)
}

func TestFilterNodeExecutor_Execute_FailsFilter(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), "ws1").
		Return(db, nil)

	// Contact doesn't match filter
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	executor := NewFilterNodeExecutor(NewQueryBuilder(), mockWorkspaceRepo)

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:   "filter1",
			Type: domain.NodeTypeFilter,
			Config: map[string]interface{}{
				"continue_node_id": "continue_node",
				"exit_node_id":     "exit_node",
				"conditions":       buildSimpleConditionMap(),
			},
		},
		ContactData: &domain.Contact{Email: "other@example.com"},
	}

	result, err := executor.Execute(context.Background(), params)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "exit_node", *result.NextNodeID)
	assert.Equal(t, false, result.Output["filter_passed"])
	assert.Equal(t, domain.ContactAutomationStatusActive, result.Status)
}

func TestFilterNodeExecutor_Execute_DBConnectionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), "ws1").
		Return(nil, errors.New("database connection error"))

	executor := NewFilterNodeExecutor(NewQueryBuilder(), mockWorkspaceRepo)

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:   "filter1",
			Type: domain.NodeTypeFilter,
			Config: map[string]interface{}{
				"continue_node_id": "continue_node",
				"exit_node_id":     "exit_node",
				"conditions":       buildSimpleConditionMap(),
			},
		},
		ContactData: &domain.Contact{Email: "test@example.com"},
	}

	result, err := executor.Execute(context.Background(), params)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to get db connection")
}

func TestFilterNodeExecutor_Execute_QueryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), "ws1").
		Return(db, nil)

	// Query fails
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnError(sql.ErrConnDone)

	executor := NewFilterNodeExecutor(NewQueryBuilder(), mockWorkspaceRepo)

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:   "filter1",
			Type: domain.NodeTypeFilter,
			Config: map[string]interface{}{
				"continue_node_id": "continue_node",
				"exit_node_id":     "exit_node",
				"conditions":       buildSimpleConditionMap(),
			},
		},
		ContactData: &domain.Contact{Email: "test@example.com"},
	}

	result, err := executor.Execute(context.Background(), params)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to evaluate filter")
}

func TestFilterNodeExecutor_NodeType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	executor := NewFilterNodeExecutor(NewQueryBuilder(), mockWorkspaceRepo)
	assert.Equal(t, domain.NodeTypeFilter, executor.NodeType())
}

func TestAddToListNodeExecutor_Execute_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockContactListRepo := mocks.NewMockContactListRepository(ctrl)
	mockContactListRepo.EXPECT().
		AddContactToList(gomock.Any(), "ws1", gomock.Any()).
		Return(nil)

	executor := NewAddToListNodeExecutor(mockContactListRepo)

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:         "add_to_list1",
			Type:       domain.NodeTypeAddToList,
			NextNodeID: strPtr("next_node"),
			Config: map[string]interface{}{
				"list_id": "list123",
				"status":  "subscribed",
			},
		},
		Contact: &domain.ContactAutomation{
			ID:           "ca1",
			ContactEmail: "test@example.com",
		},
	}

	result, err := executor.Execute(context.Background(), params)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "next_node", *result.NextNodeID)
	assert.Equal(t, domain.ContactAutomationStatusActive, result.Status)
	assert.Equal(t, "add_to_list", result.Output["node_type"])
	assert.Equal(t, "list123", result.Output["list_id"])
	assert.Equal(t, "subscribed", result.Output["status"])
	assert.NotContains(t, result.Output, "error")
}

func TestAddToListNodeExecutor_Execute_AlreadyInList(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockContactListRepo := mocks.NewMockContactListRepository(ctrl)
	// Returns error (contact already in list) but executor should not fail
	mockContactListRepo.EXPECT().
		AddContactToList(gomock.Any(), "ws1", gomock.Any()).
		Return(errors.New("contact already exists in list"))

	executor := NewAddToListNodeExecutor(mockContactListRepo)

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:         "add_to_list1",
			Type:       domain.NodeTypeAddToList,
			NextNodeID: strPtr("next_node"),
			Config: map[string]interface{}{
				"list_id": "list123",
				"status":  "subscribed",
			},
		},
		Contact: &domain.ContactAutomation{
			ID:           "ca1",
			ContactEmail: "test@example.com",
		},
	}

	result, err := executor.Execute(context.Background(), params)
	require.NoError(t, err) // Should not return error
	require.NotNil(t, result)

	assert.Equal(t, "next_node", *result.NextNodeID)
	assert.Equal(t, domain.ContactAutomationStatusActive, result.Status)
	assert.Equal(t, "list123", result.Output["list_id"])
	assert.Contains(t, result.Output, "error") // Error logged in output
}

func TestAddToListNodeExecutor_Execute_InvalidConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockContactListRepo := mocks.NewMockContactListRepository(ctrl)

	executor := NewAddToListNodeExecutor(mockContactListRepo)

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:   "add_to_list1",
			Type: domain.NodeTypeAddToList,
			Config: map[string]interface{}{
				// Missing list_id
				"status": "subscribed",
			},
		},
		Contact: &domain.ContactAutomation{
			ID:           "ca1",
			ContactEmail: "test@example.com",
		},
	}

	result, err := executor.Execute(context.Background(), params)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid add-to-list node config")
}

func TestAddToListNodeExecutor_NodeType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockContactListRepo := mocks.NewMockContactListRepository(ctrl)
	executor := NewAddToListNodeExecutor(mockContactListRepo)
	assert.Equal(t, domain.NodeTypeAddToList, executor.NodeType())
}

func TestRemoveFromListNodeExecutor_Execute_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockContactListRepo := mocks.NewMockContactListRepository(ctrl)
	mockContactListRepo.EXPECT().
		RemoveContactFromList(gomock.Any(), "ws1", "test@example.com", "list123").
		Return(nil)

	executor := NewRemoveFromListNodeExecutor(mockContactListRepo)

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:         "remove_from_list1",
			Type:       domain.NodeTypeRemoveFromList,
			NextNodeID: strPtr("next_node"),
			Config: map[string]interface{}{
				"list_id": "list123",
			},
		},
		Contact: &domain.ContactAutomation{
			ID:           "ca1",
			ContactEmail: "test@example.com",
		},
	}

	result, err := executor.Execute(context.Background(), params)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "next_node", *result.NextNodeID)
	assert.Equal(t, domain.ContactAutomationStatusActive, result.Status)
	assert.Equal(t, "remove_from_list", result.Output["node_type"])
	assert.Equal(t, "list123", result.Output["list_id"])
	assert.NotContains(t, result.Output, "error")
}

func TestRemoveFromListNodeExecutor_Execute_NotInList(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockContactListRepo := mocks.NewMockContactListRepository(ctrl)
	// Returns error (contact not in list) but executor should not fail
	mockContactListRepo.EXPECT().
		RemoveContactFromList(gomock.Any(), "ws1", "test@example.com", "list123").
		Return(errors.New("contact not found in list"))

	executor := NewRemoveFromListNodeExecutor(mockContactListRepo)

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:         "remove_from_list1",
			Type:       domain.NodeTypeRemoveFromList,
			NextNodeID: strPtr("next_node"),
			Config: map[string]interface{}{
				"list_id": "list123",
			},
		},
		Contact: &domain.ContactAutomation{
			ID:           "ca1",
			ContactEmail: "test@example.com",
		},
	}

	result, err := executor.Execute(context.Background(), params)
	require.NoError(t, err) // Should not return error
	require.NotNil(t, result)

	assert.Equal(t, "next_node", *result.NextNodeID)
	assert.Equal(t, domain.ContactAutomationStatusActive, result.Status)
	assert.Equal(t, "list123", result.Output["list_id"])
	assert.Contains(t, result.Output, "error") // Error logged in output
}

func TestRemoveFromListNodeExecutor_Execute_InvalidConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockContactListRepo := mocks.NewMockContactListRepository(ctrl)

	executor := NewRemoveFromListNodeExecutor(mockContactListRepo)

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:   "remove_from_list1",
			Type: domain.NodeTypeRemoveFromList,
			Config: map[string]interface{}{
				// Missing list_id
			},
		},
		Contact: &domain.ContactAutomation{
			ID:           "ca1",
			ContactEmail: "test@example.com",
		},
	}

	result, err := executor.Execute(context.Background(), params)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid remove-from-list node config")
}

func TestRemoveFromListNodeExecutor_NodeType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockContactListRepo := mocks.NewMockContactListRepository(ctrl)
	executor := NewRemoveFromListNodeExecutor(mockContactListRepo)
	assert.Equal(t, domain.NodeTypeRemoveFromList, executor.NodeType())
}

// ABTestNodeExecutor tests

func TestABTestNodeExecutor_NodeType(t *testing.T) {
	executor := NewABTestNodeExecutor()
	assert.Equal(t, domain.NodeTypeABTest, executor.NodeType())
}

func TestParseABTestNodeConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := map[string]interface{}{
			"variants": []map[string]interface{}{
				{
					"id":           "A",
					"name":         "Control",
					"weight":       50,
					"next_node_id": "node_a",
				},
				{
					"id":           "B",
					"name":         "Variant B",
					"weight":       50,
					"next_node_id": "node_b",
				},
			},
		}

		c, err := parseABTestNodeConfig(config)
		require.NoError(t, err)
		assert.Len(t, c.Variants, 2)
		assert.Equal(t, "A", c.Variants[0].ID)
		assert.Equal(t, "Control", c.Variants[0].Name)
		assert.Equal(t, 50, c.Variants[0].Weight)
		assert.Equal(t, "node_a", c.Variants[0].NextNodeID)
	})

	t.Run("invalid config - less than 2 variants", func(t *testing.T) {
		config := map[string]interface{}{
			"variants": []map[string]interface{}{
				{
					"id":           "A",
					"name":         "Control",
					"weight":       100,
					"next_node_id": "node_a",
				},
			},
		}

		_, err := parseABTestNodeConfig(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "at least 2 variants are required")
	})

	t.Run("invalid config - weights don't sum to 100", func(t *testing.T) {
		config := map[string]interface{}{
			"variants": []map[string]interface{}{
				{
					"id":           "A",
					"name":         "Control",
					"weight":       40,
					"next_node_id": "node_a",
				},
				{
					"id":           "B",
					"name":         "Variant B",
					"weight":       40,
					"next_node_id": "node_b",
				},
			},
		}

		_, err := parseABTestNodeConfig(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "weights must sum to 100")
	})
}

func TestABTestNodeExecutor_Execute_DeterministicAssignment(t *testing.T) {
	executor := NewABTestNodeExecutor()

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:   "ab_test_node",
			Type: domain.NodeTypeABTest,
			Config: map[string]interface{}{
				"variants": []map[string]interface{}{
					{
						"id":           "A",
						"name":         "Control",
						"weight":       50,
						"next_node_id": "node_a",
					},
					{
						"id":           "B",
						"name":         "Variant B",
						"weight":       50,
						"next_node_id": "node_b",
					},
				},
			},
		},
		Contact: &domain.ContactAutomation{
			ID:           "ca1",
			ContactEmail: "test@example.com",
		},
	}

	// Execute multiple times - should always get the same result
	result1, err := executor.Execute(context.Background(), params)
	require.NoError(t, err)
	require.NotNil(t, result1)

	result2, err := executor.Execute(context.Background(), params)
	require.NoError(t, err)
	require.NotNil(t, result2)

	// Same email + same nodeID should always give same variant
	assert.Equal(t, *result1.NextNodeID, *result2.NextNodeID)
	assert.Equal(t, result1.Output["variant_id"], result2.Output["variant_id"])
}

func TestABTestNodeExecutor_Execute_DifferentEmails(t *testing.T) {
	executor := NewABTestNodeExecutor()

	// Track distribution for statistical verification
	variantCounts := make(map[string]int)
	totalEmails := 1000

	for i := 0; i < totalEmails; i++ {
		params := NodeExecutionParams{
			WorkspaceID: "ws1",
			Node: &domain.AutomationNode{
				ID:   "ab_test_node",
				Type: domain.NodeTypeABTest,
				Config: map[string]interface{}{
					"variants": []map[string]interface{}{
						{
							"id":           "A",
							"name":         "Control",
							"weight":       90,
							"next_node_id": "node_a",
						},
						{
							"id":           "B",
							"name":         "Variant B",
							"weight":       10,
							"next_node_id": "node_b",
						},
					},
				},
			},
			Contact: &domain.ContactAutomation{
				ID:           "ca1",
				ContactEmail: fmt.Sprintf("user%d@example.com", i),
			},
		}

		result, err := executor.Execute(context.Background(), params)
		require.NoError(t, err)
		variantCounts[result.Output["variant_id"].(string)]++
	}

	// With 90/10 split, expect A to have significantly more than B
	// Allow for some variance (A should be 75-98% approximately)
	aPercent := float64(variantCounts["A"]) / float64(totalEmails) * 100
	assert.Greater(t, aPercent, 75.0, "Variant A should have more than 75% of contacts")
	assert.Less(t, aPercent, 98.0, "Variant A should have less than 98% of contacts")
}

func TestABTestNodeExecutor_Execute_OutputContainsVariantInfo(t *testing.T) {
	executor := NewABTestNodeExecutor()

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:   "ab_test_node",
			Type: domain.NodeTypeABTest,
			Config: map[string]interface{}{
				"variants": []map[string]interface{}{
					{
						"id":           "A",
						"name":         "Control",
						"weight":       50,
						"next_node_id": "node_a",
					},
					{
						"id":           "B",
						"name":         "Variant B",
						"weight":       50,
						"next_node_id": "node_b",
					},
				},
			},
		},
		Contact: &domain.ContactAutomation{
			ID:           "ca1",
			ContactEmail: "test@example.com",
		},
	}

	result, err := executor.Execute(context.Background(), params)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify output structure
	assert.Equal(t, "ab_test", result.Output["node_type"])
	assert.NotEmpty(t, result.Output["variant_id"])
	assert.NotEmpty(t, result.Output["variant_name"])
	assert.Equal(t, domain.ContactAutomationStatusActive, result.Status)
	assert.NotNil(t, result.NextNodeID)
}

func TestABTestNodeExecutor_Execute_InvalidConfig(t *testing.T) {
	executor := NewABTestNodeExecutor()

	params := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:     "ab_test_node",
			Type:   domain.NodeTypeABTest,
			Config: map[string]interface{}{}, // Missing variants
		},
		Contact: &domain.ContactAutomation{
			ID:           "ca1",
			ContactEmail: "test@example.com",
		},
	}

	result, err := executor.Execute(context.Background(), params)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid ab_test node config")
}

func TestABTestNodeExecutor_Execute_DifferentNodeID(t *testing.T) {
	executor := NewABTestNodeExecutor()

	// Same email but different node IDs may give different variants
	// (depending on hash distribution)
	params1 := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:   "ab_test_node_1",
			Type: domain.NodeTypeABTest,
			Config: map[string]interface{}{
				"variants": []map[string]interface{}{
					{
						"id":           "A",
						"name":         "Control",
						"weight":       50,
						"next_node_id": "node_a",
					},
					{
						"id":           "B",
						"name":         "Variant B",
						"weight":       50,
						"next_node_id": "node_b",
					},
				},
			},
		},
		Contact: &domain.ContactAutomation{
			ID:           "ca1",
			ContactEmail: "test@example.com",
		},
	}

	params2 := NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:   "ab_test_node_2", // Different node ID
			Type: domain.NodeTypeABTest,
			Config: map[string]interface{}{
				"variants": []map[string]interface{}{
					{
						"id":           "A",
						"name":         "Control",
						"weight":       50,
						"next_node_id": "node_a",
					},
					{
						"id":           "B",
						"name":         "Variant B",
						"weight":       50,
						"next_node_id": "node_b",
					},
				},
			},
		},
		Contact: &domain.ContactAutomation{
			ID:           "ca1",
			ContactEmail: "test@example.com",
		},
	}

	result1, err := executor.Execute(context.Background(), params1)
	require.NoError(t, err)
	require.NotNil(t, result1)

	result2, err := executor.Execute(context.Background(), params2)
	require.NoError(t, err)
	require.NotNil(t, result2)

	// Both should complete successfully - we're not asserting they're different
	// just that different node IDs produce valid results
	assert.NotNil(t, result1.NextNodeID)
	assert.NotNil(t, result2.NextNodeID)
}
