package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// The sample broadcasts' message history is dated and templated from the broadcast rows: a
// broadcast's emails leave over its own send window, and the A/B test's in its two phases.

// demoTestBroadcasts are the sample broadcasts as createSampleBroadcasts marks them sent: ten,
// eight, six and four days before now, the last of them the A/B test.
func demoTestBroadcasts(now time.Time) []*domain.Broadcast {
	var broadcasts []*domain.Broadcast
	for i, campaign := range demoBroadcastCampaigns {
		broadcast := &domain.Broadcast{ID: fmt.Sprintf("bc-%d", i+1), Name: campaign.Name,
			UTMParameters: &domain.UTMParameters{Source: demoBroadcastUTMSource, Medium: "email", Campaign: campaign.Campaign}}
		broadcast.TestSettings.Enabled = len(campaign.Templates) > 1
		broadcast.TestSettings.SamplePercentage = 10
		for v, templateID := range campaign.Templates {
			broadcast.TestSettings.Variations = append(broadcast.TestSettings.Variations, domain.BroadcastVariation{
				VariationName: fmt.Sprintf("variation-%c", 'a'+rune(v)),
				TemplateID:    templateID,
			})
		}
		demoMarkBroadcastSent(broadcast, now.AddDate(0, 0, -(10-2*i)))
		broadcasts = append(broadcasts, broadcast)
	}
	return broadcasts
}

// demoTestCampaigns are the transactional campaigns and one campaign per broadcast.
func demoTestCampaigns(t *testing.T, broadcasts []*domain.Broadcast) []demoCampaignTemplate {
	t.Helper()
	campaigns := append([]demoCampaignTemplate{}, demoTransactionalCampaigns...)
	for _, broadcast := range broadcasts {
		campaign, ok := demoBroadcastCampaign(broadcast)
		require.True(t, ok, broadcast.ID)
		campaigns = append(campaigns, campaign)
	}
	return campaigns
}

func TestDemoMarkBroadcastSent(t *testing.T) {
	startedAt := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)

	single := &domain.Broadcast{TestSettings: domain.BroadcastTestSettings{
		Variations: []domain.BroadcastVariation{{TemplateID: "newsletter-weekly"}}}}
	demoMarkBroadcastSent(single, startedAt)
	assert.Equal(t, domain.BroadcastStatusProcessed, single.Status)
	assert.Equal(t, startedAt.Add(-demoBroadcastDraftLead), single.CreatedAt, "written before it went out")
	assert.Equal(t, startedAt, *single.StartedAt)
	assert.Equal(t, startedAt.Add(demoBroadcastSendWindow), *single.CompletedAt)
	assert.Nil(t, single.WinningTemplate)
	assert.Nil(t, single.TestSentAt)
	assert.Nil(t, single.WinnerSentAt)
	assert.False(t, demoIsABTest(single))

	ab := &domain.Broadcast{TestSettings: domain.BroadcastTestSettings{Enabled: true, SamplePercentage: 10,
		Variations: []domain.BroadcastVariation{{TemplateID: "newsletter-weekly"}, {TemplateID: "newsletter-weekly-v2"}}}}
	require.True(t, demoIsABTest(ab), "a test with two variations is the A/B test before it is marked")
	demoMarkBroadcastSent(ab, startedAt)
	require.NotNil(t, ab.WinningTemplate)
	assert.Equal(t, "newsletter-weekly-v2", *ab.WinningTemplate, "the last variation is the winner")
	assert.Equal(t, startedAt, *ab.TestSentAt, "the sample leaves as the send starts")
	assert.Equal(t, startedAt.Add(demoABWinnerDelay), *ab.WinnerSentAt)
	assert.True(t, ab.TestSentAt.Add(demoABTestSendWindow).Before(*ab.WinnerSentAt), "the sample has left before the winner is sent")
	assert.True(t, ab.WinnerSentAt.Before(*ab.CompletedAt), "the winner leaves before the send completes")

	// Two variations with the test off, or a test with one variation, is not an A/B test: one
	// predicate decides it for the broadcast's dates and for its messages.
	twoOff := &domain.Broadcast{TestSettings: domain.BroadcastTestSettings{
		Variations: []domain.BroadcastVariation{{TemplateID: "newsletter-weekly"}, {TemplateID: "newsletter-weekly-v2"}}}}
	demoMarkBroadcastSent(twoOff, startedAt)
	assert.False(t, demoIsABTest(twoOff))
	assert.Nil(t, twoOff.WinningTemplate)
	oneOn := &domain.Broadcast{TestSettings: domain.BroadcastTestSettings{Enabled: true,
		Variations: []domain.BroadcastVariation{{TemplateID: "newsletter-weekly"}}}}
	demoMarkBroadcastSent(oneOn, startedAt)
	assert.False(t, demoIsABTest(oneOn))
	assert.Nil(t, oneOn.WinningTemplate)
}

