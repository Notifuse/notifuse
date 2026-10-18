package domain

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// JSON carries U+0000 happily as \u0000, and PostgreSQL stores it in neither
// text nor jsonb. flushOnce writes a whole workspace batch in one transaction,
// so a single such string from one visitor aborted every other visitor's rows
// with it, and once retrying had failed too the buffer deleted those sessions.
// /track is public, so anyone could do that to a workspace with one beat.

// nulBeatJSON is a beat whose actions, attributes and other fields are given as
// JSON text, so that a \u0000 written in a test is exactly what travels on the
// wire rather than whatever an encoder chose to produce.
func nulBeatJSON(now time.Time, fields string) string {
	return fmt.Sprintf(`{"workspace_id":"ws1","session_id":%q,"updated_at":%d,"seq":1,%s}`,
		uuidV7At(now.Add(-time.Minute)), now.UnixMilli(), fields)
}

func decodeNULBeat(t *testing.T, now time.Time, fields string) *WebTrackPayload {
	t.Helper()
	var p WebTrackPayload
	require.NoError(t, json.Unmarshal([]byte(nulBeatJSON(now, fields)), &p))
	return &p
}

// stringAttributeFields lists every string field of WebSessionAttributes, read
// from the type itself so that a field added later is covered by the tests
// below without anyone remembering to add it to them.
func stringAttributeFields(t *testing.T) []reflect.StructField {
	t.Helper()
	var fields []reflect.StructField
	typ := reflect.TypeOf(WebSessionAttributes{})
	for i := 0; i < typ.NumField(); i++ {
		if typ.Field(i).Type.Kind() == reflect.String {
			fields = append(fields, typ.Field(i))
		}
	}
	require.NotEmpty(t, fields)
	return fields
}

func jsonName(f reflect.StructField) string {
	name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	return name
}

