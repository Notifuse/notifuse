package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// Sample-data seeding contract: individual contact failures never abort the
// batch (a demo with 998 of 1000 contacts beats no demo), while a failing
// first step (templates) aborts the whole seed with the step named — later
// steps build on templates and would fail confusingly without them.

func demoSeedService(t *testing.T) (*DemoService, *mocks.MockAuthService) {
	t.Helper()
	ctrl := gomock.NewController(t)
	log := logger.NewLoggerWithLevel("disabled")
	authService := mocks.NewMockAuthService(ctrl)

	contactService := NewContactService(
		mocks.NewMockContactRepository(ctrl),
		mocks.NewMockWorkspaceRepository(ctrl),
		authService,
		mocks.NewMockMessageHistoryRepository(ctrl),
		mocks.NewMockInboundWebhookEventRepository(ctrl),
		mocks.NewMockContactListRepository(ctrl),
		mocks.NewMockContactTimelineRepository(ctrl),
		nil, // webAnalyticsRepo
		mocks.NewMockEmailQueueRepository(ctrl),
		mocks.NewMockCustomEventRepository(ctrl),
		mocks.NewMockSegmentRepository(ctrl),
		mocks.NewMockContactSegmentQueueRepository(ctrl),
		log,
	)
	templateService := NewTemplateService(
		mocks.NewMockTemplateRepository(ctrl),
		mocks.NewMockWorkspaceRepository(ctrl),
		authService,
		log, "", nil,
	)
	svc := &DemoService{
		logger:          log,
		contactService:  contactService,
		templateService: templateService,
	}
	return svc, authService
}

func TestDemoService_GenerateAndAddSampleContacts_FailuresDoNotAbort(t *testing.T) {
	svc, authService := demoSeedService(t)
	// Every upsert fails authentication; the batch must still attempt all
	// contacts and report success overall.
	authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").
		Return(context.Background(), nil, nil, errors.New("no session")).Times(3)

	require.NoError(t, svc.generateAndAddSampleContacts(context.Background(), "ws-1", 3))
}

func TestDemoService_GenerateSampleContactsBatch_Shape(t *testing.T) {
	svc, _ := demoSeedService(t)
	batch := svc.generateSampleContactsBatch(5, 100)
	require.Len(t, batch, 5)

	seen := map[string]bool{}
	for _, c := range batch {
		assert.NotEmpty(t, c.Email)
		assert.True(t, strings.Contains(c.Email, "@"), "email %q must look like an address", c.Email)
		assert.False(t, seen[c.Email], "emails must be unique across the demo, got %q twice", c.Email)
		seen[c.Email] = true
		require.NotNil(t, c.FirstName)
		assert.False(t, c.FirstName.IsNull)
		assert.NotEmpty(t, c.FirstName.String)
	}

	// The index offset keeps batches disjoint: batch 2 must not re-mint
	// batch 1's addresses (the upsert would merge two demo people into one).
	more := svc.generateSampleContactsBatch(5, 105)
	for _, c := range more {
		assert.False(t, seen[c.Email], "second batch re-used %q", c.Email)
	}
}

func TestDemoService_AddSampleData_TemplateFailureAborts(t *testing.T) {
	svc, authService := demoSeedService(t)
	sentinel := errors.New("no session")
	authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").
		Return(context.Background(), nil, nil, sentinel).Times(1)

	err := svc.addSampleData(context.Background(), "ws-1")
	require.ErrorContains(t, err, "demo: newsletter template:")
	require.ErrorContains(t, err, "failed to authenticate user:")
	assert.ErrorIs(t, err, sentinel)
}
