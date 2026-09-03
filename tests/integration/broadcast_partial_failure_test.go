//go:build integration

package integration

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// quotaSMTPServer accepts a fixed number of messages and then refuses every one after
// that with a 452, the way a provider behaves once a daily sending quota is reached.
// This is the shape of failure the unit tests cannot reproduce: the arithmetic that
// decided a broadcast was complete only goes wrong when some recipients succeed and
// others do not.
type quotaSMTPServer struct {
	listener net.Listener
	wg       sync.WaitGroup

	mu       sync.Mutex
	accepted int
	// quota, when above zero, refuses everything past that many messages with a 452 —
	// a provider that has hit its daily allowance.
	quota int
	// deadMailboxes are refused at RCPT TO with a 550, whoever else succeeds. This is
	// the partial failure a list with stale addresses produces, and unlike a quota it
	// says nothing about the provider's health.
	deadMailboxes map[string]bool
	refused       int
}

func newQuotaSMTPServer(t *testing.T, quota int, dead map[string]bool) *quotaSMTPServer {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	s := &quotaSMTPServer{listener: listener, quota: quota, deadMailboxes: dead}
	s.wg.Add(1)
	go s.serve()
	return s
}

func (s *quotaSMTPServer) serve() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handle(conn)
		}()
	}
}

func (s *quotaSMTPServer) handle(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	conn.Write([]byte("220 quota-mock ESMTP\r\n"))

	inData := false
	for {
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)

		if inData {
			if line == "." {
				inData = false
				s.mu.Lock()
				overQuota := s.quota > 0 && s.accepted >= s.quota
				if overQuota {
					s.refused++
				} else {
					s.accepted++
				}
				s.mu.Unlock()

				if overQuota {
					conn.Write([]byte("452 4.3.1 Daily sending quota exceeded\r\n"))
				} else {
					conn.Write([]byte("250 OK queued\r\n"))
				}
			}
			continue
		}

		switch {
		case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
			conn.Write([]byte("250-quota-mock\r\n250 OK\r\n"))
		case strings.HasPrefix(line, "RCPT TO"):
			addr := strings.Trim(strings.TrimPrefix(line, "RCPT TO:"), "<> ")
			s.mu.Lock()
			isDead := s.deadMailboxes[addr]
			s.mu.Unlock()
			if isDead {
				s.mu.Lock()
				s.refused++
				s.mu.Unlock()
				conn.Write([]byte("550 5.1.1 User unknown; no such mailbox\r\n"))
				continue
			}
			conn.Write([]byte("250 OK\r\n"))
		case line == "DATA":
			conn.Write([]byte("354 Start mail input\r\n"))
			inData = true
		case line == "QUIT":
			conn.Write([]byte("221 Bye\r\n"))
			return
		default:
			conn.Write([]byte("250 OK\r\n"))
		}
	}
}

// healDeadMailboxes is the operator fixing the addresses before pressing Retry.
func (s *quotaSMTPServer) healDeadMailboxes() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deadMailboxes = nil
}

// liftQuota is the daily allowance resetting before the operator presses Resume.
func (s *quotaSMTPServer) liftQuota() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quota = 0
}

func (s *quotaSMTPServer) counts() (accepted, refused int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.accepted, s.refused
}

func (s *quotaSMTPServer) Port() int { return s.listener.Addr().(*net.TCPAddr).Port }

func (s *quotaSMTPServer) Close() {
	s.listener.Close()
	s.wg.Wait()
}

