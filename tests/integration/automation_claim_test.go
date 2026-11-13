//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	shortuuid "github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// Two instances on one database: a journey is run by one of them at a time. The app's own
// scheduler is not started; the test drives two executors of its own.
func TestAutomationClaim(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		// The webhooks post to an httptest server on 127.0.0.1.
		cfg.Webhook.AllowPrivateDeliveryHosts = true
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()
	ctx := context.Background()

	factory := suite.DataFactory
	client := suite.APIClient
	a := suite.ServerManager.GetApp()

	owner, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(owner.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(owner.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)

	workspaceRepo := a.GetWorkspaceRepository()
	automationRepo := repository.NewAutomationRepository(workspaceRepo, service.NewAutomationTriggerGenerator(service.NewQueryBuilder()))
	newExecutor := func() *service.AutomationExecutor {
		return service.NewAutomationExecutor(automationRepo, a.GetContactRepository(), workspaceRepo,
			a.GetContactListRepository(), a.GetListRepository(), a.GetTemplateRepository(), a.GetEmailQueueRepository(),
			repository.NewContactTimelineRepository(workspaceRepo), a.GetLogger(), a.GetConfig().APIEndpoint,
			&http.Client{Timeout: 30 * time.Second})
	}

	readBody := func(resp *http.Response) string {
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	// trigger → webhook, enrolled by a custom event.
	createAutomation := func(t *testing.T, eventName, webhookURL string) (automationID, webhookID string) {
		t.Helper()
		automationID, triggerID, webhookID := shortuuid.New(), shortuuid.New(), shortuuid.New()
		node := func(id, nodeType string, config map[string]interface{}, next string) map[string]interface{} {
			n := map[string]interface{}{"id": id, "automation_id": automationID, "type": nodeType, "config": config,
				"position": map[string]interface{}{"x": 0, "y": 0}}
			if next != "" {
				n["next_node_id"] = next
			}
			return n
		}
		resp, err := client.CreateAutomation(map[string]interface{}{
			"workspace_id": workspace.ID,
			"automation": map[string]interface{}{
				"id": automationID, "workspace_id": workspace.ID, "name": "Claim " + eventName, "status": "draft",
				"list_id":      list.ID,
				"trigger":      map[string]interface{}{"event_kind": "custom_event", "custom_event_name": eventName, "frequency": "once"},
				"root_node_id": triggerID,
				"nodes": []map[string]interface{}{
					node(triggerID, "trigger", map[string]interface{}{}, webhookID),
					node(webhookID, "webhook", map[string]interface{}{"url": webhookURL}, ""),
				},
			},
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, resp.StatusCode, readBody(resp))
		resp, err = client.ActivateAutomation(map[string]interface{}{"workspace_id": workspace.ID, "automation_id": automationID})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, readBody(resp))
		return automationID, webhookID
	}
	// Enrols n contacts and waits for their journeys, all due now.
	enrol := func(t *testing.T, automationID, eventName string, n int) []string {
		t.Helper()
		emails := make([]string, n)
		for i := range emails {
			emails[i] = eventName + "-" + shortuuid.New() + "@example.com"
			_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(emails[i]))
			require.NoError(t, err)
			_, err = factory.CreateContactList(workspace.ID, testutil.WithContactListEmail(emails[i]),
				testutil.WithContactListListID(list.ID), testutil.WithContactListStatus(domain.ContactListStatusActive))
			require.NoError(t, err)
			require.NoError(t, factory.CreateCustomEvent(workspace.ID, emails[i], eventName, nil))
		}
		require.Eventually(t, func() bool {
			var count int
			require.NoError(t, db.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM contact_automations WHERE automation_id = $1 AND status = 'active'`, automationID).Scan(&count))
			return count == n
		}, 30*time.Second, 100*time.Millisecond, "the journeys enrolled")
		_, err := db.ExecContext(ctx, `UPDATE contact_automations SET scheduled_at = NOW() - INTERVAL '1 second' WHERE automation_id = $1`, automationID)
		require.NoError(t, err)
		return emails
	}
	// Safe to call from the executors' goroutines: it reports, it does not stop the test.
	activeJourneys := func(automationID string) (int, error) {
		var count int
		err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM contact_automations WHERE automation_id = $1 AND status = 'active'`, automationID).Scan(&count)
		return count, err
	}

	t.Run("two instances on one backlog run each journey once", func(t *testing.T) {
		// The receiver counts calls by contact and answers slowly, so the first instance is still
		// in its batch when the second one starts.
		var mu sync.Mutex
		calls := map[string]int{}
		firstCall := make(chan struct{})
		var once sync.Once
		receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var payload struct {
				Email string `json:"email"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			mu.Lock()
			calls[payload.Email]++
			mu.Unlock()
			once.Do(func() { close(firstCall) })
			time.Sleep(50 * time.Millisecond)
			_, _ = w.Write([]byte(`{}`))
		}))
		defer receiver.Close()

		automationID, webhookID := createAutomation(t, "claim_backlog", receiver.URL)
		emails := enrol(t, automationID, "claim_backlog", 40)

		runCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		run := func(executor *service.AutomationExecutor) {
			for runCtx.Err() == nil {
				active, err := activeJourneys(automationID)
				if !assert.NoError(t, err) || active == 0 {
					return
				}
				processed, err := executor.ProcessBatch(runCtx, 50)
				assert.NoError(t, err)
				if processed == 0 {
					time.Sleep(50 * time.Millisecond)
				}
			}
		}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); run(newExecutor()) }()
		go func() {
			defer wg.Done()
			// The second instance ticks once the first is under way, its batch still running.
			select {
			case <-firstCall:
			case <-runCtx.Done():
				return
			}
			run(newExecutor())
		}()
		wg.Wait()
		active, err := activeJourneys(automationID)
		require.NoError(t, err)
		require.Equal(t, 0, active, "every journey ran to its end")

		mu.Lock()
		defer mu.Unlock()
		for _, email := range emails {
			assert.Equal(t, 1, calls[email], "one webhook call for %s", email)
		}
		var executions, journeys int
		require.NoError(t, db.QueryRowContext(ctx, `
			SELECT COUNT(*), COUNT(DISTINCT contact_automation_id) FROM automation_node_executions
			WHERE automation_id = $1 AND node_id = $2 AND action = 'completed'`, automationID, webhookID).Scan(&executions, &journeys))
		assert.Equal(t, len(emails), journeys)
		assert.Equal(t, journeys, executions, "one completed webhook step per journey")
	})

	t.Run("two claims at once get disjoint journeys", func(t *testing.T) {
		automationID, _ := createAutomation(t, "claim_disjoint", "http://127.0.0.1:1/unused")
		enrol(t, automationID, "claim_disjoint", 60)

		const lease = 5 * time.Minute
		var results [2][]*domain.ContactAutomation
		var errs [2]error
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range results {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				results[i], errs[i] = automationRepo.ClaimScheduledContactAutomations(ctx, workspace.ID, 50, lease)
			}(i)
		}
		close(start)
		wg.Wait()
		require.NoError(t, errs[0])
		require.NoError(t, errs[1])

		seen := map[string]bool{}
		for _, claimed := range results {
			for _, ca := range claimed {
				assert.False(t, seen[ca.ID], "journey %s claimed twice", ca.ID)
				seen[ca.ID] = true
				require.NotNil(t, ca.ScheduledAt)
				assert.True(t, ca.ScheduledAt.After(time.Now().Add(lease/2)), "the claim moves scheduled_at ahead")
			}
		}
		assert.Len(t, seen, 60, "between them, every due journey")
		assert.Equal(t, 0, func() int {
			var due int
			require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM contact_automations
				WHERE automation_id = $1 AND scheduled_at <= NOW()`, automationID).Scan(&due))
			return due
		}(), "no claimed journey is due any more")
	})

	t.Run("an expired claim is taken over, and the first holder's renewal fails", func(t *testing.T) {
		automationID, _ := createAutomation(t, "claim_expired", "http://127.0.0.1:1/unused")
		enrol(t, automationID, "claim_expired", 2)

		// Instance A claims both journeys with a lease that ends at once.
		claimedByA, err := automationRepo.ClaimScheduledContactAutomations(ctx, workspace.ID, 50, time.Millisecond)
		require.NoError(t, err)
		require.Len(t, claimedByA, 2)
		time.Sleep(20 * time.Millisecond)

		// Instance B claims the first journey again once A's lease ended.
		_, err = db.ExecContext(ctx, `UPDATE contact_automations SET scheduled_at = scheduled_at + INTERVAL '1 hour'
			WHERE id = $1`, claimedByA[1].ID) // keeps the second journey out of B's reach
		require.NoError(t, err)
		claimedByB, err := automationRepo.ClaimScheduledContactAutomations(ctx, workspace.ID, 50, 5*time.Minute)
		require.NoError(t, err)
		require.Len(t, claimedByB, 1)
		require.Equal(t, claimedByA[0].ID, claimedByB[0].ID)

		renewed, err := automationRepo.RenewContactAutomationClaim(ctx, workspace.ID, claimedByA[0].ID, *claimedByA[0].ScheduledAt, 5*time.Minute)
		require.NoError(t, err)
		assert.False(t, renewed, "A lost the journey to B")
		renewed, err = automationRepo.RenewContactAutomationClaim(ctx, workspace.ID, claimedByB[0].ID, *claimedByB[0].ScheduledAt, 5*time.Minute)
		require.NoError(t, err)
		assert.True(t, renewed, "B holds it")

		// The same token, after its claim moved on, no longer matches.
		renewed, err = automationRepo.RenewContactAutomationClaim(ctx, workspace.ID, claimedByB[0].ID, *claimedByB[0].ScheduledAt, 5*time.Minute)
		require.NoError(t, err)
		assert.False(t, renewed, "a renewal moves the claim, so its old token is spent")

		// One instance alone: a claim whose lease ended is still its own to renew.
		alone, err := automationRepo.ClaimScheduledContactAutomations(ctx, workspace.ID, 50, time.Millisecond)
		require.NoError(t, err)
		require.Empty(t, alone, "nothing else is due")
		var token time.Time
		require.NoError(t, db.QueryRowContext(ctx, `UPDATE contact_automations SET scheduled_at = NOW() - INTERVAL '1 second'
			WHERE id = $1 RETURNING scheduled_at`, claimedByA[1].ID).Scan(&token))
		renewed, err = automationRepo.RenewContactAutomationClaim(ctx, workspace.ID, claimedByA[1].ID, token, 5*time.Minute)
		require.NoError(t, err)
		assert.True(t, renewed, "an expired claim nobody took over")

		// A journey that ended is not renewed.
		var exitedToken time.Time
		require.NoError(t, db.QueryRowContext(ctx, `UPDATE contact_automations SET status = 'exited', exit_reason = 'replied'
			WHERE id = $1 RETURNING scheduled_at`, claimedByA[1].ID).Scan(&exitedToken))
		renewed, err = automationRepo.RenewContactAutomationClaim(ctx, workspace.ID, claimedByA[1].ID, exitedToken, 5*time.Minute)
		require.NoError(t, err)
		assert.False(t, renewed, "an exited journey")
	})
}
