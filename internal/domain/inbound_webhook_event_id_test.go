package domain

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderEventID(t *testing.T) {
	id := func(key ...string) string {
		return ProviderEventID("integration-1", WebhookSourceSendGrid, EmailEventBounce, key...)
	}

	t.Run("the same event gets the same version 5 UUID each time", func(t *testing.T) {
		first := id("sg-event-1")
		assert.Equal(t, first, id("sg-event-1"))
		parsed, err := uuid.Parse(first)
		require.NoError(t, err)
		assert.Equal(t, uuid.Version(5), parsed.Version())
	})

	t.Run("each part of the name sets the ID", func(t *testing.T) {
		base := id("sg-event-1")
		assert.NotEqual(t, base, ProviderEventID("integration-2", WebhookSourceSendGrid, EmailEventBounce, "sg-event-1"), "integration")
		assert.NotEqual(t, base, ProviderEventID("integration-1", WebhookSourceMailgun, EmailEventBounce, "sg-event-1"), "source")
		assert.NotEqual(t, base, ProviderEventID("integration-1", WebhookSourceSendGrid, EmailEventDelivered, "sg-event-1"), "event type")
		assert.NotEqual(t, base, id("sg-event-2"), "key")
		assert.NotEqual(t, base, id("sg-event-1", "x"), "an extra key part")
	})

	t.Run("parts that would read the same once joined give different IDs", func(t *testing.T) {
		assert.NotEqual(t, id("ab", "c"), id("a", "bc"))
		assert.NotEqual(t, id("a\x00b"), id("a", "b"))
	})

	t.Run("without the provider's identifier every call gets a new random UUID", func(t *testing.T) {
		for name, key := range map[string][]string{
			"no key":         nil,
			"an empty key":   {""},
			"one empty part": {"bounce", ""},
			"an empty first": {"", "1672574400"},
		} {
			t.Run(name, func(t *testing.T) {
				first, second := id(key...), id(key...)
				assert.NotEqual(t, first, second)
				parsed, err := uuid.Parse(first)
				require.NoError(t, err)
				assert.Equal(t, uuid.Version(4), parsed.Version())
			})
		}
	})
}
