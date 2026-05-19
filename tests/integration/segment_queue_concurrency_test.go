//go:build integration

package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestSegmentQueueDrainsUnderConcurrentEngagement is a regression guard for
// the segment-queue write path under realistic concurrent load. With a
// broadcast send + simulated /opens + simulated delivery-webhook UPDATEs all
// firing against the same workspace, it asserts that:
//
//   1. The broadcast reaches `processed` within the deadline.
//   2. The workspace pool's connection count never drops to zero mid-run
//      (precursor to `sql: database is closed`).
//   3. contact_segment_queue drains to ≤50% of contacts within the drain
//      window — i.e. the queue worker is actually making progress and not
//      stalled on row-lock contention.
//
// Output: on completion the sampled timeline is written to
//   tests/integration/diagnostics/segment_cascade_<timestamp>.json
// so failures can be inspected post-hoc without re-running.
//
// Tunables (env vars):
//   CASCADE_CONTACTS         default 200   contacts in the broadcast list
//   CASCADE_SEGMENTS         default 5     segments configured before send
//   CASCADE_OPEN_RATE_MS     default 50    delay between simulated /opens hits
//   CASCADE_DELIVERY_RATE_MS default 50    delay between simulated delivery webhooks
//   CASCADE_DEADLINE_SEC     default 90    total test budget
//   CASCADE_SAMPLE_MS        default 500   DB sampler tick
func TestSegmentQueueDrainsUnderConcurrentEngagement(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	cfg := loadCascadeKnobs()

	suite := testutil.NewIntegrationTestSuiteWithLiveScheduler(t, func(c *config.Config) testutil.AppInterface {
		return app.NewApp(c)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory
	tag := uuid.New().String()[:8]
	uniqueSubject := fmt.Sprintf("Cascade Diag %s", tag)

	user, err := factory.CreateUser()
	require.NoError(t, err)
	ws, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, ws.ID, "owner"))

	_, err = factory.SetupWorkspaceWithSMTPProvider(ws.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("noreply@notifuse.test", "Cascade Diag"),
			},
			SMTP: &domain.SMTPSettings{
				Host: "localhost", Port: 1025, UseTLS: false,
			},
			RateLimitPerMinute: 6000,
		}))
	require.NoError(t, err)

	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(ws.ID)

	// The factory's CreateWorkspace bypasses the workspace service, which
	// normally seeds the recurring process_contact_segment_queue task. Without
	// this, the queue piles up forever and the cascade we want to observe
	// never runs. (Same pattern as broadcast_scheduled_live_scheduler_test.go:283.)
	taskRepo := suite.ServerManager.GetApp().GetTaskRepository()
	require.NoError(t,
		service.EnsureContactSegmentQueueProcessingTask(context.Background(), taskRepo, ws.ID),
		"seed segment queue task")

	list, err := factory.CreateList(ws.ID, testutil.WithListName("Cascade Diag List"))
	require.NoError(t, err)

	// Create N contacts on the list. We use the factory direct-DB path so the
	// setup itself does not flood contact_timeline with INSERT events that
	// would warm up the queue before the broadcast even starts.
	for i := 0; i < cfg.contacts; i++ {
		email := fmt.Sprintf("cascade-%s-%04d@example.com", tag, i)
		c, cErr := factory.CreateContact(ws.ID, testutil.WithContactEmail(email))
		require.NoError(t, cErr)
		_, clErr := factory.CreateContactList(ws.ID,
			testutil.WithContactListEmail(c.Email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, clErr)
	}

	// Create segments so the queue worker has real work to do. Membership
	// writes by the worker fire track_contact_segment_changes (init.go:681),
	// which used to cascade through contact_timeline_queue_trigger
	// (init.go:836) and re-enqueue the same contact for re-processing — the
	// self-loop fixed by the trigger short-circuit in
	// queue_contact_for_segment_recomputation. This test reproduces the
	// load shape that exposed that self-loop.
	for i := 0; i < cfg.segments; i++ {
		_, sErr := factory.CreateSegment(ws.ID)
		require.NoError(t, sErr)
	}

	tmpl, err := factory.CreateTemplate(ws.ID,
		testutil.WithTemplateName("Cascade Diag Template"),
		testutil.WithTemplateSubject(uniqueSubject))
	require.NoError(t, err)

	bc, err := factory.CreateBroadcast(ws.ID,
		testutil.WithBroadcastName("Cascade Diag Broadcast"),
		testutil.WithBroadcastAudience(domain.AudienceSettings{
			List:                list.ID,
			ExcludeUnsubscribed: true,
		}))
	require.NoError(t, err)

	bc.TestSettings.Variations[0].TemplateID = tmpl.ID
	uResp, err := client.UpdateBroadcast(map[string]interface{}{
		"workspace_id":  ws.ID,
		"id":            bc.ID,
		"name":          bc.Name,
		"audience":      bc.Audience,
		"schedule":      bc.Schedule,
		"test_settings": bc.TestSettings,
	})
	require.NoError(t, err)
	uResp.Body.Close()

	// --- Start DB sampler before triggering the broadcast. -----------------
	workspaceDB, err := suite.DBManager.GetWorkspaceDB(ws.ID)
	require.NoError(t, err)
	sampler := newDBSampler(workspaceDB, cfg.sampleEvery)
	samplerCtx, stopSampler := context.WithCancel(context.Background())
	go sampler.run(samplerCtx)

	// --- Schedule past-due so the live scheduler picks it up. --------------
	past := time.Now().UTC().Add(-2 * time.Minute)
	sResp, err := client.ScheduleBroadcast(map[string]interface{}{
		"workspace_id":           ws.ID,
		"id":                     bc.ID,
		"send_now":               false,
		"scheduled_date":         past.Format("2006-01-02"),
		"scheduled_time":         past.Format("15:04"),
		"timezone":               "UTC",
		"use_recipient_timezone": false,
	})
	require.NoError(t, err)
	sResp.Body.Close()
	require.Equal(t, http.StatusOK, sResp.StatusCode)

	// --- Open simulator: as soon as messages start landing in message_history
	//     with sent_at, fire /opens for them. This is the same path SES
	//     opens hit, so it exercises track_message_history_changes() and
	//     webhook_message_history_trigger() the same way production does. ---
	opener := newOpenSimulator(suite.ServerManager.GetURL(), workspaceDB, ws.ID, cfg.openRate)
	openerCtx, stopOpener := context.WithCancel(context.Background())
	go opener.run(openerCtx, t)

	// --- Delivery webhook simulator: third concurrent writer per email,
	//     mimicking SES `delivered` callbacks. Issues the same
	//     `UPDATE message_history SET delivered_at = ...` write the webhook
	//     handler performs, exercising the same trigger cascade as opens. ---
	deliverer := newDeliverySimulator(workspaceDB, cfg.deliveryRate)
	delivererCtx, stopDeliverer := context.WithCancel(context.Background())
	go deliverer.run(delivererCtx, t)

	// --- Wait for broadcast to finish or deadline. -------------------------
	deadline := time.Now().Add(time.Duration(cfg.deadlineSec) * time.Second)
	var broadcastStatus string
	var enqueued int
	for time.Now().Before(deadline) {
		broadcastStatus, enqueued = getBroadcastStatusAndEnqueued(t, client, bc.ID)
		if broadcastStatus == string(domain.BroadcastStatusProcessed) {
			break
		}
		time.Sleep(1 * time.Second)
	}

	// Drain window. The queue processor's getPendingEmailsInTx applies a 15s
	// debounce (queued_at < NOW - 15s) so newly-enqueued items are not
	// immediately visible to the worker. We also need to leave room for one
	// full Process cycle (up to ~50s) plus the 100ms inner-loop tick. 30s is
	// enough to observe membership writes start landing without waiting for
	// the full task cycle to recycle.
	time.Sleep(30 * time.Second)

	stopOpener()
	stopDeliverer()
	stopSampler()
	// One final sample after sampler stop, to capture the post-drain state.
	final := sampler.sampleOnce()
	sampler.append(final)

	// --- Dump the timeline to disk. ----------------------------------------
	diagPath := sampler.dump(t, "segment_cascade")
	t.Logf("diagnostics written to %s", diagPath)
	t.Logf("samples=%d opens_fired=%d deliveries_fired=%d broadcast.status=%s enqueued=%d",
		sampler.count(), opener.fired(), deliverer.fired(), broadcastStatus, enqueued)
	sampler.logSummary(t)

	// --- Symptom-match assertions (fail on bug signals, not on slow CI). ---

	// Symptom 1: broadcast never reached processed.
	if broadcastStatus != string(domain.BroadcastStatusProcessed) {
		t.Fatalf("REGRESSION:broadcast did not reach processed within %ds — status=%s enqueued=%d (diag: %s)",
			cfg.deadlineSec, broadcastStatus, enqueued, diagPath)
	}

	// Symptom 2: app connection count to the workspace DB dropped to zero
	// while work was still in flight — this is the precursor to
	// `sql: database is closed`. We exempt the very last sample (post-drain
	// the pool may have legitimately released idle conns).
	if poolCollapsed := sampler.detectPoolCollapse(); poolCollapsed != "" {
		t.Fatalf("REGRESSION:workspace pool collapse detected — %s (diag: %s)",
			poolCollapsed, diagPath)
	}

	// Symptom 3: contact_segment_queue still has rows long after broadcast
	// finished. We allow up to 5% of contacts as residual (eval lag), but a
	// stuck queue (≥50% of contacts after drain window) means the worker
	// could not keep up — which is the actual symptom the reporter saw.
	if residual := queueSize(t, workspaceDB); residual > cfg.contacts/2 {
		t.Fatalf("REGRESSION:contact_segment_queue stuck at %d/%d after drain — worker not progressing (diag: %s)",
			residual, cfg.contacts, diagPath)
	}
}

