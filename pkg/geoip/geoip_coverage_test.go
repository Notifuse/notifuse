package geoip

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/oschwald/geoip2-golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fillCache(r *Resolver, n int, expiresAt time.Time, prefix string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := 0; i < n; i++ {
		r.cache[fmt.Sprintf("%s-%d", prefix, i)] = cacheEntry{
			result: Result{Country: "FR"}, expiresAt: expiresAt,
		}
	}
}

func TestCoverage_StoreBasics(t *testing.T) {
	t.Run("store then cached hit returns exact result", func(t *testing.T) {
		r := newWithReader(&fakeReader{result: parisCity()})
		want := Result{Country: "DE", Region: "Bavaria", City: "Munich"}
		r.store("198.51.100.7", want)
		got, ok := r.cached("198.51.100.7")
		require.True(t, ok)
		assert.Equal(t, want, got)
	})

	t.Run("store sets expiry about one TTL ahead", func(t *testing.T) {
		r := newWithReader(&fakeReader{result: parisCity()})
		before := time.Now()
		r.store("198.51.100.8", Result{Country: "FR"})
		after := time.Now()
		r.mu.Lock()
		exp := r.cache["198.51.100.8"].expiresAt
		r.mu.Unlock()
		assert.False(t, exp.Before(before.Add(cacheTTL)), "expiry must not be earlier than store+TTL")
		assert.False(t, exp.After(after.Add(cacheTTL)), "expiry must not be later than store+TTL")
	})

	t.Run("store overwrites an existing key in place", func(t *testing.T) {
		r := newWithReader(&fakeReader{result: parisCity()})
		r.store("198.51.100.9", Result{Country: "FR"})
		r.store("198.51.100.9", Result{Country: "ES"})
		got, ok := r.cached("198.51.100.9")
		require.True(t, ok)
		assert.Equal(t, "ES", got.Country)
		r.mu.Lock()
		assert.Len(t, r.cache, 1)
		r.mu.Unlock()
	})

	t.Run("cached miss on empty cache and on expired entry", func(t *testing.T) {
		r := newWithReader(&fakeReader{result: parisCity()})
		_, ok := r.cached("203.0.113.99")
		assert.False(t, ok)
		r.mu.Lock()
		r.cache["203.0.113.99"] = cacheEntry{result: Result{Country: "FR"}, expiresAt: time.Now().Add(-time.Second)}
		r.mu.Unlock()
		got, ok := r.cached("203.0.113.99")
		assert.False(t, ok)
		assert.Equal(t, Result{}, got)
	})
}

func TestCoverage_StoreEviction(t *testing.T) {
	t.Run("full cache of live entries evicts arbitrary entries to stay at the cap", func(t *testing.T) {
		r := newWithReader(&fakeReader{result: parisCity()})
		fillCache(r, cacheMaxEntries, time.Now().Add(time.Hour), "k")
		r.store("203.0.113.1", Result{Country: "IT"})
		r.mu.Lock()
		size := len(r.cache)
		_, present := r.cache["203.0.113.1"]
		r.mu.Unlock()
		assert.Equal(t, cacheMaxEntries, size, "cache must never exceed the cap")
		assert.True(t, present, "the freshly stored entry must survive eviction")
		got, ok := r.cached("203.0.113.1")
		require.True(t, ok)
		assert.Equal(t, "IT", got.Country)
	})

	t.Run("full cache of expired entries is purged before storing", func(t *testing.T) {
		r := newWithReader(&fakeReader{result: parisCity()})
		fillCache(r, cacheMaxEntries, time.Now().Add(-time.Minute), "old")
		r.store("203.0.113.2", Result{Country: "PT"})
		r.mu.Lock()
		size := len(r.cache)
		r.mu.Unlock()
		assert.Equal(t, 1, size, "every expired entry must be purged, leaving only the new one")
	})

	t.Run("mixed cache keeps live entries and drops expired ones", func(t *testing.T) {
		r := newWithReader(&fakeReader{result: parisCity()})
		fillCache(r, cacheMaxEntries-1, time.Now().Add(-time.Minute), "old")
		r.mu.Lock()
		r.cache["live-key"] = cacheEntry{result: Result{Country: "NL"}, expiresAt: time.Now().Add(time.Hour)}
		r.mu.Unlock()
		r.store("203.0.113.3", Result{Country: "BE"})
		r.mu.Lock()
		_, livePresent := r.cache["live-key"]
		_, newPresent := r.cache["203.0.113.3"]
		size := len(r.cache)
		r.mu.Unlock()
		assert.True(t, livePresent)
		assert.True(t, newPresent)
		assert.Equal(t, 2, size)
	})

	t.Run("one below the cap does not evict", func(t *testing.T) {
		r := newWithReader(&fakeReader{result: parisCity()})
		fillCache(r, cacheMaxEntries-1, time.Now().Add(time.Hour), "k")
		r.store("203.0.113.4", Result{Country: "SE"})
		r.mu.Lock()
		size := len(r.cache)
		r.mu.Unlock()
		assert.Equal(t, cacheMaxEntries, size)
	})
}

