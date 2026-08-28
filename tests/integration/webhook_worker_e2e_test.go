//go:build integration

package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestWebhookWorkerEndToEnd drives the outbound webhook chain from a real write
// to a real receiver, through a genuinely running delivery worker.
//
// Everything else that covers this chain covers one of its two ends. The trigger
// tests write a contact and count rows in webhook_deliveries; the signature tests
// stand up an httptest receiver and drive it from
// POST /api/webhookSubscriptions.test, which calls SendTestWebhook directly. The
// middle — claim, renew, POST, release, retry, 410, 429, auto-disable, the stale
// claim sweep — had never once executed against a database, because the worker
// starts on a thirty-second delay (internal/app/app.go) and the harness that
// would have run it never calls app.Start() at all. Every integration test in
// this package finishes long before that timer fires, so the loop's own
// behaviour was proven only against mocks, in a package where every repository
// answer is whatever the test said it would be.
//
// That middle is where the concurrency lives, so it is where the interesting
// defects live too: a release that resurrected an already-delivered row, a
// cached failure window that outlived the run it described, a lease measured
// against one delivery while the claim was stamped for a batch of a hundred.
// None of them are visible from either end of the chain.
//
// The worker here is the production one, built by the production constructor and
// started through Start — only its poll interval, and where the case is about
// the sweep its lease, are shortened, so what runs is the real ticker driving the
// real guarded poll. Nothing reaches past Start into the loop.
//
// Two pieces of that list are deliberately left short of end to end, so the list
// is not read as a promise:
//
//   - Auto-disable is covered up to its first increment. Crossing the threshold
//     takes twenty consecutive failures spread over a two-hour window, and
//     neither number is injectable — deliberately, since every knob that exists
//     only for a test is a production behaviour nothing checks. The 404 case
//     proves the run opens and is counted; the threshold itself stays with the
//     unit tests.
//   - The response-body drain in readLimitedResponseBody is a connection-reuse
//     property rather than a behavioural one, and every receiver here answers
//     well inside the read limit, so it is consumed either way. Proving it needs
//     a receiver answering past the limit and an assertion counting the TCP
//     connections N deliveries opened, which belongs beside the HTTP client.
func TestWebhookWorkerEndToEnd(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	f := &webhookWorkerFixture{
		app:         suite.ServerManager.GetApp(),
		client:      client,
		workspaceID: workspace.ID,
		db:          db,
	}

	t.Run("a triggered event is signed, delivered and its claim released", func(t *testing.T) {
		receiver := newWebhookReceiver(http.StatusOK, "OK")
		defer receiver.Close()

		subscriptionID, secret := f.createSubscription(t, "worker happy path", receiver.URL, "")
		defer f.removeSubscription(t, subscriptionID)

		contact, err := factory.CreateContact(f.workspaceID)
		require.NoError(t, err)

		// Asserted before a worker exists, so a failure here is the trigger's and
		// not the worker's. Everything after this point is the worker's alone.
		queued := f.mustDeliveries(t, subscriptionID)
		require.Len(t, queued, 1, "the PL/pgSQL trigger must enqueue one delivery for the new contact")
		require.Equal(t, domain.WebhookDeliveryStatusPending, queued[0].status)

		stop := f.startWorker(t)
		defer stop()

		// At least one, not exactly one, and then a window of its own for the
		// overshoot — the shape the cases below use, for the same reason. A
		// regression that POSTs this row twice inside one fifty-millisecond
		// sample never shows the counter a one at all, so an equality here would
		// spend its whole budget and report a delivery that went out twice as
		// one that never went out.
		require.Eventually(t, func() bool { return receiver.count() >= 1 }, 15*time.Second, 50*time.Millisecond,
			"the worker must claim the queued row and POST it to the subscriber")
		require.Never(t, func() bool { return receiver.count() > 1 }, 2*time.Second, 100*time.Millisecond,
			"one queued event is one POST")

		f.eventuallyState(t, subscriptionID, func(s webhookWorkerState) bool {
			return allDelivered(s.rows, 1)
		}, 15*time.Second, 50*time.Millisecond,
			"the row must reach 'delivered'; the POST alone proves only that bytes left the process")

		row := f.mustSingleDelivery(t, subscriptionID)
		assert.Equal(t, 1, row.attempts, "one POST is one attempt")
		assert.True(t, row.deliveredAt.Valid)
		assert.Equal(t, int64(http.StatusOK), row.lastResponseStatus.Int64)
		// The claim and the status move together. A delivered row still carrying a
		// claimed_at reads as in flight, and the sweep would hand it to the next
		// worker and have it POSTed a second time.
		assert.False(t, row.claimedAt.Valid, "delivering must release the claim it took")

		sent := receiver.all()
		require.Len(t, sent, 1)
		assert.Equal(t, row.id, sent[0].headers.Get("webhook-id"),
			"the delivery row's id is the message id a subscriber deduplicates on")

		valid, err := verifyWebhookSignature(
			sent[0].headers.Get("webhook-id"),
			sent[0].headers.Get("webhook-timestamp"),
			sent[0].headers.Get("webhook-signature"),
			secret,
			sent[0].body)
		require.NoError(t, err)
		assert.True(t, valid, "the signature must verify against the subscription's own secret")

		var envelope struct {
			ID          string `json:"id"`
			Type        string `json:"type"`
			WorkspaceID string `json:"workspace_id"`
			Data        struct {
				Contact struct {
					Email string `json:"email"`
				} `json:"contact"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(sent[0].body, &envelope))
		assert.Equal(t, row.id, envelope.ID)
		assert.Equal(t, "contact.created", envelope.Type)
		assert.Equal(t, f.workspaceID, envelope.WorkspaceID)
		// The contact the trigger serialised is the contact that reaches the
		// subscriber: the payload survives the row, the claim and the envelope.
		assert.Equal(t, contact.Email, envelope.Data.Contact.Email)

		// Its own wait, not an assertion on the row above: MarkDelivered and
		// UpdateLastDeliveryAt are two separate statements, and the gate that
		// released this case watched the first of them. Reading the subscription
		// straight afterwards assumes the second has landed, which is an ordering
		// assumption rather than synchronisation and would come apart first on the
		// runner least able to explain it.
		f.eventuallyState(t, subscriptionID, func(s webhookWorkerState) bool {
			return s.sub.exists && s.sub.lastDeliveryAt.Valid
		}, 15*time.Second, 50*time.Millisecond,
			"a delivery must stamp last_delivery_at on the subscription")
	})

	t.Run("two workers deliver each queued event exactly once", func(t *testing.T) {
		// The receiver holds each request open, so one worker is still mid-batch
		// while the other polls, sweeps and claims. Answering instantly would let
		// the first worker finish its whole batch inside a single poll interval and
		// the second would never overlap it — the test would pass without the two
		// having contended for anything.
		receiver := newSlowWebhookReceiver(http.StatusOK, "OK", 100*time.Millisecond)
		defer receiver.Close()

		subscriptionID, _ := f.createSubscription(t, "worker exactly once", receiver.URL, "")
		defer f.removeSubscription(t, subscriptionID)

		const queuedEvents = 5
		for i := 0; i < queuedEvents; i++ {
			_, err := factory.CreateContact(f.workspaceID)
			require.NoError(t, err)
		}
		require.Len(t, f.mustDeliveries(t, subscriptionID), queuedEvents)

		// Two workers against one workspace is the deployment this claim exists
		// for: without it every replica delivers every event, and a duplicate
		// webhook is a duplicate side effect in whatever consumes it.
		//
		// Whether the two ever collide over a given row is up to the scheduler,
		// which is why this case proves the outcome and the case after it proves
		// the mechanism.
		stopFirst := f.startWorker(t)
		defer stopFirst()
		stopSecond := f.startWorker(t)
		defer stopSecond()

		// At least, not exactly: a count that has already overshot is the duplicate
		// this case is looking for, and waiting for equality would report it as
		// nothing having arrived at all.
		require.Eventually(t, func() bool { return receiver.count() >= queuedEvents }, 20*time.Second, 50*time.Millisecond,
			"between them the two workers must deliver every queued event")

		// Eventually stops at the first moment its condition holds, so a duplicate
		// that arrives a beat later needs a window of its own.
		require.Never(t, func() bool { return receiver.count() > queuedEvents }, 2*time.Second, 100*time.Millisecond,
			"no event may be POSTed twice")

		// The receiver logs a request before it answers it, so the last POST can
		// still be open when its count lands: wait for the rows the worker writes
		// after the response rather than assume they are already there.
		f.eventuallyState(t, subscriptionID, func(s webhookWorkerState) bool {
			return allDelivered(s.rows, queuedEvents)
		}, 20*time.Second, 50*time.Millisecond, "every queued row must end 'delivered'")

		perMessage := map[string]int{}
		for _, sent := range receiver.all() {
			perMessage[sent.headers.Get("webhook-id")]++
		}
		assert.Len(t, perMessage, queuedEvents, "every POST must carry a distinct delivery id")
		for id, count := range perMessage {
			assert.Equal(t, 1, count, "delivery %s was POSTed more than once", id)
		}

		rows := f.mustDeliveries(t, subscriptionID)
		require.Len(t, rows, queuedEvents)
		for _, row := range rows {
			assert.Equal(t, domain.WebhookDeliveryStatusDelivered, row.status)
			// A row re-claimed and re-sent shows up here as a second attempt even if
			// the receiver's counter happened to be read between the two POSTs.
			assert.Equal(t, 1, row.attempts, "delivery %s was attempted more than once", row.id)
			assert.False(t, row.claimedAt.Valid)
		}
	})

	t.Run("the claim steps over a row another worker holds and takes the rest", func(t *testing.T) {
		// The case above proves each event went out once. It does not prove the
		// claim is what made that true, and the difference is not academic: two
		// workers over a queue that fits in one batch do not reliably collide at
		// all. Whichever polls first takes the whole batch in a single statement,
		// those rows leave the pending predicate, and the second finds nothing
		// rather than contending for anything — so the SELECT's locking clause can
		// be deleted outright and that case stays green.
		//
		// This one contends on purpose, at the level where the clause lives. A
		// transaction holds one queued row locked, which is exactly what a worker
		// mid-claim looks like from another connection, and the claim runs against
		// it under a deadline. With SKIP LOCKED the claim steps over the held row
		// and takes the others. With a plain FOR UPDATE the subquery waits on the
		// lock; with no locking clause at all the outer UPDATE waits on it instead,
		// because a row it intends to write is a row it must lock first. Both of
		// those end at the deadline, and the deadline is the assertion.
		// Checked by hand because this is the one case that starts no worker of
		// its own, and startWorker is where every other case is told that an
		// earlier one left theirs running. The count asserted at the end is
		// exactly what a leaked worker would break.
		f.requireNoLeakedWorker(t)

		receiver := newWebhookReceiver(http.StatusOK, "OK")
		defer receiver.Close()

		subscriptionID, _ := f.createSubscription(t, "worker claim skips locked", receiver.URL, "")
		defer f.removeSubscription(t, subscriptionID)

		const queuedEvents = 3
		for i := 0; i < queuedEvents; i++ {
			_, err := factory.CreateContact(f.workspaceID)
			require.NoError(t, err)
		}
		queued := f.mustDeliveries(t, subscriptionID)
		require.Len(t, queued, queuedEvents)

		// No worker runs here. The claim is called directly, so the only things
		// that touch these rows are this test's lock and this test's claim.
		holder, err := db.BeginTx(context.Background(), nil)
		require.NoError(t, err)
		defer func() { _ = holder.Rollback() }()

		lockedID := queued[0].id
		var held string
		require.NoError(t,
			holder.QueryRow(`SELECT id FROM webhook_deliveries WHERE id = $1 FOR UPDATE`, lockedID).Scan(&held),
			"the lock this case contends with must actually be held")

		deliveryRepo := repository.NewWebhookDeliveryRepository(f.app.GetWorkspaceRepository(), f.app.GetLogger())

		// Bounded because a claim that queues behind the lock never returns on its
		// own: unbounded, the failure would arrive as a package-wide timeout with
		// no name on it instead of as this assertion.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		claimed, err := deliveryRepo.GetPendingForWorkspace(ctx, f.workspaceID, queuedEvents+1)
		require.NoError(t, err,
			"the claim must walk past a locked row rather than wait on it, which is what SKIP LOCKED buys")

		claimedIDs := map[string]bool{}
		for _, delivery := range claimed {
			claimedIDs[delivery.ID] = true
		}
		assert.False(t, claimedIDs[lockedID], "a row another worker is claiming must be left to it")
		for _, row := range queued[1:] {
			assert.True(t, claimedIDs[row.id],
				"delivery %s was neither locked nor claimed: the whole batch waited on one row", row.id)
		}

		// Read back rather than trusted from the return value: the claim's promise
		// is a durable status change, and stepping over the locked row means
		// leaving it claimable by whoever holds it.
		for _, row := range f.mustDeliveries(t, subscriptionID) {
			if row.id == lockedID {
				assert.Equal(t, domain.WebhookDeliveryStatusPending, row.status,
					"the skipped row must still be there for its owner to claim")
				continue
			}
			assert.Equal(t, domain.WebhookDeliveryStatusDelivering, row.status,
				"delivery %s was returned by the claim without being claimed", row.id)
		}

		assert.Equal(t, 0, receiver.count(), "no worker runs in this case, so nothing may be POSTed")
	})

	t.Run("a claim left behind by a dead worker is reclaimed once its lease expires", func(t *testing.T) {
		receiver := newWebhookReceiver(http.StatusOK, "OK")
		defer receiver.Close()

		subscriptionID, _ := f.createSubscription(t, "worker stale claim", receiver.URL, "")
		defer f.removeSubscription(t, subscriptionID)

		_, err := factory.CreateContact(f.workspaceID)
		require.NoError(t, err)
		queued := f.mustDeliveries(t, subscriptionID)
		require.Len(t, queued, 1)

		// A worker that was killed between the claim and the POST leaves exactly
		// this: 'delivering' with a claimed_at and nothing coming. No predicate
		// selects that row again — the claim query excludes 'delivering' on purpose
		// — so without the sweep it sits there until the retention window deletes
		// it, holding one of the batch's slots the entire time.
		//
		// Stamped from before the UPDATE, so the elapsed time asserted below can
		// only overstate how long the sweep left the row alone.
		strandedAt := time.Now()
		_, err = db.Exec(
			`UPDATE webhook_deliveries SET status = 'delivering', claimed_at = NOW() WHERE id = $1`,
			queued[0].id)
		require.NoError(t, err)

		// Deliberately shorter than startWorker's five-second HTTP timeout, which
		// is the one configuration the production comments call a duplicate
		// factory: a lease that expires while a POST is still in flight hands the
		// row to a second worker and both send it. It is safe here only because
		// this receiver answers in microseconds, so no delivery is ever in flight
		// when the sweep runs. Swapping newSlowWebhookReceiver in below would
		// double-deliver, and it would read as a worker bug rather than as this
		// line.
		const lease = 4 * time.Second
		stop := f.startWorker(t, service.WithWebhookClaimLease(lease))
		defer stop()

		// The lease has to be respected in both directions. Reclaiming early is not
		// a harmless eagerness: the row it takes may belong to a worker whose POST
		// is still in flight, and then two workers deliver it.
		require.Never(t, func() bool { return receiver.count() > 0 }, lease/2, 100*time.Millisecond,
			"a claim younger than the lease must be left alone")

		// Budgeted against the lease this case injected rather than generously,
		// because the budget is the only thing here that can tell an applied
		// lease from an ignored one. Drop the option and the worker falls back
		// to the lease it derives from its own HTTP client, which is longer than
		// this whole window — a budget wide enough to cover that would pass on
		// the default and say nothing about the option at all.
		//
		// At least one rather than exactly one, with a window of its own below
		// for the overshoot: a sweep that returned the row twice can POST it
		// twice inside a single sample, and an equality would report that as
		// nothing having gone out.
		require.Eventually(t, func() bool { return receiver.count() >= 1 }, 2*lease, 50*time.Millisecond,
			"past the lease the stranded row must return to 'pending' and go out")

		// The lease's other direction, which the window above only half covers:
		// it watches lease/2, and a reclaim anywhere between that and the lease
		// is still early. Measured from before the row was stranded, so the
		// measurement can only overstate the wait — and both ends of the sweep's
		// own comparison are the database's clock, so the interval survives any
		// offset between that clock and this process's.
		assert.GreaterOrEqual(t, time.Since(strandedAt), lease,
			"the sweep must wait out the lease it was given before reclaiming a row")

		f.eventuallyState(t, subscriptionID, func(s webhookWorkerState) bool {
			return allDelivered(s.rows, 1)
		}, 15*time.Second, 50*time.Millisecond,
			"the reclaimed row must reach 'delivered'")

		row := f.mustSingleDelivery(t, subscriptionID)
		assert.False(t, row.claimedAt.Valid)
		// The sweep is at-least-once by construction, which is exactly why this
		// case has to say once. A reclaim that returned the row while it was still
		// selectable — or one that ran again after the delivery landed — shows up
		// here as a second attempt, and the count below catches the same fault from
		// the subscriber's side.
		assert.Equal(t, 1, row.attempts, "a reclaimed row must be sent once, not once per sweep")
		require.Never(t, func() bool { return receiver.count() > 1 }, 2*time.Second, 100*time.Millisecond,
			"a delivered row must not be swept back out")
	})

	t.Run("a slow batch renews as it goes, so a swept row is delivered by its new owner alone", func(t *testing.T) {
		// The claim is stamped once, for the whole batch, and then the batch is
		// walked one row at a time. RenewClaim is what keeps that safe, and until
		// this case nothing here touched it: the only injected lease was four
		// seconds against receivers answering in microseconds, and the only slow
		// receiver was a tenth of a second, so no delivery ever came close to
		// outliving its claim. The whole renewal block could be deleted from
		// processWorkspaceDeliveries with every other case in this file still
		// green.
		//
		// The shape below is the one it exists for. One worker takes the whole
		// queue in a single statement and then walks it a row at a time, for far
		// longer than the lease, so well before it reaches the tail its claim on
		// those rows has been sitting untouched past expiry. A second worker's
		// sweep takes exactly them, and the two halves of the renewal decide what
		// happens next:
		//
		//   - the re-stamp keeps the row being POSTed right now out of the sweep's
		//     reach, because its claim is always as young as its own request;
		//   - the ownership answer tells the first worker that the rest of its
		//     batch is no longer its own, so it declines those rows instead of
		//     POSTing them alongside the worker that took them.
		//
		// Delete the block and both fail together, in the same direction: rows go
		// out from the worker that lost them and from the worker that took them.
		//
		// The lease sits between one delivery and the batch on purpose. Longer
		// than a single POST, so a row in flight is never swept — a lease under
		// that would double-deliver whatever was on the wire and read as a worker
		// bug rather than as this constant. Far shorter than the batch, so the
		// tail is certain to go stale while the first worker is still busy: that
		// certainty is what keeps this case from passing vacuously on a fast
		// machine, and lostClaims below is what proves it did not.
		const (
			deliveryDelay = 300 * time.Millisecond
			claimLease    = 1200 * time.Millisecond
			queuedEvents  = 10
		)

		receiver := newSlowWebhookReceiver(http.StatusOK, "OK", deliveryDelay)
		defer receiver.Close()

		subscriptionID, _ := f.createSubscription(t, "worker claim renewal", receiver.URL, "")
		defer f.removeSubscription(t, subscriptionID)

		for i := 0; i < queuedEvents; i++ {
			_, err := factory.CreateContact(f.workspaceID)
			require.NoError(t, err)
		}
		require.Len(t, f.mustDeliveries(t, subscriptionID), queuedEvents)

		// The observer only counts an answer the production repository already
		// gives; it delegates every call. Assigned inside the wrap callback, which
		// runs on this goroutine before Start is reached, so the worker never sees
		// a half-built wrapper.
		observer := &webhookRenewalObserverRepo{}
		stopHolder := f.startWorkerWithDeliveryRepo(t,
			func(repo domain.WebhookDeliveryRepository) domain.WebhookDeliveryRepository {
				observer.WebhookDeliveryRepository = repo
				return observer
			},
			service.WithWebhookClaimLease(claimLease))
		defer stopHolder()

		// Started second, and only once the first worker has the batch in hand.
		// Both starting together is a coin toss over who claims what: the claim
		// takes every pending row in one statement, so a split batch would leave
		// nothing for the sweep to find and the case would prove nothing while
		// still passing.
		require.Eventually(t, func() bool { return receiver.count() >= 1 }, 15*time.Second, 25*time.Millisecond,
			"the first worker must be inside its batch before the second one exists")

		stopSweeper := f.startWorker(t, service.WithWebhookClaimLease(claimLease))
		defer stopSweeper()

		f.eventuallyState(t, subscriptionID, func(s webhookWorkerState) bool {
			return allDelivered(s.rows, queuedEvents)
		}, 30*time.Second, 50*time.Millisecond,
			"between them the two workers must deliver every queued row")

		// Eventually stops at the first moment its condition holds, and a batch
		// the first worker is still walking can produce its duplicate a beat
		// later, so the overshoot needs a window of its own.
		require.Never(t, func() bool { return receiver.count() > queuedEvents }, 2*time.Second, 100*time.Millisecond,
			"no row may be POSTed by both the worker that lost it and the worker that took it")

		// Without this the case would be satisfied by a run where the two workers
		// never contended at all — one delivers every row, the other polls an
		// empty queue, and every assertion above passes with the renewal deleted. A
		// renewal that answered "not yours" is the sweep having taken a row out of
		// a batch that was still being walked, which is the whole situation under
		// test.
		assert.Greater(t, observer.lostClaims(), 0,
			"the sweep must have taken rows from the first worker mid-batch, or nothing here is being tested")

		perMessage := map[string]int{}
		for _, sent := range receiver.all() {
			perMessage[sent.headers.Get("webhook-id")]++
		}
		assert.Len(t, perMessage, queuedEvents, "every POST must carry a distinct delivery id")
		for id, count := range perMessage {
			assert.Equal(t, 1, count, "delivery %s was POSTed more than once", id)
		}

		for _, row := range f.mustDeliveries(t, subscriptionID) {
			// A row POSTed by both workers shows up here even if the receiver's
			// counter happened to be read between the two requests.
			assert.Equal(t, 1, row.attempts, "delivery %s was attempted more than once", row.id)
			assert.False(t, row.claimedAt.Valid)
		}
	})

	t.Run("a release cannot resurrect a delivery it no longer holds", func(t *testing.T) {
		// The release is the one branch of the loop nothing outside the process can
		// provoke. It has two callers — deliverOne's recover, and a subscription
		// lookup that failed for some reason other than "not found" — which is a
		// bug in our own code and a database that is unwell. No subscriber and no
		// row can produce either, so every other case in this file runs green
		// without the release executing once, which is precisely how the defect it
		// now guards against got in: the recover releases unconditionally, so a
		// panic AFTER the row had been marked delivered pushed it back to 'pending'
		// with an attempt to spare and a due date already past, and the next poll
		// sent an event that had already arrived.
		//
		// The fault goes in from the test side, through a constructor argument the
		// worker already takes: a delivery repository that marks the row delivered
		// for real and then panics. Everything else — the claim, the renewal, the
		// POST, the recover, the release — is the production path, and the two
		// cases below are the two ways a release can be asked to write to a row its
		// caller does not own.
		for _, tc := range []struct {
			name           string
			dropClaimToken bool
		}{
			{
				name: "a release naming a claim the row has already settled",
			},
			{
				name:           "a release that can name no claim at all",
				dropClaimToken: true,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				receiver := newWebhookReceiver(http.StatusOK, "OK")
				defer receiver.Close()

				subscriptionID, _ := f.createSubscription(t, "worker release "+tc.name, receiver.URL, "")
				defer f.removeSubscription(t, subscriptionID)

				_, err := factory.CreateContact(f.workspaceID)
				require.NoError(t, err)
				require.Len(t, f.mustDeliveries(t, subscriptionID), 1)

				stop := f.startWorkerWithDeliveryRepo(t,
					func(repo domain.WebhookDeliveryRepository) domain.WebhookDeliveryRepository {
						return &webhookPanicAfterDeliveryRepo{
							WebhookDeliveryRepository: repo,
							dropClaimToken:            tc.dropClaimToken,
						}
					})
				defer stop()

				// Gated on the endpoint rather than on the row, and at least rather
				// than exactly, because under a release that does resurrect the row
				// 'delivered' is never observable: the worker writes it and the
				// release takes it back within the same microseconds, so a gate that
				// waited for it would spend its whole budget and then report this
				// defect as "nothing was ever delivered".
				require.Eventually(t, func() bool { return receiver.count() >= 1 }, 15*time.Second, 50*time.Millisecond,
					"the delivery must go out before the release that follows it means anything")

				// The release runs inside the recover, immediately after MarkDelivered
				// committed. This window is both what gives it room to land and what
				// catches it if it landed on the row: a delivered row put back to
				// 'pending' has an attempt to spare and a due date already past, so the
				// very next poll claims it and the subscriber is told twice.
				require.Never(t, func() bool { return receiver.count() > 1 }, 2*time.Second, 100*time.Millisecond,
					"a released delivery must not be POSTed a second time")

				f.eventuallyState(t, subscriptionID, func(s webhookWorkerState) bool {
					return allDelivered(s.rows, 1)
				}, 15*time.Second, 50*time.Millisecond,
					"the row must be left 'delivered': a release that no longer holds it must match no rows")

				// Re-read once the windows above have closed, so what is asserted is
				// the state the row settled in rather than the first moment it looked
				// right. Between them these say the release matched no rows at all:
				// its UPDATE would have moved the status, and its last_error is the
				// one column nothing on the delivered path writes.
				row := f.mustSingleDelivery(t, subscriptionID)
				assert.Equal(t, domain.WebhookDeliveryStatusDelivered, row.status)
				assert.Equal(t, 1, row.attempts, "nothing was sent by the release, so nothing was attempted")
				assert.False(t, row.claimedAt.Valid)
				assert.False(t, row.lastError.Valid,
					"a release that wrote nothing must not leave its reason behind either")
			})
		}
	})

	t.Run("a release that still holds its claim returns the delivery to the queue", func(t *testing.T) {
		// The case above proves what a release must not do, and neither half of it
		// ever made ReleaseClaim move a row. MarkDelivered clears claimed_at in
		// the same statement that marks the row delivered, so by the time the
		// injected panic fires the token the release carries is already gone from
		// the row and its UPDATE matches nothing — which, from outside the
		// process, is indistinguishable from a release that never ran at all.
		// No-op releaseDelivery entirely and both sub-cases above stay green.
		//
		// So the fault goes in one statement earlier here: the panic fires BEFORE
		// MarkDelivered, while the claim is still live and still ours. That is the
		// state the release is actually written for — a bug in us, mid-delivery,
		// with nothing wrong with the delivery itself — and what it owes there is
		// positive. The row goes back to 'pending' carrying the reason, keeps the
		// attempt it never spent, and is claimed and sent again on a later poll.
		//
		// One shot, so the poll after the panic is a healthy one. A panic on every
		// delivery would leave the row cycling and prove only that it never
		// settles.
		receiver := newWebhookReceiver(http.StatusOK, "OK")
		defer receiver.Close()

		subscriptionID, _ := f.createSubscription(t, "worker release returns row", receiver.URL, "")
		defer f.removeSubscription(t, subscriptionID)

		_, err := factory.CreateContact(f.workspaceID)
		require.NoError(t, err)
		queued := f.mustDeliveries(t, subscriptionID)
		require.Len(t, queued, 1)

		// Slower than every other case here, and the released state is why. The
		// row sits in 'pending' only between the poll that released it and the
		// next one, so at the fifty milliseconds the rest of this file runs at
		// there would be nothing to observe: the assertion would be reading a
		// window narrower than the gap between two samples of it.
		const pollInterval = 2 * time.Second

		panicker := &webhookPanicBeforeDeliveryRepo{deliveryID: queued[0].id}
		stop := f.startWorkerWithDeliveryRepo(t,
			func(repo domain.WebhookDeliveryRepository) domain.WebhookDeliveryRepository {
				panicker.WebhookDeliveryRepository = repo
				return panicker
			},
			service.WithWebhookPollInterval(pollInterval))
		defer stop()

		// Budgeted well inside the claim lease the worker derives from its own
		// five-second HTTP client. Past that the reclaim sweep would return this
		// row too, and a case about the release would be passing on the sweep.
		f.eventuallyState(t, subscriptionID, func(s webhookWorkerState) bool {
			return len(s.rows) == 1 &&
				s.rows[0].status == domain.WebhookDeliveryStatusPending &&
				s.rows[0].lastError.Valid
		}, 8*time.Second, 25*time.Millisecond,
			"a panic holding a live claim must put the row back in 'pending' for the next poll")

		released := f.mustSingleDelivery(t, subscriptionID)
		assert.Contains(t, released.lastError.String, "panic while delivering",
			"the reason is what a user debugging their webhook is left with")
		assert.False(t, released.claimedAt.Valid, "a released row still carrying a claim is one nothing can take")
		assert.Equal(t, 0, released.attempts,
			"nothing was sent, so nothing was attempted: spending an attempt on our own bug is how a transient one loses a delivery")
		assert.False(t, released.deliveredAt.Valid)

		f.eventuallyState(t, subscriptionID, func(s webhookWorkerState) bool {
			return allDelivered(s.rows, 1)
		}, 15*time.Second, 50*time.Millisecond,
			"the released row must be claimed and sent again, not left in the queue")

		assert.True(t, panicker.fired(), "the injected panic never ran, so nothing was released")

		row := f.mustSingleDelivery(t, subscriptionID)
		assert.Equal(t, int64(http.StatusOK), row.lastResponseStatus.Int64)
		assert.True(t, row.deliveredAt.Valid)
		assert.False(t, row.claimedAt.Valid)
		// Two POSTs, one attempt. The first one really did reach the subscriber —
		// the panic is on our side of the response — and the release deliberately
		// leaves attempts alone, so the ladder keeps every rung it has for failures
		// the endpoint is actually responsible for.
		assert.Equal(t, 2, receiver.count(), "the endpoint saw the delivery that panicked and the one that settled")
		assert.Equal(t, 1, row.attempts, "the release must not spend one of the delivery's attempts")
	})

	t.Run("an endpoint answering 410 Gone retires its subscription", func(t *testing.T) {
		// 410 is the one response that ends a subscription rather than a delivery,
		// and what "ends" means depends on who created it. A Zap re-creates its
		// subscription through performSubscribe when it is switched back on, so
		// deleting the row loses nothing and spares the user a webhook they never
		// made sitting in their settings. One a person typed in by hand is theirs:
		// disabling is reversible and the reason says what happened.
		for _, tc := range []struct {
			name          string
			source        string
			expectDeleted bool
		}{
			{
				name:          "a Zapier subscription is deleted with its queue",
				source:        domain.WebhookSubscriptionSourceZapier,
				expectDeleted: true,
			},
			{
				name:          "a user subscription is disabled with a reason",
				source:        domain.WebhookSubscriptionSourceUser,
				expectDeleted: false,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if tc.expectDeleted {
					// Two mechanisms can empty this queue and only one of them is
					// the worker's. Every fresh workspace gets ON DELETE CASCADE on
					// webhook_deliveries (internal/database/init.go), so deleting
					// the subscription takes its rows whether or not the worker asks
					// — delete handleGoneEndpoint's DeleteBySubscriptionID call
					// outright and an assertion made against a fresh workspace still
					// holds. That call's own comment says what it is for: workspaces
					// created before v39 added the constraint. This is that
					// configuration, and it is the only one where the assertion below
					// is about the worker at all.
					defer f.withoutDeliveryCascade(t)()
				}

				receiver := newWebhookReceiver(http.StatusGone, "gone")
				defer receiver.Close()

				subscriptionID, _ := f.createSubscription(t, "worker gone "+tc.name, receiver.URL, tc.source)
				defer f.removeSubscription(t, subscriptionID)

				_, err := factory.CreateContact(f.workspaceID)
				require.NoError(t, err)
				require.Len(t, f.mustDeliveries(t, subscriptionID), 1)

				stop := f.startWorker(t)
				defer stop()

				require.Eventually(t, func() bool { return receiver.count() >= 1 }, 15*time.Second, 50*time.Millisecond,
					"the worker must POST once before it can be told the endpoint is gone")

				if tc.expectDeleted {
					f.eventuallyState(t, subscriptionID, func(s webhookWorkerState) bool {
						return !s.sub.exists
					}, 15*time.Second, 50*time.Millisecond,
						"a Zapier subscription reported gone must be deleted, not merely disabled")

					// The queue goes with it. Left behind, its rows would match the
					// claim predicate for the whole retention window and take a slot
					// in every batch while pointing at an endpoint that has said it is
					// finished.
					//
					// Its own wait rather than a read after the one above: the
					// subscription and its queue are two un-transacted writes, in
					// that order, so a read that followed the first alone would land
					// in the window before the second.
					f.eventuallyState(t, subscriptionID, func(s webhookWorkerState) bool {
						return len(s.rows) == 0
					}, 15*time.Second, 50*time.Millisecond,
						"deleting the subscription must take its queued deliveries with it")
					return
				}

				// The subscription and its queue are two un-transacted writes, in that
				// order, so waiting on the subscription alone and then reading the row
				// lands inside the window where the row is still 'delivering' with no
				// attempts on it. Both halves belong in the same gate — the state this
				// case is about is the pair, not either one.
				f.eventuallyState(t, subscriptionID, func(s webhookWorkerState) bool {
					return s.sub.exists && !s.sub.enabled &&
						len(s.rows) == 1 &&
						s.rows[0].status == domain.WebhookDeliveryStatusFailed
				}, 15*time.Second, 50*time.Millisecond,
					"a user subscription reported gone must be disabled and its delivery drained")

				sub := f.mustSubscription(t, subscriptionID)
				assert.True(t, sub.disabledReason.Valid,
					"a user who finds a switched-off webhook has to be able to tell it from one they switched off themselves")
				assert.Contains(t, sub.disabledReason.String, "410")

				// The delivery is drained rather than retried: pinned at its own
				// ceiling, which is what puts it outside the claim predicate for good.
				row := f.mustSingleDelivery(t, subscriptionID)
				assert.Equal(t, domain.WebhookDeliveryStatusFailed, row.status)
				assert.Equal(t, row.maxAttempts, row.attempts,
					"a drained row must sit at its ceiling or the next poll claims it again")
				assert.Equal(t, int64(http.StatusGone), row.lastResponseStatus.Int64)
				assert.False(t, row.claimedAt.Valid)
			})
		}
	})

	t.Run("an endpoint answering 429 schedules a retry without counting a failure", func(t *testing.T) {
		receiver := newWebhookReceiver(http.StatusTooManyRequests, "slow down")
		defer receiver.Close()

		subscriptionID, _ := f.createSubscription(t, "worker throttled", receiver.URL, "")
		defer f.removeSubscription(t, subscriptionID)

		_, err := factory.CreateContact(f.workspaceID)
		require.NoError(t, err)
		require.Len(t, f.mustDeliveries(t, subscriptionID), 1)

		stop := f.startWorker(t)
		defer stop()

		f.eventuallyState(t, subscriptionID, func(s webhookWorkerState) bool {
			return len(s.rows) == 1 && s.rows[0].attempts == 1
		}, 15*time.Second, 50*time.Millisecond,
			"the throttled delivery must be recorded as attempted")

		row := f.mustSingleDelivery(t, subscriptionID)
		assert.Equal(t, domain.WebhookDeliveryStatusFailed, row.status)
		assert.Less(t, row.attempts, row.maxAttempts, "429 must leave the delivery retryable")
		assert.True(t, row.nextAttemptAt.After(time.Now()),
			"the retry must be scheduled into the future, not left ready to claim")
		assert.Equal(t, int64(http.StatusTooManyRequests), row.lastResponseStatus.Int64)

		// The scheduled retry is what keeps the receiver from being hammered by the
		// very worker it asked to slow down.
		require.Never(t, func() bool { return receiver.count() > 1 }, 2*time.Second, 100*time.Millisecond,
			"a throttled delivery must not be re-POSTed before its next attempt is due")

		// The whole point of the branch: rate limiting is a receiver asking for
		// less, not an endpoint dying. Counting it would let a workspace busy
		// enough to be throttled have its integration switched off for being busy.
		sub := f.mustSubscription(t, subscriptionID)
		assert.True(t, sub.enabled)
		assert.Equal(t, 0, sub.consecutiveFailures, "429 must not count against the auto-disable threshold")
		assert.False(t, sub.failingSince.Valid, "429 must not open a run of failures")
	})

	t.Run("a disabled subscription drains its queue instead of holding it", func(t *testing.T) {
		receiver := newWebhookReceiver(http.StatusOK, "OK")
		defer receiver.Close()

		subscriptionID, _ := f.createSubscription(t, "worker disabled drain", receiver.URL, "")
		defer f.removeSubscription(t, subscriptionID)

		// Enqueue first, then disable: the trigger only fans out to enabled
		// subscriptions, so this is the only order that produces the state the
		// worker has to survive — a queue whose subscription has since been
		// switched off, by a user here and by the auto-disable in production.
		_, err := factory.CreateContact(f.workspaceID)
		require.NoError(t, err)
		require.Len(t, f.mustDeliveries(t, subscriptionID), 1)
		f.disableSubscription(t, subscriptionID, receiver.URL)

		stop := f.startWorker(t)
		defer stop()

		f.eventuallyState(t, subscriptionID, func(s webhookWorkerState) bool {
			return len(s.rows) == 1 && s.rows[0].status == domain.WebhookDeliveryStatusFailed
		}, 15*time.Second, 50*time.Millisecond,
			"a delivery whose subscription is disabled must be moved to a terminal state, not skipped")

		row := f.mustSingleDelivery(t, subscriptionID)
		// The ceiling is what makes it terminal. Marked failed below it, the row
		// still matches `status IN ('pending','failed') AND attempts < max_attempts`
		// and is claimed again on the very next poll — a slot in every batch, for
		// the whole retention window, for a subscription that is switched off.
		assert.Equal(t, row.maxAttempts, row.attempts)
		assert.False(t, row.claimedAt.Valid, "an untouched skip would have left the claim standing")
		require.True(t, row.lastError.Valid)
		assert.Contains(t, row.lastError.String, "disabled")

		assert.Equal(t, 0, receiver.count(), "nothing may be POSTed for a disabled subscription")

		// Terminal means terminal: later polls must walk past it.
		f.neverState(t, subscriptionID, func(s webhookWorkerState) bool {
			return len(s.rows) == 1 && s.rows[0].status != domain.WebhookDeliveryStatusFailed
		}, 2*time.Second, 100*time.Millisecond,
			"the drained row must not be claimed again")
	})

	t.Run("a single 404 is counted, retried, and delivered once the endpoint returns", func(t *testing.T) {
		receiver := newWebhookReceiver(http.StatusNotFound, "no such hook")
		defer receiver.Close()

		subscriptionID, _ := f.createSubscription(t, "worker missing endpoint", receiver.URL, "")
		defer f.removeSubscription(t, subscriptionID)

		_, err := factory.CreateContact(f.workspaceID)
		require.NoError(t, err)
		require.Len(t, f.mustDeliveries(t, subscriptionID), 1)

		stop := f.startWorker(t)
		defer stop()

		f.eventuallyState(t, subscriptionID, func(s webhookWorkerState) bool {
			return len(s.rows) == 1 && s.rows[0].attempts == 1
		}, 15*time.Second, 50*time.Millisecond,
			"the 404 must be recorded as an attempt on the row")

		row := f.mustSingleDelivery(t, subscriptionID)
		assert.Equal(t, domain.WebhookDeliveryStatusFailed, row.status)
		assert.Less(t, row.attempts, row.maxAttempts, "one 404 must leave the delivery retryable")
		assert.True(t, row.nextAttemptAt.After(time.Now()))
		assert.Equal(t, int64(http.StatusNotFound), row.lastResponseStatus.Int64)
		// The counted attempt is read off the row above; this reads it off the
		// endpoint, so a case that counts an attempt nothing sent cannot pass.
		assert.Equal(t, 1, receiver.count(), "one attempt is one request")

		// Which rung, not merely "some time in the future". The line above passes
		// for every delay the ladder holds and for any bug that picks the wrong
		// one: the backoff is indexed at attempts-1, so an off-by-one there hands
		// the first failure the rung above the one it should get, and nothing that
		// only looks for a future timestamp can tell.
		//
		// Measured between the row's own two columns rather than against this
		// test's clock. ScheduleRetry writes last_attempt_at and next_attempt_at
		// in one statement, so their difference is the delay the worker chose and
		// carries nothing of how long the poll took to reach this assertion.
		require.True(t, row.lastAttemptAt.Valid, "a counted attempt must be stamped")
		assert.InDelta(t, (30 * time.Second).Seconds(),
			row.nextAttemptAt.Sub(row.lastAttemptAt.Time).Seconds(), 2,
			"one failure must schedule the ladder's first rung")

		// A Catch Hook that is turned off answers 404 and answers 200 again the
		// moment it is turned back on, which is why the REST Hooks specification
		// asks for a consistent 404 over time before an endpoint is written off.
		// One is not a verdict — it is counted, and that is all.
		sub := f.mustSubscription(t, subscriptionID)
		assert.True(t, sub.enabled, "a single 404 must never retire a subscription")
		assert.False(t, sub.disabledReason.Valid)
		assert.Equal(t, 1, sub.consecutiveFailures)
		assert.True(t, sub.failingSince.Valid, "the failure must open a run, which is what the window measures")

		// The ladder's second rung. Every other retry case stops at "a retry is
		// scheduled", which leaves the attempt-indexed backoff and the
		// give-up-at-max_attempts branch proven against mocks alone. Pulling the
		// schedule forward — now that the rung the worker chose has been read off
		// the row above — is a scheduling column, not a hand-enqueued row: the
		// delivery is still the trigger's, and the worker still has to claim it,
		// send it, and count the attempt itself.
		receiver.answerWith(http.StatusOK, "OK")
		_, err = db.Exec(`UPDATE webhook_deliveries SET next_attempt_at = NOW() WHERE id = $1`, row.id)
		require.NoError(t, err)

		f.eventuallyState(t, subscriptionID, func(s webhookWorkerState) bool {
			return allDelivered(s.rows, 1)
		}, 15*time.Second, 50*time.Millisecond,
			"an endpoint that comes back must have its scheduled retry delivered")

		retried := f.mustSingleDelivery(t, subscriptionID)
		assert.Equal(t, 2, retried.attempts, "the retry is this delivery's second attempt, not a fresh first one")
		assert.Equal(t, int64(http.StatusOK), retried.lastResponseStatus.Int64)
		assert.Equal(t, 2, receiver.count(), "the retry must reach the endpoint exactly once more")

		// Safe to read without a wait of its own: the counter is cleared before the
		// row is marked delivered, so the gate above already passed through it. A
		// run that survived a successful delivery would be counting lifetime
		// failures, and the threshold would eventually retire an endpoint that
		// works.
		recovered := f.mustSubscription(t, subscriptionID)
		assert.Equal(t, 0, recovered.consecutiveFailures, "a delivery that gets through ends the run")
		assert.False(t, recovered.failingSince.Valid, "the window must close with the run it measured")
		assert.True(t, recovered.enabled)
	})

	t.Run("an endpoint that refuses the connection fails the delivery with no status at all", func(t *testing.T) {
		// Every other case here points the worker at a live httptest server, so
		// the one failure a receiver cannot describe — the request that never got
		// an answer — had no case at all. Gut the transport branch in
		// processDelivery to a bare return and the whole file stayed green, while
		// in production that row would keep its claim until the lease expired and
		// come back around the sweep on every poll for the rest of the retention
		// window.
		//
		// It is also the commonest way a webhook fails in the field: a receiver
		// torn down, a container between restarts, a port that moved. Closing the
		// endpoint before the worker starts is what all three look like from here.
		receiver := newWebhookReceiver(http.StatusOK, "OK")
		endpoint := receiver.URL

		subscriptionID, _ := f.createSubscription(t, "worker refused connection", endpoint, "")
		defer f.removeSubscription(t, subscriptionID)

		_, err := factory.CreateContact(f.workspaceID)
		require.NoError(t, err)
		require.Len(t, f.mustDeliveries(t, subscriptionID), 1)

		// Closed here rather than deferred: the subscription has to be created
		// against a URL that validates, and the endpoint has to be gone before the
		// worker's first poll. Closing twice would be harmless, but there is
		// nothing left to close.
		receiver.Close()

		stop := f.startWorker(t)
		defer stop()

		f.eventuallyState(t, subscriptionID, func(s webhookWorkerState) bool {
			return len(s.rows) == 1 && s.rows[0].attempts == 1
		}, 15*time.Second, 50*time.Millisecond,
			"a request that never reached anyone is still an attempt, and a row that records none is a row nothing writes to")

		row := f.mustSingleDelivery(t, subscriptionID)
		assert.Equal(t, domain.WebhookDeliveryStatusFailed, row.status)
		assert.Less(t, row.attempts, row.maxAttempts,
			"a refused connection is a receiver that is down, not one that is finished: the delivery stays retryable")
		assert.True(t, row.nextAttemptAt.After(time.Now()))
		assert.False(t, row.claimedAt.Valid, "an unhandled transport error would have left the claim standing")

		// The absent status is the point. Nothing answered, so there is nothing to
		// record, and a zero written here would show up in the delivery log as a
		// response the endpoint never sent.
		assert.False(t, row.lastResponseStatus.Valid, "there is no HTTP status when there was no HTTP response")
		require.True(t, row.lastError.Valid)
		assert.Contains(t, row.lastError.String, `Post "`+endpoint,
			"the transport error names the endpoint the POST never reached")
		assert.NotContains(t, row.lastError.String, "HTTP ",
			"an HTTP nnn here would mean the status branch ran on a request that got no response")

		// Counted like any other failure: this is precisely the endpoint the
		// auto-disable exists to retire, and a run that never opens is a run that
		// never reaches the threshold.
		sub := f.mustSubscription(t, subscriptionID)
		assert.True(t, sub.enabled, "one refused connection is a restart, not a verdict")
		assert.Equal(t, 1, sub.consecutiveFailures)
		assert.True(t, sub.failingSince.Valid, "the failure must open a run, which is what the window measures")
	})

	t.Run("a panic in the reclaim sweep costs the poll and not the worker", func(t *testing.T) {
		// processDeliveriesGuarded's own comment says a panic escaping it "takes
		// the whole server down with it" — Start runs on a bare goroutine, with
		// every in-flight request and every other worker in the same process.
		// Nothing here had ever made one escape. Both panic harnesses above go in
		// at MarkDelivered, which deliverOne's own recover catches one frame
		// lower, so the outer guard could be deleted outright with every case in
		// this file still green while the batch loop, the sweep, the drain and the
		// retention cleanup all ran naked beneath it.
		//
		// The sweep is the right place to put the fault. It runs first, on every
		// poll, before anything has been claimed — so the poll dies with the queue
		// untouched, which is the "a poll is the right unit to lose" claim the
		// guard is written around. One shot, so the poll after it is a healthy
		// one, and the delivery that poll makes is the proof the goroutine
		// outlived the panic rather than the process having been lucky.
		receiver := newWebhookReceiver(http.StatusOK, "OK")
		defer receiver.Close()

		subscriptionID, _ := f.createSubscription(t, "worker guarded poll", receiver.URL, "")
		defer f.removeSubscription(t, subscriptionID)

		_, err := factory.CreateContact(f.workspaceID)
		require.NoError(t, err)
		require.Len(t, f.mustDeliveries(t, subscriptionID), 1)

		panicker := &webhookPanicOnceInSweepRepo{}
		stop := f.startWorkerWithDeliveryRepo(t,
			func(repo domain.WebhookDeliveryRepository) domain.WebhookDeliveryRepository {
				panicker.WebhookDeliveryRepository = repo
				return panicker
			})
		defer stop()

		f.eventuallyState(t, subscriptionID, func(s webhookWorkerState) bool {
			return allDelivered(s.rows, 1)
		}, 15*time.Second, 50*time.Millisecond,
			"the poll after the panic must run, claim the row and deliver it")

		// Without this the case passes on a worker that never panicked at all,
		// which is the same green a missing guard gives.
		assert.True(t, panicker.fired(), "the injected panic never ran, so the guard was never asked to catch anything")

		row := f.mustSingleDelivery(t, subscriptionID)
		assert.Equal(t, 1, row.attempts, "the lost poll claimed nothing, so it cost the row no attempt")
		assert.False(t, row.claimedAt.Valid)
		assert.Equal(t, 1, receiver.count(),
			"the lost poll put nothing on the wire, and the poll that recovered sent the row once")
	})
}

// webhookWorkerFixture holds the one workspace every case above shares, plus the
// pieces needed to build a production worker against it.
//
// One workspace rather than one per case: the worker polls every workspace the
// system database lists, so a fresh workspace would isolate nothing anyway.
// What isolates the cases is that each owns its own subscription and removes it
// when it is done — which, through the delivery table's cascade, takes that
// case's queue with it and leaves nothing for the next case's worker to claim.
type webhookWorkerFixture struct {
	app         testutil.AppInterface
	client      *testutil.APIClient
	workspaceID string
	db          *sql.DB

	// Set when a worker outlived its stop. Read by every later startWorker,
	// because from that point on no case in this file is testing what it says
	// it is.
	leakedMu sync.Mutex
	leaked   bool
}

func (f *webhookWorkerFixture) markWorkerLeaked() {
	f.leakedMu.Lock()
	defer f.leakedMu.Unlock()
	f.leaked = true
}

// requireNoLeakedWorker stops a case before it starts if a worker from an
// earlier one is still polling. Nothing this case is about to observe would be
// its own: claims are taken per workspace and every case here shares one, so
// that worker will take rows out from under this one. Said here, once, rather
// than left to surface as whatever it happens to break.
func (f *webhookWorkerFixture) requireNoLeakedWorker(t *testing.T) {
	t.Helper()
	f.leakedMu.Lock()
	defer f.leakedMu.Unlock()
	if f.leaked {
		t.Fatal("an earlier case's webhook delivery worker never stopped; it is still claiming this workspace's rows")
	}
}

// startWorker builds a production worker over the real repositories and runs it
// through Start, returning a function that stops it and waits for the loop to
// leave.
//
// Waiting matters: Start returns only between polls, and a worker still running
// when the next case begins would claim that case's rows out from under the
// worker that case starts. The returned function is safe to call twice, so a
// case can stop the worker early and still defer the stop.
func (f *webhookWorkerFixture) startWorker(t *testing.T, opts ...service.WebhookDeliveryWorkerOption) func() {
	t.Helper()
	return f.startWorkerWithDeliveryRepo(t, nil, opts...)
}

// startWorkerWithDeliveryRepo is startWorker with the delivery repository run
// through wrap first, so a case can put a fault in the middle of the loop
// without a production seam for it.
//
// Only the release cases need it, and only because releaseDelivery is reachable
// from nowhere outside the process: its callers are deliverOne's recover and a
// subscription lookup that failed for a reason other than "not found", and
// neither can be provoked by anything a subscriber or a database row does. The
// repositories are already constructor arguments, so the fault goes in from the
// test side and the worker under test stays the production one.
func (f *webhookWorkerFixture) startWorkerWithDeliveryRepo(
	t *testing.T,
	wrap func(domain.WebhookDeliveryRepository) domain.WebhookDeliveryRepository,
	opts ...service.WebhookDeliveryWorkerOption,
) func() {
	t.Helper()

	f.requireNoLeakedWorker(t)

	workspaceRepo := f.app.GetWorkspaceRepository()
	log := f.app.GetLogger()

	var deliveryRepo domain.WebhookDeliveryRepository = repository.NewWebhookDeliveryRepository(workspaceRepo, log)
	if wrap != nil {
		deliveryRepo = wrap(deliveryRepo)
	}

	// The poll interval is the only thing every case overrides. Ten seconds is
	// right in production and would make each case below a ten-second wait for a
	// single tick; the loop it drives is the same loop either way.
	options := append([]service.WebhookDeliveryWorkerOption{
		service.WithWebhookPollInterval(50 * time.Millisecond),
	}, opts...)

	worker := service.NewWebhookDeliveryWorker(
		repository.NewWebhookSubscriptionRepository(workspaceRepo),
		deliveryRepo,
		workspaceRepo,
		log,
		// Short enough that a hung receiver fails the case rather than the suite,
		// long enough to leave the derived claim lease well clear of it.
		&http.Client{Timeout: 5 * time.Second},
		options...,
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Start(ctx)
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			select {
			case <-done:
			case <-time.After(30 * time.Second):
				// Fatal, and recorded on the fixture, because the damage is not
				// this case's. Claims are taken per workspace and every case
				// here shares one, so a worker still polling is a worker that
				// will claim the next case's rows and deliver them to a
				// receiver that case never started. Left as a plain error, the
				// first thing anyone sees is an unrelated case failing several
				// cases later.
				f.markWorkerLeaked()
				t.Fatal("the webhook delivery worker did not stop when its context was cancelled")
			}
		})
	}
}