func TestWebTrackActionCarryingNULIsDropped(t *testing.T) {
	now := time.Date(2026, 8, 8, 15, 0, 0, 0, time.UTC)
	ts := now.UnixMilli()
	cleanPageview := fmt.Sprintf(`{"type":"pageview","path":"/home","page_number":1,"duration":10,"entered_at":%d,"exited_at":%d}`, ts-60000, ts)

	for _, tc := range []struct {
		name string
		// escaped is the action as JSON text, the NUL written as \u0000.
		escaped string
		// raw is the same action as the decoder hands it to Validate, the NUL a
		// real byte.
		raw WebTrackAction
	}{
		{
			name:    "pageview path",
			escaped: `{"type":"pageview","path":"/pri\u0000cing","page_number":2}`,
			raw:     WebTrackAction{Type: WebActionTypePageview, Path: "/pri\x00cing", PageNumber: 2},
		},
		{
			name:    "goal path",
			escaped: fmt.Sprintf(`{"type":"goal","name":"signup","goal_type":"lead","path":"/check\u0000out","page_number":1,"timestamp":%d}`, ts),
			raw:     WebTrackAction{Type: WebActionTypeGoal, Name: "signup", GoalType: GoalTypeLead, Path: "/check\x00out", PageNumber: 1, Timestamp: ts},
		},
		{
			name:    "goal name",
			escaped: fmt.Sprintf(`{"type":"goal","name":"sign\u0000up","goal_type":"lead","path":"/home","page_number":1,"timestamp":%d}`, ts),
			raw:     WebTrackAction{Type: WebActionTypeGoal, Name: "sign\x00up", GoalType: GoalTypeLead, Path: "/home", PageNumber: 1, Timestamp: ts},
		},
		{
			name:    "goal property key",
			escaped: fmt.Sprintf(`{"type":"goal","name":"signup","goal_type":"lead","path":"/home","page_number":1,"timestamp":%d,"properties":{"pl\u0000an":"pro"}}`, ts),
			raw: WebTrackAction{Type: WebActionTypeGoal, Name: "signup", GoalType: GoalTypeLead, Path: "/home", PageNumber: 1, Timestamp: ts,
				Properties: map[string]string{"pl\x00an": "pro"}},
		},
		{
			name:    "goal property value",
			escaped: fmt.Sprintf(`{"type":"goal","name":"signup","goal_type":"lead","path":"/home","page_number":1,"timestamp":%d,"properties":{"plan":"pro\u0000"}}`, ts),
			raw: WebTrackAction{Type: WebActionTypeGoal, Name: "signup", GoalType: GoalTypeLead, Path: "/home", PageNumber: 1, Timestamp: ts,
				Properties: map[string]string{"plan": "pro\x00"}},
		},
	} {
		t.Run(tc.name+` escaped as \u0000 costs that action alone`, func(t *testing.T) {
			p := decodeNULBeat(t, now, `"actions":[`+cleanPageview+`,`+tc.escaped+`]`)
			require.NoError(t, p.Validate(now), "actions[] is cumulative: a bad action must never reject the beat")
			require.Len(t, p.Actions, 1, "the action carrying the NUL is dropped, the clean one kept")
			assert.Equal(t, "/home", p.Actions[0].Path)
			assert.Equal(t, WebActionTypePageview, p.Actions[0].Type)
		})

		t.Run(tc.name+" as a raw byte is invalid", func(t *testing.T) {
			action := tc.raw
			assert.ErrorContains(t, action.Validate(), "null character")
		})
	}

	t.Run("the same actions without the NUL are valid", func(t *testing.T) {
		// The counterpart, so the cases above cannot pass by being invalid for
		// some other reason.
		for _, action := range []WebTrackAction{
			{Type: WebActionTypePageview, Path: "/pricing", PageNumber: 2},
			{Type: WebActionTypeGoal, Name: "signup", GoalType: GoalTypeLead, Path: "/checkout", PageNumber: 1, Timestamp: ts,
				Properties: map[string]string{"plan": "pro"}},
		} {
			assert.NoError(t, action.Validate())
		}
	})

	t.Run("a goal type carrying one is recorded as other, never as sent", func(t *testing.T) {
		// The type is mapped onto a fixed list and an unknown one becomes
		// "other", so it can never take a NUL to the database, and dropping the
		// goal would lose a conversion that rule exists to keep.
		p := decodeNULBeat(t, now, fmt.Sprintf(
			`"actions":[{"type":"goal","name":"purchase","goal_type":"purchase\u0000","path":"/home","page_number":1,"timestamp":%d}]`, ts))
		require.NoError(t, p.Validate(now))
		require.Len(t, p.Actions, 1)
		assert.Equal(t, GoalTypeOther, p.Actions[0].GoalType)
	})
}

