package notifuse_mjml

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The same tree converts to the same MJML every time: a template read twice through the
// API or MCP, or exported twice, must not show a change that is only a different
// attribute order.
func TestConvertJSONToMJML_AttributeOrderIsStable(t *testing.T) {
	button := NewBaseBlock("button-1", MJMLComponentMjButton)
	button.Attributes = map[string]interface{}{
		"width":            "200px",
		"background-color": "#4f46e5",
		"href":             "https://example.com",
		"color":            "#ffffff",
		"align":            "center",
		"border-radius":    "6px",
		"font-size":        "16px",
		"padding":          "12px 24px",
	}
	button.Content = stringPtr("Open")

	first := convertBlockToMJML(&MJButtonBlock{BaseBlock: button}, 0, "")
	for i := 0; i < 50; i++ {
		require.Equal(t, first, convertBlockToMJML(&MJButtonBlock{BaseBlock: button}, 0, ""), "conversion %d differs", i)
	}

	// Alphabetical order, so the output does not depend on how the map was built.
	order := []string{"align=", "background-color=", "border-radius=", "color=", "font-size=", "href=", "padding=", "width="}
	last := -1
	for _, attr := range order {
		at := strings.Index(first, " "+attr)
		require.GreaterOrEqual(t, at, 0, "missing %s in %s", attr, first)
		assert.Greater(t, at, last, "%s is out of order in %s", attr, first)
		last = at
	}
}
