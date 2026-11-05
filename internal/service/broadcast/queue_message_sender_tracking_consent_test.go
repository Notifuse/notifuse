package broadcast

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
)

// enqueueForConsent runs SendBatch for recipients and returns the entries it
// enqueued, keyed by recipient.
func enqueueForConsent(t *testing.T, broadcast *domain.Broadcast, policy domain.EmailTrackingPolicy, recipients []*domain.ContactWithList) map[string]*domain.EmailQueueEntry {
	t.Helper()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockBroadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	emailSender := domain.NewEmailSender("sender@example.com", "Test Sender")
	emailProvider := &domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, Senders: []domain.EmailSender{emailSender}}
	template := &domain.Template{
		ID: "template-1",
		Email: &domain.EmailTemplate{
			SenderID:         emailSender.ID,
			Subject:          "Subject",
			VisualEditorTree: createQueueValidTestTree(createQueueTestTextBlock("txt1", "Hello")),
		},
	}

	mockBroadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-1", broadcast.ID).Return(broadcast, nil)
	got := map[string]*domain.EmailQueueEntry{}
	mockQueueRepo.EXPECT().Enqueue(gomock.Any(), "workspace-1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, entries []*domain.EmailQueueEntry) error {
			for _, e := range entries {
				got[e.ContactEmail] = e
			}
			return nil
		})

	sender := NewQueueMessageSender(mockQueueRepo, mockBroadcastRepo, mocks.NewMockMessageHistoryRepository(ctrl), mocks.NewMockTemplateRepository(ctrl), nil, mockLogger, nil, "https://api.example.com")
	_, _, err := sender.SendBatch(context.Background(), "workspace-1", "integration-1",
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"https://track.example.com", "", policy, nil, broadcast.ID, recipients,
		map[string]*domain.Template{"template-1": template}, emailProvider, time.Now().Add(time.Minute), "")
	require.NoError(t, err)
	return got
}

func consentContact(email, opens, clicks string) *domain.ContactWithList {
	c := &domain.Contact{Email: email}
	if opens != "" {
		c.TrackingOpensConsent = &domain.NullableString{String: opens}
	}
	if clicks != "" {
		c.TrackingClicksConsent = &domain.NullableString{String: clicks}
	}
	return &domain.ContactWithList{Contact: c, ListID: "list-1"}
}

func hasPixel(entry *domain.EmailQueueEntry) bool {
	return strings.Contains(entry.Payload.HTMLContent, "https://track.example.com/t/")
}

func TestQueueMessageSender_SendBatch_FollowsEachRecipientsConsent(t *testing.T) {
	broadcast := &domain.Broadcast{ID: "broadcast-1", WorkspaceID: "workspace-1", Name: "B"}
	policy := domain.EmailTrackingPolicy{OpensEnabled: true, ClicksEnabled: true, UnknownConsent: domain.UnknownConsentTrack}

	entries := enqueueForConsent(t, broadcast, policy, []*domain.ContactWithList{
		consentContact("unknown@example.com", "", ""),
		consentContact("noopens@example.com", domain.TrackingConsentDenied, ""),
		consentContact("noclicks@example.com", "", domain.TrackingConsentDenied),
	})
	require.Len(t, entries, 3)

	unknown := entries["unknown@example.com"]
	assert.True(t, hasPixel(unknown))
	assert.Equal(t, true, *unknown.Payload.OpensTracked)
	assert.Equal(t, true, *unknown.Payload.ClicksTracked)
	assert.Contains(t, unknown.Payload.TemplateData["tracking_opens_url"], "https://track.example.com/t/")

	noOpens := entries["noopens@example.com"]
	assert.False(t, hasPixel(noOpens), "a refused open gets no pixel")
	assert.Equal(t, false, *noOpens.Payload.OpensTracked)
	assert.Equal(t, true, *noOpens.Payload.ClicksTracked)
	assert.Equal(t, "", noOpens.Payload.TemplateData["tracking_opens_url"], "nor a pixel URL for the template")

	noClicks := entries["noclicks@example.com"]
	assert.True(t, hasPixel(noClicks))
	assert.Equal(t, true, *noClicks.Payload.OpensTracked)
	assert.Equal(t, false, *noClicks.Payload.ClicksTracked)
}

