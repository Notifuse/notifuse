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

func TestEmailNodeExecutor_Execute_CustomText(t *testing.T) {
	text := func(s string) *string { return &s }
	email := func(body string, custom *string) *domain.EmailTemplate {
		return &domain.EmailTemplate{
			Subject:          "Subject",
			SenderID:         "sender1",
			VisualEditorTree: createValidMJMLTree(createTestTextBlock("txt1", body)),
			Text:             custom,
		}
	}
	translated := &domain.Template{
		ID: "tpl123", Name: "T", Version: 1, Channel: "email",
		Email: email("Hello", text("Hi {{ contact.email }}")),
		Translations: map[string]domain.TemplateTranslation{
			"fr": {Email: email("Bonjour", text("Salut {{ contact.email }}"))},
			"de": {Email: email("Hallo", nil)},
		},
	}

	cases := []struct {
		name     string
		template *domain.Template
		language string
		want     string
	}{
		{"default language custom text is rendered", translated, "", "Hi recipient@example.com"},
		{"translation with its own text", translated, "fr", "Salut recipient@example.com"},
		{"translation without a text gets the automatic one", translated, "de", ""},
		{"automatic template", &domain.Template{ID: "tpl123", Name: "T", Version: 1, Channel: "email", Email: email("Hello", nil)}, "", ""},
		{"a text that cannot be rendered gets the automatic one", &domain.Template{ID: "tpl123", Name: "T", Version: 1, Channel: "email", Email: email("Hello", text("{% if x %}open"))}, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockEmailQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
			mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
			mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			mockListRepo := mocks.NewMockListRepository(ctrl)
			executor := NewEmailNodeExecutor(mockEmailQueueRepo, mockTemplateRepo, mockWorkspaceRepo, mockListRepo,
				mocks.NewMockContactListRepository(ctrl), "https://api.example.com", setupMockLoggerForNodeExecutor(ctrl))

			workspace := createTestWorkspaceWithEmailProvider()
			workspace.Settings.DefaultLanguage = "en"
			mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "ws1").Return(workspace, nil)
			mockTemplateRepo.EXPECT().GetTemplateByID(gomock.Any(), "ws1", "tpl123", int64(0)).Return(tc.template, nil)

			var payload domain.EmailQueuePayload
			mockEmailQueueRepo.EXPECT().Enqueue(gomock.Any(), "ws1", gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, entries []*domain.EmailQueueEntry) error {
					require.Len(t, entries, 1)
					payload = entries[0].Payload
					return nil
				})

			contact := &domain.Contact{Email: "recipient@example.com"}
			if tc.language != "" {
				contact.Language = &domain.NullableString{String: tc.language}
			}
			_, err := executor.Execute(context.Background(), NodeExecutionParams{
				WorkspaceID: "ws1",
				Node: &domain.AutomationNode{
					ID: "email_node1", Type: domain.NodeTypeEmail, NextNodeID: strPtr("next_node"),
					Config: map[string]interface{}{"template_id": "tpl123"},
				},
				Contact:     &domain.ContactAutomation{ID: "ca1", ContactEmail: "recipient@example.com"},
				ContactData: contact,
				Automation:  &domain.Automation{ID: "auto1", Name: "A"},
			})
			require.NoError(t, err)
			assert.Equal(t, tc.want, payload.TextContent)
		})
	}
}
