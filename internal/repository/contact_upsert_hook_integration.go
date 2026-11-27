//go:build integration

package repository

import (
	"context"
	"sync"
)

// The integration tests stop UpsertContact between its lookup, which found no contact, and
// its insert, to write the same contact from another connection in between.
var (
	contactUpsertHookMu sync.RWMutex
	contactUpsertHook   func(ctx context.Context, email string)
)

// SetContactUpsertAfterLookupHook installs the function UpsertContact calls after a lookup
// that found no contact, before it inserts one (nil removes it).
func SetContactUpsertAfterLookupHook(hook func(ctx context.Context, email string)) {
	contactUpsertHookMu.Lock()
	defer contactUpsertHookMu.Unlock()
	contactUpsertHook = hook
}

func contactUpsertAfterLookup(ctx context.Context, email string) {
	contactUpsertHookMu.RLock()
	hook := contactUpsertHook
	contactUpsertHookMu.RUnlock()
	if hook != nil {
		hook(ctx, email)
	}
}
