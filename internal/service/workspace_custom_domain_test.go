package service

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// A custom domain belongs to one workspace: one that saved another workspace's domain
// could show its own emails, web versions included, under that workspace's brand.
func TestWorkspaceService_UpdateWorkspace_CustomDomainIsNotShared(t *testing.T) {
	ctx := context.Background()
	settingsWith := func(endpoint string) domain.WorkspaceSettings {
		return domain.WorkspaceSettings{Timezone: "UTC", DefaultLanguage: "en", Languages: []string{"en"}, CustomEndpointURL: &endpoint}
	}

	t.Run("refuses a domain another workspace uses, however it is written", func(t *testing.T) {
		for _, endpoint := range []string{
			"https://mail.brand-b.example",
			"https://MAIL.Brand-B.example:443/",
			"https://mail.brand-b.example.",
		} {
			svc, deps := covWorkspaceService(gomock.NewController(t), nil, nil, nil)
			deps.expectOwner(ctx, "ws1", "user-1")
			deps.repo.EXPECT().GetByID(gomock.Any(), "ws1").Return(existingWorkspaceForUpdate(), nil)
			deps.repo.EXPECT().ListCustomEndpoints(gomock.Any()).Return(map[string]string{
				"brand_b": "https://mail.brand-b.example",
			}, nil)
			deps.repo.EXPECT().Update(gomock.Any(), gomock.Any()).Times(0)

			_, err := svc.UpdateWorkspace(ctx, "ws1", "Old Name", settingsWith(endpoint))
			var validation domain.ValidationError
			require.ErrorAs(t, err, &validation, endpoint)
			assert.Contains(t, validation.Message, "another workspace")
		}
	})

	t.Run("keeps a domain the workspace already has without looking it up", func(t *testing.T) {
		svc, deps := covWorkspaceService(gomock.NewController(t), nil, nil, nil)
		deps.expectOwner(ctx, "ws1", "user-1")
		existing := existingWorkspaceForUpdate()
		endpoint := "https://mail.brand-a.example"
		existing.Settings.CustomEndpointURL = &endpoint
		deps.repo.EXPECT().GetByID(gomock.Any(), "ws1").Return(existing, nil)
		deps.repo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)

		_, err := svc.UpdateWorkspace(ctx, "ws1", "Old Name", settingsWith(endpoint))
		require.NoError(t, err)
	})

	t.Run("a failure to read the other domains refuses the save", func(t *testing.T) {
		svc, deps := covWorkspaceService(gomock.NewController(t), nil, nil, nil)
		deps.expectOwner(ctx, "ws1", "user-1")
		deps.repo.EXPECT().GetByID(gomock.Any(), "ws1").Return(existingWorkspaceForUpdate(), nil)
		deps.repo.EXPECT().ListCustomEndpoints(gomock.Any()).Return(nil, errors.New("db down"))
		deps.repo.EXPECT().Update(gomock.Any(), gomock.Any()).Times(0)

		_, err := svc.UpdateWorkspace(ctx, "ws1", "Old Name", settingsWith("https://mail.brand-a.example"))
		assert.Error(t, err)
	})
}
