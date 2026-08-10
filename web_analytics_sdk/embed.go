// Package webanalyticssdk embeds the built browser SDK so the Go binary
// serves it directly (routes /na.js and /na.<hash>.js). The dist file is
// committed so `go build` works from a fresh clone; the Dockerfile rebuilds
// it from source at image build time.
package webanalyticssdk

import _ "embed"

//go:embed dist/notifuse-analytics.min.js
var JS []byte
