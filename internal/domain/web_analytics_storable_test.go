package domain

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// PostgreSQL refuses to store a byte sequence that is not UTF-8, in text and
// jsonb alike, and the flush writes a workspace's batch in one transaction, so
// one such string from one visitor failed every other visitor's rows with it.
// A beat's JSON body cannot carry one (see
// TestWebTrackJSONDecodingReplacesInvalidUTF8); a request header can, and so can
// any byte that percent-decoding produces.

func TestEscapeUnstorableBytes(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"empty", "", ""},
		{"ASCII", "/pricing?plan=pro", "/pricing?plan=pro"},
		{"multi-byte characters", "/café/☕/𝄞", "/café/☕/𝄞"},
		{"a replacement character that is really there", "a�b", "a�b"},
		{"a NUL", "a\x00b", "a%00b"},
		{"a byte that starts no character", "a\xffb", "a%FFb"},
		{"a stray continuation byte", "\x80abc", "%80abc"},
		{"a two-byte character cut short", "caf\xc3", "caf%C3"},
		{"a three-byte character cut short", "\xe2\x82", "%E2%82"},
		{"a cut-short character before a whole one", "\xc3\xc3\xa9", "%C3é"},
		{"an encoded UTF-16 surrogate", "\xed\xa0\x80", "%ED%A0%80"},
		{"an overlong encoding", "\xc0\x80", "%C0%80"},
		{"past U+10FFFF", "\xf4\x90\x80\x80", "%F4%90%80%80"},
		{"Latin-1 bytes in an old link", "r\xe9sum\xe9", "r%E9sum%E9"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := EscapeUnstorableBytes(tc.in)
			assert.Equal(t, tc.want, got)
			assert.True(t, utf8.ValidString(got), "%q", got)
			assert.NotContains(t, got, "\x00")
		})
	}
}

func TestWebTrackPayloadEscapesInvalidUTF8InSessionText(t *testing.T) {
	now := time.Date(2026, 8, 8, 15, 0, 0, 0, time.UTC)
	rawPayload := func() *WebTrackPayload {
		return &WebTrackPayload{
			WorkspaceID: "ws1",
			SessionID:   uuidV7At(now.Add(-time.Minute)),
			UpdatedAt:   now.UnixMilli(),
			Seq:         1,
			Actions:     []WebTrackAction{{Type: WebActionTypePageview, Path: "/home", PageNumber: 1}},
		}
	}

	// Payloads built in Go, because a JSON body cannot carry these bytes: Track
	// fills an absent user agent in from the request header, which can, before
	// it calls Validate. Kept rather than refused, for the same reason as a NUL:
	// these values ride on every beat of the visit, and the SDK reads a 400 as
	// permanent.

	t.Run("every string attribute keeps its value, the bytes written as %XX", func(t *testing.T) {
		var attrs WebSessionAttributes
		value := reflect.ValueOf(&attrs).Elem()
		for _, f := range stringAttributeFields(t) {
			value.FieldByIndex(f.Index).SetString("Mozilla\xff/5.0 (caf\xc3)")
		}
		p := rawPayload()
		p.Attributes = &attrs

		require.NoError(t, p.Validate(now))
		require.Len(t, p.Actions, 1)
		for _, f := range stringAttributeFields(t) {
			assert.Equal(t, "Mozilla%FF/5.0 (caf%C3)", value.FieldByIndex(f.Index).String(), jsonName(f))
		}
	})

	t.Run("sdk_version and custom dimensions", func(t *testing.T) {
		p := rawPayload()
		p.SDKVersion = "1.0\xfe"
		p.Dimensions = map[string]string{"custom_1": "vari\xe2\x82ant"}
		require.NoError(t, p.Validate(now))
		assert.Equal(t, "1.0%FE", p.SDKVersion)
		assert.Equal(t, "vari%E2%82ant", p.Dimensions["custom_1"])
	})

	t.Run("a NUL beside them is still removed", func(t *testing.T) {
		p := rawPayload()
		p.Attributes = &WebSessionAttributes{UserAgent: "Mozilla\x00/5.0\xff"}
		require.NoError(t, p.Validate(now))
		assert.Equal(t, "Mozilla/5.0%FF", p.Attributes.UserAgent)
	})

	t.Run("valid text is left exactly as sent", func(t *testing.T) {
		p := rawPayload()
		p.Attributes = &WebSessionAttributes{UTMCampaign: "été 2026 ☀ 50%FF off", UserAgent: "Mozilla/5.0"}
		require.NoError(t, p.Validate(now))
		assert.Equal(t, "été 2026 ☀ 50%FF off", p.Attributes.UTMCampaign)
		assert.Equal(t, "Mozilla/5.0", p.Attributes.UserAgent)
	})
}

