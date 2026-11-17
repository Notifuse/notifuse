//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// The Contacts page of the console: search, subscriber counts per list, and a contact's
// activity filtered by type and by what the member may read.
func TestContactsListsRedesign(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()
	factory := suite.DataFactory
	client := suite.APIClient
	ctx := context.Background()

	owner, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(owner.ID, workspace.ID, "owner"))
	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	text := func(s string) *domain.NullableString { return &domain.NullableString{String: s} }
	contact := func(email, first, last, full, externalID string) {
		t.Helper()
		_, err := factory.CreateContact(workspace.ID, func(c *domain.Contact) {
			c.Email, c.FirstName, c.LastName, c.FullName, c.ExternalID = email, text(first), text(last), text(full), text(externalID)
		})
		require.NoError(t, err)
	}
	contact("ann@example.com", "Annabel", "Lee", "Annabel Lee", "ext-ann-1")
	contact("bob@example.com", "Robert", "Nesta", "Bob Marley", "ext-bob-2")
	contact("a_b@example.com", "Plain", "Name", "Plain Name", "x1")
	contact("axb@example.com", "Plain", "Name", "Plain Name", "x2")
	contact("jane@example.com", "Jane", "Doe", "", "x3")

	get := func(t *testing.T, endpoint string, params map[string]string, out interface{}) int {
		t.Helper()
		params["workspace_id"] = workspace.ID
		resp, err := client.Get(endpoint, params)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode == http.StatusOK && out != nil {
			require.NoError(t, json.NewDecoder(resp.Body).Decode(out))
		}
		return resp.StatusCode
	}

	require.NoError(t, client.Login(owner.Email, "password"))

	t.Run("contacts.list search matches email, names and external ID, wildcards literal", func(t *testing.T) {
		search := func(term string) []string {
			var body struct {
				Contacts []struct {
					Email string `json:"email"`
				} `json:"contacts"`
			}
			require.Equal(t, http.StatusOK, get(t, "/api/contacts.list", map[string]string{"search": term}, &body))
			emails := []string{}
			for _, c := range body.Contacts {
				emails = append(emails, c.Email)
			}
			sort.Strings(emails)
			return emails
		}
		assert.Equal(t, []string{"ann@example.com"}, search("annab"), "first name, any case")
		assert.Equal(t, []string{"bob@example.com"}, search(" MARLEY "), "full name, trimmed")
		assert.Equal(t, []string{"ann@example.com"}, search("ext-ann"), "external ID")
		assert.Equal(t, []string{"bob@example.com"}, search("bob@"), "email")
		assert.Equal(t, []string{"a_b@example.com"}, search("a_b"), "an underscore matches only itself")
		assert.Equal(t, []string{"jane@example.com"}, search("jane doe"), "first and last name together, with no full name")
	})

	t.Run("contacts.list refuses a search over 100 characters", func(t *testing.T) {
		long := make([]byte, 101)
		for i := range long {
			long[i] = 'a'
		}
		assert.Equal(t, http.StatusBadRequest, get(t, "/api/contacts.list", map[string]string{"search": string(long)}, nil))
	})

	t.Run("lists.list with_counts gives each list its active subscribers", func(t *testing.T) {
		news, err := factory.CreateList(workspace.ID, func(l *domain.List) { l.Name = "News" })
		require.NoError(t, err)
		empty, err := factory.CreateList(workspace.ID, func(l *domain.List) { l.Name = "Empty" })
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID, testutil.WithContactListEmail("ann@example.com"), testutil.WithContactListListID(news.ID))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID, testutil.WithContactListEmail("bob@example.com"), testutil.WithContactListListID(news.ID),
			testutil.WithContactListStatus(domain.ContactListStatusUnsubscribed))
		require.NoError(t, err)

		var body struct {
			Lists []map[string]interface{} `json:"lists"`
		}
		require.Equal(t, http.StatusOK, get(t, "/api/lists.list", map[string]string{"with_counts": "true"}, &body))
		counts := map[string]interface{}{}
		for _, l := range body.Lists {
			counts[l["id"].(string)] = l["active_count"]
		}
		assert.Equal(t, map[string]interface{}{news.ID: float64(1), empty.ID: float64(0)}, counts)

		var plain struct {
			Lists []map[string]interface{} `json:"lists"`
		}
		require.Equal(t, http.StatusOK, get(t, "/api/lists.list", map[string]string{}, &plain))
		require.Len(t, plain.Lists, 2)
		for _, l := range plain.Lists {
			assert.NotContains(t, l, "active_count")
		}
	})

	// ann's activity: her creation (contact), a message, and a bounce reported by the provider.
	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)
	message, err := factory.CreateMessageHistory(workspace.ID, testutil.WithMessageContact("ann@example.com"), testutil.WithMessageTemplate(template.ID))
	require.NoError(t, err)
	_, err = workspaceDB.ExecContext(ctx, `
		INSERT INTO inbound_webhook_events (id, type, source, integration_id, recipient_email, message_id, raw_payload, timestamp, created_at)
		VALUES ($1, 'bounce', 'ses', 'int-1', 'ann@example.com', $2, '{}', NOW(), NOW())`, uuid.NewString(), message.ID)
	require.NoError(t, err)

	timelineTypes := func(t *testing.T, params map[string]string) map[string]bool {
		t.Helper()
		params["email"] = "ann@example.com"
		var body struct {
			Timeline []struct {
				EntityType string `json:"entity_type"`
			} `json:"timeline"`
		}
		require.Equal(t, http.StatusOK, get(t, "/api/timeline.list", params, &body))
		types := map[string]bool{}
		for _, e := range body.Timeline {
			types[e.EntityType] = true
		}
		return types
	}

	t.Run("timeline.list entity_types keeps only those types", func(t *testing.T) {
		all := timelineTypes(t, map[string]string{})
		require.True(t, all["contact"], "%v", all)
		require.True(t, all["message_history"], "%v", all)
		require.True(t, all["inbound_webhook_event"], "%v", all)

		assert.Equal(t, map[string]bool{"contact": true}, timelineTypes(t, map[string]string{"entity_types": "contact"}))
		assert.Equal(t, map[string]bool{"contact": true, "message_history": true},
			timelineTypes(t, map[string]string{"entity_types": "message_history,contact"}))
		assert.Equal(t, http.StatusBadRequest, get(t, "/api/timeline.list",
			map[string]string{"email": "ann@example.com", "entity_types": "bogus"}, nil))
	})

	t.Run("timeline.list gives a message's data decrypted, as messages.list does", func(t *testing.T) {
		// Message history encrypts its data at rest; the contact sheet previews an email from
		// this data, which rendered without personalisation while it came encrypted.
		var body struct {
			Timeline []struct {
				EntityType string                 `json:"entity_type"`
				EntityID   string                 `json:"entity_id"`
				EntityData map[string]interface{} `json:"entity_data"`
			} `json:"timeline"`
		}
		require.Equal(t, http.StatusOK, get(t, "/api/timeline.list",
			map[string]string{"email": "ann@example.com", "entity_types": "message_history"}, &body))
		require.Len(t, body.Timeline, 1)
		messageData, ok := body.Timeline[0].EntityData["message_data"].(map[string]interface{})
		require.True(t, ok, "%v", body.Timeline[0].EntityData)
		assert.Equal(t, map[string]interface{}{"subject": "Test Message", "body": "This is a test message"}, messageData["data"])
		assert.Equal(t, map[string]interface{}{"test": true}, messageData["metadata"])
		// What sent the message, for the sheet to name it as its Messages tab does.
		for _, key := range []string{"broadcast_id", "automation_id", "transactional_notification_id"} {
			assert.Contains(t, body.Timeline[0].EntityData, key)
		}
	})

	t.Run("timeline.list of an email with no entry is an empty list, not null", func(t *testing.T) {
		resp, err := client.Get("/api/timeline.list", map[string]string{"workspace_id": workspace.ID, "email": "nobody@example.com"})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		raw, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(raw))
		var body map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(raw, &body))
		assert.Equal(t, "[]", string(body["timeline"]))
	})

	t.Run("a member who may not read message history sees neither messages nor provider events", func(t *testing.T) {
		member, err := factory.CreateUser()
		require.NoError(t, err)
		perms := domain.NewFullPermissions()
		perms[domain.PermissionResourceMessageHistory] = domain.ResourcePermissions{}
		require.NoError(t, factory.AddUserToWorkspaceWithPermissions(member.ID, workspace.ID, "member", perms))
		require.NoError(t, client.Login(member.Email, "password"))
		defer func() { require.NoError(t, client.Login(owner.Email, "password")) }()

		types := timelineTypes(t, map[string]string{})
		assert.True(t, types["contact"], "%v", types)
		assert.False(t, types["message_history"], "%v", types)
		assert.False(t, types["inbound_webhook_event"], "%v", types)

		var body struct {
			Timeline []interface{} `json:"timeline"`
		}
		require.Equal(t, http.StatusOK, get(t, "/api/timeline.list",
			map[string]string{"email": "ann@example.com", "entity_types": "message_history"}, &body))
		assert.NotNil(t, body.Timeline, "an empty list, not null")
		assert.Empty(t, body.Timeline)
	})

	// ann's website visit: a session and its page, as the web analytics projection writes them.
	for _, entityType := range []string{domain.TimelineEntityWebSession, domain.TimelineEntityWebPage} {
		require.NoError(t, factory.CreateContactTimelineEvent(workspace.ID, "ann@example.com", entityType,
			map[string]interface{}{"entity_type": entityType, "entity_id": uuid.NewString()}))
	}

	t.Run("a member who may not read web analytics sees no website visit", func(t *testing.T) {
		all := timelineTypes(t, map[string]string{})
		require.True(t, all[domain.TimelineEntityWebSession], "%v", all)
		require.True(t, all[domain.TimelineEntityWebPage], "%v", all)

		member, err := factory.CreateUser()
		require.NoError(t, err)
		perms := domain.NewFullPermissions()
		perms[domain.PermissionResourceWebAnalytics] = domain.ResourcePermissions{}
		require.NoError(t, factory.AddUserToWorkspaceWithPermissions(member.ID, workspace.ID, "member", perms))
		require.NoError(t, client.Login(member.Email, "password"))
		defer func() { require.NoError(t, client.Login(owner.Email, "password")) }()

		types := timelineTypes(t, map[string]string{})
		assert.True(t, types["contact"], "%v", types)
		assert.True(t, types["message_history"], "%v", types)
		assert.False(t, types[domain.TimelineEntityWebSession], "%v", types)
		assert.False(t, types[domain.TimelineEntityWebPage], "%v", types)
		assert.Empty(t, timelineTypes(t, map[string]string{"entity_types": "web_session,web_page"}))
	})

	t.Run("timeline.list gives the instant an entry was written, whatever the writer's time zone", func(t *testing.T) {
		// Written by a session in Tokyo as a local wall time, with no offset: PostgreSQL reads
		// it in the session's zone. The API must give the same instant, not the same digits.
		conn, err := workspaceDB.Conn(ctx)
		require.NoError(t, err)
		defer func() { _ = conn.Close() }()
		_, err = conn.ExecContext(ctx, `SET TIME ZONE 'Asia/Tokyo'`)
		require.NoError(t, err)
		defer func() { _, _ = conn.ExecContext(ctx, `RESET TIME ZONE`) }()
		entityID := uuid.NewString()
		_, err = conn.ExecContext(ctx, `INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, created_at)
			VALUES ('ann@example.com', 'insert', 'custom_event', 'custom_event.tz_probe', $1, '2026-01-15 09:00:00')`, entityID)
		require.NoError(t, err)

		var body struct {
			Timeline []struct {
				EntityID  string    `json:"entity_id"`
				CreatedAt time.Time `json:"created_at"`
			} `json:"timeline"`
		}
		require.Equal(t, http.StatusOK, get(t, "/api/timeline.list", map[string]string{"email": "ann@example.com", "entity_types": "custom_event"}, &body))
		var found bool
		for _, e := range body.Timeline {
			if e.EntityID == entityID {
				found = true
				want := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
				assert.True(t, e.CreatedAt.Equal(want), "got %s, want the instant %s", e.CreatedAt, want)
			}
		}
		assert.True(t, found, "the entry written in Tokyo is listed")
	})

	t.Run("an API key reads timeline.list within its permissions", func(t *testing.T) {
		authService := suite.ServerManager.GetApp().GetAuthService().(*service.AuthService)
		read := func(t *testing.T, permissions domain.UserPermissions) (int, map[string]bool) {
			t.Helper()
			key, err := factory.CreateAPIKey(workspace.ID, testutil.WithAPIKeyPermissions(permissions))
			require.NoError(t, err)
			apiClient := testutil.NewAPIClient(suite.ServerManager.GetURL())
			apiClient.SetToken(authService.GenerateAPIAuthToken(key))
			resp, err := apiClient.Get("/api/timeline.list", map[string]string{"workspace_id": workspace.ID, "email": "ann@example.com"})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			var body struct {
				Timeline []struct {
					EntityType string `json:"entity_type"`
				} `json:"timeline"`
			}
			types := map[string]bool{}
			if resp.StatusCode == http.StatusOK {
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
				for _, e := range body.Timeline {
					types[e.EntityType] = true
				}
			}
			return resp.StatusCode, types
		}

		status, types := read(t, domain.UserPermissions{domain.PermissionResourceContacts: {Read: true}})
		require.Equal(t, http.StatusOK, status)
		assert.True(t, types["contact"], "%v", types)
		assert.False(t, types["message_history"], "%v", types)
		assert.False(t, types[domain.TimelineEntityWebSession], "%v", types)

		status, types = read(t, domain.UserPermissions{
			domain.PermissionResourceContacts:       {Read: true},
			domain.PermissionResourceMessageHistory: {Read: true},
			domain.PermissionResourceWebAnalytics:   {Read: true},
		})
		require.Equal(t, http.StatusOK, status)
		assert.True(t, types["message_history"], "%v", types)
		assert.True(t, types[domain.TimelineEntityWebSession], "%v", types)

		status, _ = read(t, domain.UserPermissions{domain.PermissionResourceMessageHistory: {Read: true}})
		assert.Equal(t, http.StatusForbidden, status, "contacts read is required")
	})
}