// -----------------------------------------------------------------------------
// Knobs
// -----------------------------------------------------------------------------

type cascadeConfig struct {
	contacts     int
	segments     int
	openRate     time.Duration
	deliveryRate time.Duration
	deadlineSec  int
	sampleEvery  time.Duration
}

func loadCascadeKnobs() cascadeConfig {
	return cascadeConfig{
		contacts:     envInt("CASCADE_CONTACTS", 200),
		segments:     envInt("CASCADE_SEGMENTS", 5),
		openRate:     time.Duration(envInt("CASCADE_OPEN_RATE_MS", 50)) * time.Millisecond,
		deliveryRate: time.Duration(envInt("CASCADE_DELIVERY_RATE_MS", 50)) * time.Millisecond,
		deadlineSec:  envInt("CASCADE_DEADLINE_SEC", 90),
		sampleEvery:  time.Duration(envInt("CASCADE_SAMPLE_MS", 500)) * time.Millisecond,
	}
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// -----------------------------------------------------------------------------
// DB sampler
// -----------------------------------------------------------------------------

type dbSample struct {
	T              time.Time      `json:"t"`
	AppConns       int            `json:"app_conns"`
	ActiveQueries  int            `json:"active_queries"`
	LockWaiters    int            `json:"lock_waiters"`
	WaitEvents     map[string]int `json:"wait_events"`
	QueueSize      int            `json:"queue_size"`
	WriteCounts    map[string]int `json:"write_counts"`
	TopActiveQuery string         `json:"top_active_query,omitempty"`
}

type dbSampler struct {
	db    *sql.DB
	every time.Duration
	mu    sync.Mutex
	rows  []dbSample
}

func newDBSampler(db *sql.DB, every time.Duration) *dbSampler {
	return &dbSampler{db: db, every: every, rows: make([]dbSample, 0, 256)}
}

func (s *dbSampler) run(ctx context.Context) {
	tick := time.NewTicker(s.every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.append(s.sampleOnce())
		}
	}
}

