//go:build integration
// +build integration

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestWebAnalyticsBrowserIdentify drives a real browser through both ways a
// visitor becomes a known contact.
//
// Nothing else proves the two halves of identity agree. The SDK writes
// contact_email / contact_email_hmac / identify_token; the server reads them off
// domain.WebTrackPayload. Neither side typechecks the other, so renaming a field
// on either one turns every identified visitor anonymous while every hand-built
// payload in this package keeps passing — the same blind spot that once hid an
// SDK that did not send `seq` at all.
//
// The credentials are minted here, in Go, exactly as a customer's backend mints
// them, and every assertion is on what Postgres holds: SDK state would only
// prove the SDK agrees with itself.
//
// Enable with BROWSER_E2E=true (needs Chrome installed).
func TestWebAnalyticsBrowserIdentify(t *testing.T) {
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

	t.Run("identify() attaches the contact", func(t *testing.T) {
		runBrowserIdentifyCall(t, suite, chrome)
	})

	t.Run("an email-click nf_id token identifies with no code", func(t *testing.T) {
		runBrowserIdentifyToken(t, suite, chrome)
	})
}

// runBrowserIdentifyCall exercises the credential a customer's own backend
// mints: identify(email, hmac) from page JS, then a conversion.
func runBrowserIdentifyCall(t *testing.T, suite *testutil.IntegrationTestSuite, chrome string) {
	t.Helper()

	// Stored lowercased, claimed in mixed case. Both sides document this split —
	// the SDK sends the address exactly as signed, the server verifies the raw
	// string and only then normalizes — so a browser that "helpfully" lowercases
	// the address before sending would invalidate the HMAC and silently drop the
	// identity.
	const contactEmail = "browser.identify@example.com"
	const claimedEmail = "Browser.Identify@Example.com"

	workspace, wsDB := waBrowserIdentityWorkspace(t, suite, contactEmail)
	apiURL := suite.ServerManager.GetURL()
	buffer := suite.ServerManager.GetApp().GetWebAnalyticsBuffer()
	flush := func() { buffer.FlushAll(context.Background()) }

	hmac := domain.ComputeWebIdentifyHMAC(claimedEmail, workspace.Settings.SecretKey)
	require.NotEmpty(t, hmac)

	page, diag := waBrowserIdentityPage(fmt.Sprintf(`<!doctype html><html><head><title>WA identify E2E</title>
<script>
  // Installed before the SDK so a failure during its own initialization is
  // captured too.
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
  function whenReady(fn) {
    var tries = 0;
    (function poll() {
      var sdk = window.NotifuseAnalytics;
      if (sdk && typeof sdk.identify === 'function') { fn(sdk); return; }
      if (++tries > 100) { window.__errs.push('the SDK never installed itself'); return; }
      setTimeout(poll, 100);
    })();
  }
  function report(sdk) {
    return Promise.all([sdk.getSessionId(), sdk.getIdentity()]).then(function (state) {
      fetch('/diag', { method: 'POST', body: JSON.stringify({
        session: state[0],
        identity: state[1] ? Object.keys(state[1]).sort().join(',') : null,
        errors: window.__errs.slice(0, 6),
        ua: navigator.userAgent.slice(0, 40)
      })});
    }).catch(function () {});
  }
</script>
<script>window.NotifuseAnalyticsConfig={workspace_id:%q,endpoint:%q};</script>
<script async src="%s/na.js"></script>
</head><body style="font-family:sans-serif;padding:20px">
<h1>Notifuse Web Analytics — identify()</h1>
<script>
  // Minted server-side by the test, as a customer's backend mints it: the
  // browser is never trusted to sign anything.
  var EMAIL = %q, HMAC = %q;
  whenReady(function (sdk) {
    sdk.identify(EMAIL, HMAC)
      .then(function () { return sdk.trackGoal({ action: 'browser_identify', type: 'other', value: 12.5 }); })
      .catch(function (e) {
        window.__errs.push('identify/trackGoal: ' + (e && (e.stack || e.message) || e));
      })
      .then(function () { return report(sdk); });
  });
</script>
</body></html>`, workspace.ID, apiURL, apiURL, claimedEmail, hmac))
	defer page.Close()

	// Deferred in this frame, after the page server's Close, so LIFO stops Chrome
	// while the origin it is beating at is still answering.
	stopBrowser := waLaunchBrowser(t, chrome, page.URL+"/pricing")
	defer stopBrowser()

	// The goal and the identity ride the same beat — trackGoal sends
	// immediately, and identify() resolved before it — so once the goal is
	// stored the identity has already been decided. Anything missing below is a
	// disagreement about the wire, not a race.
	waWaitForBrowserBeats(t, "the browser's goal", 90*time.Second, flush, diag, func() bool {
		var goals int
		_ = wsDB.QueryRow(`SELECT COUNT(*) FROM web_goals`).Scan(&goals)
		return goals > 0
	})

	// Asserted before the LIMIT 1 reads below, which only mean "the visit's row"
	// while there is exactly one of them.
	var sessions int
	require.NoError(t, wsDB.QueryRow(`SELECT COUNT(*) FROM web_sessions`).Scan(&sessions))
	require.Equal(t, 1, sessions, "the whole visit must upsert into a single session row")

	var sessionEmail, goalEmail *string
	require.NoError(t, wsDB.QueryRow(
		`SELECT contact_email FROM web_sessions LIMIT 1`).Scan(&sessionEmail))
	require.NoError(t, wsDB.QueryRow(
		`SELECT contact_email FROM web_goals LIMIT 1`).Scan(&goalEmail))

	require.NotNil(t, sessionEmail,
		"identify()'s credential never reached the server as a verifiable pair\npage diagnostic: %s", diag())
	assert.Equal(t, contactEmail, *sessionEmail,
		"the address is signed raw and stored normalized")
	require.NotNil(t, goalEmail, "the goal must carry the identity of the beat that brought it")
	assert.Equal(t, contactEmail, *goalEmail)

	// The third table stamped from that one attribution snapshot, and the one the
	// mid-session re-stamp rule exists for: a page row is inserted by whichever
	// beat reached it first, so a server that stamped sessions and goals but let
	// pages keep their insert-time value would lose the pages of every visitor who
	// identifies after landing — while the two assertions above stayed green.
	var pageRows, identifiedPageRows int
	require.NoError(t, wsDB.QueryRow(
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE contact_email = $1) FROM web_pages`,
		contactEmail).Scan(&pageRows, &identifiedPageRows))
	require.NotZero(t, pageRows, "the beat that carried the goal carries its pageview too")
	assert.Equal(t, pageRows, identifiedPageRows,
		"every page of an identified visit must name the contact")

	// The bridge runs inside the same flush that wrote the goal — but the
	// background flusher may be the one running it, and it emits just after the
	// batch commits, so the goal row can be visible for an instant before the
	// custom_events row is. Waiting closes that window without weakening
	// anything: a goal that reached Postgres anonymous to the contact side never
	// becomes bridged, however long we wait.
	bridged := func() int {
		var events int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM custom_events WHERE email = $1 AND event_name = $2`,
			contactEmail, "browser_identify").Scan(&events))
		return events
	}
	waWaitForBrowserBeats(t, "the bridged custom_events row", 30*time.Second, flush, diag, func() bool {
		return bridged() > 0
	})
	require.Equal(t, 1, bridged(), "the browser's conversion must reach the contact exactly once")

	var source string
	var goalValue *float64
	require.NoError(t, wsDB.QueryRow(
		`SELECT source, goal_value FROM custom_events WHERE email = $1 AND event_name = $2`,
		contactEmail, "browser_identify").Scan(&source, &goalValue))
	assert.Equal(t, "web_analytics", source, "the source distinguishes bridged goals from API ones")
	require.NotNil(t, goalValue)
	assert.InDelta(t, 12.5, *goalValue, 0.01, "the value the browser passed must survive the trip")

	t.Logf("browser identify(): contact=%s goal_value=%v", *sessionEmail, *goalValue)
}

