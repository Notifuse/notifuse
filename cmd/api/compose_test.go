package main

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Docker kills a container stop_grace_period after SIGTERM, 10 s by default. The Compose files
// must give the api service more than the graceful shutdown takes: Shutdown can run a couple of
// seconds past gracefulShutdownTimeout, and the database still has to close.
func TestComposeFilesGiveTheGracefulShutdownTime(t *testing.T) {
	for _, file := range []string{"../../compose.yaml", "../../compose.alloydb.yaml"} {
		t.Run(file, func(t *testing.T) {
			raw, err := os.ReadFile(file)
			require.NoError(t, err)
			var compose struct {
				Services map[string]struct {
					StopGracePeriod string `yaml:"stop_grace_period"`
				} `yaml:"services"`
			}
			require.NoError(t, yaml.Unmarshal(raw, &compose))
			api, ok := compose.Services["api"]
			require.True(t, ok, "an api service")

			grace, err := time.ParseDuration(api.StopGracePeriod)
			require.NoError(t, err, "stop_grace_period of the api service")
			assert.GreaterOrEqual(t, grace, gracefulShutdownTimeout+5*time.Second)
		})
	}
}

func TestGracefulShutdownTimeout(t *testing.T) {
	assert.Equal(t, 70*time.Second, gracefulShutdownTimeout)
}
