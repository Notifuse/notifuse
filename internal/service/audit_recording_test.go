package service

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// Recording() is the write gate for the whole audit trail: an unwired
// provider means "not wired", never "unlicensed" (every community install
// would go dark), while a resolved grant without the feature stops writes.

type staticEntitlements struct{ entitlements domain.Entitlements }

func (s staticEntitlements) Entitlements() domain.Entitlements { return s.entitlements }

func TestAuditLogService_Recording(t *testing.T) {
	log := logger.NewLoggerWithLevel("disabled")

	t.Run("no provider wired still records", func(t *testing.T) {
		svc := NewAuditLogService(AuditLogServiceConfig{Logger: log})
		assert.True(t, svc.Recording())
	})

	t.Run("a grant without the audit feature stops recording", func(t *testing.T) {
		svc := NewAuditLogService(AuditLogServiceConfig{
			Logger:       log,
			Entitlements: staticEntitlements{domain.CommunityEntitlements()},
		})
		assert.False(t, svc.Recording())
	})

	t.Run("a grant with the audit feature records", func(t *testing.T) {
		grants := domain.CommunityEntitlements()
		grants.Features = []domain.Feature{domain.FeatureAuditLogs}
		svc := NewAuditLogService(AuditLogServiceConfig{
			Logger:       log,
			Entitlements: staticEntitlements{grants},
		})
		assert.True(t, svc.Recording())
	})
}

func TestAuditLogService_Init_SeedsLicenceState(t *testing.T) {
	ctx := context.Background()
	log := logger.NewLoggerWithLevel("disabled")
	licensed := func() domain.Entitlements {
		e := domain.CommunityEntitlements()
		e.Features = []domain.Feature{domain.FeatureAuditLogs}
		return e
	}

	t.Run("first boot (no row) remembers the current state", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		settingRepo := mocks.NewMockSettingRepository(ctrl)
		settingRepo.EXPECT().Get(gomock.Any(), domain.AuditLogsRecordingSettingKey).
			Return(nil, &domain.ErrSettingNotFound{}).Times(1)
		settingRepo.EXPECT().Set(gomock.Any(), domain.AuditLogsRecordingSettingKey, "true").
			Return(nil).Times(1)

		svc := NewAuditLogService(AuditLogServiceConfig{
			Logger: log, SettingRepo: settingRepo,
			Entitlements: staticEntitlements{licensed()},
		})
		svc.Init(ctx)
	})

	t.Run("an unreadable row leaves the state unknown and writes nothing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		settingRepo := mocks.NewMockSettingRepository(ctrl)
		settingRepo.EXPECT().Get(gomock.Any(), domain.AuditLogsRecordingSettingKey).
			Return(nil, errors.New("settings table gone")).Times(1)
		// No Set expectation: guessing here would invent a transition marker
		// at the next boot.

		svc := NewAuditLogService(AuditLogServiceConfig{
			Logger: log, SettingRepo: settingRepo,
			Entitlements: staticEntitlements{licensed()},
		})
		assert.NotPanics(t, func() { svc.Init(ctx) })
	})

	t.Run("rememberRecordingState encodes booleans and tolerates a missing repo", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		settingRepo := mocks.NewMockSettingRepository(ctrl)
		settingRepo.EXPECT().Set(gomock.Any(), domain.AuditLogsRecordingSettingKey, "false").
			Return(nil).Times(1)

		svc := NewAuditLogService(AuditLogServiceConfig{Logger: log, SettingRepo: settingRepo})
		svc.rememberRecordingState(ctx, false)

		bare := NewAuditLogService(AuditLogServiceConfig{Logger: log})
		assert.NotPanics(t, func() { bare.rememberRecordingState(ctx, true) })
	})

}

func TestAuditCategoryOf(t *testing.T) {
	// Catalogue actions resolve to their declared category.
	assert.Equal(t, domain.AuditCategoryAuth, auditCategoryOf("user.signin"))
	// Logged reads are keyed by route but matched on their recorded Action:
	// the export read is recorded as contacts.export.
	assert.Equal(t, domain.AuditCategoryContacts, auditCategoryOf("contacts.export"))
	// The system-actions catalogue is consulted second: these actions are
	// recorded by background jobs, not handlers, and must not fall through to
	// the system bucket (licence rows filed as system would vanish from the
	// licence audit view).
	assert.Equal(t, domain.AuditCategoryLicence, auditCategoryOf("licence.recordingStopped"))
	assert.Equal(t, domain.AuditCategoryLicence, auditCategoryOf("licence.recordingResumed"))
	assert.Equal(t, domain.AuditCategoryAudit, auditCategoryOf("audit.purged"))
	// Anything unknown lands in system rather than dropping the row.
	assert.Equal(t, domain.AuditCategorySystem, auditCategoryOf("no.such.action"))
	assert.Equal(t, domain.AuditCategorySystem, auditCategoryOf(""))
}
