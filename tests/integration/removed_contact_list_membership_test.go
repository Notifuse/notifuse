package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	shortuuid "github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRemovedContactListMembership checks, through the public API on a fresh workspace,
// that a contact removed from a list (contactLists.removeContact, or an automation's
// Remove from List node) is no longer a member of it anywhere: broadcasts, segments,
// automation trigger conditions and filter nodes, webhooks, the API's contact_lists and
// Liquid's contact.contact_lists. It also checks that adding them back makes them a member
// again, which needs the re-add to write a timeline event. The notification center already
// hid removed rows before this fix; its subtest keeps that pinned end to end now that the
// contact it reads no longer carries them.
func TestRemovedContactListMembership(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		// The webhook subscription below targets a placeholder URL; nothing delivers it
		// (the harness starts no delivery worker), the queued deliveries are read instead.
		cfg.Webhook.AllowPrivateDeliveryHosts = true
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(ctx))

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	serverURL := suite.ServerManager.GetURL()
	require.NoError(t, factory.SetWorkspaceCustomEndpointURL(workspace.ID, serverURL))

	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	wsRepo := suite.ServerManager.GetApp().GetWorkspaceRepository()
	processor := service.NewContactSegmentQueueProcessor(
		repository.NewContactSegmentQueueRepository(wsRepo),
		repository.NewSegmentRepository(wsRepo),
		repository.NewContactRepository(wsRepo),
		wsRepo,
		logger.NewLoggerWithLevel("error"),
	)

	tag := strings.ToLower(strings.ReplaceAll(uuid.New().String(), "-", ""))[:8]

	listX, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	listY, err := factory.CreateList(workspace.ID, testutil.WithListPublic(true))
	require.NoError(t, err)
	listZ, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	require.False(t, listZ.IsPublic)

	// --- helpers ---------------------------------------------------------------------

	post := func(t *testing.T, path string, body map[string]interface{}) (int, string) {
		t.Helper()
		resp, err := client.Post(path, body)
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode, string(raw)
	}

	newContact := func(t *testing.T, name string) string {
		t.Helper()
		email := fmt.Sprintf("b24-%s-%s@example.com", name, tag)
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		return email
	}

	addTo := func(t *testing.T, email, listID string) {
		t.Helper()
		_, err := factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(listID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
	}

	removeViaAPI := func(t *testing.T, email, listID string) {
		t.Helper()
		status, body := post(t, "/api/contactLists.removeContact", map[string]interface{}{
			"workspace_id": workspace.ID,
			"email":        email,
			"list_id":      listID,
		})
		require.Equal(t, http.StatusOK, status, "removeContact: %s", body)
	}

	subscribeViaAPI := func(t *testing.T, email, listID string) {
		t.Helper()
		status, body := post(t, "/api/lists.subscribe", map[string]interface{}{
			"workspace_id": workspace.ID,
			"contact":      map[string]interface{}{"email": email},
			"list_ids":     []string{listID},
		})
		require.Equal(t, http.StatusOK, status, "lists.subscribe: %s", body)
	}

	isRemoved := func(t *testing.T, email, listID string) bool {
		t.Helper()
		var deletedAt sql.NullTime
		err := db.QueryRowContext(context.Background(),
			`SELECT deleted_at FROM contact_lists WHERE email = $1 AND list_id = $2`, email, listID).Scan(&deletedAt)
		if err == sql.ErrNoRows {
			return false
		}
		require.NoError(t, err)
		return deletedAt.Valid
	}

	isMember := func(t *testing.T, email, listID string) bool {
		t.Helper()
		var n int
		require.NoError(t, db.QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM contact_lists WHERE email = $1 AND list_id = $2 AND deleted_at IS NULL`,
			email, listID).Scan(&n))
		return n == 1
	}

	fireEvent := func(t *testing.T, email, eventName string) {
		t.Helper()
		status, body := post(t, "/api/customEvents.upsert", map[string]interface{}{
			"workspace_id": workspace.ID,
			"email":        email,
			"event_name":   eventName,
			"external_id":  shortuuid.New(),
			"properties":   map[string]interface{}{},
			"occurred_at":  time.Now().UTC().Format(time.RFC3339),
		})
		require.Equal(t, http.StatusOK, status, "customEvents.upsert: %s", body)
	}

	inListX := func(status string) map[string]interface{} {
		contactList := map[string]interface{}{"operator": "in", "list_id": listX.ID}
		if status != "" {
			contactList["status"] = status
		}
		return map[string]interface{}{
			"kind": "leaf",
			"leaf": map[string]interface{}{"source": "contact_lists", "contact_list": contactList},
		}
	}
	notInListX := map[string]interface{}{
		"kind": "leaf",
		"leaf": map[string]interface{}{
			"source":       "contact_lists",
			"contact_list": map[string]interface{}{"operator": "not_in", "list_id": listX.ID},
		},
	}

	// createLive creates and activates an automation on a custom event.
	createLive := func(t *testing.T, name, eventName string, conditions map[string]interface{}, nodes func(automationID, firstNodeID string) []map[string]interface{}) string {
		t.Helper()
		automationID := shortuuid.New()
		triggerNodeID := shortuuid.New()
		firstNodeID := shortuuid.New()
		trigger := map[string]interface{}{
			"event_kind":        "custom_event",
			"custom_event_name": eventName,
			"frequency":         "once",
		}
		if conditions != nil {
			trigger["conditions"] = conditions
		}
		allNodes := append([]map[string]interface{}{{
			"id":            triggerNodeID,
			"automation_id": automationID,
			"type":          "trigger",
			"config":        map[string]interface{}{},
			"next_node_id":  firstNodeID,
			"position":      map[string]interface{}{"x": 0, "y": 0},
		}}, nodes(automationID, firstNodeID)...)

		status, body := post(t, "/api/automations.create", map[string]interface{}{
			"workspace_id": workspace.ID,
			"automation": map[string]interface{}{
				"id":           automationID,
				"workspace_id": workspace.ID,
				"name":         name,
				"status":       "draft",
				"trigger":      trigger,
				"root_node_id": triggerNodeID,
				"nodes":        allNodes,
				"stats":        map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
			},
		})
		require.Equal(t, http.StatusCreated, status, "automations.create: %s", body)
		status, body = post(t, "/api/automations.activate", map[string]interface{}{
			"workspace_id":  workspace.ID,
			"automation_id": automationID,
		})
		require.Equal(t, http.StatusOK, status, "automations.activate: %s", body)
		return automationID
	}

	listNode := func(automationID, nodeID, nodeType, listID string) map[string]interface{} {
		config := map[string]interface{}{"list_id": listID}
		if nodeType == "add_to_list" {
			config["status"] = "active"
		}
		return map[string]interface{}{
			"id":            nodeID,
			"automation_id": automationID,
			"type":          nodeType,
			"config":        config,
			"position":      map[string]interface{}{"x": 0, "y": 100},
		}
	}

	createSegment := func(t *testing.T, name string, tree map[string]interface{}) string {
		t.Helper()
		segmentID := "b24" + name + tag
		status, body := post(t, "/api/segments.create", map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           segmentID,
			"name":         "B24 " + name,
			"color":        "#00BFFF",
			"timezone":     "UTC",
			"tree":         tree,
		})
		require.Contains(t, []int{http.StatusOK, http.StatusCreated}, status, "segments.create: %s", body)
		_, err := testutil.WaitForSegmentBuilt(t, client, workspace.ID, segmentID, 60*time.Second)
		require.NoError(t, err)
		return segmentID
	}

	// drainQueue ages every queued contact past the debounce and processes the queue
	// until it is empty, as the recurring queue task would.
	drainQueue := func(t *testing.T) {
		t.Helper()
		for i := 0; i < 20; i++ {
			_, err := db.ExecContext(context.Background(),
				`UPDATE contact_segment_queue SET queued_at = NOW() - INTERVAL '1 minute'`)
			require.NoError(t, err)
			n, err := processor.ProcessQueue(context.Background(), workspace.ID)
			require.NoError(t, err)
			if n == 0 {
				return
			}
		}
		t.Fatal("the segment queue never drained")
	}

	// --- contacts --------------------------------------------------------------------

	member := newContact(t, "member")
	addTo(t, member, listX.ID)

	apiRemoved := newContact(t, "apiremoved")
	addTo(t, apiRemoved, listX.ID)
	addTo(t, apiRemoved, listY.ID)
	addTo(t, apiRemoved, listZ.ID)
	removeViaAPI(t, apiRemoved, listX.ID)
	removeViaAPI(t, apiRemoved, listZ.ID)

	nodeRemoved := newContact(t, "noderemoved")
	addTo(t, nodeRemoved, listX.ID)
	removeEvent := "b24_remove_" + tag
	createLive(t, "B24 remover", removeEvent, nil, func(automationID, firstNodeID string) []map[string]interface{} {
		return []map[string]interface{}{listNode(automationID, firstNodeID, "remove_from_list", listX.ID)}
	})
	fireEvent(t, nodeRemoved, removeEvent)
	testutil.WaitForCondition(t, func() bool { return isRemoved(t, nodeRemoved, listX.ID) },
		30*time.Second, "waiting for the Remove from List node")

	never := newContact(t, "never")

	require.True(t, isRemoved(t, apiRemoved, listX.ID))
	require.True(t, isRemoved(t, apiRemoved, listZ.ID))

	// --- 1. broadcasts ---------------------------------------------------------------

	t.Run("broadcasts skip removed contacts, whatever exclude_unsubscribed says", func(t *testing.T) {
		for _, exclude := range []bool{true, false} {
			subject := fmt.Sprintf("b24-broadcast-%t-%s", exclude, uuid.New().String()[:8])
			template, err := factory.CreateTemplate(workspace.ID, testutil.WithTemplateSubject(subject))
			require.NoError(t, err)

			resp, err := client.CreateBroadcast(map[string]interface{}{
				"workspace_id": workspace.ID,
				"name":         subject,
				"audience":     map[string]interface{}{"list": listX.ID, "exclude_unsubscribed": exclude},
				"test_settings": map[string]interface{}{
					"enabled":    false,
					"variations": []map[string]interface{}{{"template_id": template.ID}},
				},
			})
			require.NoError(t, err)
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			require.Equal(t, http.StatusCreated, resp.StatusCode, "create broadcast: %s", body)
			var created struct {
				Broadcast struct {
					ID string `json:"id"`
				} `json:"broadcast"`
			}
			require.NoError(t, json.Unmarshal(body, &created))

			resp, err = client.ScheduleBroadcast(map[string]interface{}{
				"workspace_id": workspace.ID,
				"id":           created.Broadcast.ID,
				"send_now":     true,
			})
			require.NoError(t, err)
			body, _ = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode, "schedule broadcast: %s", body)

			_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, created.Broadcast.ID,
				[]string{"processed", "completed"}, 60*time.Second)
			require.NoError(t, err)

			resp, err = client.GetBroadcast(created.Broadcast.ID)
			require.NoError(t, err)
			body, _ = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode, "get broadcast: %s", body)
			var got struct {
				Broadcast struct {
					EnqueuedCount int `json:"enqueued_count"`
				} `json:"broadcast"`
			}
			require.NoError(t, json.Unmarshal(body, &got))

			var received map[string]bool
			testutil.WaitForCondition(t, func() bool {
				received, err = testutil.GetAllMailpitRecipients(t, subject)
				return err == nil && len(received) >= got.Broadcast.EnqueuedCount
			}, 30*time.Second, "waiting for the broadcast emails of "+subject)

			assert.Equal(t, map[string]bool{member: true}, received, "exclude_unsubscribed=%t", exclude)
			assert.Equal(t, 1, got.Broadcast.EnqueuedCount, "exclude_unsubscribed=%t", exclude)
		}
	})

	// --- 2. and 3. segments ----------------------------------------------------------

	inSegment := createSegment(t, "in", inListX("active"))
	notInSegment := createSegment(t, "notin", notInListX)

	t.Run("segments: a removed contact is not in the list", func(t *testing.T) {
		assert.Equal(t, 1, memberCount(t, db, member, inSegment))
		assert.Equal(t, 0, memberCount(t, db, apiRemoved, inSegment))
		assert.Equal(t, 0, memberCount(t, db, nodeRemoved, inSegment))

		assert.Equal(t, 1, memberCount(t, db, apiRemoved, notInSegment))
		assert.Equal(t, 1, memberCount(t, db, nodeRemoved, notInSegment))
		assert.Equal(t, 1, memberCount(t, db, never, notInSegment))
		assert.Equal(t, 0, memberCount(t, db, member, notInSegment))
	})

	t.Run("segments: a removal, then a re-add, moves the contact", func(t *testing.T) {
		toggled := newContact(t, "toggled")
		addTo(t, toggled, listX.ID)
		drainQueue(t)
		require.Equal(t, 1, memberCount(t, db, toggled, inSegment))
		require.Equal(t, 0, memberCount(t, db, toggled, notInSegment))

		removeViaAPI(t, toggled, listX.ID)
		drainQueue(t)
		assert.Equal(t, 0, memberCount(t, db, toggled, inSegment), "a removed contact leaves \"in list\"")
		assert.Equal(t, 1, memberCount(t, db, toggled, notInSegment), "a removed contact joins \"not in list\"")
		assert.Equal(t, 1, timelineKindCount(t, db, toggled, "segment.left", inSegment))
		assert.Equal(t, 1, timelineKindCount(t, db, toggled, "segment.joined", notInSegment))

		insertsBefore := b24TimelineInserts(t, db, toggled, "list.subscribed", listX.ID)
		subscribeViaAPI(t, toggled, listX.ID)
		assert.Equal(t, insertsBefore+1, b24TimelineInserts(t, db, toggled, "list.subscribed", listX.ID),
			"a re-add records list.subscribed with operation insert")

		drainQueue(t)
		assert.Equal(t, 1, memberCount(t, db, toggled, inSegment), "a re-added contact is back in \"in list\"")
		assert.Equal(t, 0, memberCount(t, db, toggled, notInSegment))
	})

	// Removing an already removed contact only moves deleted_at. It used to record
	// list.removed again, which re-enrolled the contact in automations triggered by it.
	t.Run("removing a contact who is already removed records nothing", func(t *testing.T) {
		again := newContact(t, "again")
		addTo(t, again, listX.ID)
		removeViaAPI(t, again, listX.ID)
		require.Equal(t, 1, timelineKindCount(t, db, again, "list.removed", listX.ID))

		removeViaAPI(t, again, listX.ID)
		assert.Equal(t, 1, timelineKindCount(t, db, again, "list.removed", listX.ID),
			"a second removal records no second list.removed")
	})

	// --- 4. trigger condition --------------------------------------------------------

	t.Run("an automation trigger condition on the list leaves removed contacts out", func(t *testing.T) {
		marker, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		eventName := "b24_condition_" + tag
		automationID := createLive(t, "B24 condition", eventName, inListX("active"),
			func(automationID, firstNodeID string) []map[string]interface{} {
				return []map[string]interface{}{listNode(automationID, firstNodeID, "add_to_list", marker.ID)}
			})

		for _, email := range []string{member, apiRemoved, never} {
			fireEvent(t, email, eventName)
		}

		// automation_enroll_contact runs inside the insert that fired the trigger, so
		// once the event is stored the enrollment is final.
		enrolled, err := factory.FindContactAutomation(workspace.ID, automationID, member)
		require.NoError(t, err)
		assert.NotNil(t, enrolled, "a member is enrolled")
		for _, email := range []string{apiRemoved, never} {
			ca, err := factory.FindContactAutomation(workspace.ID, automationID, email)
			require.NoError(t, err)
			assert.Nil(t, ca, "%s is not on the list and must not be enrolled", email)
		}
	})

	// --- 5. filter node --------------------------------------------------------------

	t.Run("a filter node on the list sends a removed contact down the exit path", func(t *testing.T) {
		passList, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		failList, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		eventName := "b24_filter_" + tag
		createLive(t, "B24 filter", eventName, nil, func(automationID, firstNodeID string) []map[string]interface{} {
			passNodeID := shortuuid.New()
			failNodeID := shortuuid.New()
			return []map[string]interface{}{
				{
					"id":            firstNodeID,
					"automation_id": automationID,
					"type":          "filter",
					"config": map[string]interface{}{
						"conditions":       inListX("active"),
						"continue_node_id": passNodeID,
						"exit_node_id":     failNodeID,
					},
					"position": map[string]interface{}{"x": 0, "y": 100},
				},
				listNode(automationID, passNodeID, "add_to_list", passList.ID),
				listNode(automationID, failNodeID, "add_to_list", failList.ID),
			}
		})

		// nodeRemoved rather than apiRemoved: the exit path adds the contact to failList,
		// and the notification center and API checks below expect apiRemoved on Y alone.
		fireEvent(t, member, eventName)
		fireEvent(t, nodeRemoved, eventName)

		testutil.WaitForCondition(t, func() bool {
			return isMember(t, member, passList.ID) && isMember(t, nodeRemoved, failList.ID)
		}, 30*time.Second, "waiting for the filter node to route both contacts")
		assert.False(t, isMember(t, member, failList.ID))
		assert.False(t, isMember(t, nodeRemoved, passList.ID), "a removed contact must not pass a membership filter")
	})

	// --- 6. webhooks -----------------------------------------------------------------

	t.Run("webhooks: a re-add is reported as a subscription", func(t *testing.T) {
		listD, err := factory.CreateList(workspace.ID, testutil.WithListPublic(true), testutil.WithListDoubleOptin(true))
		require.NoError(t, err)

		hooked := newContact(t, "hooked")
		addTo(t, hooked, listX.ID)
		hookedD := newContact(t, "hookedd")
		addTo(t, hookedD, listD.ID)

		subscriptionID := createZapierSubscription(t, suite, workspace.ID, "B24 webhooks",
			"https://hooks.example.com/b24",
			[]string{"list.removed", "list.subscribed", "list.pending"},
			[]string{listX.ID, listD.ID}, nil)

		removeViaAPI(t, hooked, listX.ID)
		subscribeViaAPI(t, hooked, listX.ID)

		deliveries := b24Deliveries(t, db, subscriptionID, hooked)
		require.Len(t, deliveries, 2, "one delivery for the removal, one for the re-add")
		assert.Equal(t, "list.removed", deliveries[0].eventType)
		assert.Equal(t, "list.subscribed", deliveries[1].eventType)
		assert.Equal(t, "active", deliveries[1].payload["status"])
		prev, present := deliveries[1].payload["previous_status"]
		assert.True(t, present, "previous_status is part of the payload")
		assert.Nil(t, prev, "a re-add reports no previous status, like a first subscription")

		// A removed contact re-adding themselves through the public form on a double
		// opt-in list goes to pending, which used to send no webhook at all.
		removeViaAPI(t, hookedD, listD.ID)
		payload, err := json.Marshal(map[string]interface{}{
			"workspace_id": workspace.ID,
			"contact":      map[string]interface{}{"email": hookedD},
			"list_ids":     []string{listD.ID},
		})
		require.NoError(t, err)
		resp, err := (&http.Client{Timeout: 10 * time.Second}).Post(serverURL+"/subscribe", "application/json", bytes.NewReader(payload))
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "/subscribe: %s", body)

		deliveries = b24Deliveries(t, db, subscriptionID, hookedD)
		require.Len(t, deliveries, 2, "one delivery for the removal, one for the re-add")
		assert.Equal(t, "list.removed", deliveries[0].eventType)
		assert.Equal(t, "list.pending", deliveries[1].eventType)
		assert.Equal(t, "pending", deliveries[1].payload["status"])
		prev, present = deliveries[1].payload["previous_status"]
		assert.True(t, present)
		assert.Nil(t, prev)
	})

	// --- 7. notification center ------------------------------------------------------

	// visibleContactLists dropped removed rows before this fix, so this passes without it.
	// It stays as an end-to-end guard: fetchContact now leaves those rows out itself, and
	// the page must still show only the lists the contact is on.
	t.Run("the notification center still hides lists the contact was removed from", func(t *testing.T) {
		template, err := factory.CreateTemplate(workspace.ID)
		require.NoError(t, err)
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastTemplateID(template.ID),
			testutil.WithBroadcastAudience(domain.AudienceSettings{List: listY.ID, ExcludeUnsubscribed: true}))
		require.NoError(t, err)
		resp, err := client.SendBroadcastToIndividual(map[string]interface{}{
			"workspace_id":    workspace.ID,
			"broadcast_id":    broadcast.ID,
			"recipient_email": apiRemoved,
			"template_id":     template.ID,
		})
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "send broadcast: %s", body)

		delivered, err := waitForEmailAndGetMessage(t, apiRemoved, 30*time.Second)
		require.NoError(t, err, "the broadcast to Y should reach Mailpit")
		signed := listUnsubscribeURL(t, delivered).Query()

		query := url.Values{
			"workspace_id": {signed.Get("wid")},
			"email":        {signed.Get("email")},
			"email_hmac":   {signed.Get("email_hmac")},
		}
		resp, err = (&http.Client{Timeout: 10 * time.Second}).Get(serverURL + "/preferences?" + query.Encode())
		require.NoError(t, err)
		body, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "preferences: %s", body)

		var preferences struct {
			ContactLists []struct {
				ListID string `json:"list_id"`
			} `json:"contact_lists"`
			Contact struct {
				ContactLists []struct {
					ListID string `json:"list_id"`
				} `json:"contact_lists"`
			} `json:"contact"`
		}
		require.NoError(t, json.Unmarshal(body, &preferences))
		var top, inContact []string
		for _, cl := range preferences.ContactLists {
			top = append(top, cl.ListID)
		}
		for _, cl := range preferences.Contact.ContactLists {
			inContact = append(inContact, cl.ListID)
		}
		assert.Equal(t, []string{listY.ID}, top)
		assert.Equal(t, []string{listY.ID}, inContact)
	})

	// --- 8. API and Liquid -----------------------------------------------------------

	t.Run("the API's contact_lists and Liquid's contact.contact_lists hold only current lists", func(t *testing.T) {
		resp, err := client.Get("/api/contacts.getByEmail", map[string]string{
			"workspace_id": workspace.ID,
			"email":        apiRemoved,
		})
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "contacts.getByEmail: %s", body)
		var got struct {
			Contact struct {
				ContactLists []struct {
					ListID string `json:"list_id"`
				} `json:"contact_lists"`
			} `json:"contact"`
		}
		require.NoError(t, json.Unmarshal(body, &got))
		var ids []string
		for _, cl := range got.Contact.ContactLists {
			ids = append(ids, cl.ListID)
		}
		sort.Strings(ids)
		assert.Equal(t, []string{listY.ID}, ids)

		subject := "b24-liquid-" + uuid.New().String()[:8]
		template, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateSubject(subject),
			testutil.WithTemplateEmailContent(`{% for l in contact.contact_lists %}[{{ l.list_id }}]{% endfor %}`))
		require.NoError(t, err)
		notification, err := factory.CreateTransactionalNotification(workspace.ID,
			testutil.WithTransactionalNotificationID("b24liquid"+tag),
			testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
				domain.TransactionalChannelEmail: domain.ChannelTemplate{
					TemplateID: template.ID,
					Settings:   map[string]interface{}{},
				},
			}))
		require.NoError(t, err)

		resp, err = client.SendTransactionalNotification(map[string]interface{}{
			"id":       notification.ID,
			"contact":  map[string]interface{}{"email": apiRemoved},
			"channels": []string{"email"},
		})
		require.NoError(t, err)
		body, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "transactional.send: %s", body)

		var html string
		testutil.WaitForCondition(t, func() bool {
			msg := b24MailpitMessage(t, apiRemoved, subject)
			if msg == nil {
				return false
			}
			html = msg.HTML
			return true
		}, 30*time.Second, "waiting for the transactional email")

		assert.Contains(t, html, "["+listY.ID+"]")
		assert.NotContains(t, html, "["+listX.ID+"]")
		assert.NotContains(t, html, "["+listZ.ID+"]")
	})
}

// b24TimelineInserts counts the contact's timeline rows of kind for list, recorded with
// operation 'insert'.
func b24TimelineInserts(t *testing.T, db *sql.DB, email, kind, listID string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM contact_timeline
		 WHERE email = $1 AND kind = $2 AND entity_id = $3 AND operation = 'insert'`,
		email, kind, listID).Scan(&n))
	return n
}