func TestWebTrackPayloadRemovesNULFromSessionText(t *testing.T) {
	now := time.Date(2026, 8, 8, 15, 0, 0, 0, time.UTC)
	ts := now.UnixMilli()
	cleanPageview := fmt.Sprintf(`{"type":"pageview","path":"/home","page_number":1,"duration":10,"entered_at":%d,"exited_at":%d}`, ts-60000, ts)
	rawPayload := func() *WebTrackPayload {
		return &WebTrackPayload{
			WorkspaceID: "ws1",
			SessionID:   uuidV7At(now.Add(-time.Minute)),
			UpdatedAt:   ts,
			Seq:         1,
			Actions:     []WebTrackAction{{Type: WebActionTypePageview, Path: "/home", PageNumber: 1}},
		}
	}

	// Session-level text is removed from, never rejected over: the SDK keeps
	// these values for the whole session and resends them on every beat, and it
	// reads a 400 as permanent, so a rejection would lose every later beat of
	// the visit — for a value a visitor gets just by following a link carrying
	// utm_source=%00, which the SDK decodes.

	t.Run(`every string attribute escaped as \u0000 keeps the beat and the rest of its value`, func(t *testing.T) {
		var pairs []string
		for _, f := range stringAttributeFields(t) {
			pairs = append(pairs, fmt.Sprintf(`%q:"a\u0000b"`, jsonName(f)))
		}
		p := decodeNULBeat(t, now, `"actions":[`+cleanPageview+`],"attributes":{`+strings.Join(pairs, ",")+`}`)

		require.NoError(t, p.Validate(now))
		require.Len(t, p.Actions, 1, "the beat's actions are kept")
		attrs := reflect.ValueOf(*p.Attributes)
		for _, f := range stringAttributeFields(t) {
			assert.Equal(t, "ab", attrs.FieldByIndex(f.Index).String(), jsonName(f))
		}
	})

	t.Run("every string attribute as a raw byte keeps the rest of its value", func(t *testing.T) {
		var attrs WebSessionAttributes
		value := reflect.ValueOf(&attrs).Elem()
		for _, f := range stringAttributeFields(t) {
			value.FieldByIndex(f.Index).SetString("a\x00b")
		}
		p := rawPayload()
		p.Attributes = &attrs

		require.NoError(t, p.Validate(now))
		for _, f := range stringAttributeFields(t) {
			assert.Equal(t, "ab", value.FieldByIndex(f.Index).String(), jsonName(f))
		}
	})

	t.Run("a URL attribute keeps its percent-encoded NUL", func(t *testing.T) {
		// %00 is ordinary URL text, and it is what a browser reports for a
		// visit to such an address. Decoding it is enrichment's business.
		p := decodeNULBeat(t, now, `"actions":[`+cleanPageview+`],"attributes":{"landing_page":"https://shop.example.com/a%00b"}`)
		require.NoError(t, p.Validate(now))
		assert.Equal(t, "https://shop.example.com/a%00b", p.Attributes.LandingPage)
	})

	t.Run("sdk_version", func(t *testing.T) {
		p := decodeNULBeat(t, now, `"actions":[`+cleanPageview+`],"sdk_version":"1.0\u00002"`)
		require.NoError(t, p.Validate(now))
		assert.Equal(t, "1.02", p.SDKVersion)

		raw := rawPayload()
		raw.SDKVersion = "1.0\x002"
		require.NoError(t, raw.Validate(now))
		assert.Equal(t, "1.02", raw.SDKVersion)
	})

	t.Run("custom dimensions", func(t *testing.T) {
		p := decodeNULBeat(t, now, `"actions":[`+cleanPageview+`],"dimensions":{"custom_1":"vari\u0000ant","custom_10":"\u0000"}`)
		require.NoError(t, p.Validate(now))
		assert.Equal(t, "variant", p.Dimensions["custom_1"])
		assert.Equal(t, "", p.Dimensions["custom_10"])

		raw := rawPayload()
		raw.Dimensions = map[string]string{"custom_2": "b\x00eta"}
		require.NoError(t, raw.Validate(now))
		assert.Equal(t, "beta", raw.Dimensions["custom_2"])
	})

	t.Run("a dimension is held to its bound once the NUL is gone", func(t *testing.T) {
		raw := rawPayload()
		raw.Dimensions = map[string]string{"custom_1": strings.Repeat("v", WebTrackMaxDimensionValueLength) + "\x00"}
		require.NoError(t, raw.Validate(now), "the bound measures what would be stored")
		assert.Equal(t, strings.Repeat("v", WebTrackMaxDimensionValueLength), raw.Dimensions["custom_1"])

		raw.Dimensions = map[string]string{"custom_1": strings.Repeat("v", WebTrackMaxDimensionValueLength+1) + "\x00"}
		assert.ErrorContains(t, raw.Validate(now), "custom_1", "and still bounds it")
	})
}

