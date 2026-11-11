package service_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func displayConditionTree(t *testing.T, condition string) notifuse_mjml.EmailBlock {
	t.Helper()
	tree, err := notifuse_mjml.UnmarshalEmailBlock([]byte(`{"id":"root","type":"mjml","children":[{"id":"body","type":"mj-body","children":[
		{"id":"sec","type":"mj-section","children":[{"id":"col","type":"mj-column","children":[
			{"id":"txt","type":"mj-text","content":"always"},
			{"id":"img","type":"mj-image","attributes":{"src":"https://example.com/promo.png"},"display_condition":` + condition + `}]}]}]}]}`))
	require.NoError(t, err)
	return tree
}

func templateWriter() *domain.UserWorkspace {
	return &domain.UserWorkspace{UserID: "u1", WorkspaceID: "ws-1", Role: "member", Permissions: domain.UserPermissions{
		domain.PermissionResourceTemplates: {Read: true, Write: true},
	}}
}

func TestTemplateService_DisplayConditions(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateTemplate refuses an invalid condition without writing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, mockRepo, _, mockAuth, _ := setupTemplateServiceTest(ctrl)
		mockAuth.EXPECT().AuthenticateUserForWorkspace(ctx, "ws-1").Return(ctx, &domain.User{ID: "u1"}, templateWriter(), nil)
		mockRepo.EXPECT().CreateTemplate(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		err := svc.CreateTemplate(ctx, "ws-1", &domain.Template{
			ID: "tpl", Name: "Promo", Channel: domain.ChannelEmail, Category: "marketing",
			Email: &domain.EmailTemplate{Subject: "s", VisualEditorTree: displayConditionTree(t,
				`{"match":"all","rules":[{"variable":"order.total","operator":"gt","value":"lots"}]}`)},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "needs a number")
	})

	t.Run("CompileTemplate hides the block when the condition fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, _, mockWorkspaceRepo, mockAuth, _ := setupTemplateServiceTest(ctrl)
		mockAuth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").Return(ctx, &domain.User{ID: "u1"}, templateWriter(), nil).Times(2)
		mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(&domain.Workspace{ID: "ws-1"}, nil).Times(2)

		condition := `{"match":"all","rules":[{"variable":"order.total","operator":"gte","value":"100"}]}`
		for total, shown := range map[float64]bool{150: true, 50: false} {
			resp, err := svc.CompileTemplate(ctx, domain.CompileTemplateRequest{
				WorkspaceID: "ws-1", MessageID: "m", VisualEditorTree: displayConditionTree(t, condition),
				TemplateData: notifuse_mjml.MapOfAny{"order": map[string]any{"total": total}},
			})
			require.NoError(t, err)
			require.True(t, resp.Success)
			assert.Equal(t, shown, strings.Contains(*resp.HTML, "promo.png"), "total %v", total)
			assert.Contains(t, *resp.HTML, "always")
		}
	})
}
