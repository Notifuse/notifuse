package config

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withGitCommit(t *testing.T, commit string) {
	t.Helper()
	previous := GitCommit
	GitCommit = commit
	t.Cleanup(func() { GitCommit = previous })
}

func TestBuildLabel(t *testing.T) {
	tests := []struct {
		name   string
		commit string
		want   string
	}{
		{"a plain go build prints the version alone", "", VERSION},
		{"a blank value counts as none", "  ", VERSION},
		{"a full SHA is shortened", "5ff3a16d90169a700fcc919ccae14378ef13671f", VERSION + " (commit 5ff3a16d)"},
		{"a value already short is kept", "5ff3a16", VERSION + " (commit 5ff3a16)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withGitCommit(t, tt.commit)
			assert.Equal(t, tt.want, BuildLabel())
		})
	}
}

// The Dockerfile sets GitCommit through -ldflags -X, which silently does nothing when the path names no variable.
// This keeps its path in step with the package and the variable name.
func TestDockerfileSetsGitCommit(t *testing.T) {
	data, err := os.ReadFile("../Dockerfile")
	require.NoError(t, err)
	dockerfile := string(data)

	builder := dockerfile[strings.Index(dockerfile, "AS backend-builder"):]
	builder = builder[:strings.Index(builder, "\nFROM ")]
	assert.Contains(t, builder, "ARG GIT_COMMIT", "the build argument is declared in the stage that builds the server")
	assert.Contains(t, builder, "-X github.com/Notifuse/notifuse/config.GitCommit=${GIT_COMMIT}")
}