func TestCoverage_LookupMissHitAndErrors(t *testing.T) {
	t.Run("miss then hit then expiry forces a fresh read", func(t *testing.T) {
		reader := &fakeReader{result: parisCity()}
		r := newWithReader(reader)

		first, err := r.Lookup("203.0.113.20")
		require.NoError(t, err)
		assert.Equal(t, "Paris", first.City)
		assert.Equal(t, 1, reader.calls)

		second, err := r.Lookup("203.0.113.20")
		require.NoError(t, err)
		assert.Equal(t, first, second)
		assert.Equal(t, 1, reader.calls, "hit must not touch the reader")

		// Expire the entry by hand; the next lookup must re-read.
		r.mu.Lock()
		entry := r.cache["203.0.113.20"]
		entry.expiresAt = time.Now().Add(-time.Second)
		r.cache["203.0.113.20"] = entry
		r.mu.Unlock()
		city := parisCity()
		city.City.Names["en"] = "Lyon"
		reader.result = city
		third, err := r.Lookup("203.0.113.20")
		require.NoError(t, err)
		assert.Equal(t, "Lyon", third.City, "expired entry must be refreshed from the reader")
		assert.Equal(t, 2, reader.calls)
	})

	t.Run("distinct IPs each miss once", func(t *testing.T) {
		reader := &fakeReader{result: parisCity()}
		r := newWithReader(reader)
		_, err := r.Lookup("203.0.113.21")
		require.NoError(t, err)
		_, err = r.Lookup("203.0.113.22")
		require.NoError(t, err)
		assert.Equal(t, 2, reader.calls)
	})

	t.Run("reader error propagates exactly and is not cached", func(t *testing.T) {
		wantErr := errors.New("mmdb corrupted")
		reader := &fakeReader{err: wantErr}
		r := newWithReader(reader)
		result, err := r.Lookup("203.0.113.23")
		assert.Equal(t, Result{}, result)
		require.ErrorIs(t, err, wantErr)
		assert.Equal(t, "mmdb corrupted", err.Error())

		_, err = r.Lookup("203.0.113.23")
		require.ErrorIs(t, err, wantErr)
		assert.Equal(t, 2, reader.calls, "errors must not be cached")
		r.mu.Lock()
		assert.Empty(t, r.cache)
		r.mu.Unlock()
	})

	t.Run("no subdivisions yields empty region, one-sided coordinates still set both pointers", func(t *testing.T) {
		city := &geoip2.City{}
		city.Country.IsoCode = "JP"
		city.City.Names = map[string]string{"en": "Tokyo"}
		city.Location.Longitude = 139.6917 // latitude deliberately left zero
		r := newWithReader(&fakeReader{result: city})
		result, err := r.Lookup("203.0.113.24")
		require.NoError(t, err)
		assert.Equal(t, "JP", result.Country)
		assert.Equal(t, "Tokyo", result.City)
		assert.Equal(t, "", result.Region)
		require.NotNil(t, result.Latitude, "either coordinate non-zero must set both pointers")
		require.NotNil(t, result.Longitude)
		assert.Equal(t, 0.0, *result.Latitude)
		assert.InDelta(t, 139.6917, *result.Longitude, 1e-9)
	})
}
