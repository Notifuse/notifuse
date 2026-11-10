package service

import (
	"context"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An automation email records the template version it was rendered from and its
// template data, as broadcasts do: the web version is rendered from them, and without
// them message history held version 0 and no data for every automation email.
func TestEmailNodeExecutor_Execute_RecordsRenderDataForTheWebVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmailQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockListRepo := mocks.NewMockListRepository(ctrl)
	executor := NewEmailNodeExecutor(mockEmailQueueRepo, mockTemplateRepo, mockWorkspaceRepo, mockListRepo,
		mocks.NewMockContactListRepository(ctrl), "https://api.example.com", setupMockLoggerForNodeExecutor(ctrl))

	template := createTestTemplate()
	template.Version = 7
	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "ws1").Return(createTestWorkspaceWithEmailProvider(), nil)
	mockTemplateRepo.EXPECT().GetTemplateByID(gomock.Any(), "ws1", "tpl123", int64(0)).Return(template, nil)
	mockListRepo.EXPECT().GetListByID(gomock.Any(), "ws1", "list1").Return(&domain.List{ID: "list1", Name: "Test List"}, nil)

	var enqueued []*domain.EmailQueueEntry
	mockEmailQueueRepo.EXPECT().Enqueue(gomock.Any(), "ws1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, entries []*domain.EmailQueueEntry) error {
			enqueued = entries
			return nil
		})

	_, err := executor.Execute(context.Background(), NodeExecutionParams{
		WorkspaceID: "ws1",
		Node: &domain.AutomationNode{
			ID: "email_node1", Type: domain.NodeTypeEmail, NextNodeID: strPtr("next_node"),
			Config: map[string]interface{}{"template_id": "tpl123"},
		},
		Contact:     &domain.ContactAutomation{ID: "ca1", ContactEmail: "recipient@example.com"},
		ContactData: &domain.Contact{Email: "recipient@example.com"},
		Automation:  &domain.Automation{ID: "auto1", Name: "Welcome", ListID: "list1"},
	})
	require.NoError(t, err)
	require.Len(t, enqueued, 1)

	payload := enqueued[0].Payload
	assert.Equal(t, 7, payload.TemplateVersion)
	require.NotEmpty(t, payload.TemplateData)
	assert.Equal(t, "Welcome", payload.TemplateData["automation_name"])
	link, ok := payload.TemplateData["view_in_browser_url"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(link, "https://api.example.com/v/"), link)
}
