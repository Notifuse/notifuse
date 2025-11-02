package templates

import (
	"encoding/json"
	"testing"

	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSupabaseTemplateCreation(t *testing.T) {
	t.Run("All Supabase templates can be created", func(t *testing.T) {
		templates := AllSupabaseTemplates()
		
		for name, createFunc := range templates {
			t.Run(name, func(t *testing.T) {
				block, err := createFunc()
				require.NoError(t, err, "Should be able to create %s template", name)
				require.NotNil(t, block, "Template block should not be nil")
				
				// Verify it's a valid MJML structure
				assert.Equal(t, notifuse_mjml.MJMLComponentMjml, block.GetType(), 
					"Root block should be mjml type")
				assert.NotNil(t, block.GetChildren(), "Root block should have children")
			})
		}
	})
	
	t.Run("Signup template has correct structure", func(t *testing.T) {
		block, err := CreateSupabaseSignupEmailStructure()
		require.NoError(t, err)
		
		// Verify root
		assert.Equal(t, notifuse_mjml.MJMLComponentMjml, block.GetType())
		
		// Verify has head and body
		children := block.GetChildren()
		require.Len(t, children, 2, "Should have head and body")
		
		// Check head
		head := children[0]
		assert.Equal(t, notifuse_mjml.MJMLComponentMjHead, head.GetType())
		
		// Check body
		body := children[1]
		assert.Equal(t, notifuse_mjml.MJMLComponentMjBody, body.GetType())
		assert.NotEmpty(t, body.GetChildren(), "Body should have children")
	})
	
	t.Run("Magic link template has correct structure", func(t *testing.T) {
		block, err := CreateSupabaseMagicLinkEmailStructure()
		require.NoError(t, err)
		
		// Verify root
		assert.Equal(t, notifuse_mjml.MJMLComponentMjml, block.GetType())
		
		// Verify has head and body
		children := block.GetChildren()
		require.Len(t, children, 2, "Should have head and body")
	})
}

func TestSupabaseTemplateRoundTrip(t *testing.T) {
	t.Run("All templates can be marshaled and unmarshaled", func(t *testing.T) {
		templates := AllSupabaseTemplates()
		
		for name, createFunc := range templates {
			t.Run(name, func(t *testing.T) {
				// Create template
				original, err := createFunc()
				require.NoError(t, err, "Should be able to create %s template", name)
				
				// Marshal to JSON
				originalJSON, err := json.Marshal(original)
				require.NoError(t, err, "Should be able to marshal template")
				
				// Unmarshal back
				retrieved, err := notifuse_mjml.UnmarshalEmailBlock(originalJSON)
				require.NoError(t, err, "Should be able to unmarshal template")
				
				// Marshal retrieved
				retrievedJSON, err := json.Marshal(retrieved)
				require.NoError(t, err, "Should be able to marshal retrieved template")
				
				// Compare JSON
				assert.JSONEq(t, string(originalJSON), string(retrievedJSON), 
					"Round-trip should preserve structure for %s", name)
			})
		}
	})
}

func TestSupabaseTemplateComponents(t *testing.T) {
	t.Run("Signup template uses all required components", func(t *testing.T) {
		block, err := CreateSupabaseSignupEmailStructure()
		require.NoError(t, err)
		
		// Find all component types used
		componentTypes := findAllComponentTypes(block)
		
		// Should contain these components at minimum
		expectedComponents := []notifuse_mjml.MJMLComponentType{
			notifuse_mjml.MJMLComponentMjml,
			notifuse_mjml.MJMLComponentMjHead,
			notifuse_mjml.MJMLComponentMjBody,
			notifuse_mjml.MJMLComponentMjWrapper,
			notifuse_mjml.MJMLComponentMjSection,
			notifuse_mjml.MJMLComponentMjColumn,
			notifuse_mjml.MJMLComponentMjImage,
			notifuse_mjml.MJMLComponentMjText,
			notifuse_mjml.MJMLComponentMjButton,
		}
		
		for _, expected := range expectedComponents {
			assert.Contains(t, componentTypes, expected, 
				"Signup template should contain %s component", expected)
		}
	})
}

// Helper function to recursively find all component types in a tree
func findAllComponentTypes(block notifuse_mjml.EmailBlock) []notifuse_mjml.MJMLComponentType {
	if block == nil {
		return nil
	}
	
	types := []notifuse_mjml.MJMLComponentType{block.GetType()}
	
	for _, child := range block.GetChildren() {
		childTypes := findAllComponentTypes(child)
		types = append(types, childTypes...)
	}
	
	return types
}

