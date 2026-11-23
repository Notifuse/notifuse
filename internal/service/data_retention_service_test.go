package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

type retentionTestDeps struct {
	svc        *DataRetentionService
	workspaces *mocks.MockWorkspaceRepository
	settings   *mocks.MockSettingRepository
	repo       *mocks.MockDataRetentionRepository
	auth       *mocks.MockAuthService
	mailer     *pkgmocks.MockMailer
	eraser     *fakeEraser
	audit      *fakeAudit
	now        time.Time
}

type fakeEraser struct {
	erased []string
	err    error
}

func (f *fakeEraser) EraseHistoryOfDeletedContact(_ context.Context, _ string, email string) error {
	f.erased = append(f.erased, email)
	return f.err
}

type fakeAudit struct{ logs []*domain.AuditLog }

func (f *fakeAudit) Record(_ context.Context, log *domain.AuditLog) { f.logs = append(f.logs, log) }

func newRetentionTest(t *testing.T) *retentionTestDeps {
	t.Helper()
	ctrl := gomock.NewController(t)
	d := &retentionTestDeps{
		workspaces: mocks.NewMockWorkspaceRepository(ctrl),
		settings:   mocks.NewMockSettingRepository(ctrl),
		repo:       mocks.NewMockDataRetentionRepository(ctrl),
		auth:       mocks.NewMockAuthService(ctrl),
		mailer:     pkgmocks.NewMockMailer(ctrl),
		eraser:     &fakeEraser{},
		audit:      &fakeAudit{},
		now:        time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC),
	}
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()
	d.svc = NewDataRetentionService(d.workspaces, d.settings, d.repo, d.eraser, d.auth, d.mailer, d.audit, log)
	d.svc.SetClock(func() time.Time { return d.now })
	return d
}

// withNoDefaults: root never set any deployment defaults.
func (d *retentionTestDeps) withNoDefaults() *retentionTestDeps {
	d.settings.EXPECT().Get(gomock.Any(), DataRetentionDefaultsSettingKey).
		Return(nil, &domain.ErrSettingNotFound{Key: DataRetentionDefaultsSettingKey}).AnyTimes()
	return d
}

func (d *retentionTestDeps) as(role string) {
	d.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws").
		Return(context.Background(), &domain.User{Email: "owner@example.com"}, &domain.UserWorkspace{Role: role}, nil).AnyTimes()
}

func workspaceWith(settings *domain.DataRetentionSettings) *domain.Workspace {
	return &domain.Workspace{ID: "ws", Name: "Acme", Settings: domain.WorkspaceSettings{DataRetention: settings}}
}

func contactsPolicy(days int, exempt ...string) *domain.DataRetentionPolicy {
	return &domain.DataRetentionPolicy{Categories: map[domain.RetentionCategory]int{domain.RetentionInactiveContacts: days}, ExemptListIDs: exempt}
}

