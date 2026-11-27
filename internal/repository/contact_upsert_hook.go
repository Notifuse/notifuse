//go:build !integration

package repository

import "context"

// SetContactUpsertAfterLookupHook does nothing outside integration builds: see
// contact_upsert_hook_integration.go.
func SetContactUpsertAfterLookupHook(func(ctx context.Context, email string)) {}

func contactUpsertAfterLookup(context.Context, string) {}