// TestBroadcastPartialFailure reproduces a campaign that reaches most of its audience
// and not all of it: a list carrying stale addresses, which the receiving server
// refuses with a 550 while every other message goes through.
//
// Before the fix this reported itself complete. sent_at is stamped on the first attempt
// whatever its outcome, so a refused recipient counted as both sent and failed, the
// remaining count reached zero and the badge went green; the queue rows were deleted,
// so nothing remembered who had missed the email; and the reason sat in a column that
// read a field the API never returned.
func TestBroadcastPartialFailure(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	const contactCount = 12
	const deadCount = 4

	dead := map[string]bool{}
	for i := 0; i < deadCount; i++ {
		dead[fmt.Sprintf("dead-%04d@example.com", i)] = true
	}

	smtpServer := newQuotaSMTPServer(t, 0, dead)
	defer smtpServer.Close()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	workspace, broadcastID, status := setUpFailingBroadcast(t, suite, smtpServer, contactCount, dead, "Partial Failure")

	appInstance := suite.ServerManager.GetApp()
	queueRepo := appInstance.GetEmailQueueRepository()
	ctx := context.Background()

	var counts *domain.EmailQueueSourceCounts
	require.Eventually(t, func() bool {
		var err error
		counts, err = queueRepo.GetSourceCounts(ctx, workspace.ID, domain.EmailQueueSourceBroadcast, broadcastID)
		if err != nil {
			return false
		}
		a, r := smtpServer.counts()
		t.Logf("queue=%+v accepted=%d refused=%d", counts, a, r)
		return counts.InFlight() == 0 && counts.FailedTerminal > 0
	}, 40*time.Second, 4*time.Second, "the queue should settle with the dead addresses held")

	accepted, refused := smtpServer.counts()
	t.Logf("provider accepted %d, refused %d; queue %+v; broadcast %s", accepted, refused, counts, status)

	// A rejection that names the recipient says nothing about the provider, so it must
	// not open the circuit breaker: four dead addresses must not stop the campaign.
	assert.Equal(t, "processed", status, "dead mailboxes are not a provider outage")

	// The recipients that were refused are still on record. Deleting them was what made
	// them unrecoverable and left the campaign looking complete.
	assert.Equal(t, int64(deadCount), counts.FailedTerminal)
	assert.Equal(t, int64(0), counts.InFlight())

	// One attempt each: a mailbox that does not exist will not start existing, and
	// retrying it twice more only delayed the campaign.
	assert.Equal(t, deadCount, refused, "a permanent rejection is not retried")

	messageHistoryRepo := appInstance.GetMessageHistoryRepository()
	stats, err := messageHistoryRepo.GetBroadcastStats(ctx, workspace.ID, broadcastID)
	require.NoError(t, err)
	assert.Equal(t, contactCount-deadCount, stats.TotalSent, "a refused message was never sent")
	assert.Equal(t, deadCount, stats.TotalFailed)

	// The provider's own words survive, so a quota can be told from a dead mailbox
	// without reading the server log.
	messages, _, err := messageHistoryRepo.ListMessages(ctx, workspace.ID, workspace.Settings.SecretKey,
		domain.MessageListParams{BroadcastID: broadcastID, Limit: 100})
	require.NoError(t, err)

	var explained int
	for _, m := range messages {
		if m.FailedAt != nil && m.StatusInfo != nil &&
			strings.Contains(*m.StatusInfo, "550") && strings.Contains(*m.StatusInfo, "User unknown") {
			explained++
		}
	}
	assert.Equal(t, deadCount, explained, "each failure should carry the code and the server's explanation")

	// Pausing and resuming the campaign must not quietly re-send to the people it gave
	// up on. Nothing offers to do that, so if it happens it happens silently: the
	// failed count would fall to zero on its own and those addresses would be mailed
	// again without anyone asking.
	pauseResp, err := client.Post("/api/broadcasts.pause", map[string]interface{}{
		"workspace_id": workspace.ID, "id": broadcastID,
	}, nil)
	require.NoError(t, err)
	defer pauseResp.Body.Close()
	require.Equal(t, http.StatusOK, pauseResp.StatusCode)

	resumeResp, err := client.Post("/api/broadcasts.resume", map[string]interface{}{
		"workspace_id": workspace.ID, "id": broadcastID,
	}, nil)
	require.NoError(t, err)
	defer resumeResp.Body.Close()
	require.Equal(t, http.StatusOK, resumeResp.StatusCode)

	require.Never(t, func() bool {
		_, refusedNow := smtpServer.counts()
		after, err := queueRepo.GetSourceCounts(ctx, workspace.ID, domain.EmailQueueSourceBroadcast, broadcastID)
		return refusedNow > deadCount || (err == nil && after.FailedTerminal != int64(deadCount))
	}, 8*time.Second, time.Second, "a resume must leave the abandoned recipients abandoned")

	// Now the operator does what the feature is for: fixes the addresses, and asks for
	// those recipients to be sent to again.
	smtpServer.healDeadMailboxes()

	retryResp, err := client.Post("/api/broadcasts.retryFailed", map[string]interface{}{
		"workspace_id": workspace.ID,
		"id":           broadcastID,
	}, nil)
	require.NoError(t, err)
	defer retryResp.Body.Close()
	require.Equal(t, http.StatusOK, retryResp.StatusCode)

	retryBody, err := client.ReadBody(retryResp)
	require.NoError(t, err)
	assert.Contains(t, retryBody, `"requeued":4`, "the operator is told how many are going out again")

	// The point is not that they were requeued. It is that they arrive.
	require.Eventually(t, func() bool {
		acceptedNow, _ := smtpServer.counts()
		after, err := queueRepo.GetSourceCounts(ctx, workspace.ID, domain.EmailQueueSourceBroadcast, broadcastID)
		return acceptedNow == contactCount && err == nil && after.InFlight() == 0 && after.FailedTerminal == 0
	}, 60*time.Second, time.Second, "the retried recipients should actually be delivered")

	// And the campaign heals: a retry that succeeds clears the failure, so the badge
	// goes back to Complete rather than carrying "4 failed" for ever.
	healed, err := messageHistoryRepo.GetBroadcastStats(ctx, workspace.ID, broadcastID)
	require.NoError(t, err)
	assert.Equal(t, contactCount, healed.TotalSent, "everyone has now been sent to")
	assert.Equal(t, 0, healed.TotalFailed, "a successful retry clears the failure it is retrying")
}