func TestWebTrackPayloadRefusesNULSessionID(t *testing.T) {
	// uuid.Parse never reads the first and last byte of an id in the braced
	// form, where the braces go, and the session row stores the id as sent, so
	// a NUL in either place would still parse and then reach PostgreSQL.
	// Refused like any other malformed id: the SDK only ever sends the
	// canonical form, so no visitor's beat is lost to it.
	now := time.Date(2026, 8, 8, 15, 0, 0, 0, time.UTC)
	id := uuidV7At(now.Add(-time.Minute))
	beat := func(sessionIDJSON string) string {
		return fmt.Sprintf(`{"workspace_id":"ws1","session_id":%s,"updated_at":%d,"seq":1,"actions":[]}`, sessionIDJSON, now.UnixMilli())
	}

	for _, tc := range []struct{ name, sessionIDJSON string }{
		{"leading and trailing", `"\u0000` + id + `\u0000"`},
		{"leading", `"\u0000` + id + `}"`},
		{"trailing", `"{` + id + `\u0000"`},
	} {
		t.Run(tc.name+` \u0000`, func(t *testing.T) {
			var p WebTrackPayload
			require.NoError(t, json.Unmarshal([]byte(beat(tc.sessionIDJSON)), &p))
			require.Contains(t, p.SessionID, "\x00")
			assert.ErrorContains(t, p.Validate(now), "session_id: contains a null character")
		})
	}

	t.Run("as a raw byte", func(t *testing.T) {
		p := &WebTrackPayload{WorkspaceID: "ws1", SessionID: "\x00" + id + "\x00", UpdatedAt: now.UnixMilli(), Seq: 1}
		assert.ErrorContains(t, p.Validate(now), "null character")
	})

	t.Run("the same id with its braces is accepted", func(t *testing.T) {
		// The counterpart: the refusal above is the NUL's, not the id's shape.
		p := &WebTrackPayload{WorkspaceID: "ws1", SessionID: "{" + id + "}", UpdatedAt: now.UnixMilli(), Seq: 1}
		assert.NoError(t, p.Validate(now))
	})
}

func TestWebTrackRawNULByteIsNotJSON(t *testing.T) {
	// JSON allows U+0000 inside a string only escaped, and nowhere outside one,
	// so a literal 0x00 byte makes the whole beat malformed: the handler answers
	// 400 before anything is buffered. No SDK sends one — JSON.stringify
	// escapes it — so it only comes from a hand-written client.
	now := time.Date(2026, 8, 8, 15, 0, 0, 0, time.UTC)
	for _, fields := range []string{
		fmt.Sprintf(`"actions":[{"type":"goal","name":"signNULup","path":"/","page_number":1,"timestamp":%d}]`, now.UnixMilli()),
		`"actions":[],"attributes":{"utm_source":"gooNULgle"}`,
	} {
		body := strings.Replace(nulBeatJSON(now, fields), "NUL", "\x00", 1)
		require.Contains(t, body, "\x00")
		var p WebTrackPayload
		assert.Error(t, json.Unmarshal([]byte(body), &p), fields)
	}
}

func TestResolveWebIdentityRefusesNUL(t *testing.T) {
	// A credential can only vouch for an address PostgreSQL can store. Refusing
	// it here costs the identity and keeps the beat, like every other malformed
	// credential, and keeps the address away from the contacts lookup.
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	const secret = "workspace-secret-key"
	email := "vis\x00itor@example.com"

	t.Run("a signed address carrying one identifies no one", func(t *testing.T) {
		hmacHex := ComputeWebIdentifyHMAC(email, secret)
		_, ok := ResolveWebIdentity(&WebTrackPayload{ContactEmail: &email, ContactEmailHMAC: &hmacHex}, secret, now)
		assert.False(t, ok)
	})

	t.Run(`escaped as \u0000 on the wire`, func(t *testing.T) {
		p := decodeNULBeat(t, now, fmt.Sprintf(`"actions":[],"contact_email":"vis\u0000itor@example.com","contact_email_hmac":%q`,
			ComputeWebIdentifyHMAC(email, secret)))
		require.Equal(t, email, *p.ContactEmail, "the escape decodes to the byte the HMAC was computed over")
		_, ok := ResolveWebIdentity(p, secret, now)
		assert.False(t, ok)
	})

	t.Run("an email-click token naming one identifies no one", func(t *testing.T) {
		token, err := BuildWebIdentifyToken(email, secret, WebIdentifyTokenTTL, now)
		require.NoError(t, err)
		_, ok := ResolveWebIdentity(&WebTrackPayload{IdentifyToken: &token}, secret, now)
		assert.False(t, ok)
	})

	t.Run("the same address without it still identifies", func(t *testing.T) {
		clean := "visitor@example.com"
		hmacHex := ComputeWebIdentifyHMAC(clean, secret)
		got, ok := ResolveWebIdentity(&WebTrackPayload{ContactEmail: &clean, ContactEmailHMAC: &hmacHex}, secret, now)
		require.True(t, ok)
		assert.Equal(t, clean, got)
	})
}
