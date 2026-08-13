package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/cache"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Characterization tests for WebAnalyticsContactBridge.
//
// These describe what the bridge does today, not what it ought to do. They exist
// so the goal-type work can change one thing deliberately and see everything else
// stay still.
//
// Two conventions worth knowing before reading them:
//
//   - The returned map is a CURSOR, not a success report. A goal in it is one the
//     caller will never offer again — which covers both "written" and "can never
//     be written". A goal absent from it comes back on the next flush, because the
//     SDK re-sends its whole cumulative action list.
//   - The bridge links, it never creates. An address that is not a contact yet is
//     left for a later flush, in case the visitor identifies mid-visit.

const bridgeTestWorkspace = "ws-1"

// bridgeHarness keeps the mocks beside the bridge so a test can both drive it and
// inspect what came out.
type bridgeHarness struct {
	bridge    *WebAnalyticsContactBridge
	contacts  *mocks.MockContactRepository
	events    *mocks.MockCustomEventRepository
	now       time.Time
	collected [][]*domain.CustomEvent
}

// newBridgeForTest builds a bridge with a frozen clock.
//
// The cache Stop is not optional: the constructor hard-codes
// cache.NewInMemoryCache(5*time.Minute), which spawns a janitor goroutine per
// instance, and a test file with a few dozen cases would otherwise leak one per
// case for the lifetime of the run.
func newBridgeForTest(t *testing.T) *bridgeHarness {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	logger := pkgmocks.NewMockLogger(ctrl)
	logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Info(gomock.Any()).AnyTimes()
	logger.EXPECT().Debug(gomock.Any()).AnyTimes()
	logger.EXPECT().Warn(gomock.Any()).AnyTimes()
	logger.EXPECT().Error(gomock.Any()).AnyTimes()

	h := &bridgeHarness{
		contacts: mocks.NewMockContactRepository(ctrl),
		events:   mocks.NewMockCustomEventRepository(ctrl),
		now:      time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC),
	}
	h.bridge = NewWebAnalyticsContactBridge(h.contacts, h.events, logger)
	h.bridge.nowFn = func() time.Time { return h.now }
	t.Cleanup(h.bridge.contactCache.Stop)

	return h
}

// expectInsert captures the events handed to the repository so assertions can be
// made on their content rather than only on whether a call happened.
func (h *bridgeHarness) expectInsert(times int, err error) {
	h.events.EXPECT().
		BatchInsertNew(gomock.Any(), bridgeTestWorkspace, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, events []*domain.CustomEvent) error {
			h.collected = append(h.collected, events)
			return err
		}).
		Times(times)
}

func (h *bridgeHarness) inserted() []*domain.CustomEvent {
	var all []*domain.CustomEvent
	for _, batch := range h.collected {
		all = append(all, batch...)
	}
	return all
}

// contactExists arms the lookup for an address, once per distinct address: the
// bridge caches definitive answers for 60s and the clock is frozen.
func (h *bridgeHarness) contactExists(email string) {
	h.contacts.EXPECT().
		GetContactByEmail(gomock.Any(), bridgeTestWorkspace, email).
		Return(&domain.Contact{Email: email}, nil).
		Times(1)
}

func (h *bridgeHarness) contactMissing(email string) {
	h.contacts.EXPECT().
		GetContactByEmail(gomock.Any(), bridgeTestWorkspace, email).
		Return(nil, domain.ErrContactNotFound).
		Times(1)
}

func strptr(s string) *string { return &s }

// goalAt builds a goal whose stored timestamp is `offset` from the frozen now.
func (h *bridgeHarness) goalAt(name, email string, offset time.Duration) *domain.WebGoal {
	g := &domain.WebGoal{
		SessionID:    "s1",
		TabID:        1,
		GoalName:     name,
		ClientTsMs:   h.now.Add(offset).UnixMilli(),
		GoalAt:       h.now.Add(offset),
		ContactEmail: strptr(email),
	}
	return g
}

// --- normalizeWebGoalEventName ----------------------------------------------

func TestNormalizeWebGoalEventName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"lowercases", "Purchase", "purchase"},
		{"trims and joins words", "  Add To Cart  ", "add_to_cart"},
		{"collapses runs of separators", "add--to__cart", "add_to_cart"},
		{"keeps dots and slashes", "order.completed/v2", "order.completed/v2"},
		{"drops accented characters", "Café", "caf"},
		{"drops a fully non-ascii name", "購入", ""},
		{"drops a name with nothing usable", "!!!", ""},
		{"never leads or trails with an underscore", "__signup__", "signup"},
		{"empty stays empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, normalizeWebGoalEventName(tc.in))
		})
	}
}

