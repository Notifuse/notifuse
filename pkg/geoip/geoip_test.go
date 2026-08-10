package geoip

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/oschwald/geoip2-golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shippedDB is the database committed to the repository, which the Dockerfile
// copies to /app/geoip. Tests run from pkg/geoip.
const shippedDB = "../../data/GeoLite2-City.mmdb"

func indexOf(paths []string, want string) int {
	for i, p := range paths {
		if p == want {
			return i
		}
	}
	return -1
}

type fakeReader struct {
	calls  int
	result *geoip2.City
	err    error
}

func (f *fakeReader) City(ip net.IP) (*geoip2.City, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func parisCity() *geoip2.City {
	city := &geoip2.City{}
	city.Country.IsoCode = "FR"
	city.City.Names = map[string]string{"en": "Paris"}
	city.Subdivisions = []struct {
		Names     map[string]string `maxminddb:"names"`
		IsoCode   string            `maxminddb:"iso_code"`
		GeoNameID uint              `maxminddb:"geoname_id"`
	}{{Names: map[string]string{"en": "Île-de-France"}}}
	city.Location.Latitude = 48.8566
	city.Location.Longitude = 2.3522
	return city
}

func TestResolverDisabled(t *testing.T) {
	resolver, err := New("")
	require.NoError(t, err)
	assert.False(t, resolver.Enabled())

	result, err := resolver.Lookup("203.0.113.10")
	require.NoError(t, err)
	assert.Equal(t, Result{}, result)
	assert.NoError(t, resolver.Close())
}

func TestNewWithMissingDatabase(t *testing.T) {
	_, err := New("/nonexistent/GeoLite2-City.mmdb")
	assert.Error(t, err)
}

func TestLookupSkipsPrivateAndInvalidIPs(t *testing.T) {
	reader := &fakeReader{result: parisCity()}
	resolver := newWithReader(reader)

	for _, ip := range []string{
		"", "not-an-ip", "127.0.0.1", "10.1.2.3", "192.168.1.1", "172.16.0.9",
		"169.254.1.1", "0.0.0.0", "::1", "fe80::1",
	} {
		result, err := resolver.Lookup(ip)
		require.NoError(t, err, ip)
		assert.Equal(t, Result{}, result, ip)
	}
	assert.Zero(t, reader.calls, "the database must never be consulted for local traffic")
}

func TestLookupResolvesAndCaches(t *testing.T) {
	reader := &fakeReader{result: parisCity()}
	resolver := newWithReader(reader)

	first, err := resolver.Lookup("203.0.113.10")
	require.NoError(t, err)
	assert.Equal(t, "FR", first.Country)
	assert.Equal(t, "Île-de-France", first.Region)
	assert.Equal(t, "Paris", first.City)
	require.NotNil(t, first.Latitude)
	assert.InDelta(t, 48.8566, *first.Latitude, 0.0001)

	second, err := resolver.Lookup("203.0.113.10")
	require.NoError(t, err)
	assert.Equal(t, first, second)
	assert.Equal(t, 1, reader.calls, "second lookup must be served from cache")
}

func TestLookupZeroCoordinatesStayNil(t *testing.T) {
	city := &geoip2.City{}
	city.Country.IsoCode = "US"
	resolver := newWithReader(&fakeReader{result: city})

	result, err := resolver.Lookup("203.0.113.11")
	require.NoError(t, err)
	assert.Equal(t, "US", result.Country)
	assert.Nil(t, result.Latitude)
	assert.Nil(t, result.Longitude)
}

func TestRoundCoord(t *testing.T) {
	assert.InDelta(t, 48.86, RoundCoord(48.8566, 2), 1e-9)
	assert.InDelta(t, 48.9, RoundCoord(48.8566, 1), 1e-9)
	assert.InDelta(t, 49, RoundCoord(48.8566, 0), 1e-9)
	assert.InDelta(t, 48.86, RoundCoord(48.8566, 5), 1e-9, "precision clamps to 2")
	assert.InDelta(t, 49, RoundCoord(48.8566, -1), 1e-9, "precision clamps to 0")
	assert.InDelta(t, -2.35, RoundCoord(-2.3522, 2), 1e-9)
}

func TestResolvePath(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "GeoLite2-City.mmdb")
	require.NoError(t, os.WriteFile(existing, []byte("not a real database"), 0o600))

	withDefaults := func(t *testing.T, paths []string) {
		original := DefaultPaths
		DefaultPaths = paths
		t.Cleanup(func() { DefaultPaths = original })
	}

	t.Run("a configured path wins over the shipped database", func(t *testing.T) {
		withDefaults(t, []string{existing})
		assert.Equal(t, "/somewhere/else.mmdb", ResolvePath("/somewhere/else.mmdb"))
	})

	t.Run("a configured path that does not exist is still returned", func(t *testing.T) {
		// So the operator gets "failed to open GeoIP database /typo.mmdb"
		// instead of silent, wrong locations from the shipped database.
		withDefaults(t, []string{existing})
		assert.Equal(t, "/typo.mmdb", ResolvePath("/typo.mmdb"))
	})

	t.Run("falls back to the first readable default", func(t *testing.T) {
		withDefaults(t, []string{filepath.Join(dir, "absent.mmdb"), existing})
		assert.Equal(t, existing, ResolvePath(""))
	})

	t.Run("ignores a directory sitting at a default path", func(t *testing.T) {
		withDefaults(t, []string{dir, existing})
		assert.Equal(t, existing, ResolvePath(""))
	})

	t.Run("no database anywhere yields a disabled resolver", func(t *testing.T) {
		withDefaults(t, []string{filepath.Join(dir, "absent.mmdb")})
		assert.Equal(t, "", ResolvePath(""))

		resolver, err := New(ResolvePath(""))
		require.NoError(t, err)
		assert.False(t, resolver.Enabled())
	})

	t.Run("an operator copy in the mounted data directory beats the shipped one", func(t *testing.T) {
		// compose bind-mounts ./data over /app/data, so the shipped database
		// cannot live there; and a fresher copy dropped in the mount must win
		// without the operator having to set GEOIP_DB_PATH.
		mounted := indexOf(DefaultPaths, "/app/data/GeoLite2-City.mmdb")
		shipped := indexOf(DefaultPaths, "/app/geoip/GeoLite2-City.mmdb")
		require.NotEqual(t, -1, mounted, "the operator drop-in path must be searched")
		require.NotEqual(t, -1, shipped, "the Dockerfile copies the database here")
		assert.Less(t, mounted, shipped, "the operator copy must be searched first")
		assert.Contains(t, DefaultPaths, "data/GeoLite2-City.mmdb", "so `go run ./cmd/api` finds it too")
	})
}

func TestShippedDatabaseResolvesRealAddresses(t *testing.T) {
	if _, err := os.Stat(shippedDB); err != nil {
		t.Skip("shipped database not present")
	}

	resolver, err := New(shippedDB)
	require.NoError(t, err, "the committed database must open with the version of geoip2-golang we build against")
	defer resolver.Close()
	require.True(t, resolver.Enabled())

	// Country only: cities and coordinates move between MaxMind builds, and
	// the point of this test is that the shipped file is a working City
	// database, not that any given block still maps to a given town.
	for ip, country := range map[string]string{
		"8.8.8.8":      "US",
		"212.27.48.10": "FR",
		"2a01:e0a::1":  "FR", // IPv6 must resolve too
	} {
		result, err := resolver.Lookup(ip)
		require.NoError(t, err, ip)
		assert.Equal(t, country, result.Country, ip)
	}

	private, err := resolver.Lookup("192.168.1.1")
	require.NoError(t, err)
	assert.Equal(t, Result{}, private, "private addresses never reach the database")
}