// Every email of a sample broadcast leaves within its send window, from its started_at, with
// one of its templates. The A/B test's leave in its two phases: the sample, with both
// variations, after test_sent_at; everyone else, with the winner, after winner_sent_at.
func TestDemoBroadcastSend_FollowsItsBroadcast(t *testing.T) {
	const draws = 5000
	now := time.Now()
	for _, broadcast := range demoTestBroadcasts(now) {
		testTemplates := map[string]int{}
		testPhase := 0
		for i := 0; i < draws; i++ {
			templateID, sentAt := demoBroadcastSend(broadcast, now)
			require.False(t, sentAt.Before(*broadcast.StartedAt), "%s: sent before it started", broadcast.Name)
			require.False(t, sentAt.After(*broadcast.CompletedAt), "%s: sent after it completed", broadcast.Name)
			require.False(t, sentAt.After(now), "%s: sent after now", broadcast.Name)
			if !demoIsABTest(broadcast) {
				require.Equal(t, broadcast.TestSettings.Variations[0].TemplateID, templateID, broadcast.Name)
				require.False(t, demoInTestPhase(broadcast, sentAt))
				continue
			}
			if demoInTestPhase(broadcast, sentAt) {
				testPhase++
				testTemplates[templateID]++
				require.False(t, sentAt.Before(*broadcast.TestSentAt), "a sample email left before test_sent_at")
				require.False(t, sentAt.After(broadcast.TestSentAt.Add(demoABTestSendWindow)), "a sample email left after the sample window")
			} else {
				require.Equal(t, *broadcast.WinningTemplate, templateID, "a winner-phase email was sent another template")
				require.False(t, sentAt.Before(*broadcast.WinnerSentAt), "a winner-phase email left before winner_sent_at")
			}
		}
		if demoIsABTest(broadcast) {
			for _, variation := range broadcast.TestSettings.Variations {
				assert.Positive(t, testTemplates[variation.TemplateID], "the sample has no email of %s", variation.TemplateID)
			}
			assert.InDelta(t, float64(broadcast.TestSettings.SamplePercentage)/100, float64(testPhase)/draws, 0.03,
				"the sample is not about its percentage of the recipients")
		}
	}
}

// A broadcast that started moments before the seed's now has none of its emails leave after it.
func TestDemoBroadcastSend_NothingAfterNow(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	broadcast := &domain.Broadcast{TestSettings: domain.BroadcastTestSettings{
		Variations: []domain.BroadcastVariation{{TemplateID: "newsletter-weekly"}}}}
	demoMarkBroadcastSent(broadcast, now.Add(-10*time.Minute))
	for i := 0; i < 1000; i++ {
		_, sentAt := demoBroadcastSend(broadcast, now)
		require.False(t, sentAt.After(now))
		require.False(t, sentAt.Before(*broadcast.StartedAt))
	}
}

func TestDemoBroadcastCampaign_NeedsASentBroadcast(t *testing.T) {
	_, ok := demoBroadcastCampaign(nil)
	assert.False(t, ok)
	_, ok = demoBroadcastCampaign(&domain.Broadcast{ID: "bc-draft", TestSettings: domain.BroadcastTestSettings{
		Variations: []domain.BroadcastVariation{{TemplateID: "newsletter-weekly"}}}})
	assert.False(t, ok, "a broadcast never marked as sent has no dates for its messages")
	noTemplate := &domain.Broadcast{ID: "bc-empty"}
	demoMarkBroadcastSent(noTemplate, time.Now().AddDate(0, 0, -1))
	_, ok = demoBroadcastCampaign(noTemplate)
	assert.False(t, ok, "a broadcast with no variation has no template for its messages")
	abWithoutWinner := demoTestBroadcasts(time.Now())[3]
	abWithoutWinner.WinnerSentAt = nil
	_, ok = demoBroadcastCampaign(abWithoutWinner)
	assert.False(t, ok, "an A/B test with no winner sent has no date for its winner phase")

	now := time.Now()
	broadcast := demoTestBroadcasts(now)[0]
	campaign, ok := demoBroadcastCampaign(broadcast)
	require.True(t, ok)
	assert.Same(t, broadcast, campaign.broadcast)
	send := campaign.draw(now)
	assert.Equal(t, "newsletter-weekly", send.templateID)
	assert.False(t, send.sentAt.Before(*broadcast.StartedAt))
}

