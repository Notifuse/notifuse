package notifuse_mjml

import (
	"strings"
	"testing"
)

func TestProcessLiquidTemplate(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		templateData map[string]interface{}
		context      string
		expected     string
		expectError  bool
	}{
		{
			name:         "no liquid tags",
			content:      "Hello World",
			templateData: map[string]interface{}{"name": "John"},
			context:      "test",
			expected:     "Hello World",
			expectError:  false,
		},
		{
			name:         "simple variable interpolation",
			content:      "Hello {{name}}!",
			templateData: map[string]interface{}{"name": "John"},
			context:      "test",
			expected:     "Hello John!",
			expectError:  false,
		},
		{
			name:         "multiple variables",
			content:      "Hello {{name}}, welcome to {{company}}!",
			templateData: map[string]interface{}{"name": "John", "company": "ACME Corp"},
			context:      "test",
			expected:     "Hello John, welcome to ACME Corp!",
			expectError:  false,
		},
		{
			name:         "conditional content",
			content:      "{% if isPremium %}Premium Member{% else %}Standard Member{% endif %}",
			templateData: map[string]interface{}{"isPremium": true},
			context:      "test",
			expected:     "Premium Member",
			expectError:  false,
		},
		{
			name:         "conditional content false",
			content:      "{% if isPremium %}Premium Member{% else %}Standard Member{% endif %}",
			templateData: map[string]interface{}{"isPremium": false},
			context:      "test",
			expected:     "Standard Member",
			expectError:  false,
		},
		{
			name:         "liquid filters",
			content:      "Hello {{name | upcase}}!",
			templateData: map[string]interface{}{"name": "john"},
			context:      "test",
			expected:     "Hello JOHN!",
			expectError:  false,
		},
		{
			name:         "empty template data",
			content:      "Hello {{name | default: 'Guest'}}!",
			templateData: nil,
			context:      "test",
			expected:     "Hello Guest!",
			expectError:  false,
		},
		{
			name:         "undefined variable with default",
			content:      "Hello {{unknown | default: 'Guest'}}!",
			templateData: map[string]interface{}{"name": "John"},
			context:      "test",
			expected:     "Hello Guest!",
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ProcessLiquidTemplate(tt.content, tt.templateData, tt.context)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestProcessLiquidTemplateEmailSubjects(t *testing.T) {
	// Specific tests for email subject line scenarios
	tests := []struct {
		name         string
		subject      string
		templateData map[string]interface{}
		expected     string
	}{
		{
			name:         "personalized subject",
			subject:      "Welcome {{firstName}}!",
			templateData: map[string]interface{}{"firstName": "John", "lastName": "Doe"},
			expected:     "Welcome John!",
		},
		{
			name:         "company and user subject",
			subject:      "{{firstName}}, your {{company}} order is ready",
			templateData: map[string]interface{}{"firstName": "Jane", "company": "ACME Corp"},
			expected:     "Jane, your ACME Corp order is ready",
		},
		{
			name:         "conditional urgency",
			subject:      "{% if urgent %}URGENT: {% endif %}Your order update",
			templateData: map[string]interface{}{"urgent": true},
			expected:     "URGENT: Your order update",
		},
		{
			name:         "non-urgent conditional",
			subject:      "{% if urgent %}URGENT: {% endif %}Your order update",
			templateData: map[string]interface{}{"urgent": false},
			expected:     "Your order update",
		},
		{
			name:         "order count simple",
			subject:      "You have {{orderCount}} order(s)",
			templateData: map[string]interface{}{"orderCount": 1},
			expected:     "You have 1 order(s)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ProcessLiquidTemplate(tt.subject, tt.templateData, "email_subject")

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestLiquidInHrefAttributes(t *testing.T) {
	tests := []struct {
		name         string
		block        EmailBlock
		templateData string
		expectedHref string
		expectError  bool
	}{
		{
			name: "button with liquid href",
			block: &MJButtonBlock{
				BaseBlock: BaseBlock{
					ID:   "btn1",
					Type: MJMLComponentMjButton,
					Attributes: map[string]interface{}{
						"href":            "{{ contact.profile_url }}",
						"backgroundColor": "#007bff",
					},
				},
				Content: stringPtr("Click me!"),
			},
			templateData: `{"contact": {"profile_url": "https://example.com/profile/123"}}`,
			expectedHref: `href="https://example.com/profile/123"`,
			expectError:  false,
		},
		{
			name: "image with liquid src",
			block: &MJImageBlock{
				BaseBlock: BaseBlock{
					ID:   "img1",
					Type: MJMLComponentMjImage,
					Attributes: map[string]interface{}{
						"src": "{{ user.avatar_url }}",
						"alt": "User avatar",
					},
				},
			},
			templateData: `{"user": {"avatar_url": "https://example.com/avatars/user123.jpg"}}`,
			expectedHref: `src="https://example.com/avatars/user123.jpg"`,
			expectError:  false,
		},
		{
			name: "social element with liquid href",
			block: &MJSocialElementBlock{
				BaseBlock: BaseBlock{
					ID:   "social1",
					Type: MJMLComponentMjSocialElement,
					Attributes: map[string]interface{}{
						"href": "{{ company.linkedin_url }}",
						"name": "linkedin",
					},
				},
			},
			templateData: `{"company": {"linkedin_url": "https://linkedin.com/company/acme"}}`,
			expectedHref: `href="https://linkedin.com/company/acme"`,
			expectError:  false,
		},
		{
			name: "non-URL attributes should not be processed",
			block: &MJTextBlock{
				BaseBlock: BaseBlock{
					ID:   "text1",
					Type: MJMLComponentMjText,
					Attributes: map[string]interface{}{
						"fontSize": "{{ font_size }}", // Not a URL attribute
						"href":     "{{ link_url }}",  // URL attribute
					},
				},
				Content: stringPtr("Hello world"),
			},
			templateData: `{"font_size": "18px", "link_url": "https://example.com"}`,
			expectedHref: `font-size="{{ font_size }}"`, // Should NOT be processed
			expectError:  false,
		},
		{
			name: "background-url with liquid",
			block: &MJSectionBlock{
				BaseBlock: BaseBlock{
					ID:   "section1",
					Type: MJMLComponentMjSection,
					Attributes: map[string]interface{}{
						"backgroundUrl": "{{ campaign.background_image }}",
					},
				},
			},
			templateData: `{"campaign": {"background_image": "https://example.com/bg.jpg"}}`,
			expectedHref: `background-url="https://example.com/bg.jpg"`,
			expectError:  false,
		},
		{
			name: "liquid with conditional logic",
			block: &MJButtonBlock{
				BaseBlock: BaseBlock{
					ID:   "btn2",
					Type: MJMLComponentMjButton,
					Attributes: map[string]interface{}{
						"href": "{% if user.is_premium %}{{ premium_url }}{% else %}{{ regular_url }}{% endif %}",
					},
				},
				Content: stringPtr("Get Started"),
			},
			templateData: `{"user": {"is_premium": true}, "premium_url": "https://premium.example.com", "regular_url": "https://example.com"}`,
			expectedHref: `href="https://premium.example.com"`,
			expectError:  false,
		},
		{
			name: "empty template data",
			block: &MJButtonBlock{
				BaseBlock: BaseBlock{
					ID:   "btn3",
					Type: MJMLComponentMjButton,
					Attributes: map[string]interface{}{
						"href": "{{ fallback_url | default: 'https://fallback.com' }}",
					},
				},
				Content: stringPtr("Fallback"),
			},
			templateData: `{}`,
			expectedHref: `href="https://fallback.com"`,
			expectError:  false,
		},
		{
			name: "liquid with non-breaking space should be cleaned",
			block: &MJButtonBlock{
				BaseBlock: BaseBlock{
					ID:   "btn_nbsp",
					Type: MJMLComponentMjButton,
					Attributes: map[string]interface{}{
						"href": "{{ \u00a0confirm_subscription_url }}",
					},
				},
				Content: stringPtr("Confirm"),
			},
			templateData: `{"confirm_subscription_url": "https://example.com/confirm"}`,
			expectedHref: `href="https://example.com/confirm"`,
			expectError:  false,
		},
		{
			name: "invalid liquid syntax should return original",
			block: &MJButtonBlock{
				BaseBlock: BaseBlock{
					ID:   "btn4",
					Type: MJMLComponentMjButton,
					Attributes: map[string]interface{}{
						"href": "{{ invalid syntax",
					},
				},
				Content: stringPtr("Error Test"),
			},
			templateData: `{"url": "https://test.com"}`,
			expectedHref: `href="{{ invalid syntax"`, // Should return original on error
			expectError:  false,                      // We don't error, just log warning
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ConvertJSONToMJMLWithData(tt.block, tt.templateData)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if !strings.Contains(result, tt.expectedHref) {
				t.Errorf("Expected result to contain '%s', got: %s", tt.expectedHref, result)
			}
		})
	}
}

func TestProcessAttributeValue(t *testing.T) {
	tests := []struct {
		name         string
		value        string
		attributeKey string
		templateData map[string]interface{}
		blockID      string
		expected     string
	}{
		{
			name:         "href attribute with liquid",
			value:        "{{ base_url }}/profile",
			attributeKey: "href",
			templateData: map[string]interface{}{"base_url": "https://example.com"},
			blockID:      "test",
			expected:     "https://example.com/profile",
		},
		{
			name:         "src attribute with liquid",
			value:        "{{ cdn_url }}/image.jpg",
			attributeKey: "src",
			templateData: map[string]interface{}{"cdn_url": "https://cdn.example.com"},
			blockID:      "test",
			expected:     "https://cdn.example.com/image.jpg",
		},
		{
			name:         "non-url attribute should not process",
			value:        "{{ font_size }}",
			attributeKey: "fontSize",
			templateData: map[string]interface{}{"font_size": "18px"},
			blockID:      "test",
			expected:     "{{ font_size }}", // Should return original
		},
		{
			name:         "action attribute with liquid",
			value:        "{{ form_action }}",
			attributeKey: "action",
			templateData: map[string]interface{}{"form_action": "https://api.example.com/submit"},
			blockID:      "test",
			expected:     "https://api.example.com/submit",
		},
		{
			name:         "custom-url attribute with liquid",
			value:        "{{ custom_value }}",
			attributeKey: "my-custom-url",
			templateData: map[string]interface{}{"custom_value": "https://custom.example.com"},
			blockID:      "test",
			expected:     "https://custom.example.com",
		},
		{
			name:         "nil template data",
			value:        "{{ some_var }}",
			attributeKey: "href",
			templateData: nil,
			blockID:      "test",
			expected:     "{{ some_var }}", // Should return original
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processAttributeValue(tt.value, tt.attributeKey, tt.templateData, tt.blockID)

			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}