// webhookPanicAfterDeliveryRepo is the real delivery repository with one fault
// added: MarkDelivered commits and then panics.
//
// It reproduces the exact sequence the claim token exists for. The row is
// genuinely delivered — status, attempts, delivered_at, claim released — and only
// then does the worker's recover reach for releaseDelivery, holding a claim that
// the delivery has already settled. A release that matched on the id alone would
// take that finished row back to 'pending' and have it sent again.
//
// dropClaimToken covers the other half of the same guard: a caller that arrives
// at the release with nothing to name its claim by. Renewal is what hands the
// worker its token, so dropping it there is where that state comes from. Such a
// row belongs to ReclaimStale, which reads a claimless 'delivering' row as
// infinitely stale — not to an UPDATE with no predicate but the id, which would
// land on whatever the row had become in the meantime.
type webhookPanicAfterDeliveryRepo struct {
	domain.WebhookDeliveryRepository
	dropClaimToken bool
}

func (r *webhookPanicAfterDeliveryRepo) RenewClaim(ctx context.Context, workspaceID, id string, claimedAt *time.Time) (bool, *time.Time, error) {
	owned, renewed, err := r.WebhookDeliveryRepository.RenewClaim(ctx, workspaceID, id, claimedAt)
	if r.dropClaimToken {
		return owned, nil, err
	}
	return owned, renewed, err
}

