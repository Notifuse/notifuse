package main

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
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

// composeService is the part of a Compose service these tests read. Both shipped files use
// the short syntax for volumes and the list syntax for environment.
type composeService struct {
	Image       string                       `yaml:"image"`
	Volumes     []string                     `yaml:"volumes"`
	Environment []string                     `yaml:"environment"`
	DependsOn   map[string]map[string]string `yaml:"depends_on"`
}

func readComposeServices(t *testing.T, file string) map[string]composeService {
	t.Helper()
	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	var compose struct {
		Services map[string]composeService `yaml:"services"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &compose))
	return compose.Services
}

// volumeTargets maps each volume's container path to its source.
func volumeTargets(volumes []string) map[string]string {
	targets := map[string]string{}
	for _, v := range volumes {
		parts := strings.Split(v, ":")
		if len(parts) >= 2 {
			targets[parts[1]] = parts[0]
		}
	}
	return targets
}

// PostgreSQL 18's image keeps its data in /var/lib/postgresql/18/docker and checks for data
// of an older major only with PGDATA at that default. So the image's major and the mount
// path go together: 17 at /var/lib/postgresql/data, 18 and later at /var/lib/postgresql. A
// tag bumped without the mount, or a PGDATA, starts an empty database beside the real one.
func TestComposePostgresImageAndMountAreAMatchedPair(t *testing.T) {
	for _, file := range []string{"../../compose.yaml", "../../compose.alloydb.yaml"} {
		t.Run(file, func(t *testing.T) {
			checked := 0
			for name, service := range readComposeServices(t, file) {
				image := strings.SplitN(service.Image, "@", 2)[0]
				repo, tag, _ := strings.Cut(image, ":")
				var want string
				switch {
				case repo == "google/alloydbomni":
					want = "/var/lib/postgresql/data" // AlloyDB Omni has no public 18 (see the file)
				case repo == "postgres" || repo == "pgautoupgrade/pgautoupgrade":
					major, err := strconv.Atoi(strings.SplitN(strings.SplitN(tag, "-", 2)[0], ".", 2)[0])
					require.NoError(t, err, "major version in the tag of %s: %s", name, service.Image)
					want = "/var/lib/postgresql/data"
					if major >= 18 {
						want = "/var/lib/postgresql"
					}
				default:
					continue
				}
				checked++

				targets := volumeTargets(service.Volumes)
				assert.Contains(t, targets, want, "%s (%s) must mount its data volume at %s", name, service.Image, want)
				for target := range targets {
					if strings.HasPrefix(target, "/var/lib/postgresql") {
						assert.Equal(t, want, target, "%s (%s) mounts %s", name, service.Image, target)
					}
				}
				for _, env := range service.Environment {
					assert.False(t, strings.HasPrefix(env, "PGDATA="), "%s sets PGDATA, which turns off the old-data check", name)
				}
			}
			assert.Positive(t, checked, "a PostgreSQL service in %s", file)
		})
	}
}

// PostgreSQL starts only once pg-upgrade has moved older data to 18, and pg-upgrade keeps its
// copy of the old data on another volume: the upgrade deletes whatever is left on the data
// volume.
func TestComposePostgresWaitsForTheUpgrade(t *testing.T) {
	services := readComposeServices(t, "../../compose.yaml")
	postgres, upgrade := services["postgres"], services["pg-upgrade"]
	require.NotEmpty(t, upgrade.Image, "a pg-upgrade service")

	assert.Equal(t, "service_completed_successfully", postgres.DependsOn["pg-upgrade"]["condition"])
	assert.Equal(t, "service_healthy", services["api"].DependsOn["postgres"]["condition"])

	data := volumeTargets(postgres.Volumes)["/var/lib/postgresql"]
	require.NotEmpty(t, data)
	upgradeVolumes := volumeTargets(upgrade.Volumes)
	assert.Equal(t, data, upgradeVolumes["/var/lib/postgresql"], "pg-upgrade works on the postgres data volume")
	backup := upgradeVolumes["/backup"]
	assert.NotEmpty(t, backup, "a backup volume at /backup")
	assert.NotEqual(t, data, backup, "the copy is not on the data volume")

	// pgautoupgrade connects to POSTGRES_DB, which defaults to POSTGRES_USER: with a DB_USER
	// other than postgres that names no database, and the upgrade fails half-way.
	assert.Contains(t, upgrade.Environment, "POSTGRES_DB=${POSTGRES_DB:-postgres}")
	assert.Contains(t, postgres.Environment, "POSTGRES_DB=${POSTGRES_DB:-postgres}")
}

// The default image is the release this file ships with, so `git pull && docker compose up -d`
// upgrades to it. Bumping VERSION without this line fails here.
func TestComposeDefaultImageIsThisVersion(t *testing.T) {
	for _, file := range []string{"../../compose.yaml", "../../compose.alloydb.yaml"} {
		api := readComposeServices(t, file)["api"]
		assert.Equal(t, "notifuse/notifuse:${NOTIFUSE_VERSION:-v"+config.VERSION+"}", api.Image, file)
	}
}
