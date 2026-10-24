package broadcast

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	domainmocks "github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// Winner-phase template resolution: without a winning template the phase
// cannot start (hard error), an already-loaded winner must not be
// re-fetched, and a fetched winner is validated before it is cached into
// the loaded set the send loop reads.

func validBroadcastTemplate(id string) *domain.Template {
	mjml := "<mjml><mj-body></mj-body></mjml>"
	return &domain.Template{
		ID: id,
		Email: &domain.EmailTemplate{
			Subject:    "Hello",
			EditorMode: domain.EditorModeCode,
			MjmlSource: &mjml,
		},
	}
}

func winnerOrchestrator(t *testing.T) (*BroadcastOrchestrator, *domainmocks.MockTemplateRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)
	templateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	return &BroadcastOrchestrator{
		logger:       logger.NewLoggerWithLevel("disabled"),
		templateRepo: templateRepo,
	}, templateRepo
}

func TestOrchestrator_WinnerTemplates(t *testing.T) {
	ctx := context.Background()
	winnerID := "tpl-winner"

	t.Run("winner phase without a winner is a hard error", func(t *testing.T) {
		o, _ := winnerOrchestrator(t)
		_, err := o.winnerTemplates(ctx, "ws-1", &domain.Broadcast{}, map[string]*domain.Template{})
		require.EqualError(t, err, "winner phase but no winning template selected")
	})

	t.Run("an already-loaded winner is reused without a fetch", func(t *testing.T) {
		o, _ := winnerOrchestrator(t)
		loaded := map[string]*domain.Template{winnerID: validBroadcastTemplate(winnerID)}
		got, err := o.winnerTemplates(ctx, "ws-1",
			&domain.Broadcast{WinningTemplate: &winnerID}, loaded)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Same(t, loaded[winnerID], got[winnerID])
	})

	t.Run("a missing winner is fetched, validated, and cached", func(t *testing.T) {
		o, templateRepo := winnerOrchestrator(t)
		fetched := validBroadcastTemplate(winnerID)
		templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "ws-1", winnerID, int64(0)).
			Return(fetched, nil).Times(1)
		loaded := map[string]*domain.Template{}

		got, err := o.winnerTemplates(ctx, "ws-1",
			&domain.Broadcast{WinningTemplate: &winnerID}, loaded)
		require.NoError(t, err)
		assert.Same(t, fetched, got[winnerID])
		assert.Same(t, fetched, loaded[winnerID],
			"the fetched winner joins the loaded set for the send loop")
	})

	t.Run("an unfetchable winner fails the phase", func(t *testing.T) {
		o, templateRepo := winnerOrchestrator(t)
		templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "ws-1", winnerID, int64(0)).
			Return(nil, errors.New("template deleted")).Times(1)

		_, err := o.winnerTemplates(ctx, "ws-1",
			&domain.Broadcast{WinningTemplate: &winnerID}, map[string]*domain.Template{})
		require.Error(t, err)
	})

	t.Run("an invalid winner template fails validation, not silently", func(t *testing.T) {
		o, templateRepo := winnerOrchestrator(t)
		templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "ws-1", winnerID, int64(0)).
			Return(&domain.Template{ID: winnerID}, nil).Times(1) // no Email block

		_, err := o.winnerTemplates(ctx, "ws-1",
			&domain.Broadcast{WinningTemplate: &winnerID}, map[string]*domain.Template{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "template missing email configuration")
	})
}

func TestOrchestrator_LoadTemplates_PartialFailure(t *testing.T) {
	ctx := context.Background()

	t.Run("one bad template does not fail the batch, zero valid does", func(t *testing.T) {
		o, templateRepo := winnerOrchestrator(t)
		templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "ws-1", "good", int64(0)).
			Return(validBroadcastTemplate("good"), nil)
		templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "ws-1", "bad", int64(0)).
			Return(nil, errors.New("gone"))

		got, err := o.LoadTemplates(ctx, "ws-1", []string{"good", "bad"})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Contains(t, got, "good")
	})

	t.Run("no valid templates is a template-missing error", func(t *testing.T) {
		o, templateRepo := winnerOrchestrator(t)
		templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "ws-1", "bad", int64(0)).
			Return(nil, errors.New("gone"))

		_, err := o.LoadTemplates(ctx, "ws-1", []string{"bad"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no valid templates found")
	})
}

func TestOrchestrator_ValidateTemplates(t *testing.T) {
	o, _ := winnerOrchestrator(t)

	assert.ErrorContains(t, o.ValidateTemplates(map[string]*domain.Template{}),
		"no templates provided")
	assert.ErrorContains(t, o.ValidateTemplates(map[string]*domain.Template{"x": nil}),
		"template is nil")
	assert.ErrorContains(t, o.ValidateTemplates(map[string]*domain.Template{
		"x": {ID: "x"},
	}), "template missing email configuration")
	assert.ErrorContains(t, o.ValidateTemplates(map[string]*domain.Template{
		"x": {ID: "x", Email: &domain.EmailTemplate{EditorMode: domain.EditorModeCode}},
	}), "template missing subject")

	err := o.ValidateTemplates(map[string]*domain.Template{
		"x": {ID: "x", Email: &domain.EmailTemplate{
			Subject: "Hi", EditorMode: domain.EditorModeCode,
		}},
	})
	require.ErrorContains(t, err, "template missing content")

	require.NoError(t, o.ValidateTemplates(map[string]*domain.Template{
		"x": validBroadcastTemplate("x"),
	}))
}