func (s *dbSampler) sampleOnce() dbSample {
	out := dbSample{T: time.Now().UTC(), WaitEvents: map[string]int{}, WriteCounts: map[string]int{}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// App connections (anything that isn't us).
	_ = s.db.QueryRowContext(ctx, `
		SELECT count(*) FROM pg_stat_activity
		WHERE datname = current_database() AND pid <> pg_backend_pid()`).Scan(&out.AppConns)

	// Active queries + wait events + a sample of the longest-running query.
	rows, err := s.db.QueryContext(ctx, `
		SELECT COALESCE(wait_event_type,'-')||':'||COALESCE(wait_event,'-'),
		       state, COALESCE(left(query, 200), '')
		FROM pg_stat_activity
		WHERE datname = current_database() AND pid <> pg_backend_pid() AND state = 'active'
		ORDER BY xact_start NULLS LAST`)
	if err == nil {
		for rows.Next() {
			var we, state, q string
			if rows.Scan(&we, &state, &q) == nil {
				out.ActiveQueries++
				out.WaitEvents[we]++
				if out.TopActiveQuery == "" {
					out.TopActiveQuery = q
				}
			}
		}
		rows.Close()
	}

	// Lock waiters.
	_ = s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM pg_locks WHERE NOT granted`).Scan(&out.LockWaiters)

	// Queue size.
	_ = s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM contact_segment_queue`).Scan(&out.QueueSize)

	// Write counts for the tables in the trigger chain.
	wrows, err := s.db.QueryContext(ctx, `
		SELECT relname, COALESCE(n_tup_ins,0)+COALESCE(n_tup_upd,0)
		FROM pg_stat_user_tables
		WHERE relname IN ('message_history','contact_timeline','contact_segment_queue',
		                  'contact_segments','webhook_deliveries')`)
	if err == nil {
		for wrows.Next() {
			var name string
			var n int
			if wrows.Scan(&name, &n) == nil {
				out.WriteCounts[name] = n
			}
		}
		wrows.Close()
	}
	return out
}