func TestWebTrackJSONDecodingReplacesInvalidUTF8(t *testing.T) {
	// encoding/json writes U+FFFD for each byte of a string that is not UTF-8,
	// in object keys as in values, so nothing decoded from a beat's body reaches
	// enrichment invalid. Pinned here because everything downstream relies on
	// it: a decoder that kept the bytes would hand them to PostgreSQL, which
	// refuses them.
	now := time.Date(2026, 8, 8, 15, 0, 0, 0, time.UTC)
	ts := now.UnixMilli()
	// The raw bytes go in through %s: a backquoted literal would hold the
	// four characters of each escape instead.
	body := nulBeatJSON(now, fmt.Sprintf(`"actions":[`+
		`{"type":"pageview","path":"/caf%s","page_number":1,"duration":10,"entered_at":%d,"exited_at":%d},`+
		`{"type":"goal","name":"sign%sup","goal_type":"lead","path":"/%s","page_number":1,"timestamp":%d,"properties":{"pl%san":"pro%s","n":1}}],`+
		`"attributes":{"utm_source":"goo%sgle","user_agent":"Mozilla%s"},"dimensions":{"custom_1":"vari%sant"},"sdk_version":"1.0%s"`,
		"\xc3", ts-60000, ts, "\xff", "\xfe", ts, "\xff", "\xc3", "\xff", "\xe2\x82", "\xc3", "\xfe"))
	require.False(t, utf8.ValidString(body), "the body itself carries the bytes")

	var p WebTrackPayload
	require.NoError(t, json.Unmarshal([]byte(body), &p))
	require.NoError(t, p.Validate(now))

	require.Len(t, p.Actions, 2, "neither action is dropped: what they hold is valid text by now")
	assert.Equal(t, "/caf�", p.Actions[0].Path)
	assert.Equal(t, "sign�up", p.Actions[1].Name)
	assert.Equal(t, "/�", p.Actions[1].Path)
	assert.Equal(t, map[string]string{"pl�an": "pro�", "n": "1"}, p.Actions[1].Properties)
	assert.Equal(t, "goo�gle", p.Attributes.UTMSource)
	assert.Equal(t, "Mozilla��", p.Attributes.UserAgent, "one replacement per byte")
	assert.Equal(t, "vari�ant", p.Dimensions["custom_1"])
	assert.Equal(t, "1.0�", p.SDKVersion)
}

func TestWebTrackPayloadStoresTheCanonicalSessionID(t *testing.T) {
	// uuid.Parse accepts more spellings than the canonical one: either case,
	// the braced form (whose braces it never reads), urn:uuid: and 32 bare hex
	// digits. PostgreSQL's uuid type refuses some of them, and two spellings of
	// one id would be two writers of the same rows in one statement, which
	// PostgreSQL refuses as well. So the id is replaced by its canonical form,
	// which is what the SDK sends and which comes back unchanged.
	now := time.Date(2026, 8, 8, 15, 0, 0, 0, time.UTC)
	id := uuidV7At(now.Add(-time.Minute))
	require.NotEqual(t, id, strings.ToUpper(id), "the id has letters, so the case variants below differ")
	beat := func(sessionIDJSON string) string {
		return fmt.Sprintf(`{"workspace_id":"ws1","session_id":%s,"updated_at":%d,"seq":1,"actions":[]}`, sessionIDJSON, now.UnixMilli())
	}

	for _, tc := range []struct{ name, sessionIDJSON string }{
		{"canonical", `"` + id + `"`},
		{"upper case", `"` + strings.ToUpper(id) + `"`},
		{"braced", `"{` + id + `}"`},
		{"braced with anything in place of the braces", `"x` + id + `y"`},
		{`braced with \u0000 in place of the braces`, `"\u0000` + id + `\u0000"`},
		{"urn", `"urn:uuid:` + id + `"`},
		{"urn in upper case", `"URN:UUID:` + strings.ToUpper(id) + `"`},
		{"bare hex", `"` + strings.ReplaceAll(id, "-", "") + `"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var p WebTrackPayload
			require.NoError(t, json.Unmarshal([]byte(beat(tc.sessionIDJSON)), &p))
			require.NoError(t, p.Validate(now))
			assert.Equal(t, id, p.SessionID)
		})
	}

	t.Run("an id that is not a UUIDv7 is still refused", func(t *testing.T) {
		for _, bad := range []string{"{" + id, id + "}", "not-a-uuid", "8a9c1a1e-6f0e-4d17-9d5a-6b1f6e2d3c4b"} {
			p := &WebTrackPayload{WorkspaceID: "ws1", SessionID: bad, UpdatedAt: now.UnixMilli(), Seq: 1}
			assert.ErrorContains(t, p.Validate(now), "session_id", bad)
		}
	})
}