func TestDataRetentionService_SetPolicy(t *testing.T) {
	t.Run("only an owner", func(t *testing.T) {
		d := newRetentionTest(t).withNoDefaults()
		d.as("member")
		_, err := d.svc.SetPolicy(context.Background(), "ws", contactsPolicy(365))
		var unauthorized *domain.ErrUnauthorized
		assert.ErrorAs(t, err, &unauthorized)
	})

	t.Run("an invalid policy is refused", func(t *testing.T) {
		d := newRetentionTest(t).withNoDefaults()
		d.as("owner")
		_, err := d.svc.SetPolicy(context.Background(), "ws", contactsPolicy(30))
		var validation domain.ValidationError
		assert.ErrorAs(t, err, &validation)
	})

	t.Run("deleting more waits 7 days and the owners are told", func(t *testing.T) {
		d := newRetentionTest(t).withNoDefaults()
		d.as("owner")
		d.workspaces.EXPECT().GetByID(gomock.Any(), "ws").Return(workspaceWith(nil), nil)
		var written *domain.DataRetentionSettings
		d.workspaces.EXPECT().SetDataRetentionSettings(gomock.Any(), "ws", 0, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, _ int, s *domain.DataRetentionSettings) (bool, error) {
				written = s
				return true, nil
			})
		d.workspaces.EXPECT().GetWorkspaceUsersWithEmail(gomock.Any(), "ws").Return([]*domain.UserWorkspaceWithEmail{
			{UserWorkspace: domain.UserWorkspace{Role: "owner"}, Email: "a@example.com", Language: "fr"},
			{UserWorkspace: domain.UserWorkspace{Role: "member"}, Email: "b@example.com"},
		}, nil)
		d.mailer.EXPECT().SendDataRetentionPending("a@example.com", "ws", "Acme", d.now.Add(domain.RetentionPendingDelay), "fr").Return(nil)

		got, err := d.svc.SetPolicy(context.Background(), "ws", contactsPolicy(365))
		require.NoError(t, err)
		assert.Same(t, written, got)
		assert.Nil(t, got.Policy, "nothing in force yet")
		require.NotNil(t, got.Pending)
		assert.Equal(t, d.now.Add(domain.RetentionPendingDelay), got.Pending.EffectiveAt)
		assert.Equal(t, "owner@example.com", got.Pending.RequestedBy)
	})

	t.Run("saving the same waiting policy again keeps its date and tells nobody", func(t *testing.T) {
		d := newRetentionTest(t).withNoDefaults()
		d.as("owner")
		earlier := d.now.Add(-time.Hour).Add(domain.RetentionPendingDelay)
		stored := &domain.DataRetentionSettings{Revision: 3, Pending: &domain.PendingRetentionChange{Policy: contactsPolicy(365), EffectiveAt: earlier}}
		d.workspaces.EXPECT().GetByID(gomock.Any(), "ws").Return(workspaceWith(stored), nil)
		d.workspaces.EXPECT().SetDataRetentionSettings(gomock.Any(), "ws", 3, gomock.Any()).Return(true, nil)
		got, err := d.svc.SetPolicy(context.Background(), "ws", contactsPolicy(365))
		require.NoError(t, err)
		assert.Equal(t, earlier, got.Pending.EffectiveAt)
	})

	t.Run("deleting less applies at once and clears what waited", func(t *testing.T) {
		d := newRetentionTest(t).withNoDefaults()
		d.as("owner")
		stored := &domain.DataRetentionSettings{Revision: 1, Policy: contactsPolicy(365),
			Pending: &domain.PendingRetentionChange{Policy: contactsPolicy(200), EffectiveAt: d.now.Add(time.Hour)}}
		d.workspaces.EXPECT().GetByID(gomock.Any(), "ws").Return(workspaceWith(stored), nil)
		d.workspaces.EXPECT().SetDataRetentionSettings(gomock.Any(), "ws", 1, gomock.Any()).Return(true, nil)
		got, err := d.svc.SetPolicy(context.Background(), "ws", contactsPolicy(730, "vip"))
		require.NoError(t, err)
		assert.Equal(t, 730, got.Policy.Categories[domain.RetentionInactiveContacts])
		assert.Nil(t, got.Pending)
	})

	t.Run("a write that lost the race is a conflict", func(t *testing.T) {
		d := newRetentionTest(t).withNoDefaults()
		d.as("owner")
		d.workspaces.EXPECT().GetByID(gomock.Any(), "ws").Return(workspaceWith(nil), nil)
		d.workspaces.EXPECT().SetDataRetentionSettings(gomock.Any(), "ws", 0, gomock.Any()).Return(false, nil)
		_, err := d.svc.SetPolicy(context.Background(), "ws", contactsPolicy(730))
		var conflict *domain.ErrRetentionConflict
		assert.ErrorAs(t, err, &conflict)
	})
}

func TestDataRetentionService_CancelPending(t *testing.T) {
	t.Run("drops what waits", func(t *testing.T) {
		d := newRetentionTest(t).withNoDefaults()
		d.as("owner")
		stored := &domain.DataRetentionSettings{Revision: 2, Policy: contactsPolicy(730),
			Pending: &domain.PendingRetentionChange{Policy: contactsPolicy(365), EffectiveAt: d.now.Add(time.Hour)}}
		d.workspaces.EXPECT().GetByID(gomock.Any(), "ws").Return(workspaceWith(stored), nil)
		d.workspaces.EXPECT().SetDataRetentionSettings(gomock.Any(), "ws", 2, gomock.Any()).Return(true, nil)
		got, err := d.svc.CancelPending(context.Background(), "ws")
		require.NoError(t, err)
		assert.Nil(t, got.Pending)
		assert.Equal(t, 730, got.Policy.Categories[domain.RetentionInactiveContacts])
	})

	t.Run("too late once it applied", func(t *testing.T) {
		d := newRetentionTest(t).withNoDefaults()
		d.as("owner")
		stored := &domain.DataRetentionSettings{Pending: &domain.PendingRetentionChange{Policy: contactsPolicy(365), EffectiveAt: d.now.Add(-time.Hour)}}
		d.workspaces.EXPECT().GetByID(gomock.Any(), "ws").Return(workspaceWith(stored), nil)
		_, err := d.svc.CancelPending(context.Background(), "ws")
		var validation domain.ValidationError
		assert.ErrorAs(t, err, &validation)
	})

	t.Run("only an owner", func(t *testing.T) {
		d := newRetentionTest(t).withNoDefaults()
		d.as("member")
		_, err := d.svc.CancelPending(context.Background(), "ws")
		var unauthorized *domain.ErrUnauthorized
		assert.ErrorAs(t, err, &unauthorized)
	})
}

