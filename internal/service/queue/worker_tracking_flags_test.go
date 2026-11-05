package queue

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

func TestEmailQueueWorker_ProcessEntry_RecordsTheTrackedFlags(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name          string
		opens, clicks *bool
	}{
		{"flags from the entry", &yes, &no},
		{"an entry queued before the flags records NULL", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
			mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
			mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
			mockLogger := pkgmocks.NewMockLogger(ctrl)
			mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
			mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

			workspace := &domain.Workspace{
				ID:       "workspace-1",
				Settings: domain.WorkspaceSettings{SecretKey: "test-secret"},
				Integrations: []domain.Integration{{
					ID:            "integration-1",
					EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, RateLimitPerMinute: 100},
				}},
			}
			entry := &domain.EmailQueueEntry{
				ID: "entry-1", Status: domain.EmailQueueStatusPending,
				SourceType: domain.EmailQueueSourceBroadcast, SourceID: "b-1",
				IntegrationID: "integration-1", ProviderKind: domain.EmailProviderKindSMTP,
				ContactEmail: "test@example.com", MessageID: "msg-1", TemplateID: "template-1",
				Payload: domain.EmailQueuePayload{
					FromAddress: "sender@example.com", Subject: "S", HTMLContent: "<p>Hi</p>", RateLimitPerMinute: 100,
					OpensTracked: tc.opens, ClicksTracked: tc.clicks,
				},
				MaxAttempts: 3,
			}

			mockQueueRepo.EXPECT().MarkAsProcessing(gomock.Any(), "workspace-1", "entry-1").Return(nil)
			mockEmailService.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(nil)
			mockMessageHistoryRepo.EXPECT().Upsert(gomock.Any(), "workspace-1", gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, _, _ string, msg *domain.MessageHistory) error {
					assert.Equal(t, tc.opens, msg.OpensTracked)
					assert.Equal(t, tc.clicks, msg.ClicksTracked)
					return nil
				})
			mockQueueRepo.EXPECT().MarkAsSent(gomock.Any(), "workspace-1", "entry-1").Return(nil)

			worker := NewEmailQueueWorker(mockQueueRepo, mocks.NewMockWorkspaceRepository(ctrl), mockEmailService,
				mockMessageHistoryRepo, DefaultWorkerConfig(), mockLogger)
			worker.initContexts(context.Background())
			processOne(worker, workspace, entry)
		})
	}
}
