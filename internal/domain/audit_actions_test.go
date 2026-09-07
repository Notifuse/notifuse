package domain

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditCatalogue_EveryRouteIsInExactlyOneList(t *testing.T) {
	for route := range AuditedActions {
		_, read := AuditLoggedReads[route]
		_, excluded := AuditExcludedRoutes[route]
		assert.False(t, read, "%s is audited and a logged read", route)
		assert.False(t, excluded, "%s is audited and excluded", route)
	}
	for route := range AuditLoggedReads {
		_, excluded := AuditExcludedRoutes[route]
		assert.False(t, excluded, "%s is a logged read and excluded", route)
	}
}

func TestAuditCatalogue_CategoriesAreKnown(t *testing.T) {
	known := make(map[string]struct{}, len(AuditCategories))
	for _, category := range AuditCategories {
		known[category] = struct{}{}
	}
	require.Len(t, known, len(AuditCategories), "categories must be unique")

	for action, spec := range AuditedActions {
		_, ok := known[spec.Category]
		assert.True(t, ok, "%s has unknown category %q", action, spec.Category)
	}
	for route, read := range AuditLoggedReads {
		_, ok := known[read.Category]
		assert.True(t, ok, "%s has unknown category %q", route, read.Category)
		assert.NotEmpty(t, read.Action)
	}
	for action, spec := range AuditSystemActions {
		_, ok := known[spec.Category]
		assert.True(t, ok, "%s has unknown category %q", action, spec.Category)
	}
}

func TestAuditCatalogue_ExcludesTheDataPlane(t *testing.T) {
	// The exclusions that matter most: the routes an integration can hit
	// thousands of times a minute. If one of these ever became audited the log
	// would turn into a firehose.
	for _, route := range []string{"contacts.upsert", "customEvents.upsert", "transactional.send", "lists.subscribe"} {
		_, excluded := AuditExcludedRoutes[route]
		assert.True(t, excluded, "%s must stay excluded", route)
		_, audited := AuditedActions[route]
		assert.False(t, audited)
	}
	// And the ones that must be there.
	for _, route := range []string{"workspaces.inviteMember", "workspaces.setUserPermissions", "workspaces.createAPIKey", "licence.set", "settings.update", "user.verify", "contacts.delete", "auditLogs.export"} {
		_, audited := AuditedActions[route]
		assert.True(t, audited, "%s must be audited", route)
	}
}

func TestAuditActionCatalogue_SortedUniqueAndComplete(t *testing.T) {
	catalogue := AuditActionCatalogue()
	require.Len(t, catalogue, len(AuditedActions)+len(AuditLoggedReads)+len(AuditSystemActions))

	names := make([]string, len(catalogue))
	seen := make(map[string]struct{}, len(catalogue))
	for i, entry := range catalogue {
		names[i] = entry.Action
		_, dup := seen[entry.Action]
		assert.False(t, dup, "duplicate action %s", entry.Action)
		seen[entry.Action] = struct{}{}
		assert.NotEmpty(t, entry.Category, entry.Action)
	}
	assert.True(t, sort.StringsAreSorted(names))

	for _, action := range []string{AuditActionPurged, AuditActionRecordingStopped, AuditActionRecordingResumed, "contacts.export"} {
		_, ok := seen[action]
		assert.True(t, ok, "%s must be in the catalogue", action)
	}
}

func TestAuditCatalogue_TargetPathsAreWellFormed(t *testing.T) {
	valid := func(path string) bool {
		if path == "" {
			return true
		}
		for _, segment := range strings.Split(path, ".") {
			if segment == "" || strings.ContainsAny(segment, " \t/") {
				return false
			}
		}
		return true
	}
	for route, spec := range AuditedActions {
		assert.True(t, valid(spec.TargetKey), "%s TargetKey %q", route, spec.TargetKey)
		assert.True(t, valid(spec.ResponseKey), "%s ResponseKey %q", route, spec.ResponseKey)
	}
	// The creates that mint their own id read it off the response.
	for _, route := range []string{"broadcasts.create", "automations.create", "templateBlocks.create", "annotations.create", "blogPosts.create", "blogCategories.create", "webhookSubscriptions.create", "transactional.create"} {
		assert.NotEmpty(t, AuditedActions[route].ResponseKey, route)
	}
}
