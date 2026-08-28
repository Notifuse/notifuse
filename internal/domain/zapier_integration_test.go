package domain

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZapierSettings_Validate(t *testing.T) {
	t.Run("empty api key email returns error", func(t *testing.T) {
		settings := &ZapierSettings{}
		err := settings.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "api key email is required")
	})

	t.Run("api key email without an @ returns error", func(t *testing.T) {
		settings := &ZapierSettings{APIKeyEmail: "zapier-support-3f9a1c02"}
		err := settings.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be an email address")
	})

	t.Run("minted address passes validation", func(t *testing.T) {
		settings := &ZapierSettings{APIKeyEmail: "zapier-support-3f9a1c02@api.notifuse.com"}
		assert.NoError(t, settings.Validate())
	})
}

func TestZapierKeyPermissions(t *testing.T) {
	// Written out verb by verb rather than built from the same helpers ZapierKeyPermissions uses.
	// An expectation derived from those helpers agrees with whatever they produce — including a
	// set with `segments` dropped, which is how that grant went missing once already.
	//
	// The three verbs granted on resources Zapier never touches are the unenforced ones: a
	// stored false there is permanent, since backfills only ever add the keys a row is missing.
	expected := UserPermissions{
		PermissionResourceContacts:             {Read: true, Write: true},
		PermissionResourceSegments:             {Read: true, Write: false},
		PermissionResourceLists:                {Read: true, Write: true},
		PermissionResourceTemplates:            {Read: false, Write: false},
		PermissionResourceBlog:                 {Read: false, Write: false},
		PermissionResourceBroadcasts:           {Read: false, Write: false},
		PermissionResourceTransactional:        {Read: false, Write: false},
		PermissionResourceAutomations:          {Read: false, Write: false},
		PermissionResourceMessageHistory:       {Read: false, Write: true},
		PermissionResourceWebAnalytics:         {Read: false, Write: false},
		PermissionResourceWebhookSubscriptions: {Read: true, Write: true},
		PermissionResourceWebhookEvents:        {Read: false, Write: true},
		PermissionResourceLLM:                  {Read: true, Write: false},
		PermissionResourceWorkspace:            {Read: false, Write: false},
	}

	t.Run("grants exactly the verbs a Zapier connection needs", func(t *testing.T) {
		assert.Equal(t, expected, ZapierKeyPermissions())
	})

	t.Run("denies every enforced verb outside the resources it grants", func(t *testing.T) {
		grantedResources := []PermissionResource{
			PermissionResourceWebhookSubscriptions,
			PermissionResourceContacts,
			PermissionResourceLists,
			PermissionResourceSegments,
		}
		permissions := ZapierKeyPermissions()

		var granted []string
		for _, resource := range AllPermissionResources {
			if slices.Contains(grantedResources, resource) {
				continue
			}
			verbs := permissions[resource]
			if verbs.Read && IsPermissionEnforced(resource, PermissionTypeRead) {
				granted = append(granted, string(resource)+":read")
			}
			if verbs.Write && IsPermissionEnforced(resource, PermissionTypeWrite) {
				granted = append(granted, string(resource)+":write")
			}
		}
		assert.Empty(t, granted)
	})

	t.Run("is narrower than full workspace access", func(t *testing.T) {
		assert.NotEqual(t, NewFullPermissions(), ZapierKeyPermissions())
	})

	t.Run("two calls return maps that do not alias", func(t *testing.T) {
		first := ZapierKeyPermissions()
		second := ZapierKeyPermissions()

		first[PermissionResourceWorkspace] = ResourcePermissions{Read: true, Write: true}

		assert.Equal(t, ResourcePermissions{}, second[PermissionResourceWorkspace])
	})
}

func TestZapierKeyPrefix(t *testing.T) {
	const hex = "3f9a1c02"

	t.Run("stays inside the address charset the server accepts", func(t *testing.T) {
		testCases := []struct {
			name     string
			label    string
			expected string
		}{
			{
				name:     "default label carries no slug",
				label:    "Zapier",
				expected: "zapier-" + hex,
			},
			{
				name:     "spaces and accents collapse to single dashes",
				label:    "Équipe  Marketing FR",
				expected: "zapier-quipe-marketing-fr-" + hex,
			},
			{
				// The expectation is the truncated slug, not the label: this one exhausts the
				// budget apiKeyEmailPrefixRegex leaves once "zapier-" and the hex are spent.
				name:     "an overlong label is truncated to the budget",
				label:    strings.Repeat("marketing ", 20),
				expected: "zapier-marketing-marketing-marketing-marketing-marketin-" + hex,
			},
			{
				name:     "a label that slugifies to nothing carries no slug",
				label:    "!!! ??? ***",
				expected: "zapier-" + hex,
			},
			{
				name:     "an underscored label keeps its underscores",
				label:    "support_desk",
				expected: "zapier-support_desk-" + hex,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				prefix, err := ZapierKeyPrefix(tc.label, hex)
				require.NoError(t, err)
				assert.Equal(t, tc.expected, prefix)
				assert.Regexp(t, apiKeyEmailPrefixRegex, prefix)
			})
		}
	})

	t.Run("the same label with different randomness yields different prefixes", func(t *testing.T) {
		first, err := ZapierKeyPrefix("Support", hex)
		require.NoError(t, err)
		second, err := ZapierKeyPrefix("Support", "0a1b2c3d")
		require.NoError(t, err)

		assert.NotEqual(t, first, second)
	})

	t.Run("rejects randomness the length budget was not computed against", func(t *testing.T) {
		_, err := ZapierKeyPrefix("Support", "")
		assert.Error(t, err)

		_, err = ZapierKeyPrefix("Support", "3F9A1C02")
		assert.Error(t, err)

		_, err = ZapierKeyPrefix("Support", "3f9a1c02aa")
		assert.Error(t, err)
	})
}