func (r *webhookPanicAfterDeliveryRepo) MarkDelivered(ctx context.Context, workspaceID, id string, responseStatus int, responseBody string) error {
	if err := r.WebhookDeliveryRepository.MarkDelivered(ctx, workspaceID, id, responseStatus, responseBody); err != nil {
		return err
	}
	panic("injected panic after the delivery was marked delivered")
}

// webhookPanicBeforeDeliveryRepo is the real delivery repository with one fault
// added: the first MarkDelivered for one named row panics INSTEAD of running.
//
// It is webhookPanicAfterDeliveryRepo's opposite, and the difference is one
// statement wide. There, the row is already delivered and its claim already
// cleared when the recover reaches releaseDelivery, so the release names a claim
// the row no longer carries and must match nothing. Here the row is untouched
// and the claim is still live and still the worker's, which is the only state in
// which a release is supposed to move anything.
//
// Named rather than firing on whatever arrives first, and one shot rather than
// on every call: the row has to be released once and then delivered for real, so
// the fault has to end after the delivery it is aimed at.
type webhookPanicBeforeDeliveryRepo struct {
	domain.WebhookDeliveryRepository

	deliveryID string

	mu      sync.Mutex
	didFire bool
}

func (r *webhookPanicBeforeDeliveryRepo) MarkDelivered(ctx context.Context, workspaceID, id string, responseStatus int, responseBody string) error {
	r.mu.Lock()
	first := id == r.deliveryID && !r.didFire
	if first {
		r.didFire = true
	}
	r.mu.Unlock()

	if first {
		panic("injected panic before the delivery was marked delivered")
	}
	return r.WebhookDeliveryRepository.MarkDelivered(ctx, workspaceID, id, responseStatus, responseBody)
}

