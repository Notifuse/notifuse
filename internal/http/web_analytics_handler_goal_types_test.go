package http

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// TestWebAnalyticsHandlerTrackWronglyTypedGoals posts raw beats through the
// real /track handler, typed the way plain-JS snippet and tag-manager callers
// type them. A goal carrying a numeric or boolean property, or a string value,
// used to fail json.Decode of the whole beat with a 400. The SDK treats a 4xx
// as permanent and re-sends its cumulative actions[] on every beat, so that
// one goal lost every later pageview and goal of the session.
func TestWebAnalyticsHandlerTrackWronglyTypedGoals(t *testing.T) {
	sessionID, err := uuid.NewV7()
	require.NoError(t, err)
	now := time.Now().UnixMilli()

	body := func(goalExtra string) string {
		return fmt.Sprintf(`{"workspace_id":"ws1","session_id":%q,"actions":[`+
			`{"type":"pageview","path":"/checkout","page_number":1,"duration":1473.4,"scroll":0,"entered_at":%d,"exited_at":%d},`+
			`{"type":"goal","name":"purchase","goal_type":"purchase","path":"/checkout","page_number":1,"timestamp":%d,%s},`+
			`{"type":"goal","name":"newsletter","goal_type":"lead","path":"/checkout","page_number":1,"timestamp":%d}`+
			`],"created_at":%d,"updated_at":%d,"sdk_version":"41.1","seq":2}`,
			sessionID.String(), now-2000, now-500, now-400, goalExtra, now-300, now-2000, now)
	}

	// Track runs Validate exactly as the real service does, so what reaches
	// it here is what would be stored.
	track := func(t *testing.T, raw string) (int, *domain.WebTrackPayload) {
		t.Helper()
		_, svc, mux := newWebAnalyticsHandlerForTest(t, nil)
		var got *domain.WebTrackPayload
		svc.EXPECT().Track(gomock.Any(), gomock.Any(), gomock.Any()).MaxTimes(1).
			DoAndReturn(func(_ context.Context, payload *domain.WebTrackPayload, _ domain.WebRequestMeta) error {
				require.NoError(t, payload.Validate(time.Now()))
				got = payload
				return nil
			})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, trackRequest(t, raw, map[string]string{"Content-Type": "text/plain;charset=UTF-8"}))
		return rec.Code, got
	}

	t.Run("numeric and boolean properties are coerced to strings", func(t *testing.T) {
		code, p := track(t, body(`"value":49.9,"properties":{"order_id":12345,"first_order":true,"currency":"EUR"}`))
		require.Equal(t, http.StatusOK, code)
		require.NotNil(t, p)
		require.Len(t, p.Actions, 3)
		assert.Equal(t, map[string]string{"order_id": "12345", "first_order": "true", "currency": "EUR"}, p.Actions[1].Properties)
		assert.Equal(t, 49.9, p.Actions[1].Value)
		assert.Equal(t, "newsletter", p.Actions[2].Name)
	})

	t.Run("a numeric string value is coerced to a number", func(t *testing.T) {
		code, p := track(t, body(`"value":"49.90"`))
		require.Equal(t, http.StatusOK, code)
		require.NotNil(t, p)
		require.Len(t, p.Actions, 3)
		assert.Equal(t, 49.90, p.Actions[1].Value)
	})

	t.Run("a goal that cannot be coerced is dropped, the rest of the beat kept", func(t *testing.T) {
		code, p := track(t, body(`"value":"forty-nine","properties":{"items":[{"sku":"A1"}]}`))
		require.Equal(t, http.StatusOK, code)
		require.NotNil(t, p)
		require.Len(t, p.Actions, 2)
		assert.Equal(t, "pageview", p.Actions[0].Type)
		assert.Equal(t, int64(1473), p.Actions[0].Duration)
		assert.Equal(t, "newsletter", p.Actions[1].Name)
	})

	t.Run("a beat that is not JSON is still a 400", func(t *testing.T) {
		code, p := track(t, `{"workspace_id":"ws1","actions":[`)
		assert.Equal(t, http.StatusBadRequest, code)
		assert.Nil(t, p)
	})
}