// A contact's emails of a broadcast are dated and templated by that broadcast, whatever else
// the contact is sent, and no event of theirs is dated after now or before the email left.
func TestDemoContactMessages_DatedByTheirBroadcast(t *testing.T) {
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}
	now := time.Now()
	broadcasts := demoTestBroadcasts(now)
	byID := map[string]*domain.Broadcast{}
	for _, broadcast := range broadcasts {
		byID[broadcast.ID] = broadcast
	}
	campaigns := demoTestCampaigns(t, broadcasts)
	ok := func(*domain.MessageHistory) error { return nil }
	seen := map[string]bool{}
	for i := 0; i < 2000; i++ {
		for _, data := range svc.demoContactMessages(&domain.Contact{Email: fmt.Sprintf("c%d@example.com", i)}, campaigns, now, ok) {
			message, engagement := data.message, data.engagement
			for name, at := range map[string]time.Time{
				"delivered": engagement.deliveredTime, "bounced": engagement.bouncedTime, "opened": engagement.openedTime,
				"clicked": engagement.clickedTime, "complained": engagement.complainedTime,
			} {
				if !at.IsZero() {
					require.False(t, at.Before(message.SentAt), "%s before the email was sent", name)
					require.False(t, at.After(now), "%s after now", name)
				}
			}
			require.False(t, message.SentAt.After(now), "sent after now")
			assert.Equal(t, message.SentAt, message.CreatedAt)
			if message.BroadcastID == nil {
				continue
			}
			broadcast := byID[*message.BroadcastID]
			require.NotNil(t, broadcast)
			seen[broadcast.ID] = true
			require.False(t, message.SentAt.Before(*broadcast.StartedAt), "%s: sent before it started", broadcast.Name)
			require.False(t, message.SentAt.After(*broadcast.CompletedAt), "%s: sent after it completed", broadcast.Name)
			assert.Equal(t, broadcast.UTMParameters.Campaign, message.MessageData.Data["utm_campaign"])
			if !demoIsABTest(broadcast) {
				require.Equal(t, broadcast.TestSettings.Variations[0].TemplateID, message.TemplateID)
			}
		}
	}
	for _, broadcast := range broadcasts {
		assert.True(t, seen[broadcast.ID], "%s sent nothing", broadcast.Name)
	}
}

// demoTestBroadcastsDB is a workspace connection that takes the query dating the broadcasts.
func demoTestBroadcastsDB(t *testing.T, broadcasts []*domain.Broadcast) *mocks.MockWorkspaceRepository {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	ids, created, updated := make([]string, 0), make([]string, 0), make([]string, 0)
	for _, b := range broadcasts {
		ids = append(ids, b.ID)
		created = append(created, b.CreatedAt.UTC().Format(time.RFC3339Nano))
		updated = append(updated, b.CompletedAt.UTC().Format(time.RFC3339Nano))
	}
	mock.ExpectExec(`UPDATE broadcasts AS b\s+SET created_at = v.created_at, updated_at = v.updated_at`).
		WithArgs(pq.Array(ids), pq.Array(created), pq.Array(updated)).
		WillReturnResult(sqlmock.NewResult(0, int64(len(broadcasts))))
	t.Cleanup(func() { assert.NoError(t, mock.ExpectationsWereMet()) })
	repo := mocks.NewMockWorkspaceRepository(gomock.NewController(t))
	repo.EXPECT().GetConnection(gomock.Any(), "demo").Return(db, nil)
	return repo
}

