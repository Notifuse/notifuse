package integration

import (
	"context"
	"fmt"
	"sort"
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
// and nobody else does, even with contacts of another list, removed members, bounced
// members and, for a broadcast that leaves them out, unsubscribed members sorting between
// them. A contact in several of the audience's segments comes back once too, and
// CountContactsForBroadcast counts what the pages return.
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
	// only, a member removed from the list, a bounced member and an unsubscribed one,
	// who is a recipient only of a broadcast that keeps unsubscribed contacts.
	const unsubscribed = 10
	var recipients []string
	for i := 0; i < 12; i++ {
		switch i % 4 {
		case 1:
			addContact(i, other.ID, domain.ContactListStatusActive)
		case 2:
			switch i {
			case 2:
				addContact(i, list.ID, domain.ContactListStatusActive)
				require.NoError(t, contactListRepo.RemoveContactFromList(ctx, workspace.ID, email(i), list.ID))
			case unsubscribed:
				addContact(i, list.ID, domain.ContactListStatusUnsubscribed)
			default:
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

	pageThrough := func(t *testing.T, audience domain.AudienceSettings, pageSize int) []string {
		t.Helper()
		var got []string
		cursor := ""
		for page := 0; page <= len(recipients); page++ {
			contacts, err := contactRepo.GetContactsForBroadcast(ctx, workspace.ID, audience, pageSize, cursor)
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

	counted := func(t *testing.T, audience domain.AudienceSettings) int {
		t.Helper()
		n, err := contactRepo.CountContactsForBroadcast(ctx, workspace.ID, audience)
		require.NoError(t, err)
		return n
	}
	addToSegment := func(t *testing.T, segmentID string, i int) {
		t.Helper()
		_, err := db.ExecContext(ctx,
			`INSERT INTO contact_segments (email, segment_id, version, matched_at) VALUES ($1, $2, 1, NOW())`,
			email(i), segmentID)
		require.NoError(t, err)
	}

	// withUnsubscribed adds the unsubscribed member to want, in email order
	withUnsubscribed := func(want []string) []string {
		out := append([]string{}, want...)
		out = append(out, email(unsubscribed))
		sort.Strings(out)
		return out
	}

	t.Run("a list", func(t *testing.T) {
		audience := domain.AudienceSettings{List: list.ID, ExcludeUnsubscribed: true}
		got := pageThrough(t, audience, 2)
		assert.Equal(t, recipients, got, "every recipient once, in email order, and nobody else")
		assert.Equal(t, len(got), counted(t, audience))

		audience.ExcludeUnsubscribed = false
		got = pageThrough(t, audience, 2)
		assert.Equal(t, withUnsubscribed(recipients), got, "the unsubscribed member too, in its place")
		assert.Equal(t, len(got), counted(t, audience))
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
			addToSegment(t, segmentID, i)
			if isRecipient {
				inSegment = append(inSegment, email(i))
			}
		}
		require.NotEmpty(t, inSegment)
		require.Less(t, len(inSegment), len(recipients))

		audience := domain.AudienceSettings{List: list.ID, Segments: []string{segmentID}, ExcludeUnsubscribed: true}
		got := pageThrough(t, audience, 2)
		assert.Equal(t, inSegment, got, "every recipient in the segment once, in email order, and nobody else")
		assert.Equal(t, len(got), counted(t, audience))

		audience.ExcludeUnsubscribed = false
		got = pageThrough(t, audience, 2)
		assert.Equal(t, withUnsubscribed(inSegment), got, "the unsubscribed member, who is in the segment, too")
		assert.Equal(t, len(got), counted(t, audience))
	})

	// A contact in both segments is one recipient. Each segment also holds contacts who are
	// not recipients, and the unsubscribed member is in both.
	t.Run("a list narrowed to two segments that share contacts", func(t *testing.T) {
		first, second := fmt.Sprintf("segA%d", ts), fmt.Sprintf("segB%d", ts)
		var inEither, inBoth []string
		var othersInFirst, othersInSecond int
		unsubscribedInBoth := false
		for i := 0; i < 12; i++ {
			a, b := i%3 != 2, i%2 == 0
			if a {
				addToSegment(t, first, i)
			}
			if b {
				addToSegment(t, second, i)
			}
			if isRecipient := i%4 == 0 || i%4 == 3; isRecipient && (a || b) {
				inEither = append(inEither, email(i))
				if a && b {
					inBoth = append(inBoth, email(i))
				}
			} else if !isRecipient && i != unsubscribed {
				if a {
					othersInFirst++
				}
				if b {
					othersInSecond++
				}
			}
			if i == unsubscribed {
				unsubscribedInBoth = a && b
			}
		}
		require.NotEmpty(t, inBoth)
		require.Less(t, len(inEither), len(recipients))
		require.Positive(t, othersInFirst)
		require.Positive(t, othersInSecond)
		require.True(t, unsubscribedInBoth)

		audience := domain.AudienceSettings{List: list.ID, Segments: []string{first, second}, ExcludeUnsubscribed: true}
		// Pages of two cross page boundaries. One page of 50 holds the whole audience, with
		// a row per segment for a contact in both, so no cursor can skip a repeated contact.
		require.LessOrEqual(t, len(inEither)+len(inBoth), 50)
		for _, pageSize := range []int{2, 50} {
			got := pageThrough(t, audience, pageSize)
			assert.Equal(t, inEither, got, "pages of %d: every recipient in either segment once, in email order", pageSize)
		}
		assert.Equal(t, len(inEither), counted(t, audience), "a contact in both segments is counted once")

		audience.ExcludeUnsubscribed = false
		got := pageThrough(t, audience, 2)
		assert.Equal(t, withUnsubscribed(inEither), got, "the unsubscribed member, who is in both segments, once too")
		assert.Equal(t, len(got), counted(t, audience))
	})
}
