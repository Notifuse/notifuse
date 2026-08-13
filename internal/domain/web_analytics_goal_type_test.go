package domain

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Goal types on the wire.
//
// The SDK makes `type` a required argument and throws without it, so a site we
// control cannot send an untyped goal. The wire is deliberately the opposite:
// anything unrecognised normalises to "other" and the action is still accepted.
//
// That asymmetry is the whole point. /na.<hash>.js is served with a one-year
// immutable cache, so a site pinned to a stale bundle will keep sending payloads
// shaped the old way for a long time. dropInvalidActions removes a failing action
// SILENTLY — no error reaches the site — so strict wire validation would turn a
// stale bundle into permanently lost conversions with nothing to show for it.
// Strict where we control the call, forgiving where we do not.

func validGoalAction() WebTrackAction {
	return WebTrackAction{
		Type:       WebActionTypeGoal,
		Path:       "/checkout",
		PageNumber: 1,
		Name:       "purchase",
		Timestamp:  1786190340000,
		Value:      49.9,
	}
}

func TestWebTrackActionNormalizesGoalType(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"absent defaults to other", "", GoalTypeOther},
		{"a valid type is kept", GoalTypePurchase, GoalTypePurchase},
		{"case is normalized", "Purchase", GoalTypePurchase},
		{"shouting is normalized", "SUBSCRIPTION", "subscription"},
		{"surrounding space is trimmed", "  lead  ", "lead"},
		{"an unknown type becomes other", "nonsense", GoalTypeOther},
		{"a near-miss becomes other", "purchases", GoalTypeOther},
		{"whitespace only becomes other", "   ", GoalTypeOther},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := validGoalAction()
			a.GoalType = tc.in

			require.NoError(t, a.Validate(), "a goal type must NEVER invalidate the action")
			assert.Equal(t, tc.want, a.GoalType)
		})
	}
}

// Every type the API accepts must survive the web path too, or a site could
// declare a type that its own segments cannot then match.
func TestWebTrackActionAcceptsEveryValidGoalType(t *testing.T) {
	for _, goalType := range ValidGoalTypes {
		t.Run(goalType, func(t *testing.T) {
			a := validGoalAction()
			a.GoalType = goalType

			require.NoError(t, a.Validate())
			assert.Equal(t, goalType, a.GoalType, "a type the API accepts must round-trip unchanged")
		})
	}
}

// The lenient-wire contract, stated where it is actually enforced: a payload
// carrying a nonsense goal type must keep the action, not drop it.
func TestWebTrackPayloadKeepsGoalsWithUnknownType(t *testing.T) {
	bad := validGoalAction()
	bad.Name = "signup"
	bad.GoalType = "not-a-real-type"

	p := &WebTrackPayload{Actions: []WebTrackAction{validGoalAction(), bad}}
	p.dropInvalidActions()

	require.Len(t, p.Actions, 2, "an unrecognised goal type must not cost the conversion")
	assert.Equal(t, GoalTypeOther, p.Actions[1].GoalType,
		"normalization must survive into the payload, not just the local copy")
}

// A pageview carries no goal type, and validating one must not invent one.
func TestWebTrackActionPageviewGetsNoGoalType(t *testing.T) {
	a := WebTrackAction{Type: WebActionTypePageview, Path: "/", PageNumber: 1}
	require.NoError(t, a.Validate())
	assert.Empty(t, a.GoalType)
}

// WebTrackAction.Type is already the action discriminator ("goal"), so the goal's
// own type has to travel under a different key. This pins that the two fields are
// independent and neither shadows the other.
func TestWebTrackActionGoalTypeIsDistinctFromActionType(t *testing.T) {
	in := validGoalAction()
	in.GoalType = GoalTypePurchase

	raw, err := json.Marshal(in)
	require.NoError(t, err)

	var asMap map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &asMap))
	assert.Equal(t, WebActionTypeGoal, asMap["type"], "the action discriminator")
	assert.Equal(t, GoalTypePurchase, asMap["goal_type"], "the goal's own type")

	var out WebTrackAction
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Equal(t, WebActionTypeGoal, out.Type)
	assert.Equal(t, GoalTypePurchase, out.GoalType)
}

// Written from an API client's perspective: a struct literal cannot express a
// field the caller never sent, and that distinction is exactly what this is about.
func TestWebTrackActionGoalTypeFromRawJSON(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			"field omitted entirely",
			`{"type":"goal","path":"/c","page_number":1,"name":"purchase","timestamp":1786190340000}`,
			GoalTypeOther,
		},
		{
			"field present but empty",
			`{"type":"goal","path":"/c","page_number":1,"name":"purchase","timestamp":1786190340000,"goal_type":""}`,
			GoalTypeOther,
		},
		{
			"field present and valid",
			`{"type":"goal","path":"/c","page_number":1,"name":"purchase","timestamp":1786190340000,"goal_type":"lead"}`,
			"lead",
		},
		{
			"field present and junk",
			`{"type":"goal","path":"/c","page_number":1,"name":"purchase","timestamp":1786190340000,"goal_type":"../../etc/passwd"}`,
			GoalTypeOther,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var a WebTrackAction
			require.NoError(t, json.Unmarshal([]byte(tc.body), &a))
			require.NoError(t, a.Validate())
			assert.Equal(t, tc.want, a.GoalType)
		})
	}
}

// An absurdly long value must not reach the database as-is.
func TestWebTrackActionGoalTypeRejectsOversizedInput(t *testing.T) {
	a := validGoalAction()
	a.GoalType = strings.Repeat("x", 10_000)

	require.NoError(t, a.Validate(), "still never fails the action")
	assert.Equal(t, GoalTypeOther, a.GoalType)
}
