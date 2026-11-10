package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/internal/repository"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/trackingtoken"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// linkClickEntry is an email.link_clicked timeline entry, as the trigger writes it.
type linkClickEntry struct {
	Email        string
	URL          string
	BroadcastID  sql.NullString
	TemplateID   sql.NullString
	AutomationID sql.NullString
}

// TestTimelineLinkClicked drives clicks through the real /r/ route and checks the
// email.link_clicked entries track_message_history_changes() writes: one per link first
// clicked through a signed token, none for a repeat, an unsigned click, a click without a
// URL, a link past the 50-link cap, an update that leaves clicked_links alone or a click on
// an erased contact's email.
func TestTimelineLinkClicked(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	baseURL := suite.ServerManager.GetURL()
	factory := suite.DataFactory
	ctx := context.Background()

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	workspaceID := workspace.ID
	key, err := trackingtoken.DeriveKey(workspace.Settings.SecretKey)
	require.NoError(t, err)

	template, err := factory.CreateTemplate(workspaceID)
	require.NoError(t, err)
	broadcast, err := factory.CreateBroadcast(workspaceID)
	require.NoError(t, err)
	workspaceDB, err := factory.GetWorkspaceDB(workspaceID)
	require.NoError(t, err)

	redirectClient := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	// Old enough to pass the too-fast-click bot gate.
	tokenTS := time.Now().Add(-30 * time.Second).Unix()

	newMessage := func(t *testing.T, email string) *domain.MessageHistory {
		t.Helper()
		_, err := factory.CreateContact(workspaceID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		msg, err := factory.CreateMessageHistory(workspaceID,
			testutil.WithMessageContact(email),
			testutil.WithMessageTemplate(template.ID),
			testutil.WithMessageBroadcast(broadcast.ID))
		require.NoError(t, err)
		return msg
	}

	// click follows a tracked link. A signed token is what an email sent since 42.2 carries;
	// an unsigned one is what an older email carries, or what a recipient can forge.
	click := func(t *testing.T, messageID, destination string, signed bool) {
		t.Helper()
		var signingKey []byte
		if signed {
			signingKey = key
		}
		token, err := trackingtoken.MintClick(signingKey, messageID, workspaceID, tokenTS, destination)
		require.NoError(t, err)
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/r/%s", baseURL, token), nil)
		require.NoError(t, err)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Safari/605.1.15")
		resp, err := redirectClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Less(t, resp.StatusCode, 400, "the click route answers a redirect or the confirmation page")
	}

	linkEntries := func(t *testing.T, messageID string) []linkClickEntry {
		t.Helper()
		rows, err := workspaceDB.QueryContext(ctx, `
			SELECT email, changes->'clicked_url'->>'new', changes->'broadcast_id'->>'new',
			       changes->'template_id'->>'new', changes->'automation_id'->>'new'
			FROM contact_timeline
			WHERE entity_id = $1 AND kind = 'email.link_clicked'
			ORDER BY changes->'clicked_url'->>'new'`, messageID)
		require.NoError(t, err)
		defer rows.Close()
		var entries []linkClickEntry
		for rows.Next() {
			var e linkClickEntry
			require.NoError(t, rows.Scan(&e.Email, &e.URL, &e.BroadcastID, &e.TemplateID, &e.AutomationID))
			entries = append(entries, e)
		}
		require.NoError(t, rows.Err())
		return entries
	}
	urls := func(entries []linkClickEntry) []string {
		out := []string{}
		for _, e := range entries {
			out = append(out, e.URL)
		}
		return out
	}
	countKind := func(t *testing.T, messageID, kind string) int {
		t.Helper()
		var n int
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT count(*) FROM contact_timeline WHERE entity_id = $1 AND kind = $2`, messageID, kind).Scan(&n))
		return n
	}

	const urlA = "https://shop.example.com/products/sneakers?utm_source=newsletter"
	const urlB = "https://shop.example.com/pricing"

	t.Run("a signed first click writes email.clicked and one entry for its link", func(t *testing.T) {
		msg := newMessage(t, "first-click@link.test")
		click(t, msg.ID, urlA, true)

		assert.Equal(t, 1, countKind(t, msg.ID, "email.clicked"))
		entries := linkEntries(t, msg.ID)
		require.Len(t, entries, 1)
		assert.Equal(t, "first-click@link.test", entries[0].Email)
		assert.Equal(t, urlA, entries[0].URL)
		assert.Equal(t, broadcast.ID, entries[0].BroadcastID.String)
		assert.Equal(t, template.ID, entries[0].TemplateID.String)
		assert.False(t, entries[0].AutomationID.Valid, "a broadcast email has no automation")
	})

	t.Run("a repeat click writes nothing, a second link writes its own entry", func(t *testing.T) {
		email := "second-link@link.test"
		msg := newMessage(t, email)
		click(t, msg.ID, urlA, true)
		click(t, msg.ID, urlA, true)
		assert.Equal(t, []string{urlA}, urls(linkEntries(t, msg.ID)))

		// The contact's segments are recomputed for the second link too: before, nothing
		// queued them, since clicked_at was already set.
		_, err := workspaceDB.ExecContext(ctx, `DELETE FROM contact_segment_queue WHERE email = $1`, email)
		require.NoError(t, err)
		click(t, msg.ID, urlB, true)
		assert.Equal(t, []string{urlB, urlA}, urls(linkEntries(t, msg.ID)), "ordered by URL")
		assert.Equal(t, 1, countKind(t, msg.ID, "email.clicked"), "email.clicked stays one per email")
		var queued int
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT count(*) FROM contact_segment_queue WHERE email = $1`, email).Scan(&queued))
		assert.Equal(t, 1, queued, "the second link queues the contact for segment recomputation")

		// And the existing "Click email" condition with a link filter matches it.
		qb := service.NewQueryBuilder()
		link := "/pricing"
		segmentSQL, args, err := qb.BuildSQL(&domain.TreeNode{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{
			Source: "contact_timeline",
			ContactTimeline: &domain.ContactTimelineCondition{
				Kind: "email.clicked", CountOperator: "at_least", CountValue: 1,
				BroadcastID: &broadcast.ID, LinkURL: &link,
			},
		}})
		require.NoError(t, err)
		rows, err := workspaceDB.QueryContext(ctx, segmentSQL, args...)
		require.NoError(t, err)
		defer rows.Close()
		found := false
		for rows.Next() {
			var e string
			require.NoError(t, rows.Scan(&e))
			found = found || e == email
		}
		assert.True(t, found, "a segment on the second link includes the contact")
	})

	t.Run("an unsigned click counts but writes no entry until a signed click on the link", func(t *testing.T) {
		msg := newMessage(t, "unsigned@link.test")
		click(t, msg.ID, urlA, false)

		assert.Equal(t, 1, countKind(t, msg.ID, "email.clicked"), "the click still counts")
		assert.Empty(t, linkEntries(t, msg.ID))
		var verified sql.NullString
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT clicked_links->$2->>'verified' FROM message_history WHERE id = $1`, msg.ID, urlA).Scan(&verified))
		assert.Equal(t, "false", verified.String, "the URL is kept for the per-link statistics")

		click(t, msg.ID, urlA, false)
		assert.Empty(t, linkEntries(t, msg.ID))

		click(t, msg.ID, urlA, true)
		assert.Equal(t, []string{urlA}, urls(linkEntries(t, msg.ID)), "the first signed click verifies the link")
		click(t, msg.ID, urlA, false)
		assert.Len(t, linkEntries(t, msg.ID), 1, "a later unsigned click never unverifies it")
	})

	t.Run("a click without a URL writes email.clicked only", func(t *testing.T) {
		msg := newMessage(t, "no-url@link.test")
		_, err := workspaceDB.ExecContext(ctx, `UPDATE message_history SET clicked_at = NOW() WHERE id = $1`, msg.ID)
		require.NoError(t, err)
		assert.Equal(t, 1, countKind(t, msg.ID, "email.clicked"))
		assert.Empty(t, linkEntries(t, msg.ID))
	})

	t.Run("at the 50-link cap a new link writes nothing, a known one still verifies", func(t *testing.T) {
		msg := newMessage(t, "cap@link.test")
		// Seeded unverified, so the seed itself writes no entry.
		_, err := workspaceDB.ExecContext(ctx, `
			UPDATE message_history SET clicked_at = NOW(), clicked_links = (
				SELECT jsonb_object_agg('https://shop.example.com/item/' || i,
				       jsonb_build_object('count', 1, 'verified', false))
				FROM generate_series(1, 50) AS i)
			WHERE id = $1`, msg.ID)
		require.NoError(t, err)
		require.Empty(t, linkEntries(t, msg.ID))

		click(t, msg.ID, "https://shop.example.com/item/51", true)
		assert.Empty(t, linkEntries(t, msg.ID), "the 51st link is not recorded")

		click(t, msg.ID, "https://shop.example.com/item/7", true)
		assert.Equal(t, []string{"https://shop.example.com/item/7"}, urls(linkEntries(t, msg.ID)))
	})

	t.Run("a malformed clicked_links heals and writes the entry", func(t *testing.T) {
		for name, malformed := range map[string]string{"json null": `'null'::jsonb`, "array": `'[1, 2]'::jsonb`, "scalar": `'"x"'::jsonb`} {
			t.Run(name, func(t *testing.T) {
				msg := newMessage(t, fmt.Sprintf("malformed-%d@link.test", time.Now().UnixNano()))
				_, err := workspaceDB.ExecContext(ctx,
					`UPDATE message_history SET clicked_links = `+malformed+` WHERE id = $1`, msg.ID)
				require.NoError(t, err)
				click(t, msg.ID, urlB, true)
				assert.Equal(t, []string{urlB}, urls(linkEntries(t, msg.ID)))
			})
		}
	})

	t.Run("an update that leaves clicked_links alone writes no entry", func(t *testing.T) {
		msg := newMessage(t, "delivered@link.test")
		click(t, msg.ID, urlA, true)
		_, err := workspaceDB.ExecContext(ctx,
			`UPDATE message_history SET delivered_at = NOW(), status_info = 'ok' WHERE id = $1`, msg.ID)
		require.NoError(t, err)
		assert.Len(t, linkEntries(t, msg.ID), 1)
	})

	t.Run("a click on an erased contact's email records nothing", func(t *testing.T) {
		email := "erased@link.test"
		msg := newMessage(t, email)
		_, err := workspaceDB.ExecContext(ctx,
			`UPDATE message_history SET contact_email = 'DELETED_EMAIL', clicked_links = NULL WHERE id = $1`, msg.ID)
		require.NoError(t, err)

		click(t, msg.ID, urlA, true)

		var clickedLinks sql.NullString
		var clickedAt, openedAt sql.NullTime
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT clicked_links::text, clicked_at, opened_at FROM message_history WHERE id = $1`, msg.ID).
			Scan(&clickedLinks, &clickedAt, &openedAt))
		assert.False(t, clickedLinks.Valid, "the URL is not put back on the redacted row")
		assert.False(t, clickedAt.Valid)
		assert.False(t, openedAt.Valid)
		assert.Zero(t, countKind(t, msg.ID, "email.clicked"))
		assert.Empty(t, linkEntries(t, msg.ID))
	})

	t.Run("messages.list filters a broadcast's emails by the exact clicked URL", func(t *testing.T) {
		user, err := factory.CreateUser()
		require.NoError(t, err)
		require.NoError(t, factory.AddUserToWorkspace(user.ID, workspaceID, "owner"))
		client := suite.APIClient
		require.NoError(t, client.Login(user.Email, "password"))
		client.SetWorkspaceID(workspaceID)

		const tricky = "https://shop.example.com/a b?x=1&y=%41#frag+"
		msg := newMessage(t, "drilldown@link.test")
		click(t, msg.ID, tricky, true)
		other := newMessage(t, "drilldown-other@link.test")
		click(t, other.ID, tricky+"more", true)

		list := func(t *testing.T, clickedURL string) []string {
			t.Helper()
			resp, err := client.Get("/api/messages.list", map[string]string{
				"workspace_id": workspaceID, "broadcast_id": broadcast.ID, "clicked_url": clickedURL,
			})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			var result struct {
				Messages []struct {
					ID string `json:"id"`
				} `json:"messages"`
			}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
			ids := []string{}
			for _, m := range result.Messages {
				ids = append(ids, m.ID)
			}
			return ids
		}

		// The URL exactly as the per-link statistics return it.
		resp, err := client.Get("/api/messages.broadcastLinkStats", map[string]string{
			"workspace_id": workspaceID, "broadcast_id": broadcast.ID,
		})
		require.NoError(t, err)
		var stats struct {
			LinkStats []struct {
				URL string `json:"url"`
			} `json:"link_stats"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&stats))
		_ = resp.Body.Close()
		var statURL string
		for _, s := range stats.LinkStats {
			if s.URL == tricky {
				statURL = s.URL
			}
		}
		require.Equal(t, tricky, statURL, "the stats return the key the click recorded")

		assert.Equal(t, []string{msg.ID}, list(t, statURL), "exact match, not a prefix")
		assert.Empty(t, list(t, "https://shop.example.com/nowhere"))

		// A malformed clicked_links on another email of the broadcast fails nothing.
		bad := newMessage(t, "drilldown-bad@link.test")
		_, err = workspaceDB.ExecContext(ctx, `UPDATE message_history SET clicked_links = '[1]'::jsonb WHERE id = $1`, bad.ID)
		require.NoError(t, err)
		assert.Equal(t, []string{msg.ID}, list(t, statURL))
	})

	t.Run("the monthly event meter leaves email.link_clicked out", func(t *testing.T) {
		webRepo := repository.NewWebAnalyticsRepository(suite.ServerManager.GetApp().GetWorkspaceRepository(), logger.NewLogger())
		month := time.Now().UTC()
		metered := func(t *testing.T) int64 {
			t.Helper()
			require.NoError(t, webRepo.RecomputeUsage(ctx, workspaceID, month, true))
			usage, err := webRepo.GetUsage(ctx, workspaceID, []time.Time{month})
			require.NoError(t, err)
			require.Len(t, usage, 1)
			return usage[0].TimelineEntries
		}

		msg := newMessage(t, "metered@link.test")
		before := metered(t)
		click(t, msg.ID, urlA, true) // email.clicked, email.opened, email.link_clicked
		click(t, msg.ID, urlB, true) // email.link_clicked
		require.Len(t, linkEntries(t, msg.ID), 2)

		var others int64
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT count(*) FROM contact_timeline WHERE entity_id = $1 AND kind NOT IN ('email.link_clicked', 'email.sent')`, msg.ID).Scan(&others)) // email.sent was written, and metered, before
		require.Positive(t, others)
		assert.Equal(t, before+others, metered(t), "one click costs what it cost before")
	})

	t.Run("v44 brings the link step to a workspace that ran 43", func(t *testing.T) {
		// The body a workspace at 42 or 43 runs: the shared one without the link step.
		linkStep := regexp.MustCompile(`(?s)\n\s*` + regexp.QuoteMeta(schema.LinkClickedStepAnchor) + `\n.*?ORDER BY e\.key;\s*END IF;`)
		_, err := workspaceDB.ExecContext(ctx, linkStep.ReplaceAllString(schema.TrackMessageHistoryChangesFunction(), ""))
		require.NoError(t, err)

		msg := newMessage(t, "upgrade@link.test")
		click(t, msg.ID, urlA, true)
		require.Empty(t, linkEntries(t, msg.ID), "before v44 no entry is written")

		tx, err := workspaceDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		require.NoError(t, (&migrations.V44Migration{}).UpdateWorkspace(ctx, &config.Config{}, workspace, tx))
		require.NoError(t, tx.Commit())

		click(t, msg.ID, urlB, true)
		assert.Equal(t, []string{urlB}, urls(linkEntries(t, msg.ID)),
			"clicks before the upgrade are not replayed; the next new link writes its entry")
	})
}