// The counters a real send leaves on a broadcast are made of the emails the seed created: every
// broadcast's enqueued_count, and the A/B test's two phases.
func TestDemoService_GenerateMessagesPerContact_BroadcastCounters(t *testing.T) {
	ctrl := gomock.NewController(t)
	ctx := context.Background()
	messages := mocks.NewMockMessageHistoryRepository(ctrl)
	broadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
	now := time.Now()
	broadcasts := demoTestBroadcasts(now)
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled"), messageHistoryRepo: messages, broadcastRepo: broadcastRepo,
		workspaceRepo: demoTestBroadcastsDB(t, broadcasts)}

	var created []*domain.MessageHistory
	sentAt := map[string]time.Time{}
	messages.EXPECT().Create(ctx, "demo", "sk", gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, message *domain.MessageHistory) error {
			created = append(created, message)
			sentAt[message.ID] = message.SentAt
			return nil
		}).AnyTimes()
	// A batch's events are set after its messages are created.
	eventAfterSend := func(id string, at time.Time) {
		sent, ok := sentAt[id]
		require.True(t, ok, "an event of %s, never created", id)
		assert.False(t, at.After(now), "an event of %s after now", id)
		assert.False(t, at.Before(sent), "an event of %s before it was sent", id)
	}
	messages.EXPECT().SetOpened(ctx, "demo", gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _, id string, at time.Time) error { eventAfterSend(id, at); return nil }).AnyTimes()
	messages.EXPECT().SetClicked(ctx, "demo", gomock.Any(), gomock.Any(), gomock.Any(), true).DoAndReturn(
		func(_ context.Context, _, id string, at time.Time, _ string, _ bool) error {
			eventAfterSend(id, at)
			return nil
		}).AnyTimes()
	// The broadcasts are written once, with their counters, by saveSampleBroadcasts: the
	// repository refuses to update a processed broadcast, so a write of the counters after the
	// status would be dropped.
	saving := false
	written := map[string]domain.Broadcast{}
	broadcastRepo.EXPECT().UpdateBroadcast(ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, b *domain.Broadcast) error {
			require.True(t, saving, "a broadcast was written before its counters were complete")
			_, again := written[b.ID]
			require.False(t, again, "%s was written twice", b.ID)
			written[b.ID] = *b
			return nil
		}).AnyTimes()

	contacts := make([]*domain.Contact, 0, 400)
	for i := 0; i < cap(contacts); i++ {
		contacts = append(contacts, &domain.Contact{Email: fmt.Sprintf("c%d@example.com", i)})
	}
	count, err := svc.generateMessagesPerContact(ctx, "demo", "sk", contacts, broadcasts, now)
	require.NoError(t, err)
	assert.Equal(t, len(created), count)
	saving = true
	require.NoError(t, svc.saveSampleBroadcasts(ctx, "demo", broadcasts))

	byBroadcast := map[string][]*domain.MessageHistory{}
	for _, message := range created {
		if message.BroadcastID != nil {
			byBroadcast[*message.BroadcastID] = append(byBroadcast[*message.BroadcastID], message)
		}
	}
	require.Len(t, written, len(broadcasts), "every broadcast gets its counters")
	for _, broadcast := range broadcasts {
		sent := byBroadcast[broadcast.ID]
		require.NotEmpty(t, sent, broadcast.Name)
		got := written[broadcast.ID]
		assert.Equal(t, len(sent), got.EnqueuedCount, "%s: enqueued_count is not its emails", broadcast.Name)
		assert.Equal(t, domain.BroadcastStatusProcessed, got.Status, "the counters are written on the broadcast as sent")
		if !demoIsABTest(broadcast) {
			assert.Zero(t, got.TestPhaseRecipientCount, broadcast.Name)
			assert.Zero(t, got.WinnerPhaseRecipientCount, broadcast.Name)
			continue
		}
		var sample, rest int
		templates := map[string]int{}
		for _, message := range sent {
			if message.SentAt.Before(*broadcast.WinnerSentAt) {
				sample++
				templates[message.TemplateID]++
				continue
			}
			rest++
			assert.Equal(t, *broadcast.WinningTemplate, message.TemplateID, "a winner-phase email was sent another template")
		}
		assert.Equal(t, sample, got.TestPhaseRecipientCount, "the test phase count is not the sample's emails")
		assert.Equal(t, rest, got.WinnerPhaseRecipientCount, "the winner phase count is not the other emails")
		assert.Equal(t, got.EnqueuedCount, got.TestPhaseRecipientCount+got.WinnerPhaseRecipientCount)
		for _, variation := range broadcast.TestSettings.Variations {
			assert.Positive(t, templates[variation.TemplateID], "the sample has no email of %s", variation.TemplateID)
		}
		assert.Positive(t, rest, "nobody was sent the winner")
	}
}

