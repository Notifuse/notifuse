//go:build integration
// +build integration

package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestListPendingWebhookOnResubscribe checks, on a fresh workspace, that a contact who
// unsubscribed from a list and subscribes again, which forces double opt-in, is reported
// to webhook subscriptions as list.pending, as their timeline records it: once, with the
// status they came from. The webhook trigger used to match no transition for
// unsubscribed -> pending, so integrations heard of the contact again only at
// list.confirmed.
func TestListPendingWebhookOnResubscribe(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

	factory := suite.DataFactory
	client := suite.APIClient
	serverURL := suite.ServerManager.GetURL()
	bg := context.Background()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	tag := strings.ToLower(strings.ReplaceAll(uuid.New().String(), "-", ""))[:8]

	// Public, without double opt-in: a first subscription is active, so the contact goes to
	// pending only because subscribing again after an unsubscribe forces double opt-in.
	list, err := factory.CreateList(workspace.ID, testutil.WithListPublic(true))
	require.NoError(t, err)

	// Nothing delivers it: the harness runs no delivery worker, so the deliveries the
	// trigger queues stay in webhook_deliveries, where they are read.
	subscriptionID := createZapierSubscription(t, suite, workspace.ID, "List pending on re-subscribe",
		"https://hooks.example.com/list-pending",
		[]string{"list.subscribed", "list.pending", "list.confirmed", "list.unsubscribed", "list.resubscribed"},
		[]string{list.ID}, nil)

	// --- helpers ---------------------------------------------------------------------

	subscribeViaAPI := func(t *testing.T, email string) {
		t.Helper()
		resp, err := client.Post("/api/lists.subscribe", map[string]interface{}{
			"workspace_id": workspace.ID,
			"contact":      map[string]interface{}{"email": email},
			"list_ids":     []string{list.ID},
		})
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "lists.subscribe: %s", body)
	}

	// subscribeViaPublicEndpoint posts /subscribe as the subscribe form does, or, with
	// signed, as the notification center and the double opt-in confirmation link do.
	subscribeViaPublicEndpoint := func(t *testing.T, email string, signed bool) {
		t.Helper()
		contact := map[string]interface{}{"email": email}
		if signed {
			contact["email_hmac"] = domain.ComputeEmailHMAC(email, workspace.Settings.SecretKey)
		}
		payload, err := json.Marshal(map[string]interface{}{
			"workspace_id": workspace.ID,
			"contact":      contact,
			"list_ids":     []string{list.ID},
		})
		require.NoError(t, err)
		resp, err := (&http.Client{Timeout: 10 * time.Second}).Post(serverURL+"/subscribe", "application/json", bytes.NewReader(payload))
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "/subscribe: %s", body)
	}

	// unsubscribe posts /unsubscribe as the notification center does.
	unsubscribe := func(t *testing.T, email string) {
		t.Helper()
		resp, err := postUnsubscribeJSON(serverURL, "/unsubscribe", domain.UnsubscribeFromListsRequest{
			WorkspaceID: workspace.ID,
			Email:       email,
			EmailHMAC:   domain.ComputeEmailHMAC(email, workspace.Settings.SecretKey),
			ListIDs:     []string{list.ID},
		})
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "/unsubscribe: %s", body)
	}

	requireOnListAs := func(t *testing.T, email, status string) {
		t.Helper()
		var got string
		var deletedAt sql.NullTime
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT status, deleted_at FROM contact_lists WHERE email = $1 AND list_id = $2`,
			email, list.ID).Scan(&got, &deletedAt))
		require.Equal(t, status, got)
		require.False(t, deletedAt.Valid, "the contact is on the list")
	}

	timelinePending := func(t *testing.T, email, listID string) int {
		t.Helper()
		var n int
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT COUNT(*) FROM contact_timeline WHERE email = $1 AND entity_id = $2 AND kind = 'list.pending'`,
			email, listID).Scan(&n))
		return n
	}

	kinds := func(deliveries []b24Delivery) []string {
		out := make([]string, 0, len(deliveries))
		for _, d := range deliveries {
			out = append(out, d.eventType)
		}
		return out
	}

	// requirePendingAfterUnsubscribe checks the deliveries queued for a contact who
	// subscribed, unsubscribed and subscribed again: one each, list.pending last, with the
	// payload of the other list events and the status the contact came from.
	requirePendingAfterUnsubscribe := func(t *testing.T, email string) {
		t.Helper()
		deliveries := b24Deliveries(t, db, subscriptionID, email)
		require.Equal(t, []string{"list.subscribed", "list.unsubscribed", "list.pending"}, kinds(deliveries))
		assert.Equal(t, map[string]interface{}{
			"email":           email,
			"list_id":         list.ID,
			"list_name":       list.Name,
			"status":          "pending",
			"previous_status": "unsubscribed",
		}, deliveries[2].payload)
		assert.Equal(t, sortedKeys(deliveries[1].payload), sortedKeys(deliveries[2].payload),
			"list.pending carries the fields list.unsubscribed carries")
		assert.Equal(t, 1, timelinePending(t, email, list.ID), "the timeline records it once too")
	}

	// --- through the API ----------------------------------------------------------------

	t.Run("lists.subscribe after an unsubscribe sends list.pending once, then list.confirmed", func(t *testing.T) {
		email := fmt.Sprintf("resub-api-%s@example.com", tag)

		subscribeViaAPI(t, email)
		unsubscribe(t, email)
		subscribeViaAPI(t, email)

		requireOnListAs(t, email, "pending")
		requirePendingAfterUnsubscribe(t, email)

		// The confirmation link moves them to active: list.confirmed, and no other
		// list.pending.
		subscribeViaPublicEndpoint(t, email, true)

		requireOnListAs(t, email, "active")
		deliveries := b24Deliveries(t, db, subscriptionID, email)
		require.Equal(t, []string{"list.subscribed", "list.unsubscribed", "list.pending", "list.confirmed"}, kinds(deliveries))
		assert.Equal(t, "pending", deliveries[3].payload["previous_status"])
		assert.Equal(t, 1, timelinePending(t, email, list.ID))
	})

	// --- through the subscribe form ---------------------------------------------------

	t.Run("the subscribe form after an unsubscribe sends list.pending once", func(t *testing.T) {
		email := fmt.Sprintf("resub-form-%s@example.com", tag)

		subscribeViaPublicEndpoint(t, email, false)
		unsubscribe(t, email)
		subscribeViaPublicEndpoint(t, email, false)
		// Submitting the form again while pending resends the confirmation and writes
		// nothing, so nothing is sent again.
		subscribeViaPublicEndpoint(t, email, false)

		requireOnListAs(t, email, "pending")
		requirePendingAfterUnsubscribe(t, email)
	})

	// --- every transition, on the triggers themselves ------------------------------------

	// Every way into pending and every other write to a row, written directly so each one
	// reaches both triggers: the webhook sends list.pending exactly where the timeline
	// records it, at most once, with the status the contact came from, or none for a re-add,
	// which is reported as a first subscription.
	t.Run("the webhook sends list.pending exactly where the timeline records it", func(t *testing.T) {
		matrixList, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		matrixSubscription := createZapierSubscription(t, suite, workspace.ID, "List pending matrix",
			"https://hooks.example.com/list-pending-matrix", []string{"list.pending"}, []string{matrixList.ID}, nil)

		pendingEvents := func(t *testing.T, email string) (int, []b24Delivery) {
			t.Helper()
			return timelinePending(t, email, matrixList.ID), b24Deliveries(t, db, matrixSubscription, email)
		}

		statuses := []string{"active", "pending", "unsubscribed", "bounced", "complained"}
		rows := []struct {
			name          string
			removedBefore bool
			removedAfter  bool
		}{
			{"on the list", false, false},
			{"added back", true, false},
			{"being removed", false, true},
			{"staying removed", true, true},
		}

		for _, row := range rows {
			for _, from := range statuses {
				for _, to := range statuses {
					name := fmt.Sprintf("%s, %s -> %s", row.name, from, to)
					email := fmt.Sprintf("matrix-%s-%s-%s-%s@example.com", strings.ReplaceAll(row.name, " ", ""), from, to, tag)

					_, err := db.ExecContext(bg, `
						INSERT INTO contact_lists (email, list_id, status, deleted_at)
						VALUES ($1, $2, $3, CASE WHEN $4 THEN NOW() END)`,
						email, matrixList.ID, from, row.removedBefore)
					require.NoError(t, err)
					// An insert records list.pending on both for a pending row, and on neither for another.
					seedTimeline, seedDeliveries := pendingEvents(t, email)
					require.Len(t, seedDeliveries, seedTimeline, "%s: the insert", name)

					_, err = db.ExecContext(bg, `
						UPDATE contact_lists
						SET status = $3, updated_at = NOW(),
						    deleted_at = CASE WHEN $4 THEN COALESCE(deleted_at, NOW()) END
						WHERE email = $1 AND list_id = $2`,
						email, matrixList.ID, to, row.removedAfter)
					require.NoError(t, err)

					timeline, deliveries := pendingEvents(t, email)
					timeline -= seedTimeline
					deliveries = deliveries[len(seedDeliveries):]

					// The timeline records list.pending when a row that is on the list after the
					// write moved to pending, or was added back as pending.
					want := 0
					if to == "pending" && !row.removedAfter && (from != to || row.removedBefore) {
						want = 1
					}
					require.Equal(t, want, timeline, "%s: the timeline", name)
					require.Len(t, deliveries, want, "%s: the webhook", name)
					if want == 0 {
						continue
					}

					var previous interface{} = from
					if row.removedBefore {
						previous = nil
					}
					assert.Equal(t, map[string]interface{}{
						"email":           email,
						"list_id":         matrixList.ID,
						"list_name":       matrixList.Name,
						"status":          "pending",
						"previous_status": previous,
					}, deliveries[0].payload, name)
				}
			}
		}
	})
}
