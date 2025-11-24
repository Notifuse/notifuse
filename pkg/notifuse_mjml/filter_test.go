package notifuse_mjml

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterBlocksByChannel(t *testing.T) {
	t.Run("mj-section visibility email_only, channel email", func(t *testing.T) {
		section := &MJSectionBlock{BaseBlock: NewBaseBlock("section-1", MJMLComponentMjSection)}
		section.Attributes = map[string]interface{}{
			"visibility": "email_only",
		}

		body := &MJBodyBlock{BaseBlock: NewBaseBlock("body-1", MJMLComponentMjBody)}
		body.Children = []EmailBlock{section}

		filtered := FilterBlocksByChannel(body, "email")

		require.NotNil(t, filtered)
		children := filtered.GetChildren()
		assert.Len(t, children, 1, "Section should be included for email channel")
	})

	t.Run("mj-section visibility email_only, channel web", func(t *testing.T) {
		section := &MJSectionBlock{BaseBlock: NewBaseBlock("section-1", MJMLComponentMjSection)}
		section.Attributes = map[string]interface{}{
			"visibility": "email_only",
		}

		body := &MJBodyBlock{BaseBlock: NewBaseBlock("body-1", MJMLComponentMjBody)}
		body.Children = []EmailBlock{section}

		filtered := FilterBlocksByChannel(body, "web")

		require.NotNil(t, filtered)
		children := filtered.GetChildren()
		assert.Len(t, children, 0, "Section should be excluded for web channel")
	})

	t.Run("mj-section visibility web_only, channel web", func(t *testing.T) {
		section := &MJSectionBlock{BaseBlock: NewBaseBlock("section-1", MJMLComponentMjSection)}
		section.Attributes = map[string]interface{}{
			"visibility": "web_only",
		}

		body := &MJBodyBlock{BaseBlock: NewBaseBlock("body-1", MJMLComponentMjBody)}
		body.Children = []EmailBlock{section}

		filtered := FilterBlocksByChannel(body, "web")

		require.NotNil(t, filtered)
		children := filtered.GetChildren()
		assert.Len(t, children, 1, "Section should be included for web channel")
	})

	t.Run("mj-section visibility web_only, channel email", func(t *testing.T) {
		section := &MJSectionBlock{BaseBlock: NewBaseBlock("section-1", MJMLComponentMjSection)}
		section.Attributes = map[string]interface{}{
			"visibility": "web_only",
		}

		body := &MJBodyBlock{BaseBlock: NewBaseBlock("body-1", MJMLComponentMjBody)}
		body.Children = []EmailBlock{section}

		filtered := FilterBlocksByChannel(body, "email")

		require.NotNil(t, filtered)
		children := filtered.GetChildren()
		assert.Len(t, children, 0, "Section should be excluded for email channel")
	})

	t.Run("mj-section visibility all, channel email", func(t *testing.T) {
		section := &MJSectionBlock{BaseBlock: NewBaseBlock("section-1", MJMLComponentMjSection)}
		section.Attributes = map[string]interface{}{
			"visibility": "all",
		}

		body := &MJBodyBlock{BaseBlock: NewBaseBlock("body-1", MJMLComponentMjBody)}
		body.Children = []EmailBlock{section}

		filtered := FilterBlocksByChannel(body, "email")

		require.NotNil(t, filtered)
		children := filtered.GetChildren()
		assert.Len(t, children, 1, "Section should be included for email channel")
	})

	t.Run("mj-section visibility all, channel web", func(t *testing.T) {
		section := &MJSectionBlock{BaseBlock: NewBaseBlock("section-1", MJMLComponentMjSection)}
		section.Attributes = map[string]interface{}{
			"visibility": "all",
		}

		body := &MJBodyBlock{BaseBlock: NewBaseBlock("body-1", MJMLComponentMjBody)}
		body.Children = []EmailBlock{section}

		filtered := FilterBlocksByChannel(body, "web")

		require.NotNil(t, filtered)
		children := filtered.GetChildren()
		assert.Len(t, children, 1, "Section should be included for web channel")
	})

	t.Run("mj-section visibility empty, channel email", func(t *testing.T) {
		section := &MJSectionBlock{BaseBlock: NewBaseBlock("section-1", MJMLComponentMjSection)}
		section.Attributes = map[string]interface{}{}

		body := &MJBodyBlock{BaseBlock: NewBaseBlock("body-1", MJMLComponentMjBody)}
		body.Children = []EmailBlock{section}

		filtered := FilterBlocksByChannel(body, "email")

		require.NotNil(t, filtered)
		children := filtered.GetChildren()
		assert.Len(t, children, 1, "Section should be included when visibility is empty")
	})

	t.Run("mj-section visibility empty, channel web", func(t *testing.T) {
		section := &MJSectionBlock{BaseBlock: NewBaseBlock("section-1", MJMLComponentMjSection)}
		section.Attributes = map[string]interface{}{}

		body := &MJBodyBlock{BaseBlock: NewBaseBlock("body-1", MJMLComponentMjBody)}
		body.Children = []EmailBlock{section}

		filtered := FilterBlocksByChannel(body, "web")

		require.NotNil(t, filtered)
		children := filtered.GetChildren()
		assert.Len(t, children, 1, "Section should be included when visibility is empty")
	})

	t.Run("nested sections filtered recursively", func(t *testing.T) {
		innerSection := &MJSectionBlock{BaseBlock: NewBaseBlock("section-2", MJMLComponentMjSection)}
		innerSection.Attributes = map[string]interface{}{
			"visibility": "email_only",
		}

		outerSection := &MJSectionBlock{BaseBlock: NewBaseBlock("section-1", MJMLComponentMjSection)}
		outerSection.Attributes = map[string]interface{}{
			"visibility": "all",
		}
		outerSection.Children = []EmailBlock{innerSection}

		body := &MJBodyBlock{BaseBlock: NewBaseBlock("body-1", MJMLComponentMjBody)}
		body.Children = []EmailBlock{outerSection}

		// Filter for email channel - both should be included
		filteredEmail := FilterBlocksByChannel(body, "email")
		childrenEmail := filteredEmail.GetChildren()
		assert.Len(t, childrenEmail, 1, "Outer section should be included")
		innerChildrenEmail := childrenEmail[0].GetChildren()
		assert.Len(t, innerChildrenEmail, 1, "Inner section should be included for email")

		// Filter for web channel - outer included, inner excluded
		filteredWeb := FilterBlocksByChannel(body, "web")
		childrenWeb := filteredWeb.GetChildren()
		assert.Len(t, childrenWeb, 1, "Outer section should be included")
		innerChildrenWeb := childrenWeb[0].GetChildren()
		assert.Len(t, innerChildrenWeb, 0, "Inner section should be excluded for web")
	})

	t.Run("mj-text non-section always included", func(t *testing.T) {
		text := &MJTextBlock{BaseBlock: NewBaseBlock("text-1", MJMLComponentMjText)}
		text.Attributes = map[string]interface{}{
			"visibility": "email_only", // Should be ignored for non-sections
		}

		body := &MJBodyBlock{BaseBlock: NewBaseBlock("body-1", MJMLComponentMjBody)}
		body.Children = []EmailBlock{text}

		filteredEmail := FilterBlocksByChannel(body, "email")
		childrenEmail := filteredEmail.GetChildren()
		assert.Len(t, childrenEmail, 1, "Text should be included for email")

		filteredWeb := FilterBlocksByChannel(body, "web")
		childrenWeb := filteredWeb.GetChildren()
		assert.Len(t, childrenWeb, 1, "Text should be included for web")
	})

	t.Run("deep copy doesn't modify original", func(t *testing.T) {
		section := &MJSectionBlock{BaseBlock: NewBaseBlock("section-1", MJMLComponentMjSection)}
		section.Attributes = map[string]interface{}{
			"visibility": "email_only",
		}

		body := &MJBodyBlock{BaseBlock: NewBaseBlock("body-1", MJMLComponentMjBody)}
		body.Children = []EmailBlock{section}

		originalChildrenCount := len(body.GetChildren())

		filtered := FilterBlocksByChannel(body, "web")

		// Original should be unchanged
		assert.Len(t, body.GetChildren(), originalChildrenCount, "Original tree should not be modified")
		// Filtered should be different
		assert.Len(t, filtered.GetChildren(), 0, "Filtered tree should be different")
	})
}