func TestQueueMessageSender_SendBatch_UnknownConsentFollowsTheWorkspace(t *testing.T) {
	broadcast := &domain.Broadcast{ID: "broadcast-1", WorkspaceID: "workspace-1", Name: "B"}
	policy := domain.EmailTrackingPolicy{OpensEnabled: true, ClicksEnabled: true, UnknownConsent: domain.UnknownConsentNoTrack}

	entries := enqueueForConsent(t, broadcast, policy, []*domain.ContactWithList{
		consentContact("unknown@example.com", "", ""),
		consentContact("granted@example.com", domain.TrackingConsentGranted, domain.TrackingConsentGranted),
	})

	assert.False(t, hasPixel(entries["unknown@example.com"]))
	assert.Equal(t, false, *entries["unknown@example.com"].Payload.OpensTracked)
	assert.True(t, hasPixel(entries["granted@example.com"]))
	assert.Equal(t, true, *entries["granted@example.com"].Payload.OpensTracked)
}

func TestQueueMessageSender_SendBatch_NoTrackingBroadcastTracksNobody(t *testing.T) {
	broadcast := &domain.Broadcast{ID: "broadcast-1", WorkspaceID: "workspace-1", Name: "B", TrackingMode: domain.BroadcastTrackingModeNoTracking}
	policy := domain.EmailTrackingPolicy{OpensEnabled: true, ClicksEnabled: true, UnknownConsent: domain.UnknownConsentTrack}

	entries := enqueueForConsent(t, broadcast, policy, []*domain.ContactWithList{
		consentContact("granted@example.com", domain.TrackingConsentGranted, domain.TrackingConsentGranted),
	})
	entry := entries["granted@example.com"]
	assert.False(t, hasPixel(entry))
	assert.Equal(t, false, *entry.Payload.OpensTracked)
	assert.Equal(t, false, *entry.Payload.ClicksTracked)
}

func TestQueueMessageSender_BuildQueueEntry_MintsNoIdentityWithoutClickConsent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	qms := &queueMessageSender{logger: mockLogger}
	emailSender := domain.NewEmailSender("sender@example.com", "Test Sender")
	emailProvider := &domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, Senders: []domain.EmailSender{emailSender}}
	template := &domain.Template{
		ID: "template-1",
		Email: &domain.EmailTemplate{
			SenderID:         emailSender.ID,
			Subject:          "Subject",
			VisualEditorTree: createQueueValidTestTree(createQueueTestTextBlock("txt1", `<a href="https://shop.example.com/x">Shop</a>`)),
		},
	}
	webAnalytics := &domain.WebAnalyticsSettings{Enabled: true, IdentifyFromEmailLinks: true, AllowedDomains: []string{"shop.example.com"}}

	build := func(decision domain.EmailTrackingDecision) *domain.EmailQueueEntry {
		entry, err := qms.buildQueueEntry(context.Background(), "workspace-1", "integration-1",
			"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			"https://track.example.com", decision, webAnalytics,
			&domain.Broadcast{ID: "broadcast-1"}, "msg-1", "a@example.com", template,
			map[string]interface{}{}, emailProvider, "", "")
		require.NoError(t, err)
		return entry
	}

	// Clicks off at the workspace but the contact's click consent allows the
	// identity: the direct link carries the token, which shows the mint ran.
	require.True(t, webAnalytics.CanIdentifyFromEmailLinks(), "the fixture must allow identification")
	identified := build(domain.EmailTrackingDecision{Opens: true, Identify: true, UTM: true})
	assert.Contains(t, identified.Payload.HTMLContent, notifuse_mjml.WebIdentifyQueryParam+"=")

	refused := build(domain.EmailTrackingDecision{Opens: true, UTM: true})
	assert.NotContains(t, refused.Payload.HTMLContent, "https://track.example.com/r/", "no click redirect")
	assert.NotContains(t, refused.Payload.HTMLContent, notifuse_mjml.WebIdentifyQueryParam+"=", "no identity token on the links")
	assert.Equal(t, true, *refused.Payload.OpensTracked)
	assert.Equal(t, false, *refused.Payload.ClicksTracked)
}
