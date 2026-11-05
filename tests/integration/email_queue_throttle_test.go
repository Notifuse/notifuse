//go:build integration

package integration

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEmailQueueThrottlePartial is a provider enforcing its sending rate: it refuses some
// emails with a 451 that asks to slow down and takes the rest. The integration slows
// down, sends the refused emails again, and the broadcast is never paused.
func TestEmailQueueThrottlePartial(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	const contactCount = 12

	smtpServer := newQuotaSMTPServer(t, 0, nil)
	smtpServer.throttleEvery = 3
	defer smtpServer.Close()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	workspace, broadcastID, _ := setUpFailingBroadcast(t, suite, smtpServer, contactCount, nil, "Throttled")

	appInstance := suite.ServerManager.GetApp()
	queueRepo := appInstance.GetEmailQueueRepository()
	ctx := context.Background()

	require.Eventually(t, func() bool {
		accepted, _ := smtpServer.counts()
		counts, err := queueRepo.GetSourceCounts(ctx, workspace.ID, domain.EmailQueueSourceBroadcast, broadcastID)
		return accepted == contactCount && err == nil && counts.InFlight() == 0
	}, 90*time.Second, time.Second, "every email should be delivered despite the refusals")

	assert.Positive(t, smtpServer.throttledCount(), "the provider did refuse some emails")
	_, refused := smtpServer.counts()
	assert.Zero(t, refused)

	resp, err := suite.APIClient.GetBroadcast(broadcastID)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := suite.APIClient.ReadBody(resp)
	require.NoError(t, err)
	assert.NotContains(t, body, `"status":"paused"`, "slowing down is enough, nothing to pause")

	// A refusal for the pace is not a failed email: the campaign reports everyone sent.
	stats := broadcastStatsOnceRecorded(t, appInstance.GetMessageHistoryRepository(), workspace.ID, broadcastID, contactCount, 0)
	assert.Equal(t, contactCount, stats.TotalSent)
	assert.Zero(t, stats.TotalFailed)
}

// TestEmailQueueThrottleEverything is a provider refusing every email for the sending
// rate, however slowly it is sent. Once the pace has sat at its floor for the dwell, the
// refusals count against the provider, the circuit opens and the broadcast pauses with
// every recipient kept. Resuming once the provider is back delivers them all.
func TestEmailQueueThrottleEverything(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	const contactCount = 12

	smtpServer := newQuotaSMTPServer(t, 0, nil)
	smtpServer.throttleEvery = 1
	defer smtpServer.Close()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	workspace, broadcastID, _ := setUpFailingBroadcast(t, suite, smtpServer, contactCount, nil, "Throttled Everything")

	appInstance := suite.ServerManager.GetApp()
	queueRepo := appInstance.GetEmailQueueRepository()
	ctx := context.Background()

	readBroadcast := func() string {
		resp, err := client.GetBroadcast(broadcastID)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := client.ReadBody(resp)
		require.NoError(t, err)
		return body
	}

	var body string
	require.Eventually(t, func() bool {
		body = readBroadcast()
		return strings.Contains(body, `"status":"paused"`)
	}, 150*time.Second, time.Second, "a provider refusing everything should stop the campaign")

	counts, err := queueRepo.GetSourceCounts(ctx, workspace.ID, domain.EmailQueueSourceBroadcast, broadcastID)
	require.NoError(t, err)
	accepted, _ := smtpServer.counts()
	assert.Zero(t, accepted)
	assert.Equal(t, int64(0), counts.FailedTerminal, "nobody should have been given up on")
	assert.Equal(t, int64(contactCount), counts.InFlight(), "every recipient is still accounted for")
	assert.Contains(t, body, "Circuit breaker triggered")
	assert.Contains(t, body, "451")

	smtpServer.stopThrottling()
	resumeResp, err := client.Post("/api/broadcasts.resume", map[string]interface{}{
		"workspace_id": workspace.ID, "id": broadcastID,
	}, nil)
	require.NoError(t, err)
	defer resumeResp.Body.Close()
	require.Equal(t, http.StatusOK, resumeResp.StatusCode)

	require.Eventually(t, func() bool {
		acceptedNow, _ := smtpServer.counts()
		after, err := queueRepo.GetSourceCounts(ctx, workspace.ID, domain.EmailQueueSourceBroadcast, broadcastID)
		return acceptedNow == contactCount && err == nil && after.InFlight() == 0
	}, 120*time.Second, time.Second, "resuming should send to everyone the pause held back")
}
