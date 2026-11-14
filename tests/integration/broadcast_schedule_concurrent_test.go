//go:build integration

package integration

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBroadcastSchedule_ConcurrentSchedulesOfOneDraft: schedules of the same draft sent at
// once are served one after the other on the broadcast's lock. The first schedules it; the
// others find it no longer a draft and are refused, instead of each starting its task over.
func TestBroadcastSchedule_ConcurrentSchedulesOfOneDraft(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{domain.NewEmailSender("noreply@notifuse.test", "Concurrent Schedule")},
			SMTP:    &domain.SMTPSettings{Host: "localhost", Port: 1025},
		}))
	require.NoError(t, err)
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	broadcast, err := factory.CreateBroadcast(workspace.ID,
		testutil.WithBroadcastAudience(domain.AudienceSettings{List: list.ID, ExcludeUnsubscribed: true}))
	require.NoError(t, err)

	const schedules = 8
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	statuses := make([]int, schedules)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < schedules; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			resp, err := client.ScheduleBroadcast(map[string]interface{}{
				"workspace_id":   workspace.ID,
				"id":             broadcast.ID,
				"send_now":       false,
				"scheduled_date": tomorrow,
				"scheduled_time": "10:00",
				"timezone":       "UTC",
			})
			if err != nil {
				return
			}
			statuses[i] = resp.StatusCode
			_ = resp.Body.Close()
		}(i)
	}
	close(start)
	wg.Wait()

	succeeded := 0
	for _, status := range statuses {
		require.NotZero(t, status, "every schedule got an answer")
		if status == http.StatusOK {
			succeeded++
		}
	}
	assert.Equal(t, 1, succeeded, "one schedule of the draft succeeds, got statuses %v", statuses)

	var tasks int
	require.NoError(t, suite.DBManager.GetDB().QueryRow(
		`SELECT COUNT(*) FROM tasks WHERE workspace_id = $1 AND broadcast_id = $2`, workspace.ID, broadcast.ID).Scan(&tasks))
	assert.Equal(t, 1, tasks, "the broadcast has one task")
}
