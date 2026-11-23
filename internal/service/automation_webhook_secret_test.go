package service

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// A webhook node's secret is a bearer credential for its endpoint. automations.get and
// automations.list give it to a caller who may write automations, and remove it for one who may
// only read them, on a copy: the automation the repository returned keeps it.
func TestAutomationService_WebhookSecretNeedsWrite(t *testing.T) {
	const workspaceID = "workspace-123"
	ctx := context.Background()

	storedAutomation := func() *domain.Automation {
		automation := createTestAutomationService("auto-123", workspaceID)
		automation.Nodes = []*domain.AutomationNode{{
			ID: "hook", AutomationID: "auto-123", Type: domain.NodeTypeWebhook,
			Config: map[string]interface{}{"url": "https://hooks.example.com/in", "secret": "s3cret"},
		}}
		return automation
	}
	caller := func(write bool) *domain.UserWorkspace {
		return &domain.UserWorkspace{
			UserID: "user-123", WorkspaceID: workspaceID, Role: "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceAutomations: {Read: true, Write: write},
			},
		}
	}

	for _, write := range []bool{false, true} {
		name := "reader"
		if write {
			name = "writer"
		}

		t.Run(name+" on get", func(t *testing.T) {
			svc, repo, auth, _ := newAutomationTransitionMocks(t)
			stored := storedAutomation()
			auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, caller(write), nil)
			repo.EXPECT().GetByID(ctx, workspaceID, "auto-123").Return(stored, nil)

			got, err := svc.Get(ctx, workspaceID, "auto-123")
			require.NoError(t, err)

			_, hasSecret := got.Nodes[0].Config["secret"]
			assert.Equal(t, write, hasSecret)
			assert.Equal(t, "https://hooks.example.com/in", got.Nodes[0].Config["url"])
			assert.Equal(t, "s3cret", stored.Nodes[0].Config["secret"], "the stored config is unchanged")
		})

		t.Run(name+" on list", func(t *testing.T) {
			svc, repo, auth, _ := newAutomationTransitionMocks(t)
			stored := []*domain.Automation{storedAutomation(), storedAutomation()}
			auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, caller(write), nil)
			repo.EXPECT().List(ctx, workspaceID, gomock.Any()).Return(stored, 2, nil)

			got, total, err := svc.List(ctx, workspaceID, domain.AutomationFilter{})
			require.NoError(t, err)
			assert.Equal(t, 2, total)
			require.Len(t, got, len(stored))

			for i := range got {
				_, hasSecret := got[i].Nodes[0].Config["secret"]
				assert.Equal(t, write, hasSecret)
				assert.Equal(t, "s3cret", stored[i].Nodes[0].Config["secret"], "the stored config is unchanged")
			}
		})
	}
}
