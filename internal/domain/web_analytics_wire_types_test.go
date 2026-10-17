package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWebTrackPayloadDecodesWrongWireTypes feeds raw JSON beats the way a
// snippet or tag-manager caller actually produces them: plain JS, no types.
// A numeric or boolean property, or a string value, used to fail json.Decode
// of the whole beat — before dropInvalidActions could run — and since
// actions[] is cumulative every later beat of the session was rejected too.
// The blast radius of a wrongly typed goal must be that goal at most.
func TestWebTrackPayloadDecodesWrongWireTypes(t *testing.T) {
	now := time.Date(2026, 8, 8, 15, 0, 0, 0, time.UTC)
	ts := now.UnixMilli()

	beat := func(goal string) string {
		return fmt.Sprintf(`{"workspace_id":"ws1","session_id":%q,"actions":[`+
			`{"type":"pageview","path":"/checkout","page_number":1,"duration":1200,"scroll":30,"entered_at":%d,"exited_at":%d},`+
			`%s,`+
			`{"type":"goal","name":"clean","goal_type":"other","path":"/checkout","page_number":1,"timestamp":%d,"value":5,"properties":{"plan":"pro"}}`+
			`],"created_at":%d,"updated_at":%d,"seq":4}`,
			uuidV7At(now.Add(-time.Minute)), ts-60000, ts, goal, ts+1, ts-60000, ts)
	}
	goal := func(extra string) string {
		return fmt.Sprintf(`{"type":"goal","name":"purchase","goal_type":"purchase","path":"/checkout","page_number":1,"timestamp":%d,%s}`, ts, extra)
	}
	decode := func(t *testing.T, body string) *WebTrackPayload {
		t.Helper()
		var p WebTrackPayload
		require.NoError(t, json.Unmarshal([]byte(body), &p), "a wrongly typed goal must not fail the beat's decode")
		require.NoError(t, p.Validate(now))
		return &p
	}
	assertOthersKept := func(t *testing.T, p *WebTrackPayload) {
		t.Helper()
		require.GreaterOrEqual(t, len(p.Actions), 2)
		assert.Equal(t, "pageview", p.Actions[0].Type)
		assert.Equal(t, "/checkout", p.Actions[0].Path)
		assert.Equal(t, int64(1200), p.Actions[0].Duration)
		last := p.Actions[len(p.Actions)-1]
		assert.Equal(t, "clean", last.Name)
		assert.Equal(t, float64(5), last.Value)
		assert.Equal(t, map[string]string{"plan": "pro"}, last.Properties)
	}

	for _, tc := range []struct {
		name      string
		extra     string
		wantValue float64
		wantProps map[string]string
	}{
		{"numeric property", `"properties":{"order_id":12345,"total":49.90}`, 0,
			map[string]string{"order_id": "12345", "total": "49.90"}},
		{"boolean property", `"properties":{"first_order":true,"gift":false}`, 0,
			map[string]string{"first_order": "true", "gift": "false"}},
		{"null property keeps the key empty, as before", `"properties":{"coupon":null,"currency":"EUR"}`, 0,
			map[string]string{"coupon": "", "currency": "EUR"}},
		{"numeric string value", `"value":"49.90"`, 49.90, nil},
		{"numeric string value with spaces", `"value":" 12 "`, 12, nil},
		{"empty string value means no value", `"value":""`, 0, nil},
		{"null value means no value", `"value":null`, 0, nil},
		{"null properties", `"properties":null`, 0, nil},
	} {
		t.Run(tc.name+" is coerced and the beat kept", func(t *testing.T) {
			p := decode(t, beat(goal(tc.extra)))
			require.Len(t, p.Actions, 3)
			assertOthersKept(t, p)
			g := p.Actions[1]
			assert.Equal(t, "purchase", g.Name)
			assert.Equal(t, tc.wantValue, g.Value)
			assert.Equal(t, tc.wantProps, g.Properties)
		})
	}

	for _, tc := range []struct {
		name  string
		extra string
	}{
		{"non-numeric string value", `"value":"49,90 EUR"`},
		{"NaN string value", `"value":"NaN"`},
		{"Infinity string value", `"value":"Infinity"`},
		{"hex string value", `"value":"0x10"`},
		{"the string null as the value", `"value":"null"`},
		{"boolean value", `"value":true`},
		{"object value", `"value":{"amount":10}`},
		{"nested object property", `"properties":{"items":{"sku":"A1"}}`},
		{"array property", `"properties":{"skus":["A1","B2"]}`},
		{"properties that are not an object", `"properties":"order=1"`},
		{"numeric string value beyond the column range", `"value":"1e300"`},
		{"wrongly typed page_number", `"page_number":"one"`},
	} {
		t.Run(tc.name+" drops only that goal", func(t *testing.T) {
			p := decode(t, beat(goal(tc.extra)))
			require.Len(t, p.Actions, 2)
			assertOthersKept(t, p)
		})
	}

	t.Run("coerced properties still respect the property bounds", func(t *testing.T) {
		props := make([]string, 0, WebTrackMaxGoalPropertyKeys+1)
		for i := 0; i <= WebTrackMaxGoalPropertyKeys; i++ {
			props = append(props, fmt.Sprintf(`"k%d":%d`, i, i))
		}
		p := decode(t, beat(goal(`"properties":{`+strings.Join(props, ",")+`}`)))
		require.Len(t, p.Actions, 2)
		assertOthersKept(t, p)
	})

	t.Run("an undecodable action still counts toward the action cap", func(t *testing.T) {
		actions := make([]string, WebTrackMaxActions+1)
		for i := range actions {
			actions[i] = `{"type":"pageview","path":"/p","page_number":"x"}`
		}
		body := fmt.Sprintf(`{"workspace_id":"ws1","session_id":%q,"actions":[%s],"updated_at":%d,"seq":1}`,
			uuidV7At(now.Add(-time.Minute)), strings.Join(actions, ","), ts)
		var p WebTrackPayload
		require.NoError(t, json.Unmarshal([]byte(body), &p))
		assert.ErrorContains(t, p.Validate(now), "actions")
	})

	// A beat past the cap is rejected on its length alone, so decoding its
	// entries first is pure cost: before actions were decoded one by one, the
	// first undecodable entry ended the decode, and a body full of them cost
	// next to nothing. Allocations stand in for that cost, being deterministic.
	t.Run("entries past the action cap are not decoded", func(t *testing.T) {
		body := func(n int) []byte {
			actions := make([]string, n)
			for i := range actions {
				actions[i] = `{"type":"goal","name":"g","page_number":"x","value":"x"}`
			}
			return []byte(fmt.Sprintf(`{"workspace_id":"ws1","session_id":%q,"actions":[%s],"updated_at":%d,"seq":1}`,
				uuidV7At(now.Add(-time.Minute)), strings.Join(actions, ","), ts))
		}
		allocs := func(raw []byte) float64 {
			return testing.AllocsPerRun(3, func() {
				var p WebTrackPayload
				_ = json.Unmarshal(raw, &p)
			})
		}
		const floodFactor = 10
		overCap, flood := body(WebTrackMaxActions+1), body(floodFactor*(WebTrackMaxActions+1))

		var p WebTrackPayload
		require.NoError(t, json.Unmarshal(flood, &p))
		assert.ErrorContains(t, p.Validate(now), "actions")
		assert.Less(t, allocs(flood), 2*allocs(overCap),
			"a flood of entries past the cap must cost about what a beat just over it does")
	})

	t.Run("payload-level type errors still fail the decode", func(t *testing.T) {
		var p WebTrackPayload
		assert.Error(t, json.Unmarshal([]byte(`{"workspace_id":1,"actions":[]}`), &p))
		for _, actions := range []string{`{"type":"goal"}`, `"goal"`, `1`, `true`} {
			assert.Error(t, json.Unmarshal([]byte(`{"workspace_id":"ws1","actions":`+actions+`}`), &p), actions)
		}
	})

	t.Run("absent, null and empty actions decode to no actions", func(t *testing.T) {
		for _, actions := range []string{``, `,"actions":null`, `,"actions":[]`, `,"actions": [ ] `} {
			var p WebTrackPayload
			require.NoError(t, json.Unmarshal([]byte(`{"workspace_id":"ws1"`+actions+`}`), &p), actions)
			assert.Empty(t, p.Actions, actions)
		}
	})
}
