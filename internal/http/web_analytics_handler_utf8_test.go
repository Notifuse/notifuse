package http

import (
	"fmt"
	"net/http"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWebAnalyticsHandlerTrackInvalidUTF8 posts beats through the real /track
// handler carrying text that would reach PostgreSQL as bytes that are not
// UTF-8, which it refuses in text and jsonb alike, failing every visitor
// batched with this one. A JSON body cannot deliver such bytes; a request header
// can, and so can the percent-decoding enrichment does.
func TestWebAnalyticsHandlerTrackInvalidUTF8(t *testing.T) {
	now := time.Now().UnixMilli()
	pageview := fmt.Sprintf(`{"type":"pageview","path":"/home","page_number":1,"duration":900,"scroll":20,"entered_at":%d,"exited_at":%d}`,
		now-60000, now)

	t.Run("raw bytes in the JSON body arrive as U+FFFD, and the beat is kept", func(t *testing.T) {
		p := newTrackPipeline(t)
		rec := p.post(t, nulBeat(t, fmt.Sprintf(`"actions":[%s,{"type":"goal","name":"sign%sup","goal_type":"lead","path":"/caf%s","page_number":1,"timestamp":%d}],`+
			`"attributes":{"landing_page":"https://shop.example.com/home","utm_source":"goo%sgle"}`, pageview, "\xff", "\xc3", now, "\xfe")), nil)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Len(t, p.sessions, 1)
		assert.Equal(t, "goo�gle", p.sessions[0].UTMSource)
		require.Len(t, p.goals, 1, "the goal is kept: what it holds is valid text by now")
		assert.Equal(t, "sign�up", p.goals[0].GoalName)
		assert.Equal(t, "/caf�", p.goals[0].Path)
	})

	t.Run("a user agent header holding bytes that are not UTF-8 keeps them written as %XX", func(t *testing.T) {
		// net/http's server accepts bytes 0x80 to 0xFF in a header value, and
		// Track takes the user agent from the header when the body has none.
		p := newTrackPipeline(t)
		rec := p.post(t, nulBeat(t, `"actions":[`+pageview+`],"attributes":{"landing_page":"https://shop.example.com/home"}`),
			map[string]string{"User-Agent": "Mozilla/5.0 (\xff\xfe) Chrome/126.0"})

		require.Equal(t, http.StatusOK, rec.Code)
		require.Len(t, p.sessions, 1)
		assert.Equal(t, "Mozilla/5.0 (%FF%FE) Chrome/126.0", p.sessions[0].UserAgent)
	})

	t.Run("URLs that decode to bytes that are not UTF-8 keep them written as %XX", func(t *testing.T) {
		p := newTrackPipeline(t)
		rec := p.post(t, nulBeat(t, `"actions":[`+pageview+`],`+
			`"attributes":{"landing_page":"https://shop.example.com/r%E9sum%E9","referrer":"https://%FF.example.org/caf%c3"}`), nil)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Len(t, p.sessions, 1)
		session := p.sessions[0]
		assert.Equal(t, "https://shop.example.com/r%E9sum%E9", session.LandingPage, "the URL itself is stored as sent")
		assert.Equal(t, "/r%E9sum%E9", session.LandingPath)
		assert.Equal(t, "%ff.example.org", session.ReferrerDomain)
		assert.Equal(t, "/caf%C3", session.ReferrerPath)
		for _, value := range []string{session.LandingDomain, session.LandingPath, session.ReferrerDomain, session.ReferrerPath} {
			assert.True(t, utf8.ValidString(value), "%q", value)
		}
	})
}
