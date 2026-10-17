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

// TestBroadcastAudiencePagination_Integration pages through a list's broadcast audience the
// way the broadcast orchestrator does, each page starting after the last email of the one
// before, against a real Postgres database.
//
// GetContactsForBroadcast bounds a later page by the cursor on both contacts and
// contact_lists, so the planner starts its contact_lists scan at the cursor instead of
// reading every row before it. The second bound must not change which contacts a page
// returns: every recipient comes back once, in email order, and nobody else does, even
// with contacts of another list, a removed member and a bounced member sorting between
// them.
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

	var recipients []string
	for i := 0; i < 12; i++ {
		switch i % 4 {
		case 1:
			// On another list only
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
			// Also on the other list, so contact_lists holds two rows for the address
			_, err := factory.CreateContactList(workspace.ID,
				testutil.WithContactListEmail(email(i)),
				testutil.WithContactListListID(other.ID),
				testutil.WithContactListStatus(domain.ContactListStatusActive))
			require.NoError(t, err)
			recipients = append(recipients, email(i))
		}
	}

	audience := domain.AudienceSettings{List: list.ID, ExcludeUnsubscribed: true}
	var got []string
	cursor := ""
	for page := 0; page < len(recipients)+1; page++ {
		contacts, err := contactRepo.GetContactsForBroadcast(ctx, workspace.ID, audience, 2, cursor)
		require.NoError(t, err)
		if len(contacts) == 0 {
			break
		}
		for _, c := range contacts {
			assert.Equal(t, list.ID, c.ListID, "each recipient comes with the broadcast's list")
			got = append(got, c.Contact.Email)
		}
		cursor = contacts[len(contacts)-1].Contact.Email
	}

	assert.Equal(t, recipients, got, "every recipient once, in email order, and nobody else")
}
