package app

import (
	"os"
	"path/filepath"

	"github.com/Notifuse/notifuse/pkg/logger"
)

// webAnalyticsSDKRelPath is where the built browser SDK lives, relative to a
// tree that carries it — the repository root in development, /app in the
// image. This mirrors how the console and notification center bundles are
// found (see the paths handed to NewRootHandler).
const webAnalyticsSDKRelPath = "web_analytics_sdk/dist/notifuse-analytics.min.js"

// webAnalyticsSDKSearchDepth bounds how far up from the working directory the
// bundle is looked for. Running from the repository root or from /app finds it
// on the first try; the depth exists so a test binary executing inside
// tests/integration still serves the same asset the server serves in
// production, rather than silently testing a build with no SDK at all.
const webAnalyticsSDKSearchDepth = 5

// loadWebAnalyticsSDK reads the built browser SDK from disk.
//
// The bundle is a static asset rather than a //go:embed for a licensing
// reason: it links ua-parser-js, which is AGPL-3.0-or-later, so compiling it
// into the binary would carry that code into every artifact Notifuse ships.
// Reading it at startup keeps the licence boundary at web_analytics_sdk/,
// where its own LICENSE and NOTICE describe it.
//
// A missing bundle is not fatal, and deliberately so. NewWebAnalyticsHandler
// accepts a nil bundle and then registers no /na.js route, which is the
// correct behaviour for a binary run outside a tree that carries the asset —
// tracking is unavailable, everything else works.
func loadWebAnalyticsSDK(log logger.Logger) []byte {
	dir := "."
	for i := 0; i <= webAnalyticsSDKSearchDepth; i++ {
		path := filepath.Join(dir, webAnalyticsSDKRelPath)
		js, err := os.ReadFile(path)
		if err == nil {
			if len(js) == 0 {
				break
			}
			return js
		}
		if !os.IsNotExist(err) {
			break
		}
		dir = filepath.Join(dir, "..")
	}

	log.WithField("path", webAnalyticsSDKRelPath).
		Warn("web analytics SDK bundle not found; /na.js will not be served")
	return nil
}
