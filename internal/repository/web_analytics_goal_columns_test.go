package repository

import (
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// webGoalColumns and webGoalValues are two hand-maintained lists that must stay
// in lockstep: Squirrel binds them by position, so inserting a column in one and
// forgetting the other does not fail to compile and does not fail to run — it
// silently writes every value into the wrong column.
//
// These tests are cheap insurance against the next column.

func TestWebGoalColumnsAndValuesStayAligned(t *testing.T) {
	values, err := webGoalValues(&domain.WebGoal{})
	require.NoError(t, err)

	assert.Equal(t, len(webGoalColumns), len(values),
		"a column was added to one list and not the other; every value after it is now bound to the wrong column")
}

func TestWebGoalValuesBindEachColumnToItsOwnField(t *testing.T) {
	at := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	goal := &domain.WebGoal{
		SessionDate: at,
		SessionID:   "11111111-1111-1111-1111-111111111111",
		TabID:       3,
		GoalName:    "purchase",
		ClientTsMs:  1786190340000,
		BeatSeq:     7,
		GoalAt:      at,
		GoalValue:   49.9,
		GoalType:    domain.GoalTypePurchase,
		Path:        "/checkout",
		PageNumber:  2,
		Country:     "FR",
	}

	values, err := webGoalValues(goal)
	require.NoError(t, err)
	require.Equal(t, len(webGoalColumns), len(values))

	at_ := func(column string) interface{} {
		for i, c := range webGoalColumns {
			if c == column {
				return values[i]
			}
		}
		t.Fatalf("column %q is not in webGoalColumns", column)
		return nil
	}

	// goal_type is the reason this file exists, but pinning its neighbours is
	// what proves nothing shifted around it.
	assert.Equal(t, domain.GoalTypePurchase, at_("goal_type"))
	assert.Equal(t, float64(49.9), at_("goal_value"))
	assert.Equal(t, "/checkout", at_("path"))
	assert.Equal(t, "purchase", at_("goal_name"))
	assert.Equal(t, int64(7), at_("beat_seq"))
	assert.Equal(t, "FR", at_("country"))
}

// An untyped goal must reach the database as '' and never as NULL: the column is
// NOT NULL, and a nil would abort the whole flush — losing every other visitor's
// conversions in the same batch.
func TestWebGoalValuesNeverBindNullGoalType(t *testing.T) {
	values, err := webGoalValues(&domain.WebGoal{})
	require.NoError(t, err)

	for i, c := range webGoalColumns {
		if c == "goal_type" {
			require.NotNil(t, values[i])
			assert.Equal(t, "", values[i])
			return
		}
	}
	t.Fatal("goal_type is missing from webGoalColumns")
}