// TestBroadcastFailuresOutliveTheirQueueRows covers what a campaign says about itself a
// week later. Abandoned queue rows are swept after their retention window, and the
// completion verdict is derived — so a verdict reading only the queue would repaint a
// campaign that never reached four people as a green "Complete" once the sweep ran.
// Whether anyone was given up on has to survive the rows that recorded it.
func TestBroadcastFailuresOutliveTheirQueueRows(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	const contactCount = 8
	const deadCount = 3

	dead := map[string]bool{}
	for i := 0; i < deadCount; i++ {
		dead[fmt.Sprintf("swept-%04d@example.com", i)] = true
	}

	smtpServer := newQuotaSMTPServer(t, 0, dead)
	defer smtpServer.Close()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	workspace, broadcastID, _ := setUpFailingBroadcast(t, suite, smtpServer, contactCount, dead, "Swept Failures")

	appInstance := suite.ServerManager.GetApp()
	queueRepo := appInstance.GetEmailQueueRepository()
	messageHistoryRepo := appInstance.GetMessageHistoryRepository()
	ctx := context.Background()

	require.Eventually(t, func() bool {
		counts, err := queueRepo.GetSourceCounts(ctx, workspace.ID, domain.EmailQueueSourceBroadcast, broadcastID)
		return err == nil && counts.InFlight() == 0 && counts.FailedTerminal == int64(deadCount)
	}, 60*time.Second, time.Second, "the dead addresses should settle as abandoned")

	// Age every abandoned row past the window in one go.
	deleted, err := queueRepo.DeleteTerminallyFailedOlderThan(ctx, workspace.ID, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(deadCount), deleted)

	counts, err := queueRepo.GetSourceCounts(ctx, workspace.ID, domain.EmailQueueSourceBroadcast, broadcastID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), counts.FailedTerminal, "the rows are gone, so a retry is no longer offered")

	// What must not go with them: the fact that the campaign did not reach everyone.
	stats, err := messageHistoryRepo.GetBroadcastStats(ctx, workspace.ID, broadcastID)
	require.NoError(t, err)
	assert.Equal(t, deadCount, stats.TotalFailed,
		"the campaign still knows it failed four people after the sweep")
	assert.Equal(t, contactCount-deadCount, stats.TotalSent)

	// And the reason survives too, so the log is still worth opening.
	messages, _, err := messageHistoryRepo.ListMessages(ctx, workspace.ID, workspace.Settings.SecretKey,
		domain.MessageListParams{BroadcastID: broadcastID, Limit: 100})
	require.NoError(t, err)
	var explained int
	for _, m := range messages {
		if m.StatusInfo != nil && strings.Contains(*m.StatusInfo, "550") {
			explained++
		}
	}
	assert.Equal(t, deadCount, explained)
}