func (r *webhookPanicBeforeDeliveryRepo) fired() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.didFire
}

// webhookPanicOnceInSweepRepo panics the first time the reclaim sweep runs, and
// delegates every call after that.
//
// The sweep is the first thing a poll does to a workspace's queue, and it runs
// outside deliverOne's recover, so this is a panic that reaches
// processDeliveriesGuarded rather than one the per-delivery guard swallows — the
// only kind that can take the process down.
type webhookPanicOnceInSweepRepo struct {
	domain.WebhookDeliveryRepository

	mu      sync.Mutex
	didFire bool
}

func (r *webhookPanicOnceInSweepRepo) ReclaimStale(ctx context.Context, workspaceID string, lease time.Duration) (int64, error) {
	r.mu.Lock()
	first := !r.didFire
	r.didFire = true
	r.mu.Unlock()

	if first {
		panic("injected panic in the webhook reclaim sweep")
	}
	return r.WebhookDeliveryRepository.ReclaimStale(ctx, workspaceID, lease)
}

func (r *webhookPanicOnceInSweepRepo) fired() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.didFire
}

// webhookRenewalObserverRepo is the real delivery repository with a counter on
// the one answer the renewal case is about: a claim that is no longer ours.
//
// It changes nothing — every call is delegated and every answer returned
// unaltered. It exists because "each row was POSTed once" is equally true of a
// run where the two workers never contended at all, and a case that cannot tell
// those apart proves nothing while passing.
type webhookRenewalObserverRepo struct {
	domain.WebhookDeliveryRepository

	mu   sync.Mutex
	lost int
}