func TestDataRetentionService_SetDefaults(t *testing.T) {
	t.Run("a default that deletes more waits, and the inheriting workspaces' owners are told", func(t *testing.T) {
		d := newRetentionTest(t).withNoDefaults()
		var saved string
		d.settings.EXPECT().Set(gomock.Any(), DataRetentionDefaultsSettingKey, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, value string) error { saved = value; return nil })
		inheriting := workspaceWith(nil)
		own := &domain.Workspace{ID: "ws2", Settings: domain.WorkspaceSettings{DataRetention: &domain.DataRetentionSettings{
			Policy: &domain.DataRetentionPolicy{Categories: map[domain.RetentionCategory]int{domain.RetentionMessageHistory: 1000}}}}}
		d.workspaces.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{inheriting, own}, nil)
		d.workspaces.EXPECT().GetWorkspaceUsersWithEmail(gomock.Any(), "ws").Return([]*domain.UserWorkspaceWithEmail{
			{UserWorkspace: domain.UserWorkspace{Role: "owner"}, Email: "a@example.com"}}, nil)
		d.mailer.EXPECT().SendDataRetentionPending("a@example.com", "ws", "Acme", d.now.Add(domain.RetentionPendingDelay), "").Return(nil)

		got, err := d.svc.SetDefaults(context.Background(), map[domain.RetentionCategory]int{domain.RetentionMessageHistory: 365})
		require.NoError(t, err)
		require.NotNil(t, got.Pending)
		assert.Equal(t, 365, got.Pending.Categories[domain.RetentionMessageHistory])
		var back domain.RetentionDefaults
		require.NoError(t, json.Unmarshal([]byte(saved), &back))
		assert.Equal(t, d.now.Add(domain.RetentionPendingDelay), back.Pending.EffectiveAt.UTC())
	})

	t.Run("an invalid default is refused", func(t *testing.T) {
		d := newRetentionTest(t).withNoDefaults()
		_, err := d.svc.SetDefaults(context.Background(), map[domain.RetentionCategory]int{domain.RetentionWebhookDeliveries: 1})
		var validation domain.ValidationError
		assert.ErrorAs(t, err, &validation)
	})
}

