// Package geoip resolves IP addresses to coarse locations using a MaxMind
// GeoLite2/GeoIP2 City database. A City database ships with Notifuse and is
// found automatically (see ResolvePath), so geo enrichment works out of the
// box; GEOIP_DB_PATH, or a copy in the mounted data directory, overrides it
// with a fresher or commercial database. The
// database stays optional in the sense that a missing or unreadable file
// leaves the resolver disabled and every lookup returns an empty result,
// rather than failing anything.
//
// This product includes GeoLite2 data created by MaxMind, available from
// https://www.maxmind.com.
package geoip

import (
	"fmt"
	"math"
	"net"
	"os"
	"sync"
	"time"

	"github.com/oschwald/geoip2-golang"
)

const (
	cacheMaxEntries = 10000
	cacheTTL        = 5 * time.Minute
)

// Result is a resolved location. Latitude/Longitude are nil when unknown.
type Result struct {
	Country   string
	Region    string
	City      string
	Latitude  *float64
	Longitude *float64
}

// cityReader is the slice of *geoip2.Reader the resolver uses; tests inject
// fakes through newWithReader.
type cityReader interface {
	City(ip net.IP) (*geoip2.City, error)
}

type cacheEntry struct {
	result    Result
	expiresAt time.Time
}

// Resolver looks up IPs against a City database with a small TTL cache.
// Safe for concurrent use. The zero value (or New("")) is a disabled resolver.
type Resolver struct {
	reader cityReader
	closer func() error

	mu    sync.Mutex
	cache map[string]cacheEntry
}

// DefaultPaths are searched, in order, when GEOIP_DB_PATH is not set:
//
//  1. /app/data is the operator's bind mount in compose.yaml. A fresher
//     database dropped there wins over the shipped one with no configuration.
//  2. /app/geoip is where the Dockerfile puts the database shipped with the
//     image. It deliberately sits outside /app/data — a bind mount over that
//     directory would hide anything the image put there.
//  3. The repository copy, so `go run ./cmd/api` resolves geo as well.
var DefaultPaths = []string{
	"/app/data/GeoLite2-City.mmdb",
	"/app/geoip/GeoLite2-City.mmdb",
	"data/GeoLite2-City.mmdb",
}

// ResolvePath picks the database to open. A configured path wins even if it
// does not exist, so a typo surfaces as an error instead of silently falling
// back to the shipped database and reporting stale locations. Otherwise the
// first readable default is used, and an empty string means "no database" —
// which New turns into a disabled resolver.
func ResolvePath(configured string) string {
	if configured != "" {
		return configured
	}
	for _, candidate := range DefaultPaths {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

// New opens the database at path. An empty path yields a disabled resolver and
// no error; an unreadable database is an error so the caller can decide to log
// and continue without geo.
func New(path string) (*Resolver, error) {
	if path == "" {
		return &Resolver{}, nil
	}
	reader, err := geoip2.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open GeoIP database %s: %w", path, err)
	}
	return &Resolver{reader: reader, closer: reader.Close, cache: map[string]cacheEntry{}}, nil
}

func newWithReader(reader cityReader) *Resolver {
	return &Resolver{reader: reader, cache: map[string]cacheEntry{}}
}

// Enabled reports whether a database is loaded.
func (r *Resolver) Enabled() bool { return r.reader != nil }

// Close releases the database.
func (r *Resolver) Close() error {
	if r.closer != nil {
		return r.closer()
	}
	return nil
}

// Lookup resolves one IP. Private, loopback, link-local and unparseable
// addresses short-circuit to an empty result without touching the database.
func (r *Resolver) Lookup(ip string) (Result, error) {
	if r.reader == nil {
		return Result{}, nil
	}
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.IsPrivate() || parsed.IsLoopback() ||
		parsed.IsLinkLocalUnicast() || parsed.IsLinkLocalMulticast() || parsed.IsUnspecified() {
		return Result{}, nil
	}

	if cached, ok := r.cached(ip); ok {
		return cached, nil
	}

	city, err := r.reader.City(parsed)
	if err != nil {
		return Result{}, err
	}

	result := Result{
		Country: city.Country.IsoCode,
		City:    city.City.Names["en"],
	}
	if len(city.Subdivisions) > 0 {
		result.Region = city.Subdivisions[0].Names["en"]
	}
	if city.Location.Latitude != 0 || city.Location.Longitude != 0 {
		lat, lon := city.Location.Latitude, city.Location.Longitude
		result.Latitude = &lat
		result.Longitude = &lon
	}

	r.store(ip, result)
	return result, nil
}

func (r *Resolver) cached(ip string) (Result, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.cache[ip]
	if !ok || time.Now().After(entry.expiresAt) {
		return Result{}, false
	}
	return entry.result, true
}

func (r *Resolver) store(ip string, result Result) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.cache) >= cacheMaxEntries {
		now := time.Now()
		for key, entry := range r.cache {
			if now.After(entry.expiresAt) {
				delete(r.cache, key)
			}
		}
		// Still full after dropping expired entries: drop arbitrary ones. A
		// beat that misses the cache only costs one mmdb read.
		for key := range r.cache {
			if len(r.cache) < cacheMaxEntries {
				break
			}
			delete(r.cache, key)
		}
	}
	r.cache[ip] = cacheEntry{result: result, expiresAt: time.Now().Add(cacheTTL)}
}

// RoundCoord rounds a coordinate to the given number of decimals (0-2), the
// per-workspace privacy fuzzing knob (2 decimals ≈ 1km).
//
// Rounds, it does not truncate — it uses math.Round, so 48.8566 at 1 decimal is
// 48.9, not 48.8. Callers should pass
// WebAnalyticsSettings.EffectiveGeoCoordsPrecision rather than the raw setting.
func RoundCoord(v float64, precision int) float64 {
	if precision < 0 {
		precision = 0
	}
	if precision > 2 {
		precision = 2
	}
	factor := math.Pow10(precision)
	return math.Round(v*factor) / factor
}