func (r *webhookRenewalObserverRepo) RenewClaim(ctx context.Context, workspaceID, id string, claimedAt *time.Time) (bool, *time.Time, error) {
	owned, renewed, err := r.WebhookDeliveryRepository.RenewClaim(ctx, workspaceID, id, claimedAt)
	// Only a definite "not ours" counts. An error means we could not tell, and
	// the worker treats it as a different case entirely.
	if err == nil && !owned {
		r.mu.Lock()
		r.lost++
		r.mu.Unlock()
	}
	return owned, renewed, err
}

func (r *webhookRenewalObserverRepo) lostClaims() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lost
}

// createSubscription creates a subscription through the public API, so the
// secret the receiver's signature is checked against is a real generated one.
// source is WebhookSubscriptionSourceUser for a user-created subscription, which
// is the empty string and therefore also the field's default.
func (f *webhookWorkerFixture) createSubscription(t *testing.T, name, url, source string) (id, secret string) {
	t.Helper()

	body := map[string]interface{}{
		"workspace_id": f.workspaceID,
		"name":         name,
		"url":          url,
		"event_types":  []string{"contact.created"},
	}
	if source != domain.WebhookSubscriptionSourceUser {
		body["source"] = source
	}

	resp, err := f.client.Post("/api/webhookSubscriptions.create", body)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var decoded struct {
		Subscription struct {
			ID     string `json:"id"`
			Secret string `json:"secret"`
		} `json:"subscription"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&decoded))
	require.NotEmpty(t, decoded.Subscription.ID)
	if source == domain.WebhookSubscriptionSourceUser {
		// Returned once, to the person who asked for it. An integration's
		// subscription is created with a real secret too, but the response blanks
		// it — there is nobody on that call to copy it down, and the body would be
		// logged by the platform that made the call.
		require.NotEmpty(t, decoded.Subscription.Secret)
	}

	return decoded.Subscription.ID, decoded.Subscription.Secret
}

// disableSubscription switches a subscription off through the update endpoint,
// which patches the switch rather than replacing it: enabled is named explicitly
// here because a body that leaves it out now keeps the stored value. This is not
// the console's route — the console moves the switch from the card, through
// .toggle — but it is a route an API client has.
func (f *webhookWorkerFixture) disableSubscription(t *testing.T, id, url string) {
	t.Helper()

	resp, err := f.client.Post("/api/webhookSubscriptions.update", map[string]interface{}{
		"workspace_id": f.workspaceID,
		"id":           id,
		"name":         "disabled by test",
		"url":          url,
		"event_types":  []string{"contact.created"},
		"enabled":      false,
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// removeSubscription is cleanup, not behaviour under test, so it goes straight
// at the table: the 410 case has already deleted its subscription through the
// worker, and a DELETE that matches nothing is the right answer there.
func (f *webhookWorkerFixture) removeSubscription(t *testing.T, id string) {
	t.Helper()
	_, err := f.db.Exec(`DELETE FROM webhook_subscriptions WHERE id = $1`, id)
	require.NoError(t, err)
}

// withoutDeliveryCascade drops the delivery table's foreign key for the
// duration of one case and returns the function that puts it back, so a case
// can run against the shape webhook_deliveries had before v39 added the
// constraint.
func (f *webhookWorkerFixture) withoutDeliveryCascade(t *testing.T) func() {
	t.Helper()
	_, err := f.db.Exec(`ALTER TABLE webhook_deliveries DROP CONSTRAINT webhook_deliveries_subscription_id_fkey`)
	require.NoError(t, err, "the cascade this case runs without has to be there to drop")

	return func() {
		// Rows orphaned by a case that failed would make the constraint
		// unaddable. Clearing them is cleanup, after the assertion about them
		// has had its say — not a second chance for it to pass.
		_, err := f.db.Exec(`DELETE FROM webhook_deliveries d
			WHERE NOT EXISTS (SELECT 1 FROM webhook_subscriptions s WHERE s.id = d.subscription_id)`)
		require.NoError(t, err)

		_, err = f.db.Exec(`ALTER TABLE webhook_deliveries
			ADD CONSTRAINT webhook_deliveries_subscription_id_fkey
			FOREIGN KEY (subscription_id) REFERENCES webhook_subscriptions(id) ON DELETE CASCADE`)
		require.NoError(t, err, "every case after this one leans on the cascade to clear its queue")
	}
}

// webhookDeliveryRow is one row of webhook_deliveries, read straight from the
// workspace database rather than through the repository the worker writes with.
type webhookDeliveryRow struct {
	id                 string
	status             string
	attempts           int
	maxAttempts        int
	nextAttemptAt      time.Time
	lastAttemptAt      sql.NullTime
	deliveredAt        sql.NullTime
	claimedAt          sql.NullTime
	lastResponseStatus sql.NullInt64
	lastError          sql.NullString
}

// deliveries returns a subscription's rows, error and all, for use inside an
// Eventually condition — testify runs those on their own goroutine, where a
// require would call FailNow on the wrong goroutine and hang the run.
//
// status, attempts, max_attempts and next_attempt_at are declared with defaults
// but not NOT NULL (internal/database/init.go), so they are read as nullable
// and rejected here by name. Scanned straight into a string or an int, a NULL
// in any of them fails the scan instead, and every caller polls this inside a
// wait that can read an error only as "not yet" — so an empty column would
// arrive as a timeout naming the worker.
func (f *webhookWorkerFixture) deliveries(subscriptionID string) ([]webhookDeliveryRow, error) {
	rows, err := f.db.Query(`
		SELECT id, status, attempts, max_attempts, next_attempt_at, last_attempt_at,
			delivered_at, claimed_at, last_response_status, last_error
		FROM webhook_deliveries
		WHERE subscription_id = $1
		ORDER BY created_at`, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("querying webhook_deliveries: %w", err)
	}
	defer rows.Close()

	var out []webhookDeliveryRow
	for rows.Next() {
		var (
			row           webhookDeliveryRow
			status        sql.NullString
			attempts      sql.NullInt64
			maxAttempts   sql.NullInt64
			nextAttemptAt sql.NullTime
		)
		if err := rows.Scan(&row.id, &status, &attempts, &maxAttempts, &nextAttemptAt,
			&row.lastAttemptAt, &row.deliveredAt, &row.claimedAt, &row.lastResponseStatus, &row.lastError); err != nil {
			return nil, fmt.Errorf("scanning webhook_deliveries: %w", err)
		}
		switch {
		case !status.Valid:
			return nil, fmt.Errorf("delivery %s has a NULL status", row.id)
		case !attempts.Valid:
			return nil, fmt.Errorf("delivery %s has a NULL attempts", row.id)
		case !maxAttempts.Valid:
			return nil, fmt.Errorf("delivery %s has a NULL max_attempts", row.id)
		case !nextAttemptAt.Valid:
			return nil, fmt.Errorf("delivery %s has a NULL next_attempt_at", row.id)
		}
		row.status = status.String
		row.attempts = int(attempts.Int64)
		row.maxAttempts = int(maxAttempts.Int64)
		row.nextAttemptAt = nextAttemptAt.Time
		out = append(out, row)
	}
	return out, rows.Err()
}

func (f *webhookWorkerFixture) mustDeliveries(t *testing.T, subscriptionID string) []webhookDeliveryRow {
	t.Helper()
	rows, err := f.deliveries(subscriptionID)
	require.NoError(t, err)
	return rows
}

func (f *webhookWorkerFixture) mustSingleDelivery(t *testing.T, subscriptionID string) webhookDeliveryRow {
	t.Helper()
	rows := f.mustDeliveries(t, subscriptionID)
	require.Len(t, rows, 1)
	return rows[0]
}

// webhookWorkerState is one sample of everything the waits below watch: a
// subscription and its queue, read together because most of what the worker
// does lands in both.
type webhookWorkerState struct {
	sub  webhookSubscriptionRow
	rows []webhookDeliveryRow
}

func (f *webhookWorkerFixture) state(subscriptionID string) (webhookWorkerState, error) {
	sub, err := f.subscription(subscriptionID)
	if err != nil {
		return webhookWorkerState{}, err
	}
	rows, err := f.deliveries(subscriptionID)
	if err != nil {
		return webhookWorkerState{}, err
	}
	return webhookWorkerState{sub: sub, rows: rows}, nil
}

// eventuallyState polls a subscription and its queue until cond holds.
//
// A helper rather than require.Eventually at each site because an Eventually
// condition can answer only true or false, and both reads can fail. Folded into
// a false, a broken query or a column this file cannot scan reads as "not yet"
// and arrives fifteen seconds later as the worker not having done its job —
// the one explanation the evidence rules out. Here the read error is kept and
// reported as itself.
func (f *webhookWorkerFixture) eventuallyState(
	t *testing.T,
	subscriptionID string,
	cond func(webhookWorkerState) bool,
	wait, tick time.Duration,
	msg string,
) {
	t.Helper()

	// Written on testify's condition goroutine and read on this one.
	var (
		mu      sync.Mutex
		lastErr error
	)
	held := assert.Eventually(t, func() bool {
		state, err := f.state(subscriptionID)
		if err != nil {
			mu.Lock()
			lastErr = err
			mu.Unlock()
			return false
		}
		return cond(state)
	}, wait, tick, msg)

	mu.Lock()
	defer mu.Unlock()
	if held {
		return
	}
	require.NoError(t, lastErr, "reading the subscription and its queue failed while waiting for: %s", msg)
	// Nothing failed to read, so the wait itself is the finding, and
	// assert.Eventually has already reported it. FailNow because every caller
	// was written around require.Eventually stopping the case here.
	t.FailNow()
}

// neverState is eventuallyState's opposite: it fails if cond ever holds inside
// the window.
//
// Keeping the read error is not symmetry for its own sake. A Never whose
// condition swallows errors is worse off than an Eventually that does: the
// condition answers false for the whole window and the case passes having
// watched nothing at all.
func (f *webhookWorkerFixture) neverState(
	t *testing.T,
	subscriptionID string,
	cond func(webhookWorkerState) bool,
	wait, tick time.Duration,
	msg string,
) {
	t.Helper()

	var (
		mu      sync.Mutex
		lastErr error
	)
	stayed := assert.Never(t, func() bool {
		state, err := f.state(subscriptionID)
		if err != nil {
			mu.Lock()
			lastErr = err
			mu.Unlock()
			return false
		}
		return cond(state)
	}, wait, tick, msg)

	mu.Lock()
	defer mu.Unlock()
	require.NoError(t, lastErr, "reading the subscription and its queue failed while watching for: %s", msg)
	if !stayed {
		t.FailNow()
	}
}

// allDelivered reports whether the queue holds exactly want rows and every one
// of them has reached 'delivered'.
func allDelivered(rows []webhookDeliveryRow, want int) bool {
	if len(rows) != want {
		return false
	}
	for _, row := range rows {
		if row.status != domain.WebhookDeliveryStatusDelivered {
			return false
		}
	}
	return true
}

// webhookSubscriptionRow is the subscription state the worker maintains as a
// side effect of delivering: the failure run, the auto-disable and its reason.
type webhookSubscriptionRow struct {
	exists              bool
	enabled             bool
	consecutiveFailures int
	failingSince        sql.NullTime
	disabledReason      sql.NullString
	lastDeliveryAt      sql.NullTime
}

func (f *webhookWorkerFixture) subscription(id string) (webhookSubscriptionRow, error) {
	var (
		row webhookSubscriptionRow
		// Declared DEFAULT true rather than NOT NULL, so it is read as nullable
		// for the same reason the delivery columns are.
		enabled sql.NullBool
	)
	err := f.db.QueryRow(`
		SELECT enabled, consecutive_failures, failing_since, disabled_reason, last_delivery_at
		FROM webhook_subscriptions
		WHERE id = $1`, id).
		Scan(&enabled, &row.consecutiveFailures, &row.failingSince, &row.disabledReason, &row.lastDeliveryAt)
	if err == sql.ErrNoRows {
		return webhookSubscriptionRow{exists: false}, nil
	}
	if err != nil {
		return row, fmt.Errorf("reading webhook_subscriptions row %s: %w", id, err)
	}
	if !enabled.Valid {
		return row, fmt.Errorf("subscription %s has a NULL enabled", id)
	}
	row.enabled = enabled.Bool
	row.exists = true
	return row, nil
}

func (f *webhookWorkerFixture) mustSubscription(t *testing.T, id string) webhookSubscriptionRow {
	t.Helper()
	row, err := f.subscription(id)
	require.NoError(t, err)
	require.True(t, row.exists, "subscription %s should still exist", id)
	return row
}

// webhookWorkerRequest is one POST as the subscriber saw it.
type webhookWorkerRequest struct {
	headers http.Header
	body    []byte
}

// webhookReceiver is an httptest server standing in for a subscriber's endpoint.
// It records every request it is sent and answers each one the same way, which
// is what lets a case say "this endpoint is gone" or "this endpoint is busy" and
// then assert on what the worker did about it.
type webhookReceiver struct {
	*httptest.Server

	mu       sync.Mutex
	status   int
	body     string
	requests []webhookWorkerRequest
}

func newWebhookReceiver(status int, body string) *webhookReceiver {
	return newSlowWebhookReceiver(status, body, 0)
}

// newSlowWebhookReceiver holds each request open for delay before answering, so
// a case can keep a worker inside its batch long enough for another worker to
// poll while it is there.
func newSlowWebhookReceiver(status int, body string, delay time.Duration) *webhookReceiver {
	receiver := &webhookReceiver{status: status, body: body}
	receiver.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			// A request whose body never fully arrived is not a delivery, and
			// recording it anyway would put it in count() — the gate every
			// Eventually and Never window in this file reads the worker
			// through. Aborts are routine here rather than exotic: every stop()
			// cancels a worker context, and one that fires mid-POST truncates
			// whatever was on the wire. Counted, such a request invents a
			// delivery nobody received; counted in a case that then checks the
			// signature, it fails as a signing bug.
			//
			// Answered rather than dropped, so a body truncated for any other
			// reason reaches the worker as a failed delivery it records on the
			// row instead of vanishing here.
			http.Error(w, "the request body did not arrive in full", http.StatusBadRequest)
			return
		}

		// Recorded before the response is written, so a request is never missing
		// from the log of a worker that has already moved on.
		receiver.mu.Lock()
		receiver.requests = append(receiver.requests, webhookWorkerRequest{
			headers: r.Header.Clone(),
			body:    payload,
		})
		status, body := receiver.status, receiver.body
		receiver.mu.Unlock()

		if delay > 0 {
			time.Sleep(delay)
		}

		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, body)
	}))
	return receiver
}

// answerWith changes what the endpoint says from the next request on, so a case
// can play an endpoint that recovers — the half of the retry ladder that only
// exists once an attempt has already failed.
func (r *webhookReceiver) answerWith(status int, body string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = status
	r.body = body
}

func (r *webhookReceiver) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

func (r *webhookReceiver) all() []webhookWorkerRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]webhookWorkerRequest(nil), r.requests...)
}
