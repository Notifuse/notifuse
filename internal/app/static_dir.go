package app

import (
	"os"
	"path/filepath"
)

// notificationCenterRelPath is where the built notification center lives, relative to a
// tree that carries it: the repository root in development, /app in the image.
const notificationCenterRelPath = "notification_center/dist"

// resolveStaticDir returns the directory of the built front-end bundle relPath (one that
// holds an index.html), relative to the working directory. It is looked for as the web
// analytics SDK is (loadWebAnalyticsSDK): running from the repository root or from /app
// finds it on the first try and returns relPath itself, and a test binary executing
// inside tests/integration finds the bundle the repository built, so a browser test
// opens the page the server really serves. When no directory up to the search depth
// holds it, relPath is returned unchanged and the server answers 404 for those pages,
// as it does without a build.
func resolveStaticDir(relPath string) string {
	dir := "."
	for i := 0; i <= webAnalyticsSDKSearchDepth; i++ {
		candidate := filepath.Join(dir, relPath)
		if info, err := os.Stat(filepath.Join(candidate, "index.html")); err == nil && !info.IsDir() {
			return candidate
		}
		dir = filepath.Join(dir, "..")
	}
	return relPath
}
