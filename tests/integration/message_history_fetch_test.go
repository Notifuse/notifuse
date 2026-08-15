package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// fetchMessageByID reads one message history row the way production reads it —
// through ListMessages, which backs messages.list and is the only path that
// decrypts message_data. It resolves the workspace secret itself, so call sites
// don't each repeat that lookup.
func fetchMessageByID(t *testing.T, app testutil.AppInterface, workspaceID, messageID string) *domain.MessageHistory {
	t.Helper()

	workspace, err := app.GetWorkspaceRepository().GetByID(context.Background(), workspaceID)
	require.NoError(t, err)

	messages, _, err := app.GetMessageHistoryRepository().ListMessages(
		context.Background(), workspaceID, workspace.Settings.SecretKey,
		domain.MessageListParams{ID: messageID, Limit: 1},
	)
	require.NoError(t, err)
	require.Len(t, messages, 1, "no message history row with id %s", messageID)

	return messages[0]
}
