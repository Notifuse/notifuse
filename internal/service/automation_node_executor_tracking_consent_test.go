package service

import (
	"context"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmailNodeExecutor_Execute_RecordsTrackedFlags(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmailQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockListRepo := mocks.NewMockListRepository(ctrl)
	mockContactListRepo := mocks.NewMockContactListRepository(ctrl)

	executor := NewEmailNodeExecutor(mockEmailQueueRepo, mockTemplateRepo, mockWorkspaceRepo, mockListRepo, mockContactListRepo, "https://api.example.com", setupMockLoggerForNodeExecutor(ctrl))

	workspace := createTestWorkspaceWithEmailProvider()
	workspace.Settings.EmailTrackingEnabled = true
	workspace.Settings.EmailTracking = domain.DefaultEmailTrackingSettings()

	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "ws1").Return(workspace, nil)
	mockTemplateRepo.EXPECT().GetTemplateByID(gomock.Any(), "ws1", "tpl123", int64(0)).Return(createTestTemplateWithLink("https://shop.example.com/p"), nil)

	var entry *domain.EmailQueueEntry
	mockEmailQueueRepo.EXPECT().Enqueue(gomock.Any(), "ws1", gomock.Any()).DoAndReturn(func(_ context.Context, _ string, entries []*domain.EmailQueueEntry) error {
		require.Len(t, entries, 1)
		entry = entries[0]
		return nil
	})

	contact := &domain.Contact{
		Email:                "recipient@example.com",
		TrackingOpensConsent: &domain.NullableString{String: domain.TrackingConsentDenied},
	}
	_, err := executor.Execute(context.Background(), NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID:         "email_node1",
			Type:       domain.NodeTypeEmail,
			NextNodeID: strPtr("next_node"),
			Config:     map[string]interface{}{"template_id": "tpl123"},
		},
		Contact:     &domain.ContactAutomation{ID: "ca1", ContactEmail: contact.Email},
		ContactData: contact,
		Automation:  &domain.Automation{ID: "auto1", Name: "Test Automation"},
	})
	require.NoError(t, err)

	require.NotNil(t, entry)
	require.NotNil(t, entry.Payload.OpensTracked)
	require.NotNil(t, entry.Payload.ClicksTracked)
	assert.False(t, *entry.Payload.OpensTracked)
	assert.True(t, *entry.Payload.ClicksTracked)
	assert.NotContains(t, entry.Payload.HTMLContent, "https://api.example.com/t/", "no open pixel for a contact who denied open tracking")
	assert.Contains(t, entry.Payload.HTMLContent, "https://api.example.com/r/", "clicks stay tracked")
}