// TestBroadcastCircuitBreakerPause covers the outage the reported incident was: a
// provider that accepts a first tranche and then refuses everything because a daily
// quota is spent.
//
// The breaker already opened here and did nothing visible with it. Because it restores
// a full failure budget after each cooldown rather than latching, it let five more real
// attempts through every minute until every remaining recipient had spent all three —
// which is how a campaign lost a third of its audience while reporting success. Pausing
// on the first open keeps those recipients, with their attempts intact.
func TestBroadcastCircuitBreakerPause(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	const contactCount = 12
	const quota = 6

	smtpServer := newQuotaSMTPServer(t, quota, nil)
	defer smtpServer.Close()

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
			Senders: []domain.EmailSender{domain.NewEmailSender("noreply@notifuse.test", "Quota Test")},
			SMTP: &domain.SMTPSettings{
				Host:   "127.0.0.1",
				Port:   smtpServer.Port(),
				UseTLS: false,
			},
			RateLimitPerMinute: 2000,
		}))
	require.NoError(t, err)

	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	list, err := factory.CreateList(workspace.ID, testutil.WithListName("Quota Test List"))
	require.NoError(t, err)

	contacts := make([]map[string]interface{}, contactCount)
	for i := 0; i < contactCount; i++ {
		contacts[i] = map[string]interface{}{
			"email":      fmt.Sprintf("quota-%04d@example.com", i),
			"first_name": fmt.Sprintf("User%d", i),
		}
	}
	importResp, err := client.BatchImportContacts(contacts, []string{list.ID})
	require.NoError(t, err)
	defer importResp.Body.Close()
	require.Equal(t, http.StatusOK, importResp.StatusCode)

	broadcastID := scheduleTestBroadcast(t, client, factory, workspace.ID, list.ID, "Quota Test Broadcast")

	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(context.Background()))

	// The orchestrator finishes enqueueing before the provider runs out of quota, so
	// the broadcast reaches 'processed' first and is paused a moment later, once the
	// worker has actually been refused five times.
	_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcastID,
		[]string{"paused", "processed", "failed"}, 120*time.Second)
	require.NoError(t, err)

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
	}, 90*time.Second, time.Second, "a provider refusing everything should stop the campaign")

	counts, err := queueRepo.GetSourceCounts(ctx, workspace.ID, domain.EmailQueueSourceBroadcast, broadcastID)
	require.NoError(t, err)
	accepted, refused := smtpServer.counts()
	t.Logf("provider accepted %d, refused %d; queue %+v", accepted, refused, counts)

	// The point of pausing: the recipients past the quota are all still there, and
	// none has been abandoned. Left to the breaker alone they would have been ground
	// down five a minute until every one of them had spent its last attempt.
	assert.Positive(t, counts.Paused)
	assert.Equal(t, int64(0), counts.FailedTerminal, "nobody should have been given up on yet")
	assert.Equal(t, int64(contactCount-quota), counts.InFlight(),
		"every recipient past the quota is still accounted for")

	// And the reason is recorded where the operator will look for it.
	assert.Contains(t, body, "Circuit breaker triggered")
	assert.Contains(t, body, "452")

	// The promise the pause makes is that nothing is lost — which is only true if
	// Resume finishes the campaign once the quota is behind you. Asserting the pause
	// alone would leave the recipients stopped for ever and call it a success.
	smtpServer.liftQuota()

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
	}, 90*time.Second, time.Second, "resuming should send to everyone the pause held back")

	messageHistoryRepo := appInstance.GetMessageHistoryRepository()
	stats, err := messageHistoryRepo.GetBroadcastStats(ctx, workspace.ID, broadcastID)
	require.NoError(t, err)
	assert.Equal(t, contactCount, stats.TotalSent, "the whole audience is reached in the end")
	assert.Equal(t, 0, stats.TotalFailed, "a send that succeeds on resume clears its earlier failure")
}