func (s *dbSampler) append(v dbSample) {
	s.mu.Lock()
	s.rows = append(s.rows, v)
	s.mu.Unlock()
}

func (s *dbSampler) count() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.rows) }

// detectPoolCollapse looks for app_conns dropping to 0 between samples that
// previously showed >0 conns, ignoring the final sample (idle release is
// legitimate post-drain).
func (s *dbSampler) detectPoolCollapse() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.rows) < 3 {
		return ""
	}
	sawConns := false
	for i, r := range s.rows[:len(s.rows)-1] {
		if r.AppConns > 0 {
			sawConns = true
			continue
		}
		if sawConns && r.AppConns == 0 {
			return fmt.Sprintf("app_conns dropped to 0 at sample %d (t=%s) after previously being >0",
				i, r.T.Format(time.RFC3339Nano))
		}
	}
	return ""
}

// logSummary prints a single-line summary per sample so test output is grep'able.
func (s *dbSampler) logSummary(t *testing.T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.rows) == 0 {
		return
	}
	// Print first, peak, last.
	first := s.rows[0]
	last := s.rows[len(s.rows)-1]
	peakActive, peakWaiters, peakQueue := 0, 0, 0
	for _, r := range s.rows {
		if r.ActiveQueries > peakActive {
			peakActive = r.ActiveQueries
		}
		if r.LockWaiters > peakWaiters {
			peakWaiters = r.LockWaiters
		}
		if r.QueueSize > peakQueue {
			peakQueue = r.QueueSize
		}
	}
	t.Logf("cascade summary: peak active_queries=%d peak lock_waiters=%d peak queue=%d "+
		"writes(start→end): mh=%d→%d timeline=%d→%d queue=%d→%d segments=%d→%d deliveries=%d→%d",
		peakActive, peakWaiters, peakQueue,
		first.WriteCounts["message_history"], last.WriteCounts["message_history"],
		first.WriteCounts["contact_timeline"], last.WriteCounts["contact_timeline"],
		first.WriteCounts["contact_segment_queue"], last.WriteCounts["contact_segment_queue"],
		first.WriteCounts["contact_segments"], last.WriteCounts["contact_segments"],
		first.WriteCounts["webhook_deliveries"], last.WriteCounts["webhook_deliveries"],
	)
}