// Marking the broadcasts as sent is part of the seed: a broadcast left a draft with a whole
// history behind it could be sent for real, so a failure stops the seed.
func TestDemoService_SaveSampleBroadcasts_Failures(t *testing.T) {
	ctx := context.Background()
	broadcasts := demoTestBroadcasts(time.Now())

	t.Run("a broadcast that cannot be written", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		broadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
		broadcastRepo.EXPECT().UpdateBroadcast(ctx, broadcasts[0]).Return(nil)
		broadcastRepo.EXPECT().UpdateBroadcast(ctx, broadcasts[1]).Return(errors.New("db down"))
		svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled"), broadcastRepo: broadcastRepo}
		assert.EqualError(t, svc.saveSampleBroadcasts(ctx, "demo", broadcasts), "failed to mark sample broadcast bc-2 as sent: db down")
	})

	t.Run("no connection to date them", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		broadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
		broadcastRepo.EXPECT().UpdateBroadcast(ctx, gomock.Any()).Return(nil).Times(len(broadcasts))
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(ctx, "demo").Return(nil, errors.New("gone"))
		svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled"), broadcastRepo: broadcastRepo, workspaceRepo: workspaceRepo}
		assert.EqualError(t, svc.saveSampleBroadcasts(ctx, "demo", broadcasts), "failed to get workspace connection: gone")
	})

	t.Run("the dating fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		broadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
		broadcastRepo.EXPECT().UpdateBroadcast(ctx, gomock.Any()).Return(nil).Times(len(broadcasts))
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()
		mock.ExpectExec("UPDATE broadcasts").WillReturnError(errors.New("locked"))
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(ctx, "demo").Return(db, nil)
		svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled"), broadcastRepo: broadcastRepo, workspaceRepo: workspaceRepo}
		assert.EqualError(t, svc.saveSampleBroadcasts(ctx, "demo", broadcasts), "failed to date the sample broadcasts: locked")
	})
}

// Dating the timeline is part of the seed: without it every event of the history shows at the
// moment of the seed, so a failure stops the seed. What the dating writes is checked against a
// real database by the demo seed integration test.
func TestDemoService_BackdateDemoMessageTimeline_Failures(t *testing.T) {
	ctx := context.Background()
	newSvc := func(t *testing.T) (*DemoService, *mocks.MockWorkspaceRepository) {
		ctrl := gomock.NewController(t)
		repo := mocks.NewMockWorkspaceRepository(ctrl)
		return &DemoService{logger: logger.NewLoggerWithLevel("disabled"), workspaceRepo: repo}, repo
	}

	t.Run("no connection", func(t *testing.T) {
		svc, repo := newSvc(t)
		repo.EXPECT().GetConnection(ctx, "demo").Return(nil, errors.New("gone"))
		assert.EqualError(t, svc.backdateDemoMessageTimeline(ctx, "demo"), "failed to get workspace connection: gone")
	})

	t.Run("the update fails", func(t *testing.T) {
		svc, repo := newSvc(t)
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()
		repo.EXPECT().GetConnection(ctx, "demo").Return(db, nil)
		mock.ExpectExec("UPDATE contact_timeline").WillReturnError(errors.New("locked"))
		assert.EqualError(t, svc.backdateDemoMessageTimeline(ctx, "demo"), "failed to date the message timeline entries: locked")
	})
}

// Contacts are given addresses only at the domains reserved for examples: the demo is public.
func TestDemoContacts_UseReservedExampleDomains(t *testing.T) {
	reserved := map[string]bool{"example.com": true, "example.org": true, "example.net": true}
	for _, d := range emailDomains {
		assert.True(t, reserved[d], "%s is not a reserved example domain", d)
	}
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}
	for _, contact := range svc.generateSampleContactsBatch(1000, 0, map[string]bool{}) {
		at := strings.LastIndex(contact.Email, "@")
		require.Positive(t, at, contact.Email)
		require.True(t, reserved[contact.Email[at+1:]], "%s is not at a reserved example domain", contact.Email)
	}
}

// Every generated contact has an address of its own, across the batches: UpsertContact would
// merge two contacts given the same one, and the demo would have fewer than it says.
func TestDemoContacts_UniqueAddresses(t *testing.T) {
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}
	used := map[string]bool{}
	const batches, perBatch = 30, 100
	distinct := map[string]bool{}
	for b := 0; b < batches; b++ {
		for _, contact := range svc.generateSampleContactsBatch(perBatch, b*perBatch, used) {
			require.False(t, distinct[contact.Email], "%s given twice", contact.Email)
			distinct[contact.Email] = true
		}
	}
	assert.Len(t, distinct, batches*perBatch)
	assert.Len(t, used, batches*perBatch)
}

func TestUniqueDemoEmail(t *testing.T) {
	used := map[string]bool{}
	assert.Equal(t, "ann.lee@example.com", uniqueDemoEmail("ann.lee@example.com", used))
	assert.Equal(t, "ann.lee-2@example.com", uniqueDemoEmail("ann.lee@example.com", used))
	assert.Equal(t, "ann.lee-3@example.com", uniqueDemoEmail("ann.lee@example.com", used))
	used["bob@example.org"] = true
	used["bob-2@example.org"] = true
	assert.Equal(t, "bob-3@example.org", uniqueDemoEmail("bob@example.org", used))
	assert.True(t, used["bob-3@example.org"], "the address returned is recorded")
}
