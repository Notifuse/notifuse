package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestSegmentGetContactsExecutes exercises the /api/segments.contacts endpoint end-to-end.
// GetSegmentContacts hand-wrote `ORDER BY created_at`, but contact_segments has no
// created_at column (its timestamps are matched_at / computed_at), so the query raised
// "column created_at does not exist" and the endpoint returned 500 for every real
// workspace. The service unit test mocked the DB connection to an error, so the bad SQL
// never executed. This test seeds real rows and asserts a 200 with matched_at ordering.
func TestSegmentGetContactsExecutes(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	client := suite.APIClient

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	workspaceID := workspace.ID

	user, err := factory.CreateUser()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspaceID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspaceID)

	workspaceDB, err := factory.GetWorkspaceDB(workspaceID)
	require.NoError(t, err)
	ctx := context.Background()

	segmentID := shortuuid.New()
	// Seed two members with distinct matched_at values to assert ordering (newest first).
	_, err = workspaceDB.ExecContext(ctx,
		`INSERT INTO contact_segments (email, segment_id, version, matched_at)
		 VALUES ($1, $2, 1, NOW()), ($3, $2, 1, NOW() - INTERVAL '1 hour')`,
		"recent@seg.com", segmentID, "older@seg.com")
	require.NoError(t, err)

	resp, err := client.Get("/api/segments.contacts", map[string]string{
		"workspace_id": workspaceID,
		"segment_id":   segmentID,
		"limit":        "50",
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	// Before the fix this endpoint returned 500 (invalid ORDER BY column).
	require.Equal(t, http.StatusOK, resp.StatusCode, "segments.contacts must not 500")

	var body struct {
		Emails []string `json:"emails"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, []string{"recent@seg.com", "older@seg.com"}, body.Emails,
		"members should be returned newest-matched first")
}