// setUpFailingBroadcast puts a workspace, a list and a scheduled broadcast in front of
// the given mock provider, with `dead` among its recipients and the rest live.
func setUpFailingBroadcast(
	t *testing.T,
	suite *testutil.IntegrationTestSuite,
	smtpServer *quotaSMTPServer,
	contactCount int,
	dead map[string]bool,
	name string,
) (*domain.Workspace, string, string) {
	t.Helper()

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
			Senders: []domain.EmailSender{domain.NewEmailSender("noreply@notifuse.test", name)},
			SMTP: &domain.SMTPSettings{
				Host:   "127.0.0.1",
				Port:   smtpServer.Port(),
				UseTLS: false,
			},
			RateLimitPerMinute: 2000,
		}))
	require.NoError(t, err)

	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	list, err := factory.CreateList(workspace.ID, testutil.WithListName(name+" List"))
	require.NoError(t, err)

	contacts := make([]map[string]interface{}, 0, contactCount)
	for addr := range dead {
		contacts = append(contacts, map[string]interface{}{"email": addr, "first_name": "Dead"})
	}
	for i := len(dead); i < contactCount; i++ {
		contacts = append(contacts, map[string]interface{}{
			"email":      fmt.Sprintf("live-%s-%04d@example.com", uuid.New().String()[:6], i),
			"first_name": fmt.Sprintf("User%d", i),
		})
	}
	importResp, err := client.BatchImportContacts(contacts, []string{list.ID})
	require.NoError(t, err)
	defer importResp.Body.Close()
	require.Equal(t, http.StatusOK, importResp.StatusCode)

	broadcastID := scheduleTestBroadcast(t, client, factory, workspace.ID, list.ID, name+" Broadcast")

	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(context.Background()))

	// The harness does not run the task scheduler, so nothing enqueues the broadcast
	// unless the test drives it.
	status, err := testutil.WaitForBroadcastStatusWithExecution(t, client, broadcastID,
		[]string{"processed", "failed", "paused"}, 90*time.Second)
	require.NoError(t, err)

	return workspace, broadcastID, status
}

// scheduleTestBroadcast wires a template into a new broadcast and sends it now.
func scheduleTestBroadcast(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID, listID, name string) string {
	t.Helper()

	subject := fmt.Sprintf("%s %s", name, uuid.New().String()[:8])
	template, err := factory.CreateTemplate(workspaceID,
		testutil.WithTemplateName(name+" Template"),
		testutil.WithTemplateSubject(subject))
	require.NoError(t, err)

	broadcast, err := factory.CreateBroadcast(workspaceID,
		testutil.WithBroadcastName(name),
		testutil.WithBroadcastAudience(domain.AudienceSettings{
			List:                listID,
			ExcludeUnsubscribed: true,
		}))
	require.NoError(t, err)

	broadcast.TestSettings.Variations[0].TemplateID = template.ID
	updateResp, err := client.UpdateBroadcast(map[string]interface{}{
		"workspace_id":  workspaceID,
		"id":            broadcast.ID,
		"name":          broadcast.Name,
		"audience":      broadcast.Audience,
		"schedule":      broadcast.Schedule,
		"test_settings": broadcast.TestSettings,
	})
	require.NoError(t, err)
	defer updateResp.Body.Close()
	require.Equal(t, http.StatusOK, updateResp.StatusCode)

	scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
		"workspace_id": workspaceID,
		"id":           broadcast.ID,
		"send_now":     true,
	})
	require.NoError(t, err)
	defer scheduleResp.Body.Close()
	require.Equal(t, http.StatusOK, scheduleResp.StatusCode)

	return broadcast.ID
}
