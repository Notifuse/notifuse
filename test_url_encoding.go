package main

import (
	"fmt"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"strings"
)

func stringPtr(s string) *string {
	return &s
}

func main() {
	// Test case 1: Button with URL containing query parameters
	buttonBlock := &notifuse_mjml.MJButtonBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "button-1",
			Type: notifuse_mjml.MJMLComponentMjButton,
			Attributes: map[string]interface{}{
				"href": "https://example.com/page?foo=bar&baz=qux",
			},
		},
		Content: stringPtr("Click Me"),
	}

	columnBlock := &notifuse_mjml.MJColumnBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "column-1",
			Type:     notifuse_mjml.MJMLComponentMjColumn,
			Children: []interface{}{buttonBlock},
		},
	}

	sectionBlock := &notifuse_mjml.MJSectionBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "section-1",
			Type:     notifuse_mjml.MJMLComponentMjSection,
			Children: []interface{}{columnBlock},
		},
	}

	bodyBlock := &notifuse_mjml.MJBodyBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "body-1",
			Type:     notifuse_mjml.MJMLComponentMjBody,
			Children: []interface{}{sectionBlock},
		},
	}

	mjmlBlock := &notifuse_mjml.MJMLBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "mjml-1",
			Type:     notifuse_mjml.MJMLComponentMjml,
			Children: []interface{}{bodyBlock},
		},
	}

	// Convert to MJML
	mjmlString := notifuse_mjml.ConvertJSONToMJML(mjmlBlock)
	
	fmt.Println("Generated MJML:")
	fmt.Println(mjmlString)
	fmt.Println("\n" + strings.Repeat("=", 80))
	
	// Test case 2: Button with already escaped ampersands
	buttonBlock2 := &notifuse_mjml.MJButtonBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "button-2",
			Type: notifuse_mjml.MJMLComponentMjButton,
			Attributes: map[string]interface{}{
				"href": "https://example.com/page?foo=bar&amp;baz=qux",
			},
		},
		Content: stringPtr("Click Me 2"),
	}

	columnBlock2 := &notifuse_mjml.MJColumnBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "column-2",
			Type:     notifuse_mjml.MJMLComponentMjColumn,
			Children: []interface{}{buttonBlock2},
		},
	}

	sectionBlock2 := &notifuse_mjml.MJSectionBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "section-2",
			Type:     notifuse_mjml.MJMLComponentMjSection,
			Children: []interface{}{columnBlock2},
		},
	}

	bodyBlock2 := &notifuse_mjml.MJBodyBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "body-2",
			Type:     notifuse_mjml.MJMLComponentMjBody,
			Children: []interface{}{sectionBlock2},
		},
	}

	mjmlBlock2 := &notifuse_mjml.MJMLBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "mjml-2",
			Type:     notifuse_mjml.MJMLComponentMjml,
			Children: []interface{}{bodyBlock2},
		},
	}

	mjmlString2 := notifuse_mjml.ConvertJSONToMJML(mjmlBlock2)
	
	fmt.Println("\nGenerated MJML (with pre-escaped ampersands):")
	fmt.Println(mjmlString2)
}
