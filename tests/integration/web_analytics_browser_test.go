//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// findChrome locates a Chrome/Chromium binary, or returns "" when none is
// installed.
func findChrome() string {
	candidates := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		"/usr/bin/google-chrome",
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
	}
	if runtime.GOOS == "windows" {
		candidates = append(candidates, `C:\Program Files\Google\Chrome\Application\chrome.exe`)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	for _, name := range []string{"google-chrome", "chromium", "chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// TestWebAnalyticsBrowserEndToEnd drives a real browser against the real
// server with the real minified bundle.
//
// It exists because every other test in this package hand-builds its payloads,
// which cannot prove the SDK and the server agree. That blind spot already hid
// a total-data-loss bug once: the SDK did not send `seq`, so the server's
// `beat_seq >` upsert guard would have frozen every session on its first beat —
// one pageview, no duration, no goals — while all hand-built tests passed.
//
// Enable with BROWSER_E2E=true (needs Chrome installed).
func TestWebAnalyticsBrowserEndToEnd(t *testing.T) {
	if os.Getenv("BROWSER_E2E") != "true" {
		t.Skip("set BROWSER_E2E=true to run the browser end-to-end test")
	}
	chrome := findChrome()
	if chrome == "" {
		t.Skip("no Chrome/Chromium binary found")
	}
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	// Both install shapes must work. The generated snippet uses `async`, but
	// customers paste the tag by hand and tag managers rewrite it, so the
	// blocking variant — which runs the SDK from <head> while document.body is
	// still null — is the one that actually breaks in the field.
	for _, mode := range []struct{ name, scriptTag string }{
		{"async snippet", `<script async src="%s/na.js"></script>`},
		{"blocking script in head", `<script src="%s/na.js"></script>`},
	} {
		t.Run(mode.name, func(t *testing.T) {
			runBrowserVisit(t, suite, chrome, mode.scriptTag)
		})
	}
}

// runBrowserVisit performs one full visit (landing → SPA navigation → goal) in
// a real browser on its own workspace, then asserts what reached Postgres.
func runBrowserVisit(t *testing.T, suite *testutil.IntegrationTestSuite, chrome, scriptTag string) {
	t.Helper()

	workspace, err := suite.DataFactory.CreateWorkspace(func(w *domain.Workspace) {
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{
			Enabled: true,
			Filters: domain.DefaultWebFilters(),
		}
	})
	require.NoError(t, err)

	apiURL := suite.ServerManager.GetURL()
	buffer := suite.ServerManager.GetApp().GetWebAnalyticsBuffer()
	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	// A page that loads the SDK exactly as a customer would, then navigates
	// (SPA) and converts. The extra beats are the point: they only persist if
	// the SDK sends a monotonic seq the server accepts.
	var diagMu sync.Mutex
	var diag string
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/diag" {
			body, _ := io.ReadAll(r.Body)
			diagMu.Lock()
			diag = string(body)
			diagMu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!doctype html><html><head><title>WA browser E2E</title>
<script>
  // Installed before the SDK so that a failure during its own initialization
  // is captured too — that is exactly what the blocking variant exercises.
  window.__errs = [];
  window.addEventListener('error', function (e) { window.__errs.push('onerror: ' + e.message); });
  window.addEventListener('unhandledrejection', function (e) {
    window.__errs.push('rejection: ' + (e.reason && (e.reason.stack || e.reason.message) || e.reason));
  });
  ['error', 'warn'].forEach(function (lvl) {
    var orig = console[lvl];
    console[lvl] = function () {
      window.__errs.push(lvl + ': ' + Array.prototype.join.call(arguments, ' '));
      orig.apply(console, arguments);
    };
  });
</script>
<script>window.NotifuseAnalyticsConfig={workspace_id:%q,endpoint:%q};</script>
`+scriptTag+`
</head><body style="font-family:sans-serif;padding:20px">
<h1>Notifuse Web Analytics — browser end-to-end</h1>
<p>SDK: <b id="diag">?</b></p>
<div style="height:3000px"></div>
<script>
  // Give the SDK time to auto-init and send the first beat, then produce a
  // second pageview and a conversion.
  setTimeout(function () {
    try { window.scrollTo(0, 1500); } catch (e) {}
    history.pushState({}, '', '/pricing');
  }, 1500);
  setTimeout(function () {
    var sdk = window.NotifuseAnalytics;
    if (sdk && typeof sdk.trackGoal === 'function') {
      // trackGoal rejects an untyped goal. The catch is not decoration: the SDK
      // loads cross-origin without crossorigin="anonymous", exactly as a
      // customer's snippet does, so Chrome mutes its rejections from the
      // window 'unhandledrejection' hook above and the failure would otherwise
      // reach the assertions as an unexplained goals=0.
      sdk.trackGoal({ action: 'browser_e2e', type: 'other', value: 42 })
        .catch(function (e) { window.__errs.push('trackGoal: ' + (e && (e.stack || e.message) || e)); });
    }
  }, 3000);
  // Report state back to this page's own origin so a failure explains itself.
  setTimeout(function () {
    var sdk = window.NotifuseAnalytics;
    Promise.resolve()
      .then(function () { return sdk.init(window.NotifuseAnalyticsConfig); })
      .then(function () { return sdk.getSessionId(); })
      .catch(function (e) {
        window.__errs.push('init/getSessionId: ' + (e && (e.stack || e.message) || e));
        return 'error';
      })
      .then(function (sid) {
        fetch('/diag', {
          method: 'POST',
          body: JSON.stringify({
            global: typeof window.NotifuseAnalytics,
            hasTrackGoal: !!(sdk && sdk.trackGoal),
            webdriver: navigator.webdriver,
            session: sid,
            errors: window.__errs.slice(0, 6),
            ua: navigator.userAgent.slice(0, 40)
          })
        });
      });
  }, 6000);
</script>
</body></html>`, workspace.ID, apiURL, apiURL)
	}))
	defer page.Close()

	// Not t.TempDir(): its cleanup fails the test when Chrome is still flushing
	// its profile as the subtest ends. Removing it is housekeeping, not an
	// assertion.
	profile, err := os.MkdirTemp("", "wa-chrome-profile-")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(profile) }()

	landing := page.URL + "/?utm_source=google&utm_medium=cpc&gclid=e2e-browser&custom_1=variant-a"

	cmd := exec.Command(chrome,
		"--headless=new", "--disable-gpu", "--no-sandbox",
		"--user-data-dir="+profile,
		// A realistic UA: the SDK's own bot detection correctly refuses to
		// track a browser advertising itself as HeadlessChrome.
		"--user-agent=Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		landing,
	)
	require.NoError(t, cmd.Start())
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait() // reap before the profile directory is removed
	}()

	// Poll until the browser's beats have landed.
	deadline := time.Now().Add(90 * time.Second)
	var sessions, pages, goals int
	for {
		if time.Now().After(deadline) {
			diagMu.Lock()
			d := diag
			diagMu.Unlock()
			t.Fatalf("no browser beat arrived in time (sessions=%d pages=%d goals=%d)\npage diagnostic: %s",
				sessions, pages, goals, d)
		}
		buffer.FlushAll(context.Background())
		_ = wsDB.QueryRow(`SELECT COUNT(*) FROM web_sessions`).Scan(&sessions)
		_ = wsDB.QueryRow(`SELECT COUNT(*) FROM web_pages`).Scan(&pages)
		_ = wsDB.QueryRow(`SELECT COUNT(*) FROM web_goals`).Scan(&goals)
		if sessions > 0 && pages >= 2 && goals > 0 {
			break
		}
		time.Sleep(time.Second)
	}

	assert.Equal(t, 1, sessions, "the whole visit must upsert into a single session row")

	var beatSeq int64
	var pageviewCount, maxScroll int
	var channel, device, browser, os_ string
	require.NoError(t, wsDB.QueryRow(`
		SELECT beat_seq, pageview_count, max_scroll, channel, device, browser, os
		FROM web_sessions LIMIT 1`).
		Scan(&beatSeq, &pageviewCount, &maxScroll, &channel, &device, &browser, &os_))

	// The regression this test exists for: later beats must actually apply.
	assert.Greater(t, beatSeq, int64(1),
		"the session must have been updated by beats after the first (seq must advance)")
	assert.Equal(t, 2, pageviewCount, "the SPA navigation must be recorded on the same session")

	// Device fields are parsed in the browser by the SDK and sent as-is.
	assert.Equal(t, "desktop", device)
	assert.Equal(t, "Chrome", browser)
	assert.Contains(t, []string{"macOS", "Mac OS"}, os_)

	// Attribution ran server-side on the real query string.
	assert.Equal(t, "google-ads", channel, "gclid must be classified by the default rules")

	var custom1 string
	require.NoError(t, wsDB.QueryRow(`SELECT custom_1 FROM web_sessions LIMIT 1`).Scan(&custom1))
	assert.Equal(t, "variant-a", custom1, "custom_1 must be captured from the landing URL")

	var goalName string
	var goalValue float64
	require.NoError(t, wsDB.QueryRow(`SELECT goal_name, goal_value FROM web_goals LIMIT 1`).Scan(&goalName, &goalValue))
	assert.Equal(t, "browser_e2e", goalName)
	assert.InDelta(t, 42.0, goalValue, 0.01)

	var exitPages int
	require.NoError(t, wsDB.QueryRow(`SELECT COUNT(*) FROM web_pages WHERE is_exit`).Scan(&exitPages))
	assert.Equal(t, 1, exitPages, "exactly one page carries the exit flag")

	diagMu.Lock()
	d := diag
	diagMu.Unlock()
	assert.NotContains(t, d, "scrollHeight",
		"the SDK must initialize without touching a document.body that does not exist yet")

	t.Logf("browser E2E: beat_seq=%d pageviews=%d scroll=%d%% channel=%s %s/%s/%s",
		beatSeq, pageviewCount, maxScroll, channel, device, browser, os_)
}
