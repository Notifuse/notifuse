package notifuse_mjml

import (
	"encoding/json"
)

// FilterBlocksByChannel returns a new tree with only blocks visible for the given channel
// channel should be "email" or "web"
func FilterBlocksByChannel(tree EmailBlock, channel string) EmailBlock {
	// Deep copy the tree
	filtered := deepCopyBlock(tree)

	// Recursively filter mj-section blocks based on visibility attribute
	filterChildren(filtered, channel)

	return filtered
}

// filterChildren recursively filters child blocks based on channel visibility
func filterChildren(block EmailBlock, channel string) {
	children := block.GetChildren()
	if len(children) == 0 {
		return
	}

	filtered := []EmailBlock{}
	for _, child := range children {
		// Check if this is an mj-section with visibility
		if child.GetType() == MJMLComponentMjSection {
			attrs := child.GetAttributes()
			visibility, _ := attrs["visibility"].(string)

			shouldInclude := false
			// Filter logic:
			// - nil or "all": include in both channels
			// - "email_only": include only if channel == "email"
			// - "web_only": include only if channel == "web"
			if visibility == "" || visibility == "all" {
				shouldInclude = true
			} else if visibility == "email_only" && channel == "email" {
				shouldInclude = true
			} else if visibility == "web_only" && channel == "web" {
				shouldInclude = true
			}

			if shouldInclude {
				filtered = append(filtered, child)
				filterChildren(child, channel) // Recurse
			}
			// Skip this section if visibility doesn't match channel
		} else {
			// Not a section, include it and recurse
			filtered = append(filtered, child)
			filterChildren(child, channel)
		}
	}

	block.SetChildren(filtered)
}

// deepCopyBlock creates a deep copy of an EmailBlock via JSON serialization
func deepCopyBlock(block EmailBlock) EmailBlock {
	// Implement deep copy via JSON marshal/unmarshal
	data, err := json.Marshal(block)
	if err != nil {
		// If marshaling fails, return the original block
		return block
	}

	copied, err := UnmarshalEmailBlock(data)
	if err != nil {
		// If unmarshaling fails, return the original block
		return block
	}

	return copied
}