// runBrowserIdentifyToken exercises the credential Notifuse itself mints into a
// tracked email link: nf_id, adopted with no customer code at all.
func runBrowserIdentifyToken(t *testing.T, suite *testutil.IntegrationTestSuite, chrome string) {
	t.Helper()

	const contactEmail = "token.reader@example.com"

	workspace, wsDB := waBrowserIdentityWorkspace(t, suite, contactEmail)
	apiURL := suite.ServerManager.GetURL()
	buffer := suite.ServerManager.GetApp().GetWebAnalyticsBuffer()
	flush := func() { buffer.FlushAll(context.Background()) }

	token, err := domain.BuildWebIdentifyToken(
		contactEmail, workspace.Settings.SecretKey, 30*24*time.Hour, time.Now().UTC())
	require.NoError(t, err)
	require.NotEmpty(t, token)

	page, diag := waBrowserIdentityPage(fmt.Sprintf(`<!doctype html><html><head><title>WA nf_id E2E</title>
<script>
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
<script async src="%s/na.js"></script>
</head><body style="font-family:sans-serif;padding:20px">
<h1>Notifuse Web Analytics — nf_id</h1>
<script>
  // No customer code participates: the whole point is that landing on the link
  // is enough. This only reports what the page ended up with.
  var tries = 0;
  (function poll() {
    var sdk = window.NotifuseAnalytics;
    if (!sdk || typeof sdk.getIdentity !== 'function') {
      if (++tries > 100) { window.__errs.push('the SDK never installed itself'); return; }
      setTimeout(poll, 100);
      return;
    }
    Promise.all([sdk.getSessionId(), sdk.getIdentity()]).then(function (state) {
      fetch('/diag', { method: 'POST', body: JSON.stringify({
        session: state[0],
        identity: state[1] ? Object.keys(state[1]).sort().join(',') : null,
        search: window.location.search,
        errors: window.__errs.slice(0, 6),
        ua: navigator.userAgent.slice(0, 40)
      })});
    }).catch(function () {});
  })();
</script>
</body></html>`, workspace.ID, apiURL, apiURL))
	defer page.Close()

	// utm_source rides along so the assertions below can tell "nf_id was
	// stripped" apart from "the query string was thrown away wholesale".
	landing := page.URL + "/?nf_id=" + token + "&utm_source=newsletter"
	// Deferred in this frame, after the page server's Close, so LIFO stops Chrome
	// while the origin it is beating at is still answering.
	stopBrowser := waLaunchBrowser(t, chrome, landing)
	defer stopBrowser()

	// The token is consumed during init, before the first beat leaves the page,
	// so the very first session row must already name the contact.
	waWaitForBrowserBeats(t, "the browser's first beat", 90*time.Second, flush, diag, func() bool {
		var sessions int
		_ = wsDB.QueryRow(`SELECT COUNT(*) FROM web_sessions`).Scan(&sessions)
		return sessions > 0
	})

	// Asserted before the LIMIT 1 read below, which only means "the visit's row"
	// while there is exactly one of them.
	var sessions int
	require.NoError(t, wsDB.QueryRow(`SELECT COUNT(*) FROM web_sessions`).Scan(&sessions))
	require.Equal(t, 1, sessions, "the whole visit must upsert into a single session row")

	var storedEmail *string
	var landingPage string
	require.NoError(t, wsDB.QueryRow(
		`SELECT contact_email, landing_page FROM web_sessions LIMIT 1`).Scan(&storedEmail, &landingPage))

	require.NotNil(t, storedEmail,
		"the nf_id token never reached the server as identify_token\npage diagnostic: %s", diag())
	assert.Equal(t, contactEmail, *storedEmail,
		"landing on the link is the whole integration: no page code identified this visitor")

	// The page rows are stamped from the same snapshot as the session. The token
	// is consumed during init, so here the very first insert already knows the
	// contact — nothing has to re-stamp anything for these to match.
	var pageRows, identifiedPageRows int
	require.NoError(t, wsDB.QueryRow(
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE contact_email = $1) FROM web_pages`,
		contactEmail).Scan(&pageRows, &identifiedPageRows))
	require.NotZero(t, pageRows, "a beat that opened a session carried a pageview with it")
	assert.Equal(t, pageRows, identifiedPageRows,
		"every page of an identified visit must name the contact")

	// The regression guard. The strip used to happen after getOrCreateSession()
	// had already snapshotted window.location.href, which froze a
	// workspace-signed bearer credential into the session's landing page — sent
	// back on every beat, and from there into the customer's own reports, their
	// server logs and every third-party Referer.
	assert.NotContains(t, landingPage, token,
		"the identify token must never be stored as part of the landing page")
	assert.NotContains(t, landingPage, "nf_id",
		"not even the parameter name should survive into the landing page")
	assert.Contains(t, landingPage, "utm_source=newsletter",
		"stripping must remove nf_id alone, not the query string it lives in")

	t.Logf("browser nf_id: contact=%s landing=%s", *storedEmail, landingPage)
}

// waBrowserIdentityWorkspace creates a workspace that both stores identities and
// bridges their goals onto the contact, plus the contact the browser will claim
// to be. Each subtest gets its own so one visit's rows cannot answer another's
// assertions.
func waBrowserIdentityWorkspace(t *testing.T, suite *testutil.IntegrationTestSuite, email string) (*domain.Workspace, *sql.DB) {
	t.Helper()

	workspace, err := suite.DataFactory.CreateWorkspace(func(w *domain.Workspace) {
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{
			Enabled:              true,
			Filters:              domain.DefaultWebFilters(),
		}
	})
	require.NoError(t, err)
	require.NotEmpty(t, workspace.Settings.SecretKey, "the workspace secret is what signs an identity")

	// Ingest refuses to remember an address that is not already a contact, so
	// without this the credential would verify and the identity still be dropped.
	_, err = suite.DataFactory.CreateContact(workspace.ID, func(c *domain.Contact) {
		c.Email = email
	})
	require.NoError(t, err)

	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	return workspace, wsDB
}

// waBrowserIdentityPage serves the given page and captures the diagnostic it
// posts back to /diag, so a timeout reports what the browser did rather than
// only what the database is missing.
func waBrowserIdentityPage(html string) (*httptest.Server, func() string) {
	var mu sync.Mutex
	var diag string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/diag" {
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			diag = string(body)
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, html)
	}))

	return server, func() string {
		mu.Lock()
		defer mu.Unlock()
		return diag
	}
}

// waLaunchBrowser opens url in headless Chrome and returns the stop that kills
// the process and removes its profile.
//
// The caller defers that stop itself, in the same frame as the page server's
// Close and after it, so LIFO stops the browser first. t.Cleanup would run it
// after the subtest function returned — with the page server already closed
// under a browser still posting to /diag. httptest's Close blocks on the request
// in flight and the one issued just after is refused, which costs exactly the
// diagnostic a failing run needs.
func waLaunchBrowser(t *testing.T, chrome, url string) func() {
	t.Helper()

	// A profile per subtest, and not t.TempDir(). The origins are already
	// distinct — each subtest serves from its own ephemeral port and localStorage
	// is keyed by origin — but a released port can be handed straight back to the
	// next subtest, and a shared profile would then resurrect the previous visit's
	// session id and landing page into a workspace that never saw them.
	// t.TempDir() is the wrong owner for the removal: its cleanup fails the test
	// when Chrome is still flushing its profile as the subtest ends.
	profile, err := os.MkdirTemp("", "wa-chrome-identity-")
	require.NoError(t, err)

	cmd := exec.Command(chrome,
		"--headless=new", "--disable-gpu", "--no-sandbox",
		"--user-data-dir="+profile,
		// A realistic UA: the SDK's own bot detection correctly refuses to
		// track a browser advertising itself as HeadlessChrome.
		"--user-agent=Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		url,
	)
	require.NoError(t, cmd.Start())
	return func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait() // reap before the profile directory is removed
		_ = os.RemoveAll(profile)
	}
}

// waWaitForBrowserBeats flushes the ingest buffer until landed reports the
// browser's beats are in Postgres.
func waWaitForBrowserBeats(t *testing.T, what string, timeout time.Duration, flush func(), diag func() string, landed func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		flush()
		if landed() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s never reached the database\npage diagnostic: %s", what, diag())
		}
		time.Sleep(time.Second)
	}
}
