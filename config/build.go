package config

import "strings"

// GitCommit is the commit the binary was built from. It is set at build time, as the Dockerfile does with the
// GIT_COMMIT build argument the Docker workflows pass:
//
//	go build -ldflags "-X github.com/Notifuse/notifuse/config.GitCommit=<sha>" ./cmd/api
//
// A plain go build leaves it empty. Two builds of the same VERSION, such as a release rebuilt under the same
// tag, can then still be told apart from the startup log.
var GitCommit = ""

// shortCommitLength is how much of the commit the startup log prints, as git log --oneline does.
const shortCommitLength = 8

// BuildLabel is the version as the startup log prints it: "45.0 (commit 5ff3a16d)", or just "45.0" when the
// binary was built without a commit.
func BuildLabel() string {
	commit := strings.TrimSpace(GitCommit)
	if commit == "" {
		return VERSION
	}
	if len(commit) > shortCommitLength {
		commit = commit[:shortCommitLength]
	}
	return VERSION + " (commit " + commit + ")"
}