// The cap is not cosmetic: contact_timeline.kind is VARCHAR(150) and the trigger
// prefixes "custom_event.", which leaves exactly 100.
func TestNormalizeWebGoalEventNameCapsLength(t *testing.T) {
	got := normalizeWebGoalEventName(strings.Repeat("a", 250))
	assert.Len(t, got, domain.WebTrackMaxGoalNameLength)
	assert.Equal(t, 100, domain.WebTrackMaxGoalNameLength,
		"if this constant moves, the VARCHAR(150) budget needs rechecking")
}

// --- Which goals get bridged -------------------------------------------------

func TestWebAnalyticsContactBridgeSkipsAnonymousGoals(t *testing.T) {
	h := newBridgeForTest(t)
	h.events.EXPECT().BatchInsertNew(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	noEmail := h.goalAt("purchase", "", 0)
	noEmail.ContactEmail = nil
	empty := h.goalAt("purchase", "", 0)

	written := h.bridge.EmitGoals(context.Background(), bridgeTestWorkspace, []*domain.WebGoal{noEmail, empty})

	// Absent, not present-and-false. The caller only ever tests !written[goal], so
	// the two are equivalent to it — this pins which one the bridge actually does.
	assert.NotContains(t, written, noEmail, "an anonymous goal must be retried once the visitor identifies")
	assert.NotContains(t, written, empty)
}

func TestWebAnalyticsContactBridgeAgeWindow(t *testing.T) {
	cases := []struct {
		name    string
		offset  time.Duration
		bridged bool
	}{
		{"just inside the past window", -webBridgeMaxGoalAge + time.Minute, true},
		{"just outside the past window", -webBridgeMaxGoalAge - time.Minute, false},
		{"slightly in the future is tolerated", 59 * time.Minute, true},
		{"too far in the future", 61 * time.Minute, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newBridgeForTest(t)
			goal := h.goalAt("purchase", "a@example.com", tc.offset)

			if tc.bridged {
				h.contactExists("a@example.com")
				h.expectInsert(1, nil)
			} else {
				h.events.EXPECT().BatchInsertNew(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			}

			written := h.bridge.EmitGoals(context.Background(), bridgeTestWorkspace, []*domain.WebGoal{goal})

			// Marked either way: a goal that is bridgeable is done, and one that is
			// too old can never become bridgeable, so re-examining it every flush
			// for the rest of the visit would be pure waste.
			assert.True(t, written[goal])
		})
	}
}

// A zero ClientTsMs is the 1970 epoch, which is outside the window by fifty-odd
// years — worth pinning because it is what an unset field looks like.
func TestWebAnalyticsContactBridgeSkipsZeroTimestamp(t *testing.T) {
	h := newBridgeForTest(t)
	h.events.EXPECT().BatchInsertNew(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	goal := h.goalAt("purchase", "a@example.com", 0)
	goal.ClientTsMs = 0

	written := h.bridge.EmitGoals(context.Background(), bridgeTestWorkspace, []*domain.WebGoal{goal})
	assert.True(t, written[goal])
}

func TestWebAnalyticsContactBridgeSkipsUnnormalizableNames(t *testing.T) {
	h := newBridgeForTest(t)
	h.contactExists("a@example.com")
	h.expectInsert(1, nil)

	junk := h.goalAt("!!!", "a@example.com", 0)
	valid := h.goalAt("purchase", "a@example.com", 0)

	written := h.bridge.EmitGoals(context.Background(), bridgeTestWorkspace, []*domain.WebGoal{junk, valid})

	assert.True(t, written[junk], "a name with nothing usable in it can never be bridged")
	assert.True(t, written[valid])
	require.Len(t, h.inserted(), 1, "the junk goal must not poison the rest of the batch")
	assert.Equal(t, "purchase", h.inserted()[0].EventName)
}

// The bridge links to an existing contact and never creates one. The mock carries
// no other EXPECT, so gomock fails the test if any creation method is reached.
func TestWebAnalyticsContactBridgeLinksOnlyNeverCreates(t *testing.T) {
	h := newBridgeForTest(t)
	h.contactMissing("ghost@example.com")
	h.events.EXPECT().BatchInsertNew(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	goal := h.goalAt("purchase", "ghost@example.com", 0)

	written := h.bridge.EmitGoals(context.Background(), bridgeTestWorkspace, []*domain.WebGoal{goal})

	assert.NotContains(t, written, goal,
		"not marked, so a later flush retries it if the visitor becomes a contact mid-visit")
}

// --- The per-session cap -----------------------------------------------------

// Pinned as it behaves today: the counter lives in EmitGoals, so it bounds one
// write rather than a session's lifetime. Sustained-rate protection is the rate
// limiter's job.
func TestWebAnalyticsContactBridgePerSessionBatchCap(t *testing.T) {
	h := newBridgeForTest(t)
	h.contactExists("a@example.com")
	h.expectInsert(1, nil)

	total := webBridgeMaxGoalsPerSessionPerBatch + 5
	goals := make([]*domain.WebGoal, 0, total)
	for i := 0; i < total; i++ {
		goals = append(goals, h.goalAt(fmt.Sprintf("goal%d", i), "a@example.com", 0))
	}

	written := h.bridge.EmitGoals(context.Background(), bridgeTestWorkspace, goals)

	// The literal, not the constant: asserting against the constant makes the
	// test move with it, so changing the cap would pass silently.
	assert.Equal(t, 100, webBridgeMaxGoalsPerSessionPerBatch,
		"the cap is a deliberate value; moving it should require updating this test")
	assert.Len(t, h.inserted(), 100)
	assert.Len(t, written, total, "the goals over the cap are marked so a hostile session is not retried forever")
}

// The cap is per session, so one noisy session must not silently discard another
// visitor's conversions in the same batch.
func TestWebAnalyticsContactBridgeCapIsPerSessionNotPerBatch(t *testing.T) {
	h := newBridgeForTest(t)
	h.contactExists("noisy@example.com")
	h.contactExists("quiet@example.com")
	h.expectInsert(1, nil)

	var goals []*domain.WebGoal
	for i := 0; i < webBridgeMaxGoalsPerSessionPerBatch+5; i++ {
		g := h.goalAt(fmt.Sprintf("goal%d", i), "noisy@example.com", 0)
		g.SessionID = "noisy"
		goals = append(goals, g)
	}
	quiet := h.goalAt("purchase", "quiet@example.com", 0)
	quiet.SessionID = "quiet"
	goals = append(goals, quiet)

	h.bridge.EmitGoals(context.Background(), bridgeTestWorkspace, goals)

	var sawQuiet bool
	for _, e := range h.inserted() {
		if e.Email == "quiet@example.com" {
			sawQuiet = true
		}
	}
	assert.True(t, sawQuiet, "a second session's conversion must survive a first session hitting the cap")
}

// --- What lands in the event -------------------------------------------------

// A visitor must not be able to forge server-side context by sending it as a goal
// property. Spreading the maps the other way round would let them.
func TestWebAnalyticsContactBridgeRejectsForgedProperties(t *testing.T) {
	h := newBridgeForTest(t)
	h.contactExists("a@example.com")
	h.expectInsert(1, nil)

	goal := h.goalAt("purchase", "a@example.com", 0)
	goal.Path = "/real"
	goal.UTMSource = "real-source"
	goal.Country = "FR"
	goal.Properties = map[string]string{
		"session_id": "forged",
		"path":       "/forged",
		"utm_source": "forged-source",
		"country":    "XX",
		"basket":     "3 items",
	}

	h.bridge.EmitGoals(context.Background(), bridgeTestWorkspace, []*domain.WebGoal{goal})

	require.Len(t, h.inserted(), 1)
	props := h.inserted()[0].Properties
	assert.Equal(t, "s1", props["session_id"])
	assert.Equal(t, "/real", props["path"])
	assert.Equal(t, "real-source", props["utm_source"])
	assert.Equal(t, "FR", props["country"])
	assert.Equal(t, "3 items", props["basket"], "an unrelated client property must survive")
}

// channel, channel_group and custom_N are omitted on purpose: the attribution
// backfill rewrites them on historical rows, and nothing would re-emit this
// frozen copy, so segment authors would compare a stale snapshot to a live
// dashboard.
func TestWebAnalyticsContactBridgeOmitsAttributionSnapshot(t *testing.T) {
	h := newBridgeForTest(t)
	h.contactExists("a@example.com")
	h.expectInsert(1, nil)

	goal := h.goalAt("purchase", "a@example.com", 0)
	goal.Channel = "search-paid"
	goal.ChannelGroup = "paid"
	goal.Custom1 = "vip"

	h.bridge.EmitGoals(context.Background(), bridgeTestWorkspace, []*domain.WebGoal{goal})

	require.Len(t, h.inserted(), 1)
	props := h.inserted()[0].Properties
	for _, key := range []string{"channel", "channel_group", "custom_1", "custom_3", "custom_10"} {
		assert.NotContains(t, props, key)
	}
}

// The external id is the web_goals primary key, so a replayed beat dedups onto
// the same row. It uses the RAW goal name, not the normalized one.
func TestWebAnalyticsContactBridgeExternalIDShape(t *testing.T) {
	h := newBridgeForTest(t)
	h.contactExists("a@example.com")
	h.expectInsert(1, nil)

	goal := h.goalAt("Add To Cart", "a@example.com", 0)
	goal.TabID = 3

	h.bridge.EmitGoals(context.Background(), bridgeTestWorkspace, []*domain.WebGoal{goal})

	require.Len(t, h.inserted(), 1)
	event := h.inserted()[0]
	assert.Equal(t, fmt.Sprintf("s1:3:Add To Cart:%d", goal.ClientTsMs), event.ExternalID)
	assert.LessOrEqual(t, len(event.ExternalID), 255, "CustomEvent.Validate rejects anything longer")
	assert.Equal(t, "add_to_cart", event.EventName, "the event name IS normalized, unlike the external id")
}

// OccurredAt is the client's own timestamp, not the skew-corrected GoalAt: skew
// is recomputed per beat, so GoalAt drifts and a replay would look like a newer
// event.
func TestWebAnalyticsContactBridgeUsesClientTimestampNotSkewCorrected(t *testing.T) {
	h := newBridgeForTest(t)
	h.contactExists("a@example.com")
	h.expectInsert(1, nil)

	goal := h.goalAt("purchase", "a@example.com", -time.Hour)
	goal.GoalAt = h.now.Add(-30 * time.Minute) // drifted by a later beat

	h.bridge.EmitGoals(context.Background(), bridgeTestWorkspace, []*domain.WebGoal{goal})

	require.Len(t, h.inserted(), 1)
	assert.Equal(t, time.UnixMilli(goal.ClientTsMs).UTC(), h.inserted()[0].OccurredAt)
}

func TestWebAnalyticsContactBridgeGoalValue(t *testing.T) {
	cases := []struct {
		name  string
		value float64
		want  *float64
	}{
		{"a positive value is carried", 49.9, func() *float64 { v := 49.9; return &v }()},
		{"zero is left unset", 0, nil},
		{"a negative value is left unset", -10, nil},
		{
			"an overflowing value is clamped to what DECIMAL(15,2) holds",
			1e20,
			func() *float64 { v := webBridgeMaxGoalValue; return &v }(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newBridgeForTest(t)
			h.contactExists("a@example.com")
			h.expectInsert(1, nil)

			goal := h.goalAt("purchase", "a@example.com", 0)
			goal.GoalValue = tc.value

			h.bridge.EmitGoals(context.Background(), bridgeTestWorkspace, []*domain.WebGoal{goal})

			require.Len(t, h.inserted(), 1)
			got := h.inserted()[0].GoalValue
			if tc.want == nil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.InDelta(t, *tc.want, *got, 0.001)
		})
	}
}

func TestWebAnalyticsContactBridgeStampsSource(t *testing.T) {
	h := newBridgeForTest(t)
	h.contactExists("a@example.com")
	h.expectInsert(1, nil)

	h.bridge.EmitGoals(context.Background(), bridgeTestWorkspace,
		[]*domain.WebGoal{h.goalAt("purchase", "a@example.com", 0)})

	require.Len(t, h.inserted(), 1)
	event := h.inserted()[0]
	assert.Equal(t, "web_analytics", event.Source)
	assert.Equal(t, "a@example.com", event.Email)
	require.NotNil(t, event.GoalName)
	assert.Equal(t, "purchase", *event.GoalName, "GoalName carries the NORMALIZED name, matching EventName")
}

// --- Failure and guard behaviour ---------------------------------------------

// A failed insert must retry only what could succeed. Goals rejected before the
// insert can never succeed, so they stay marked.
func TestWebAnalyticsContactBridgeInsertFailureRetriesOnlyTheRecoverable(t *testing.T) {
	h := newBridgeForTest(t)
	h.contactExists("a@example.com")
	h.expectInsert(1, errors.New("deadlock detected"))

	tooOld := h.goalAt("purchase", "a@example.com", -48*time.Hour)
	valid := h.goalAt("signup", "a@example.com", 0)

	written := h.bridge.EmitGoals(context.Background(), bridgeTestWorkspace,
		[]*domain.WebGoal{tooOld, valid})

	assert.True(t, written[tooOld], "a permanently unbridgeable goal stays marked even when the insert fails")
	assert.NotContains(t, written, valid, "the recoverable goal must come back on the next flush")
}

// The caller ranges over the result, so it must never be nil.
func TestWebAnalyticsContactBridgeEmitGoalsGuards(t *testing.T) {
	goal := &domain.WebGoal{}

	t.Run("nil receiver", func(t *testing.T) {
		var b *WebAnalyticsContactBridge
		got := b.EmitGoals(context.Background(), bridgeTestWorkspace, []*domain.WebGoal{goal})
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})

	t.Run("nil event repository", func(t *testing.T) {
		b := &WebAnalyticsContactBridge{}
		got := b.EmitGoals(context.Background(), bridgeTestWorkspace, []*domain.WebGoal{goal})
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})

	t.Run("no goals", func(t *testing.T) {
		h := newBridgeForTest(t)
		got := h.bridge.EmitGoals(context.Background(), bridgeTestWorkspace, nil)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})
}

// --- webContactExists --------------------------------------------------------

// Only a definitive answer is cached. Caching a transient failure would drop
// every identity for that address for the whole TTL over one blip.
func TestWebContactExistsCachesOnlyDefinitiveAnswers(t *testing.T) {
	t.Run("a hit is cached", func(t *testing.T) {
		h := newBridgeForTest(t)
		h.contactExists("a@example.com") // Times(1)

		for i := 0; i < 3; i++ {
			assert.True(t, h.bridge.contactExists(context.Background(), bridgeTestWorkspace, "a@example.com"))
		}
	})

	t.Run("a definitive absence is cached", func(t *testing.T) {
		h := newBridgeForTest(t)
		h.contactMissing("ghost@example.com") // Times(1)

		for i := 0; i < 3; i++ {
			assert.False(t, h.bridge.contactExists(context.Background(), bridgeTestWorkspace, "ghost@example.com"))
		}
	})

	t.Run("sql.ErrNoRows counts as definitive", func(t *testing.T) {
		h := newBridgeForTest(t)
		h.contacts.EXPECT().
			GetContactByEmail(gomock.Any(), bridgeTestWorkspace, "ghost@example.com").
			Return(nil, sql.ErrNoRows).
			Times(1)

		for i := 0; i < 2; i++ {
			assert.False(t, h.bridge.contactExists(context.Background(), bridgeTestWorkspace, "ghost@example.com"))
		}
	})

	t.Run("a transient error is not cached", func(t *testing.T) {
		h := newBridgeForTest(t)
		h.contacts.EXPECT().
			GetContactByEmail(gomock.Any(), bridgeTestWorkspace, "a@example.com").
			Return(nil, errors.New("connection refused")).
			Times(3)

		for i := 0; i < 3; i++ {
			assert.False(t, h.bridge.contactExists(context.Background(), bridgeTestWorkspace, "a@example.com"))
		}
	})
}

// The cache key must separate prefix, workspace and address, or one workspace's
// answer would satisfy another's lookup.
func TestWebContactExistsKeyIsolation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockContactRepository(ctrl)
	c := cache.NewInMemoryCache(5 * time.Minute)
	defer c.Stop()

	repo.EXPECT().
		GetContactByEmail(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&domain.Contact{}, nil).
		Times(3)

	ctx := context.Background()
	assert.True(t, webContactExists(ctx, c, repo, "bridge:", "ws-1", "a@example.com"))
	assert.True(t, webContactExists(ctx, c, repo, "identity:", "ws-1", "a@example.com"), "a different prefix must miss")
	assert.True(t, webContactExists(ctx, c, repo, "bridge:", "ws-2", "a@example.com"), "a different workspace must miss")

	// The first key is now warm and must not hit the repository again.
	assert.True(t, webContactExists(ctx, c, repo, "bridge:", "ws-1", "a@example.com"))
}

func TestWebContactExistsGuards(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	c := cache.NewInMemoryCache(time.Minute)
	defer c.Stop()

	assert.False(t, webContactExists(context.Background(), c, nil, "bridge:", "ws-1", "a@example.com"))
	assert.False(t, webContactExists(context.Background(), nil,
		mocks.NewMockContactRepository(ctrl), "bridge:", "ws-1", "a@example.com"))
}

// --- Goal type ---------------------------------------------------------------

func TestWebAnalyticsContactBridgeStampsDeclaredGoalType(t *testing.T) {
	for _, goalType := range domain.ValidGoalTypes {
		t.Run(goalType, func(t *testing.T) {
			h := newBridgeForTest(t)
			h.contactExists("a@example.com")
			h.expectInsert(1, nil)

			goal := h.goalAt("purchase", "a@example.com", 0)
			goal.GoalType = goalType

			h.bridge.EmitGoals(context.Background(), bridgeTestWorkspace, []*domain.WebGoal{goal})

			require.Len(t, h.inserted(), 1)
			require.NotNil(t, h.inserted()[0].GoalType)
			assert.Equal(t, goalType, *h.inserted()[0].GoalType)
		})
	}
}

// A goal buffered before the type shipped, or sent by a client that omitted it,
// must still be segmentable — the wildcard "All types" condition filters on
// goal_type IS NOT NULL, so a nil here means the conversion can never match.
func TestWebAnalyticsContactBridgeDefaultsGoalTypeToOther(t *testing.T) {
	h := newBridgeForTest(t)
	h.contactExists("a@example.com")
	h.expectInsert(1, nil)

	goal := h.goalAt("purchase", "a@example.com", 0)
	goal.GoalType = ""

	h.bridge.EmitGoals(context.Background(), bridgeTestWorkspace, []*domain.WebGoal{goal})

	require.Len(t, h.inserted(), 1)
	require.NotNil(t, h.inserted()[0].GoalType, "a nil type is invisible to every goal segment condition")
	assert.Equal(t, domain.GoalTypeOther, *h.inserted()[0].GoalType)
}

// The type must be stamped outside the `if goal.GoalValue > 0` guard. A lead or a
// signup usually carries no value, and typing only the conversions with money
// attached would leave exactly the wrong half of the funnel unsegmentable.
func TestWebAnalyticsContactBridgeTypesValuelessGoalsToo(t *testing.T) {
	h := newBridgeForTest(t)
	h.contactExists("a@example.com")
	h.expectInsert(1, nil)

	goal := h.goalAt("newsletter_signup", "a@example.com", 0)
	goal.GoalType = "signup"
	goal.GoalValue = 0

	h.bridge.EmitGoals(context.Background(), bridgeTestWorkspace, []*domain.WebGoal{goal})

	require.Len(t, h.inserted(), 1)
	event := h.inserted()[0]
	assert.Nil(t, event.GoalValue, "no value was declared")
	require.NotNil(t, event.GoalType, "but the goal must still be typed")
	assert.Equal(t, "signup", *event.GoalType)
}

// Each event owns its type. A shared pointer would make every goal in a batch
// report whichever type happened to be written last.
func TestWebAnalyticsContactBridgeGoalTypePointersAreNotShared(t *testing.T) {
	h := newBridgeForTest(t)
	h.contactExists("a@example.com")
	h.expectInsert(1, nil)

	purchase := h.goalAt("purchase", "a@example.com", 0)
	purchase.GoalType = domain.GoalTypePurchase
	lead := h.goalAt("demo_request", "a@example.com", 0)
	lead.GoalType = "lead"

	h.bridge.EmitGoals(context.Background(), bridgeTestWorkspace, []*domain.WebGoal{purchase, lead})

	require.Len(t, h.inserted(), 2)
	byName := map[string]string{}
	for _, e := range h.inserted() {
		require.NotNil(t, e.GoalType)
		byName[e.EventName] = *e.GoalType
	}
	assert.Equal(t, domain.GoalTypePurchase, byName["purchase"])
	assert.Equal(t, "lead", byName["demo_request"])
}