func TestDataRetentionService_RunWorkspace(t *testing.T) {
	inForce := &domain.DataRetentionSettings{Policy: &domain.DataRetentionPolicy{
		Categories:           map[domain.RetentionCategory]int{domain.RetentionInactiveContacts: 365, domain.RetentionContactTimeline: 90},
		ExemptListIDs:        []string{"vip"},
		EngagementEventNames: []string{"purchase"},
	}}

	expectRunRecorded := func(d *retentionTestDeps) *domain.RetentionRun {
		finished := &domain.RetentionRun{}
		d.repo.EXPECT().InsertRun(gomock.Any(), "ws", gomock.Any()).Return(nil)
		d.repo.EXPECT().FinishRun(gomock.Any(), "ws", gomock.Any()).DoAndReturn(func(_ context.Context, _ string, run *domain.RetentionRun) error {
			*finished = *run
			return nil
		})
		return finished
	}

	t.Run("another server holds the lock: nothing, not even a report", func(t *testing.T) {
		d := newRetentionTest(t).withNoDefaults()
		d.repo.EXPECT().TryLock(gomock.Any(), "ws").Return(nil, false, nil)
		require.NoError(t, d.svc.RunWorkspace(context.Background(), workspaceWith(inForce), nil))
	})

	t.Run("deletes the expired contacts, erases their history, then runs the other categories", func(t *testing.T) {
		d := newRetentionTest(t).withNoDefaults()
		released := false
		d.repo.EXPECT().TryLock(gomock.Any(), "ws").Return(func() { released = true }, true, nil)
		run := expectRunRecorded(d)
		d.repo.EXPECT().BackfillCompleted(gomock.Any(), "ws").Return(true, nil)
		d.repo.EXPECT().MissingExemptions(gomock.Any(), "ws", []string{"vip"}, []string(nil)).Return(nil, nil)
		before := d.now.AddDate(0, 0, -365)
		d.repo.EXPECT().ListExpiredContacts(gomock.Any(), "ws", gomock.Any()).DoAndReturn(
			func(_ context.Context, _ string, q domain.ExpiredContactsQuery) ([]string, error) {
				assert.Equal(t, before, q.EngagedBefore)
				assert.Equal(t, []string{"vip"}, q.ExemptListIDs)
				assert.Equal(t, []string{"purchase"}, q.EngagementEventNames)
				return []string{"a@x", "b@x"}, nil
			})
		d.repo.EXPECT().DeleteExpiredContacts(gomock.Any(), "ws", gomock.Any(), gomock.Any(), []string{"a@x", "b@x"}).
			Return([]string{"a@x"}, int64(2), nil) // b engaged meanwhile
		d.repo.EXPECT().ErasureQueue(gomock.Any(), "ws", retentionDrainBatch).Return([]string{"a@x", "c@x"}, nil)
		d.repo.EXPECT().ContactExists(gomock.Any(), "ws", "a@x").Return(false, nil)
		d.repo.EXPECT().ContactExists(gomock.Any(), "ws", "c@x").Return(true, nil) // re-created: its rows are a live contact's
		d.repo.EXPECT().Dequeue(gomock.Any(), "ws", "a@x").Return(nil)
		d.repo.EXPECT().Dequeue(gomock.Any(), "ws", "c@x").Return(nil)
		d.repo.EXPECT().ErasureQueue(gomock.Any(), "ws", retentionDrainBatch).Return(nil, nil)
		d.repo.EXPECT().PurgeCategory(gomock.Any(), "ws", domain.RetentionContactTimeline, d.now.AddDate(0, 0, -90), retentionCategoryBatch, []string{"purchase"}).Return(int64(12), nil)
		d.repo.EXPECT().PurgeCategory(gomock.Any(), "ws", domain.RetentionWebhookDeliveries, gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(0), nil)
		d.repo.EXPECT().PurgeCategory(gomock.Any(), "ws", domain.RetentionFailedEmails, gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(0), nil)

		require.NoError(t, d.svc.RunWorkspace(context.Background(), workspaceWith(inForce), nil))
		assert.True(t, released)
		assert.Equal(t, []string{"a@x"}, d.eraser.erased)
		assert.Equal(t, domain.RetentionRunCompleted, run.Status)
		assert.EqualValues(t, 1, run.Results[domain.RetentionInactiveContacts].Affected)
		assert.EqualValues(t, 2, run.Results[domain.RetentionInactiveContacts].OptOutsKept)
		assert.EqualValues(t, 12, run.Results[domain.RetentionContactTimeline].Affected)
		assert.Equal(t, "off", run.Results[domain.RetentionMessageHistory].Skipped)

		require.Len(t, d.audit.logs, 1)
		raw, err := json.Marshal(d.audit.logs[0].Metadata)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "@x", "counts, never an address")
		assert.Equal(t, domain.AuditActionRetentionPurged, d.audit.logs[0].Action)
	})

	t.Run("an exemption that cannot be trusted skips the contacts; a failing category leaves the others run", func(t *testing.T) {
		d := newRetentionTest(t).withNoDefaults()
		d.repo.EXPECT().TryLock(gomock.Any(), "ws").Return(func() {}, true, nil)
		run := expectRunRecorded(d)
		d.repo.EXPECT().BackfillCompleted(gomock.Any(), "ws").Return(true, nil)
		d.repo.EXPECT().MissingExemptions(gomock.Any(), "ws", gomock.Any(), gomock.Any()).Return([]string{"list:vip"}, nil)
		d.repo.EXPECT().ErasureQueue(gomock.Any(), "ws", gomock.Any()).Return(nil, nil)
		d.repo.EXPECT().PurgeCategory(gomock.Any(), "ws", domain.RetentionContactTimeline, gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(0), errors.New("lock timeout"))
		d.repo.EXPECT().PurgeCategory(gomock.Any(), "ws", domain.RetentionWebhookDeliveries, gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(3), nil)
		d.repo.EXPECT().PurgeCategory(gomock.Any(), "ws", domain.RetentionFailedEmails, gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(0), nil)

		require.NoError(t, d.svc.RunWorkspace(context.Background(), workspaceWith(inForce), nil))
		assert.Equal(t, "exemption_unavailable", run.Results[domain.RetentionInactiveContacts].Skipped)
		assert.Contains(t, run.Results[domain.RetentionContactTimeline].Error, "lock timeout")
		assert.EqualValues(t, 3, run.Results[domain.RetentionWebhookDeliveries].Affected)
		assert.Equal(t, domain.RetentionRunPartial, run.Status)
	})

	t.Run("the contacts wait for the engagement backfill", func(t *testing.T) {
		d := newRetentionTest(t).withNoDefaults()
		d.repo.EXPECT().TryLock(gomock.Any(), "ws").Return(func() {}, true, nil)
		run := expectRunRecorded(d)
		d.repo.EXPECT().BackfillCompleted(gomock.Any(), "ws").Return(false, nil)
		d.repo.EXPECT().ErasureQueue(gomock.Any(), "ws", gomock.Any()).Return(nil, nil)
		d.repo.EXPECT().PurgeCategory(gomock.Any(), "ws", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(0), nil).Times(3)
		require.NoError(t, d.svc.RunWorkspace(context.Background(), workspaceWith(inForce), nil))
		assert.Equal(t, "backfill_pending", run.Results[domain.RetentionInactiveContacts].Skipped)
		assert.Empty(t, d.audit.logs, "nothing happened: no audit row")
	})

	t.Run("a change whose date has come is written as the policy in force", func(t *testing.T) {
		d := newRetentionTest(t).withNoDefaults()
		due := &domain.DataRetentionSettings{Revision: 4, Pending: &domain.PendingRetentionChange{
			Policy: &domain.DataRetentionPolicy{Categories: map[domain.RetentionCategory]int{domain.RetentionWebhookDeliveries: 30}}, EffectiveAt: d.now.Add(-time.Minute)}}
		d.repo.EXPECT().TryLock(gomock.Any(), "ws").Return(func() {}, true, nil)
		d.workspaces.EXPECT().SetDataRetentionSettings(gomock.Any(), "ws", 4, gomock.Any()).DoAndReturn(
			func(_ context.Context, _ string, _ int, s *domain.DataRetentionSettings) (bool, error) {
				assert.Nil(t, s.Pending)
				assert.Equal(t, 30, s.Policy.Categories[domain.RetentionWebhookDeliveries])
				return true, nil
			})
		run := expectRunRecorded(d)
		d.repo.EXPECT().ErasureQueue(gomock.Any(), "ws", gomock.Any()).Return(nil, nil)
		d.repo.EXPECT().PurgeCategory(gomock.Any(), "ws", domain.RetentionWebhookDeliveries, d.now.AddDate(0, 0, -30), gomock.Any(), gomock.Any()).Return(int64(0), nil)
		d.repo.EXPECT().PurgeCategory(gomock.Any(), "ws", domain.RetentionFailedEmails, gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(0), nil)
		require.NoError(t, d.svc.RunWorkspace(context.Background(), workspaceWith(due), nil))
		assert.Equal(t, 30, run.Policy[domain.RetentionWebhookDeliveries])
	})
}