func (s *dbSampler) dump(t *testing.T, label string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join("diagnostics")
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, fmt.Sprintf("%s_%s.json", label, time.Now().UTC().Format("20060102T150405Z")))
	f, err := os.Create(path)
	if err != nil {
		t.Logf("could not write diagnostics: %v", err)
		return ""
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	_ = enc.Encode(s.rows)
	return path
}

// -----------------------------------------------------------------------------
// Open simulator
// -----------------------------------------------------------------------------

type openSimulator struct {
	baseURL     string
	db          *sql.DB
	workspaceID string
	every       time.Duration
	hits        atomic.Int64
	http        *http.Client
}

func newOpenSimulator(baseURL string, db *sql.DB, workspaceID string, every time.Duration) *openSimulator {
	return &openSimulator{
		baseURL: strings.TrimRight(baseURL, "/"), db: db, workspaceID: workspaceID,
		every: every, http: &http.Client{Timeout: 5 * time.Second},
	}
}

func (o *openSimulator) fired() int64 { return o.hits.Load() }

func (o *openSimulator) run(ctx context.Context, t *testing.T) {
	tick := time.NewTicker(o.every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			id, ok := o.pickUnopened(ctx)
			if !ok {
				continue
			}
			// /opens?mid=X&wid=Y — omit ts so the 7-second bot-detection
			// shortcut (email_handler.go:230) does not skip the write.
			url := fmt.Sprintf("%s/opens?mid=%s&wid=%s", o.baseURL, id, o.workspaceID)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				continue
			}
			req.Header.Set("User-Agent", "CascadeDiagOpener/1.0")
			resp, err := o.http.Do(req)
			if err != nil {
				continue
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			o.hits.Add(1)
		}
	}
}

func (o *openSimulator) pickUnopened(ctx context.Context) (string, bool) {
	q, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var id string
	err := o.db.QueryRowContext(q, `
		SELECT id FROM message_history
		WHERE sent_at IS NOT NULL AND opened_at IS NULL
		ORDER BY random() LIMIT 1`).Scan(&id)
	if err != nil {
		return "", false
	}
	return id, true
}

// -----------------------------------------------------------------------------
// Delivery webhook simulator
// -----------------------------------------------------------------------------

// deliverySimulator issues the same UPDATE that the SES delivery webhook
// handler performs, marking a sent message as delivered. Each update fires
// track_message_history_changes → contact_timeline → contact_segment_queue,
// adding a third concurrent writer per email alongside opens and the queue
// worker.
type deliverySimulator struct {
	db    *sql.DB
	every time.Duration
	hits  atomic.Int64
}

func newDeliverySimulator(db *sql.DB, every time.Duration) *deliverySimulator {
	return &deliverySimulator{db: db, every: every}
}

func (d *deliverySimulator) fired() int64 { return d.hits.Load() }

func (d *deliverySimulator) run(ctx context.Context, t *testing.T) {
	tick := time.NewTicker(d.every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			id, ok := d.pickUndelivered(ctx)
			if !ok {
				continue
			}
			q, cancel := context.WithTimeout(ctx, 2*time.Second)
			// Match the production webhook write exactly: only set if not
			// already set, update updated_at. This is the same statement
			// path as message_history_postgre.go but on a single row.
			_, err := d.db.ExecContext(q, `
				UPDATE message_history
				SET delivered_at = $1, updated_at = NOW()
				WHERE id = $2 AND delivered_at IS NULL`,
				time.Now().UTC(), id)
			cancel()
			if err != nil {
				continue
			}
			d.hits.Add(1)
		}
	}
}

func (d *deliverySimulator) pickUndelivered(ctx context.Context) (string, bool) {
	q, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var id string
	err := d.db.QueryRowContext(q, `
		SELECT id FROM message_history
		WHERE sent_at IS NOT NULL AND delivered_at IS NULL
		ORDER BY random() LIMIT 1`).Scan(&id)
	if err != nil {
		return "", false
	}
	return id, true
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

func queueSize(t *testing.T, db *sql.DB) int {
	var n int
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM contact_segment_queue`).Scan(&n); err != nil {
		t.Logf("queueSize query failed: %v", err)
		return -1
	}
	return n
}
