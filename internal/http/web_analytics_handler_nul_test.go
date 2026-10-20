package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// trackPipeline is /track wired to the real ingest service and buffer, with
// only the repositories mocked, so a beat takes the path it takes in production
// from the mux to the rows the flush would write to PostgreSQL.
type trackPipeline struct {
	mux      *http.ServeMux
	buffer   *service.WebAnalyticsBuffer
	flushes  int
	sessions []*domain.WebSession
	pages    []*domain.WebPage
	goals    []*domain.WebGoal
}

func newTrackPipeline(t *testing.T) *trackPipeline {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws1").Return(&domain.Workspace{ID: "ws1", Settings: domain.WorkspaceSettings{
		WebAnalytics: &domain.WebAnalyticsSettings{Enabled: true, Filters: domain.DefaultWebFilters()},
	}}, nil).AnyTimes()

	p := &trackPipeline{}
	webRepo := mocks.NewMockWebAnalyticsRepository(ctrl)
	webRepo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, sessions []*domain.WebSession, pages []*domain.WebPage, goals []*domain.WebGoal) error {
			p.flushes++
			p.sessions = append(p.sessions, sessions...)
			p.pages = append(p.pages, pages...)
			p.goals = append(p.goals, goals...)
			return nil
		}).AnyTimes()
	webRepo.EXPECT().ProjectContactNavigation(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	p.buffer = service.NewWebAnalyticsBuffer(webRepo, logger.NewLogger(), service.WebAnalyticsBufferConfig{})
	svc := service.NewWebAnalyticsService(workspaceRepo, nil, p.buffer, nil, nil, nil, nil, logger.NewLogger())
	p.mux = http.NewServeMux()
	NewWebAnalyticsHandler(svc, nil, logger.NewLogger(), nil).RegisterRoutes(p.mux)
	return p
}

func (p *trackPipeline) post(t *testing.T, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	p.mux.ServeHTTP(rec, trackRequest(t, body, headers))
	p.buffer.FlushAll(context.Background())
	return rec
}

// rowsJSON is every row the flush received. encoding/json writes a NUL as
// \u0000 in any string it meets, map keys included, and the row types hide no
// field from it, so this is a complete search of what would reach PostgreSQL.
func (p *trackPipeline) rowsJSON(t *testing.T) string {
	t.Helper()
	raw, err := json.Marshal(map[string]interface{}{"sessions": p.sessions, "pages": p.pages, "goals": p.goals})
	require.NoError(t, err)
	return string(raw)
}

// nulBeat builds a beat from JSON text, so a \u0000 written by a test is the
// escape that travels on the wire.
func nulBeat(t *testing.T, fields string) string {
	t.Helper()
	sessionID, err := uuid.NewV7()
	require.NoError(t, err)
	return nulBeatWithSessionID(fmt.Sprintf("%q", sessionID.String()), fields)
}

// nulBeatWithSessionID is nulBeat with the session id given as JSON text too.
func nulBeatWithSessionID(sessionIDJSON, fields string) string {
	now := time.Now().UnixMilli()
	return fmt.Sprintf(`{"workspace_id":"ws1","session_id":%s,"created_at":%d,"updated_at":%d,"sent_at":%d,"seq":1,%s}`,
		sessionIDJSON, now-60000, now, now, fields)
}