func TestDataRetentionService_ExportRuns(t *testing.T) {
	d := newRetentionTest(t).withNoDefaults()
	d.as("member")
	finished := d.now.Add(time.Minute)
	d.repo.EXPECT().ListRuns(gomock.Any(), "ws", 1000).Return([]*domain.RetentionRun{{
		ID: "run-1", StartedAt: d.now, FinishedAt: &finished, Status: domain.RetentionRunPartial,
		Results: map[domain.RetentionCategory]domain.RetentionCategoryResult{
			domain.RetentionInactiveContacts: {Days: 365, Affected: 4, OptOutsKept: 1},
			domain.RetentionContactTimeline:  {Days: 90, Error: "lock timeout"},
		},
	}}, nil)
	var buf bytes.Buffer
	require.NoError(t, d.svc.ExportRuns(context.Background(), "ws", &buf))
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 3)
	assert.True(t, strings.HasPrefix(lines[0], "run_id,started_at"))
	assert.Contains(t, lines[1], "contact_timeline,90,0,0,false,,lock timeout")
	assert.Contains(t, lines[2], "inactive_contacts,365,4,1,false")
}

// Defaults that cannot be read are not the built-in ones: deleting with periods root may have
// changed is the one failure to avoid.
func TestDataRetentionService_UnreadableDefaults(t *testing.T) {
	d := newRetentionTest(t)
	d.settings.EXPECT().Get(gomock.Any(), DataRetentionDefaultsSettingKey).Return(nil, errors.New("connection reset")).AnyTimes()
	d.as("owner")

	assert.Zero(t, d.svc.EffectiveDays(context.Background(), workspaceWith(nil), domain.RetentionWebhookDeliveries), "the sweep keeps everything")

	d.workspaces.EXPECT().GetByID(gomock.Any(), "ws").Return(workspaceWith(nil), nil)
	_, err := d.svc.SetPolicy(context.Background(), "ws", contactsPolicy(365))
	assert.Error(t, err)

	d.workspaces.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{workspaceWith(nil)}, nil)
	d.svc.RunAll(context.Background()) // no lock taken, no run: the repository mock expects nothing
}

