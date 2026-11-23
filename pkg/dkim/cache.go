package dkim

import (
	"container/list"
	"crypto/rsa"
	"crypto/sha256"
	"fmt"
	"sync"
)

// KeyCacheSize is how many parsed keys a KeyCache holds by default.
const KeyCacheSize = 512

// KeyCache keeps parsed private keys, so that a key is decrypted and parsed once and not
// on every send. An entry is found by the workspace and the hash of the encrypted key:
// a regenerated key, or one re-encrypted by a SECRET_KEY rotation, is a new entry, and
// the old one ages out. The least recently used entry goes first.
type KeyCache struct {
	mu    sync.Mutex
	max   int
	order *list.List
	items map[keyCacheKey]*list.Element
	open  func(encrypted, globalSecretKey string) (*rsa.PrivateKey, error)
}

type keyCacheKey struct {
	workspaceID string
	sum         [sha256.Size]byte
}

type keyCacheEntry struct {
	key   keyCacheKey
	value *rsa.PrivateKey
}

// NewKeyCache returns a cache holding at most max keys (KeyCacheSize when max <= 0).
func NewKeyCache(max int) *KeyCache {
	if max <= 0 {
		max = KeyCacheSize
	}
	return &KeyCache{
		max:   max,
		order: list.New(),
		items: map[keyCacheKey]*list.Element{},
		open:  OpenPrivateKey,
	}
}

// Get returns the parsed key for encrypted, opening it with globalSecretKey on a miss.
// A failure is wrapped in ErrSign and is not cached.
func (c *KeyCache) Get(workspaceID, encrypted, globalSecretKey string) (*rsa.PrivateKey, error) {
	k := keyCacheKey{workspaceID: workspaceID, sum: sha256.Sum256([]byte(encrypted))}

	c.mu.Lock()
	if el, ok := c.items[k]; ok {
		c.order.MoveToFront(el)
		value := el.Value.(*keyCacheEntry).value
		c.mu.Unlock()
		return value, nil
	}
	c.mu.Unlock()

	key, err := c.open(encrypted, globalSecretKey)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSign, err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[k]; ok {
		c.order.MoveToFront(el)
		return el.Value.(*keyCacheEntry).value, nil
	}
	c.items[k] = c.order.PushFront(&keyCacheEntry{key: k, value: key})
	for c.order.Len() > c.max {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.items, oldest.Value.(*keyCacheEntry).key)
	}
	return key, nil
}

// Len is the number of keys held.
func (c *KeyCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}
