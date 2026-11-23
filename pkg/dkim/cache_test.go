package dkim

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func countingCache(t *testing.T, max int) (*KeyCache, *int) {
	t.Helper()
	key, _, _ := testKey(t)
	opens := 0
	c := NewKeyCache(max)
	c.open = func(encrypted, _ string) (*rsa.PrivateKey, error) {
		opens++
		if encrypted == "broken" {
			return nil, errors.New("cannot decrypt")
		}
		return key, nil
	}
	return c, &opens
}

func TestKeyCache_OpensOncePerBlob(t *testing.T) {
	c, opens := countingCache(t, 0)

	for i := 0; i < 3; i++ {
		_, err := c.Get("ws1", "blob-a", "secret")
		require.NoError(t, err)
	}
	assert.Equal(t, 1, *opens)

	_, err := c.Get("ws1", "blob-b", "secret")
	require.NoError(t, err)
	assert.Equal(t, 2, *opens, "a new encrypted blob (regeneration, rotation) is opened again")

	_, err = c.Get("ws2", "blob-a", "secret")
	require.NoError(t, err)
	assert.Equal(t, 3, *opens, "entries are per workspace")
}

func TestKeyCache_FailuresAreNotCached(t *testing.T) {
	c, opens := countingCache(t, 0)

	_, err := c.Get("ws1", "broken", "secret")
	assert.True(t, errors.Is(err, ErrSign))
	_, err = c.Get("ws1", "broken", "secret")
	assert.True(t, errors.Is(err, ErrSign))
	assert.Equal(t, 2, *opens)
	assert.Zero(t, c.Len())
}

func TestKeyCache_Bounded(t *testing.T) {
	c, opens := countingCache(t, 0)

	for i := 0; i < KeyCacheSize+10; i++ {
		_, err := c.Get("ws", fmt.Sprintf("blob-%d", i), "secret")
		require.NoError(t, err)
	}
	assert.Equal(t, KeyCacheSize, c.Len())

	before := *opens
	_, err := c.Get("ws", fmt.Sprintf("blob-%d", KeyCacheSize+9), "secret")
	require.NoError(t, err)
	assert.Equal(t, before, *opens, "the most recent entry is kept")

	_, err = c.Get("ws", "blob-0", "secret")
	require.NoError(t, err)
	assert.Equal(t, before+1, *opens, "the least recently used entry was evicted")
}

func TestKeyCache_OpensRealBlobs(t *testing.T) {
	key, privPEM, _ := testKey(t)
	sealed, err := EncryptPrivateKey(privPEM, "secret")
	require.NoError(t, err)

	c := NewKeyCache(0)
	got, err := c.Get("ws", sealed, "secret")
	require.NoError(t, err)
	assert.True(t, got.Equal(key))

	_, err = c.Get("ws", sealed, "wrong")
	assert.NoError(t, err, "a cached key is not reopened")

	_, err = NewKeyCache(0).Get("ws", sealed, "wrong")
	assert.True(t, errors.Is(err, ErrSign))
}