func TestDataRetentionService_DrainGoesOnPastAFailure(t *testing.T) {
	d := newRetentionTest(t).withNoDefaults()
	d.eraser.err = errors.New("boom")
	d.repo.EXPECT().ErasureQueue(gomock.Any(), "ws", retentionDrainBatch).Return([]string{"bad@x"}, nil)
	d.repo.EXPECT().ContactExists(gomock.Any(), "ws", "bad@x").Return(false, nil)
	d.repo.EXPECT().Requeue(gomock.Any(), "ws", "bad@x").Return(nil)
	// Served again (it went to the back of a one-address queue): not retried in the same run.
	d.repo.EXPECT().ErasureQueue(gomock.Any(), "ws", retentionDrainBatch).Return([]string{"bad@x"}, nil)
	failures, remaining, err := d.svc.drainErasureQueue(context.Background(), "ws")
	require.NoError(t, err)
	assert.Equal(t, 1, failures)
	assert.False(t, remaining)
}

// The preview says what the save does: the same plan, the same date.
func TestDataRetentionService_PreviewAgreesWithTheSave(t *testing.T) {
	d := newRetentionTest(t).withNoDefaults()
	d.as("member")
	earlier := d.now.Add(time.Hour)
	stored := &domain.DataRetentionSettings{Pending: &domain.PendingRetentionChange{Policy: contactsPolicy(365), EffectiveAt: earlier}}
	d.workspaces.EXPECT().GetByID(gomock.Any(), "ws").Return(workspaceWith(stored), nil)
	d.repo.EXPECT().BackfillCompleted(gomock.Any(), "ws").Return(true, nil)
	d.repo.EXPECT().CountExpiredContacts(gomock.Any(), "ws", gomock.Any()).Return(int64(4), int64(1), nil)
	d.repo.EXPECT().CountCategory(gomock.Any(), "ws", gomock.Any(), gomock.Any()).Return(int64(0), nil).Times(2)
	preview, err := d.svc.Preview(context.Background(), "ws", contactsPolicy(365))
	require.NoError(t, err)
	require.True(t, preview.Pending)
	assert.Equal(t, earlier, *preview.PendingAt, "the waiting change keeps its date")
}

// A category root leaves out goes back to its built-in default, now or when the change applies.
func TestDataRetentionService_SetDefaultsStoresTheWholeMap(t *testing.T) {
	d := newRetentionTest(t)
	stored := `{"categories": {"message_history": 365, "webhook_deliveries": 7, "failed_emails": 7}}`
	d.settings.EXPECT().Get(gomock.Any(), DataRetentionDefaultsSettingKey).Return(&domain.Setting{Value: stored}, nil).AnyTimes()
	var saved string
	d.settings.EXPECT().Set(gomock.Any(), DataRetentionDefaultsSettingKey, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, value string) error { saved = value; return nil })
	d.workspaces.EXPECT().List(gomock.Any()).Return(nil, nil)

	got, err := d.svc.SetDefaults(context.Background(), map[domain.RetentionCategory]int{domain.RetentionInactiveContacts: 365})
	require.NoError(t, err)
	require.NotNil(t, got.Pending, "switching contacts on waits")
	assert.Equal(t, 0, got.Pending.Categories[domain.RetentionMessageHistory], "left out: back to off")
	var back domain.RetentionDefaults
	require.NoError(t, json.Unmarshal([]byte(saved), &back))
	assert.Equal(t, 0, back.ResolvedDefaults(d.now.Add(domain.RetentionPendingDelay))[domain.RetentionMessageHistory])
	assert.Equal(t, 365, back.ResolvedDefaults(d.now)[domain.RetentionMessageHistory], "the current defaults hold until then")
}