// TestWebAnalyticsHandlerTrackNULCharacter posts beats carrying a NUL through
// the real /track handler. PostgreSQL stores U+0000 in neither text nor jsonb,
// and the flush writes a workspace's whole batch in one transaction, so a NUL
// that reached the buffer cost every other visitor batched with it. The answer
// stays a 200 for anything the SDK can send: it reads a 400 as permanent and
// resends its cumulative actions and its session attributes on every beat.
func TestWebAnalyticsHandlerTrackNULCharacter(t *testing.T) {
	now := time.Now().UnixMilli()
	pageview := func(path string, number int) string {
		return fmt.Sprintf(`{"type":"pageview","path":%q,"page_number":%d,"duration":900,"scroll":20,"entered_at":%d,"exited_at":%d}`,
			path, number, now-60000, now)
	}
	// goal takes the name, path and properties as JSON text, so a test can
	// write the NUL escape into any of them.
	goal := func(name, path, properties string, offset int64) string {
		return fmt.Sprintf(`{"type":"goal","name":"%s","goal_type":"lead","path":"%s","page_number":1,"timestamp":%d,"properties":%s}`,
			name, path, now+offset, properties)
	}

	t.Run(`an action carrying a \u0000 costs that action alone`, func(t *testing.T) {
		p := newTrackPipeline(t)
		rec := p.post(t, nulBeat(t, `"actions":[`+strings.Join([]string{
			pageview("/home", 1),
			`{"type":"pageview","path":"/pri\u0000cing","page_number":2}`,
			goal("newsletter", "/home", `{"plan":"pro"}`, 0),
			goal(`sign\u0000up`, "/home", `{}`, 1),
			goal("lead", `/check\u0000out`, `{}`, 2),
			goal("purchase", "/home", `{"pl\u0000an":"pro"}`, 3),
			goal("trial", "/home", `{"plan":"pro\u0000"}`, 4),
		}, ",")+`],"attributes":{"landing_page":"https://shop.example.com/home"}`), nil)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"success":true`)
		assert.NotContains(t, p.rowsJSON(t), `\u0000`)
		require.Len(t, p.pages, 1, "the clean pageview is kept")
		assert.Equal(t, "/home", p.pages[0].Path)
		require.Len(t, p.goals, 1, "the clean goal is kept")
		assert.Equal(t, "newsletter", p.goals[0].GoalName)
		assert.Equal(t, map[string]string{"plan": "pro"}, p.goals[0].Properties)
	})

	t.Run(`session-level text loses the \u0000 and keeps the beat`, func(t *testing.T) {
		p := newTrackPipeline(t)
		rec := p.post(t, nulBeat(t, `"actions":[`+pageview("/home", 1)+`,`+goal("newsletter", "/home", `{}`, 0)+`],`+
			`"attributes":{"landing_page":"https://shop.example.com/sp%00ring\u0000","referrer":"https://news.example.org/a%00b",`+
			`"utm_source":"goo\u0000gle","browser":"Chr\u0000ome","language":"en\u0000-US","user_agent":"Mozilla/5.0\u0000"},`+
			`"dimensions":{"custom_1":"vari\u0000ant"},"sdk_version":"1.0\u00002"`), nil)

		require.Equal(t, http.StatusOK, rec.Code, "a 400 would cost the whole session: these values ride on every beat")
		assert.NotContains(t, p.rowsJSON(t), `\u0000`)
		require.Len(t, p.sessions, 1)
		session := p.sessions[0]
		assert.Equal(t, "google", session.UTMSource)
		assert.Equal(t, "Chrome", session.Browser)
		assert.Equal(t, "en-US", session.Language)
		assert.Equal(t, "Mozilla/5.0", session.UserAgent)
		assert.Equal(t, "variant", session.Custom1)
		assert.Equal(t, "1.02", session.SDKVersion)
		assert.Equal(t, "https://shop.example.com/sp%00ring", session.LandingPage)
		assert.Equal(t, "/sp%00ring", session.LandingPath)
		assert.Equal(t, "/a%00b", session.ReferrerPath)
		require.Len(t, p.goals, 1)
		assert.Equal(t, "google", p.goals[0].UTMSource, "the goal's attribution snapshot is the same clean text")
		require.Len(t, p.pages, 1)
	})

	t.Run("a session id holding one in place of its braces is stored in canonical form", func(t *testing.T) {
		// uuid.Parse never reads the bytes where the braces of a braced id go,
		// so this id parses, and it names the same session as the canonical
		// spelling, which is what reaches the rows.
		p := newTrackPipeline(t)
		id, err := uuid.NewV7()
		require.NoError(t, err)
		rec := p.post(t, nulBeatWithSessionID(`"\u0000`+id.String()+`\u0000"`, `"actions":[`+pageview("/home", 1)+`]`), nil)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.NotContains(t, p.rowsJSON(t), `\u0000`)
		require.Len(t, p.sessions, 1)
		assert.Equal(t, id.String(), p.sessions[0].ID)
		require.Len(t, p.pages, 1)
		assert.Equal(t, id.String(), p.pages[0].SessionID)
	})

	t.Run("a raw NUL byte is not JSON: the beat is refused before anything is buffered", func(t *testing.T) {
		p := newTrackPipeline(t)
		body := strings.Replace(nulBeat(t, `"actions":[`+goal("signNULup", "/home", `{}`, 0)+`]`), "NUL", "\x00", 1)
		require.Contains(t, body, "\x00")

		rec := p.post(t, body, nil)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid JSON payload")
		assert.Zero(t, p.flushes)
	})

	t.Run("a user agent taken from the request header is held to the same rule", func(t *testing.T) {
		// net/http's own server refuses a header holding a NUL before any
		// handler runs; this covers whatever transport does not.
		p := newTrackPipeline(t)
		rec := p.post(t, nulBeat(t, `"actions":[`+pageview("/home", 1)+`],"attributes":{"landing_page":"https://shop.example.com/home"}`),
			map[string]string{"User-Agent": "Mozilla/5.0 (Macintosh)\x00 Chrome/126.0"})

		require.Equal(t, http.StatusOK, rec.Code)
		assert.NotContains(t, p.rowsJSON(t), `\u0000`)
		require.Len(t, p.sessions, 1)
		assert.Equal(t, "Mozilla/5.0 (Macintosh) Chrome/126.0", p.sessions[0].UserAgent)
	})
}
