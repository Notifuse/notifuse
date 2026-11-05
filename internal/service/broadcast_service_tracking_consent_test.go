package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBroadcastService_SendToIndividual_FollowsTrackingConsent(t *testing.T) {
	const recipient = "to@example.com"
	denied := &domain.NullableString{String: domain.TrackingConsentDenied}
	granted := &domain.NullableString{String: domain.TrackingConsentGranted}

	cases := []struct {
		name           string
		contact        *domain.Contact
		unknownConsent string
		mode           string
		wantOpens      bool
		wantClicks     bool
	}{
		{name: "clicks denied", contact: &domain.Contact{Email: recipient, TrackingClicksConsent: denied}, unknownConsent: domain.UnknownConsentTrack, wantOpens: true},
		{name: "no contact, unknown not tracked", unknownConsent: domain.UnknownConsentNoTrack},
		{name: "no contact, unknown tracked", unknownConsent: domain.UnknownConsentTrack, wantOpens: true, wantClicks: true},
		{name: "granted contact, unknown not tracked", contact: &domain.Contact{Email: recipient, TrackingOpensConsent: granted, TrackingClicksConsent: granted}, unknownConsent: domain.UnknownConsentNoTrack, wantOpens: true, wantClicks: true},
		{name: "broadcast without tracking", contact: &domain.Contact{Email: recipient, TrackingOpensConsent: granted, TrackingClicksConsent: granted}, unknownConsent: domain.UnknownConsentTrack, mode: notifuse_mjml.TrackingModeNoTracking},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := setupBroadcastSvc(t)
			defer d.ctrl.Finish()

			ctx := context.Background()
			req := &domain.SendToIndividualRequest{WorkspaceID: "w1", BroadcastID: "b1", RecipientEmail: recipient}
			authOK(d.authService, ctx, req.WorkspaceID)

			sender := domain.NewEmailSender("from@example.com", "From")
			workspace := &domain.Workspace{
				ID: "w1",
				Settings: domain.WorkspaceSettings{
					MarketingEmailProviderID: "mkt",
					SecretKey:                "sk_test",
					EmailTrackingEnabled:     true,
					EmailTracking:            &domain.EmailTrackingSettings{OpensEnabled: true, ClicksEnabled: true, UnknownConsent: tc.unknownConsent},
				},
				Integrations: domain.Integrations{
					{ID: "mkt", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, Senders: []domain.EmailSender{sender}}},
				},
			}
			d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(workspace, nil)

			b := testBroadcast(req.WorkspaceID, req.BroadcastID)
			b.TrackingMode = tc.mode
			b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tplA"}}
			d.repo.EXPECT().GetBroadcast(ctx, req.WorkspaceID, req.BroadcastID).Return(b, nil)

			if tc.contact != nil {
				d.contactRepo.EXPECT().GetContactByEmail(ctx, req.WorkspaceID, recipient).Return(tc.contact, nil)
			} else {
				d.contactRepo.EXPECT().GetContactByEmail(ctx, req.WorkspaceID, recipient).Return(nil, errors.New("not found"))
			}

			template := &domain.Template{
				ID:      "tplA",
				Name:    "Template A",
				Channel: "email",
				Email: &domain.EmailTemplate{
					SenderID:         sender.ID,
					Subject:          "Hello",
					VisualEditorTree: createMJMLRootBlock(),
				},
				Category: string(domain.TemplateCategoryMarketing),
			}
			d.templateSvc.EXPECT().GetTemplateByID(ctx, req.WorkspaceID, "tplA", int64(0)).Return(template, nil)

			var compiled notifuse_mjml.TrackingSettings
			compiledHTML := "<html>ok</html>"
			d.templateSvc.EXPECT().CompileTemplate(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, payload domain.CompileTemplateRequest) (*domain.CompileTemplateResponse, error) {
				compiled = payload.TrackingSettings
				return &domain.CompileTemplateResponse{Success: true, HTML: &compiledHTML}, nil
			})
			d.emailSvc.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(nil)

			var stored *domain.MessageHistory
			d.messageHistoryRepo.EXPECT().Create(gomock.Any(), req.WorkspaceID, gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _, _ string, m *domain.MessageHistory) error {
				stored = m
				return nil
			})

			require.NoError(t, d.svc.SendToIndividual(ctx, req))

			assert.Equal(t, !tc.wantOpens, compiled.SuppressOpens)
			assert.Equal(t, !tc.wantClicks, compiled.SuppressClicks)
			assert.Equal(t, tc.wantOpens || tc.wantClicks, compiled.EnableTracking)

			require.NotNil(t, stored)
			require.NotNil(t, stored.OpensTracked)
			require.NotNil(t, stored.ClicksTracked)
			assert.Equal(t, tc.wantOpens, *stored.OpensTracked)
			assert.Equal(t, tc.wantClicks, *stored.ClicksTracked)
		})
	}
}

func TestBroadcastService_GetTestResults_RatesOverTrackedEmails(t *testing.T) {
	run := func(t *testing.T, statsA, statsB *domain.MessageHistoryStatusSum) *domain.TestResultsResponse {
		t.Helper()
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()
		ctx := context.Background()
		authOK(d.authService, ctx, "w1")

		b := testBroadcast("w1", "b1")
		b.Status = domain.BroadcastStatusTesting
		b.TestSettings.Enabled = true
		b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tplA"}, {VariationName: "B", TemplateID: "tplB"}}
		d.repo.EXPECT().GetBroadcast(ctx, "w1", "b1").Return(b, nil)
		d.messageHistoryRepo.EXPECT().GetBroadcastVariationStats(ctx, "w1", "b1", "tplA").Return(statsA, nil)
		d.messageHistoryRepo.EXPECT().GetBroadcastVariationStats(ctx, "w1", "b1", "tplB").Return(statsB, nil)

		res, err := d.svc.GetTestResults(ctx, "w1", "b1")
		require.NoError(t, err)
		return res
	}

	t.Run("rates and tracked counts", func(t *testing.T) {
		res := run(t,
			&domain.MessageHistoryStatusSum{TotalSent: 100, TotalOpensTracked: 50, TotalClicksTracked: 40, TotalOpened: 25, TotalClicked: 4},
			&domain.MessageHistoryStatusSum{TotalSent: 100, TotalOpensTracked: 100, TotalClicksTracked: 100, TotalOpened: 30, TotalClicked: 5},
		)
		a := res.VariationResults["tplA"]
		assert.InDelta(t, 0.5, a.OpenRate, 1e-9)
		assert.InDelta(t, 0.1, a.ClickRate, 1e-9)
		assert.Equal(t, 50, a.OpensTracked)
		assert.Equal(t, 40, a.ClicksTracked)
		assert.Equal(t, "tplA", res.RecommendedWinner, "A: 0.7*0.1+0.3*0.5 beats B: 0.7*0.05+0.3*0.3")
	})

	t.Run("a variation with no tracked email is not recommended", func(t *testing.T) {
		res := run(t,
			&domain.MessageHistoryStatusSum{TotalSent: 100},
			&domain.MessageHistoryStatusSum{TotalSent: 100},
		)
		assert.Empty(t, res.RecommendedWinner)
		assert.Equal(t, 0.0, res.VariationResults["tplA"].OpenRate)
	})
}