type b24Delivery struct {
	eventType string
	payload   map[string]interface{}
}

// b24Deliveries returns the deliveries queued for the subscription about email, oldest
// first.
func b24Deliveries(t *testing.T, db *sql.DB, subscriptionID, email string) []b24Delivery {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), `
		SELECT event_type, payload FROM webhook_deliveries
		WHERE subscription_id = $1 AND payload->>'email' = $2
		ORDER BY created_at, id`, subscriptionID, email)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	var out []b24Delivery
	for rows.Next() {
		var d b24Delivery
		var raw []byte
		require.NoError(t, rows.Scan(&d.eventType, &raw))
		require.NoError(t, json.Unmarshal(raw, &d.payload))
		out = append(out, d)
	}
	require.NoError(t, rows.Err())
	return out
}

// b24MailpitMessage returns the full message Mailpit holds for recipient with subject,
// or nil.
func b24MailpitMessage(t *testing.T, recipient, subject string) *testutil.MailpitMessage {
	t.Helper()
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Get("http://localhost:8025/api/v1/messages?limit=1000")
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	var messages testutil.MailpitMessagesResponse
	if json.NewDecoder(resp.Body).Decode(&messages) != nil {
		return nil
	}
	for _, m := range messages.Messages {
		if m.Subject != subject {
			continue
		}
		for _, to := range m.To {
			if strings.EqualFold(to.Address, recipient) {
				full, err := testutil.GetMailpitMessage(t, m.ID)
				if err != nil {
					return nil
				}
				return full
			}
		}
	}
	return nil
}
