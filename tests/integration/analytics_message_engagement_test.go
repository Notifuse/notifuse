//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// The broadcast detail page draws its opens and clicks over time from the opened_at and
// clicked_at time dimensions of the message_history analytics schema, always narrowed to
// the broadcast and to a date range.
func TestAnalyticsMessageEngagement(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()
	factory := suite.DataFactory
	ctx := context.Background()
	messages := suite.ServerManager.GetApp().GetMessageHistoryRepository()

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	workspaceRepo := suite.ServerManager.GetApp().GetWorkspaceRepository()
	require.NoError(t, workspaceRepo.AddUserToWorkspace(ctx, &domain.UserWorkspace{
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
	nowParis := time.Now().In(paris)
	day := time.Date(nowParis.Year(), nowParis.Month(), nowParis.Day()-3, 0, 0, 0, 0, paris)
	sentAt := day.Add(8 * time.Hour)
	at := func(hour, minute int) time.Time {
		return day.Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
	}

	broadcastID, otherBroadcastID := "bc-engagement", "bc-engagement-other"
	send := func(broadcast string) string {
		t.Helper()
		m, err := factory.CreateMessageHistory(workspace.ID,
			testutil.WithMessageTemplate(template.ID),
			testutil.WithMessageBroadcast(broadcast),
			func(m *domain.MessageHistory) {
				m.CreatedAt, m.SentAt, m.UpdatedAt = sentAt.UTC(), sentAt.UTC(), sentAt.UTC()
			})
		require.NoError(t, err)
		return m.ID
	}
	open := func(id string, when time.Time) {
		t.Helper()
		require.NoError(t, messages.SetOpened(ctx, workspace.ID, id, when.UTC()))
	}

	// Three messages opened at two local hours, 09:xx twice and 14:xx once.
	open(send(broadcastID), at(9, 5))
	open(send(broadcastID), at(9, 50))
	open(send(broadcastID), at(14, 20))
	// Clicked at 16:xx without ever being opened: the click records the open too.
	clickedOnly := send(broadcastID)
	require.NoError(t, messages.SetClicked(ctx, workspace.ID, clickedOnly, at(16, 10).UTC(), "https://example.com/offer", true))
	// Never opened.
	send(broadcastID)
	// Another broadcast's message, opened in the same hour.
	open(send(otherBroadcastID), at(9, 30))

	require.NoError(t, suite.APIClient.Login("testuser@example.com", ""))

	byHour := func(t *testing.T, dimension string) map[string]float64 {
		t.Helper()
		result := waRunQuery(t, suite, workspace.ID, map[string]interface{}{
			"schema": "message_history", "measures": []string{"count"},
			"timezone": "Europe/Paris",
			"timeDimensions": []map[string]interface{}{{
				"dimension": dimension, "granularity": "hour",
			}},
			"filters": []map[string]interface{}{
				{"member": "broadcast_id", "operator": "equals", "values": []string{broadcastID}},
				{"member": dimension, "operator": "inDateRange", "values": []string{
					day.UTC().Format(time.RFC3339),
					day.AddDate(0, 0, 1).Add(-time.Second).UTC().Format(time.RFC3339),
				}},
			},
		})
		got := map[string]float64{}
		for _, row := range result.Data {
			key, _ := row[dimension+"_hour"].(string)
			got[key] += waNumber(t, row["count"])
		}
		return got
	}
	// The key is the local hour written with a Z, as for the other time dimensions.
	hourKey := func(hour int) string { return fmt.Sprintf("%sT%02d:00:00Z", day.Format("2006-01-02"), hour) }

	t.Run("opens by local hour, a click without an open counted as an open, one row per hour", func(t *testing.T) {
		assert.Equal(t, map[string]float64{
			hourKey(9):  2,
			hourKey(14): 1,
			hourKey(16): 1,
		}, byHour(t, "opened_at"))
	})

	t.Run("clicks by local hour", func(t *testing.T) {
		assert.Equal(t, map[string]float64{hourKey(16): 1}, byHour(t, "clicked_at"))
	})
}
