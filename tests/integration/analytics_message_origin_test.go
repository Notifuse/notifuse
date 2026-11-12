//go:build integration
// +build integration

package integration

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// The dashboard splits sent messages by what sent them through the origin dimension of the
// message_history analytics schema.
func TestAnalyticsMessageOrigin(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()
	factory := suite.DataFactory

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	// The seeded test user queries analytics as an owner.
	workspaceRepo := suite.ServerManager.GetApp().GetWorkspaceRepository()
	require.NoError(t, workspaceRepo.AddUserToWorkspace(context.Background(), &domain.UserWorkspace{
		UserID:      "550e8400-e29b-41d4-a716-446655440000", // testuser@example.com
		WorkspaceID: workspace.ID,
		Role:        "owner",
		Permissions: domain.FullPermissions,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}))

	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)

	paris, err := time.LoadLocation("Europe/Paris")
	require.NoError(t, err)
	// The last day of the range, three local days ago so it is complete whatever the hour.
	nowParis := time.Now().In(paris)
	day := time.Date(nowParis.Year(), nowParis.Month(), nowParis.Day()-3, 0, 0, 0, 0, paris)
	previousDay := day.AddDate(0, 0, -1)
	noon := day.Add(12 * time.Hour)
	// 00:30 in Paris is the evening before in UTC: it belongs to the local day.
	justAfterLocalMidnight := day.Add(30 * time.Minute)

	broadcastID := "bc-origin"
	automationID := "auto-origin"
	notificationID := "password_reset"

	create := func(at time.Time, opts ...testutil.MessageHistoryOption) {
		t.Helper()
		opts = append([]testutil.MessageHistoryOption{
			testutil.WithMessageTemplate(template.ID),
			func(m *domain.MessageHistory) {
				m.CreatedAt = at.UTC()
				m.SentAt = at.UTC()
				m.UpdatedAt = at.UTC()
			},
		}, opts...)
		_, err := factory.CreateMessageHistory(workspace.ID, opts...)
		require.NoError(t, err)
	}
	withAutomation := func(m *domain.MessageHistory) { m.AutomationID = &automationID }
	withNotification := func(m *domain.MessageHistory) { m.TransactionalNotificationID = &notificationID }

	// Broadcast: a message of the broadcast and its test send, which carries the broadcast ID.
	create(noon, testutil.WithMessageBroadcast(broadcastID))
	create(noon, testutil.WithMessageBroadcast(broadcastID))
	// Automation: two messages, the same day as the broadcast.
	create(noon, withAutomation)
	create(noon, withAutomation)
	// Transactional: three notification messages (one just after local midnight), plus a
	// template test and a double opt-in confirmation, which carry no ID at all.
	create(noon, withNotification)
	create(noon, withNotification)
	create(justAfterLocalMidnight, withNotification)
	create(noon)
	create(noon)
	// Outside the range: never counted.
	create(day.AddDate(0, 0, -10).Add(12*time.Hour), withNotification)

	require.NoError(t, suite.APIClient.Login("testuser@example.com", ""))

	rangeFilter := map[string]interface{}{
		"member": "created_at", "operator": "inDateRange",
		"values": []string{
			previousDay.UTC().Format(time.RFC3339),
			day.AddDate(0, 0, 1).Add(-time.Second).UTC().Format(time.RFC3339),
		},
	}

	t.Run("the origin dimension splits the messages by what sent them", func(t *testing.T) {
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema": "message_history", "measures": []string{"count"},
			"dimensions": []string{"origin"},
			"filters":    []map[string]interface{}{rangeFilter},
		})
		got := map[string]float64{}
		for _, row := range result.Data {
			origin, _ := row["origin"].(string)
			got[origin] += waNumber(t, row["count"])
		}
		assert.Equal(t, map[string]float64{"broadcast": 2, "automation": 2, "transactional": 5}, got)
	})

	t.Run("each origin filter counts its own messages", func(t *testing.T) {
		for origin, want := range map[string]float64{"broadcast": 2, "automation": 2, "transactional": 5} {
			result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
				"schema": "message_history", "measures": []string{"count"},
				"filters": []map[string]interface{}{
					rangeFilter,
					{"member": "origin", "operator": "equals", "values": []string{origin}},
				},
			})
			require.Len(t, result.Data, 1, origin)
			assert.Equal(t, want, waNumber(t, result.Data[0]["count"]), origin)
		}
	})

	t.Run("a daily series in Paris puts each message on its local day, last day included", func(t *testing.T) {
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema": "message_history", "measures": []string{"count"},
			"timezone": "Europe/Paris",
			"timeDimensions": []map[string]interface{}{{
				"dimension": "created_at", "granularity": "day",
				"dateRange": []string{previousDay.Format("2006-01-02"), day.Format("2006-01-02")},
			}},
			"filters": []map[string]interface{}{
				{"member": "origin", "operator": "equals", "values": []string{"transactional"}},
			},
		})
		byDay := map[string]float64{}
		for _, row := range result.Data {
			key, _ := row["created_at_day"].(string)
			byDay[key] += waNumber(t, row["count"])
		}
		assert.Equal(t, map[string]float64{
			previousDay.Format("2006-01-02") + "T00:00:00Z": 0,
			day.Format("2006-01-02") + "T00:00:00Z":         5,
		}, byDay)
	})

	t.Run("a member who may not read message history is refused", func(t *testing.T) {
		member, err := factory.CreateUser()
		require.NoError(t, err)
		perms := domain.NewFullPermissions()
		perms[domain.PermissionResourceMessageHistory] = domain.ResourcePermissions{Read: false, Write: false}
		require.NoError(t, factory.AddUserToWorkspaceWithPermissions(member.ID, workspace.ID, "member", perms))
		require.NoError(t, suite.APIClient.Login(member.Email, "password"))
		defer func() { _ = suite.APIClient.Login("testuser@example.com", "") }()

		resp, err := suite.APIClient.Post("/api/analytics.query", map[string]interface{}{
			"workspace_id": workspace.ID,
			"query": map[string]interface{}{
				"schema": "message_history", "measures": []string{"count"},
				"dimensions": []string{"origin"},
			},
		})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
}
