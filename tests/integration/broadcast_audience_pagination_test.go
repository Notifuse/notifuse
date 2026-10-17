package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBroadcastAudiencePagination_Integration pages through a broadcast audience the way
// the broadcast orchestrator does, each page starting after the last email of the one
// before, against a real Postgres database.
//
// GetContactsForBroadcast bounds a later page by the cursor on contacts, on contact_lists
// and, for an audience narrowed to segments, on contact_segments, so the planner can start
// each scan at the cursor instead of reading every row before it. The extra bounds must not
// change which contacts a page returns: every recipient comes back once, in email order,
// and nobody else does, even with contacts of another list, removed members and bounced
// members sorting between them.
func TestBroadcastAudiencePagination_Integration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	factory := suite.DataFactory
	contactRepo := suite.ServerManager.GetApp().GetContactRepository()
	contactListRepo := suite.ServerManager.GetApp().GetContactListRepository()
	ctx := context.Background()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	other, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)

	ts := time.Now().UnixNano()
	email := func(i int) string { return fmt.Sprintf("page-%d-%02d@example.com", ts, i) }
	addContact := func(i int, listID string, status domain.ContactListStatus) {
		t.Helper()
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email(i)))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email(i)),
			testutil.WithContactListListID(listID),
			testutil.WithContactListStatus(status))
		require.NoError(t, err)
	}

	// Recipients have a row on each list; between them sort contacts of the other list
	// only, members removed from the list and bounced members.
	var recipients []string
	for i := 0; i < 12; i++ {
		switch i % 4 {
		case 1:
			addContact(i, other.ID, domain.ContactListStatusActive)
		case 2:
			if i == 2 {
				addContact(i, list.ID, domain.ContactListStatusActive)
				require.NoError(t, contactListRepo.RemoveContactFromList(ctx, workspace.ID, email(i), list.ID))
			} else {
				addContact(i, list.ID, domain.ContactListStatusBounced)
			}
		default:
			addContact(i, list.ID, domain.ContactListStatusActive)
			_, err := factory.CreateContactList(workspace.ID,
				testutil.WithContactListEmail(email(i)),
				testutil.WithContactListListID(other.ID),
				testutil.WithContactListStatus(domain.ContactListStatusActive))
			require.NoError(t, err)
			recipients = append(recipients, email(i))
		}
	}

	pageThrough := func(t *testing.T, audience domain.AudienceSettings) []string {
		t.Helper()
		var got []string
		cursor := ""
		for page := 0; page <= len(recipients); page++ {
			contacts, err := contactRepo.GetContactsForBroadcast(ctx, workspace.ID, audience, 2, cursor)
			require.NoError(t, err)
			if len(contacts) == 0 {
				return got
			}
			for _, c := range contacts {
				assert.Equal(t, list.ID, c.ListID, "each recipient comes with the broadcast's list")
				got = append(got, c.Contact.Email)
			}
			cursor = contacts[len(contacts)-1].Contact.Email
		}
		require.FailNow(t, "paging never reached an empty page")
		return nil
	}

	t.Run("a list", func(t *testing.T) {
		got := pageThrough(t, domain.AudienceSettings{List: list.ID, ExcludeUnsubscribed: true})
		assert.Equal(t, recipients, got, "every recipient once, in email order, and nobody else")
	})

	// The segment holds every other recipient and every contact that is not a recipient,
	// so its rows sort between the ones the broadcast sends to.
	t.Run("a list narrowed to a segment", func(t *testing.T) {
		segmentID := fmt.Sprintf("seg%d", ts)
		var inSegment []string
		for i := 0; i < 12; i++ {
			isRecipient := i%4 == 0 || i%4 == 3
			if isRecipient && i%2 == 1 {
				continue
			}
			_, err := db.ExecContext(ctx,
				`INSERT INTO contact_segments (email, segment_id, version, matched_at) VALUES ($1, $2, 1, NOW())`,
				email(i), segmentID)
			require.NoError(t, err)
			if isRecipient {
				inSegment = append(inSegment, email(i))
			}
		}
		require.NotEmpty(t, inSegment)
		require.Less(t, len(inSegment), len(recipients))

		got := pageThrough(t, domain.AudienceSettings{List: list.ID, Segments: []string{segmentID}, ExcludeUnsubscribed: true})
		assert.Equal(t, inSegment, got, "every recipient in the segment once, in email order, and nobody else")
	})
}
