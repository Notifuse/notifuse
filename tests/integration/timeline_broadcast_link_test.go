package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestTimelineBroadcastLinkSegment verifies end-to-end that a segment condition scoping a
// click event to a broadcast and/or a clicked link produces SQL that runs against the real
// schema and matches the right contacts. link_url is a case-insensitive substring match
// against the clicked destination URLs stored as keys of message_history.clicked_links.
func TestTimelineBroadcastLinkSegment(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	workspaceID := workspace.ID

	template, err := factory.CreateTemplate(workspaceID)
	require.NoError(t, err)

	workspaceDB, err := factory.GetWorkspaceDB(workspaceID)
	require.NoError(t, err)
	ctx := context.Background()

	// seedClick creates a contact, a message from the given broadcast whose clicked_links
	// hold the given destination URL, and a email.clicked timeline row pointing at it.
	seedClick := func(email, broadcastID, clickedURL string) {
		_, err := factory.CreateContact(workspaceID, testutil.WithContactEmail(email))
		require.NoError(t, err)

		msg, err := factory.CreateMessageHistory(workspaceID,
			testutil.WithMessageHistoryContactEmail(email),
			testutil.WithMessageHistoryTemplateID(template.ID))
		require.NoError(t, err)

		_, err = workspaceDB.ExecContext(ctx,
			`UPDATE message_history SET broadcast_id = $1,
			     clicked_links = jsonb_build_object($2::text, jsonb_build_object('count', 1))
			 WHERE id = $3`,
			broadcastID, clickedURL, msg.ID)
		require.NoError(t, err)

		require.NoError(t, factory.CreateContactTimelineEvent(workspaceID, email, "email.clicked",
			map[string]interface{}{"entity_id": msg.ID}))
	}

	// buyer's URL carries a literal underscore ("utm_campaign") used by the wildcard test.
	seedClick("buyer@bl.com", "summer-sale", "https://acme.com/pricing?utm_campaign=summer")
	seedClick("browser@bl.com", "summer-sale", "https://acme.com/home")
	seedClick("other@bl.com", "winter-sale", "https://acme.com/pricing")

	qb := service.NewQueryBuilder()

	matches := func(t *testing.T, cond *domain.ContactTimelineCondition) map[string]bool {
		sql, args, err := qb.BuildSQL(&domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{Source: "contact_timeline", ContactTimeline: cond},
		})
		require.NoError(t, err)

		rows, err := workspaceDB.QueryContext(ctx, sql, args...)
		require.NoError(t, err, "generated broadcast/link segment SQL must execute: %s", sql)
		defer rows.Close()

		found := map[string]bool{}
		for rows.Next() {
			var email string
			require.NoError(t, rows.Scan(&email))
			found[email] = true
		}
		require.NoError(t, rows.Err())
		return found
	}

	str := func(s string) *string { return &s }

	t.Run("broadcast scope matches everyone who clicked in that broadcast", func(t *testing.T) {
		got := matches(t, &domain.ContactTimelineCondition{
			Kind: "email.clicked", CountOperator: "at_least", CountValue: 1,
			BroadcastID: str("summer-sale"),
		})
		require.True(t, got["buyer@bl.com"])
		require.True(t, got["browser@bl.com"])
		require.False(t, got["other@bl.com"], "clicked in a different broadcast")
	})

	t.Run("link substring within a broadcast narrows to the right contact", func(t *testing.T) {
		got := matches(t, &domain.ContactTimelineCondition{
			Kind: "email.clicked", CountOperator: "at_least", CountValue: 1,
			BroadcastID: str("summer-sale"), LinkURL: str("/pricing"),
		})
		require.True(t, got["buyer@bl.com"], "clicked /pricing in summer-sale")
		require.False(t, got["browser@bl.com"], "clicked /home, not /pricing")
		require.False(t, got["other@bl.com"], "clicked /pricing but in a different broadcast")
	})

	t.Run("link substring is case-insensitive", func(t *testing.T) {
		got := matches(t, &domain.ContactTimelineCondition{
			Kind: "email.clicked", CountOperator: "at_least", CountValue: 1,
			LinkURL: str("PRICING"),
		})
		require.True(t, got["buyer@bl.com"])
		require.True(t, got["other@bl.com"], "both clicked a /pricing link regardless of broadcast")
		require.False(t, got["browser@bl.com"])
	})

	t.Run("link match is literal: % and _ are not wildcards", func(t *testing.T) {
		// buyer's URL contains the literal substring "utm_campaign".
		require.True(t, matches(t, &domain.ContactTimelineCondition{
			Kind: "email.clicked", CountOperator: "at_least", CountValue: 1,
			LinkURL: str("utm_campaign"),
		})["buyer@bl.com"], "a literal underscore substring must match")

		// "utm%campaign" must NOT match "utm_campaign": '%' is matched literally, not as an
		// ILIKE 'any sequence' wildcard (which would have matched the underscore).
		require.False(t, matches(t, &domain.ContactTimelineCondition{
			Kind: "email.clicked", CountOperator: "at_least", CountValue: 1,
			LinkURL: str("utm%campaign"),
		})["buyer@bl.com"], "'%' must be literal, not a wildcard")
	})
}
