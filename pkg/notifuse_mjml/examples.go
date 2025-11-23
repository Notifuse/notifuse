package notifuse_mjml

import (
	"fmt"
	"time"
)

// NewBaseBlock creates a new base block with default values
func NewBaseBlock(id string, componentType MJMLComponentType) *BaseBlock {
	return &BaseBlock{
		ID:         id,
		Type:       componentType,
		Children:   make([]EmailBlock, 0),
		Attributes: GetDefaultAttributes(componentType),
	}
}

// CreateSimpleEmail creates a basic MJML email structure
func CreateSimpleEmail() *MJMLBlock {
	// Add title to head
	titleBase := NewBaseBlock("title-1", MJMLComponentMjTitle)
	titleBase.Content = stringPtr("Welcome Email")
	title := &MJTitleBlock{BaseBlock: titleBase}

	// Create head section
	head := &MJHeadBlock{BaseBlock: NewBaseBlock("head-1", MJMLComponentMjHead)}
	head.Children = []EmailBlock{title}

	// Create text block
	textBase := NewBaseBlock("text-1", MJMLComponentMjText)
	textBase.Content = stringPtr("Welcome to our newsletter!")
	textBase.Attributes["fontSize"] = "16px"
	textBase.Attributes["lineHeight"] = "1.5"
	textBase.Attributes["color"] = "#333333"
	textBase.Attributes["align"] = "center"
	textBlock := &MJTextBlock{BaseBlock: textBase}

	// Create button
	buttonBase := NewBaseBlock("button-1", MJMLComponentMjButton)
	buttonBase.Content = stringPtr("Get Started")
	buttonBase.Attributes["backgroundColor"] = "#007bff"
	buttonBase.Attributes["color"] = "#ffffff"
	buttonBase.Attributes["fontWeight"] = "bold"
	buttonBase.Attributes["borderRadius"] = "5px"
	buttonBase.Attributes["href"] = "https://example.com"
	buttonBase.Attributes["paddingTop"] = "10px"
	buttonBase.Attributes["paddingBottom"] = "10px"
	button := &MJButtonBlock{BaseBlock: buttonBase}

	// Create column
	columnBase := NewBaseBlock("column-1", MJMLComponentMjColumn)
	columnBase.Attributes["paddingLeft"] = "20px"
	columnBase.Attributes["paddingRight"] = "20px"
	column := &MJColumnBlock{BaseBlock: columnBase}
	column.Children = []EmailBlock{textBlock, button}

	// Create section
	sectionBase := NewBaseBlock("section-1", MJMLComponentMjSection)
	sectionBase.Attributes["backgroundColor"] = "#ffffff"
	sectionBase.Attributes["paddingTop"] = "20px"
	sectionBase.Attributes["paddingBottom"] = "20px"
	section := &MJSectionBlock{BaseBlock: sectionBase}
	section.Children = []EmailBlock{column}

	// Create body section
	bodyBase := NewBaseBlock("body-1", MJMLComponentMjBody)
	bodyBase.Attributes["backgroundColor"] = "#f4f4f4"
	bodyBase.Attributes["width"] = "600px"
	body := &MJBodyBlock{BaseBlock: bodyBase}
	body.Children = []EmailBlock{section}

	// Create root MJML block
	mjml := &MJMLBlock{BaseBlock: NewBaseBlock("mjml-1", MJMLComponentMjml)}
	mjml.Children = []EmailBlock{head, body}

	return mjml
}

// CreateEmailWithImage creates an email with an image component
func CreateEmailWithImage() *MJMLBlock {
	mjml := CreateSimpleEmail()

	// Find the body and add an image section
	if len(mjml.Children) > 1 {
		if body, ok := mjml.Children[1].(*MJBodyBlock); ok {
			// Create image
			imageBase := NewBaseBlock("image-1", MJMLComponentMjImage)
			imageBase.Attributes["src"] = "https://via.placeholder.com/600x300"
			imageBase.Attributes["alt"] = "Placeholder Image"
			imageBase.Attributes["fluidOnMobile"] = "true"
			imageBase.Attributes["width"] = "600px"
			image := &MJImageBlock{BaseBlock: imageBase}

			// Create image column
			imageColumn := &MJColumnBlock{BaseBlock: NewBaseBlock("image-column-1", MJMLComponentMjColumn)}
			imageColumn.Children = []EmailBlock{image}

			// Create new section with image
			imageSectionBase := NewBaseBlock("image-section-1", MJMLComponentMjSection)
			imageSectionBase.Attributes["backgroundColor"] = "#ffffff"
			imageSection := &MJSectionBlock{BaseBlock: imageSectionBase}
			imageSection.Children = []EmailBlock{imageColumn}

			// Insert image section before the existing section
			body.Children = append([]EmailBlock{imageSection}, body.Children...)
		}
	}

	return mjml
}

// CreateSocialEmail creates an email with social media links
func CreateSocialEmail() *MJMLBlock {
	mjml := CreateSimpleEmail()

	// Find the body and add a social section
	if len(mjml.Children) > 1 {
		if body, ok := mjml.Children[1].(*MJBodyBlock); ok {
			// Add social elements
			facebookBase := NewBaseBlock("facebook-1", MJMLComponentMjSocialElement)
			facebookBase.Attributes["name"] = "facebook"
			facebookBase.Attributes["href"] = "https://facebook.com"
			facebookBase.Attributes["backgroundColor"] = "#1877f2"
			facebookElement := &MJSocialElementBlock{BaseBlock: facebookBase}

			twitterBase := NewBaseBlock("twitter-1", MJMLComponentMjSocialElement)
			twitterBase.Attributes["name"] = "twitter"
			twitterBase.Attributes["href"] = "https://twitter.com"
			twitterBase.Attributes["backgroundColor"] = "#1da1f2"
			twitterElement := &MJSocialElementBlock{BaseBlock: twitterBase}

			// Create social block
			socialBase := NewBaseBlock("social-1", MJMLComponentMjSocial)
			socialBase.Attributes["align"] = "center"
			socialBase.Attributes["iconSize"] = "40px"
			socialBase.Attributes["mode"] = "horizontal"
			socialBase.Attributes["innerPadding"] = "4px"
			socialBlock := &MJSocialBlock{BaseBlock: socialBase}
			socialBlock.Children = []EmailBlock{facebookElement, twitterElement}

			// Create social column
			socialColumn := &MJColumnBlock{BaseBlock: NewBaseBlock("social-column-1", MJMLComponentMjColumn)}
			socialColumn.Children = []EmailBlock{socialBlock}

			// Create social section
			socialSectionBase := NewBaseBlock("social-section-1", MJMLComponentMjSection)
			socialSectionBase.Attributes["backgroundColor"] = "#f8f9fa"
			socialSectionBase.Attributes["paddingTop"] = "30px"
			socialSectionBase.Attributes["paddingBottom"] = "30px"
			socialSection := &MJSectionBlock{BaseBlock: socialSectionBase}
			socialSection.Children = []EmailBlock{socialColumn}

			// Add social section to the end
			body.Children = append(body.Children, socialSection)
		}
	}

	return mjml
}

// ConvertToEmailBuilderState converts an MJML structure to EmailBuilderState
func ConvertToEmailBuilderState(mjml EmailBlock) *EmailBuilderState {
	return &EmailBuilderState{
		SelectedBlockID: nil,
		History:         []EmailBlock{mjml},
		HistoryIndex:    0,
		ViewportMode:    stringPtr("desktop"),
	}
}

// CreateSavedBlock creates a saved block for storage
func CreateSavedBlock(id, name string, block EmailBlock) *SavedBlock {
	now := time.Now()
	return &SavedBlock{
		ID:      id,
		Name:    name,
		Block:   block,
		Created: &now,
		Updated: &now,
	}
}

// PrintEmailStructure prints the structure of an email for debugging
func PrintEmailStructure(block EmailBlock, depth int) {
	indent := ""
	for i := 0; i < depth; i++ {
		indent += "  "
	}

	fmt.Printf("%s%s (ID: %s)\n", indent, GetComponentDisplayName(block.GetType()), block.GetID())

	for _, child := range block.GetChildren() {
		if child != nil {
			PrintEmailStructure(child, depth+1)
		}
	}
}

// ValidateAndPrintEmail validates an email structure and prints any errors
func ValidateAndPrintEmail(email EmailBlock) {
	fmt.Printf("Email Structure:\n")
	PrintEmailStructure(email, 0)

	fmt.Printf("\nValidation:\n")
	if err := ValidateEmailStructure(email); err != nil {
		fmt.Printf("❌ Validation failed: %s\n", err)
	} else {
		fmt.Printf("✅ Email structure is valid\n")
	}
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}

// Helper function to create bool pointers
func boolPtr(b bool) *bool {
	return &b
}

// Helper function to create int pointers
func intPtr(i int) *int {
	return &i
}

// DemoConverter demonstrates the MJML conversion functionality
func DemoConverter() {
	fmt.Println("=== MJML Converter Demo ===")

	// Create a simple email
	email := CreateSimpleEmail()

	// Convert to MJML string
	mjmlString, err := ConvertToMJMLString(email)
	if err != nil {
		fmt.Printf("❌ Conversion error: %s\n", err)
		return
	}

	fmt.Printf("✅ Successfully converted to MJML:\n\n%s\n", mjmlString)

	// Demonstrate conversion with options
	fmt.Println("\n=== Conversion with Options ===")
	options := MJMLConvertOptions{
		Validate:      true,
		PrettyPrint:   true,
		IncludeXMLTag: true,
	}

	mjmlWithOptions, err := ConvertToMJMLWithOptions(email, options)
	if err != nil {
		fmt.Printf("❌ Conversion with options error: %s\n", err)
		return
	}

	fmt.Printf("✅ MJML with XML declaration:\n\n%s\n", mjmlWithOptions)
}

// ConvertEmailToMJMLDemo creates an email and shows the MJML output
func ConvertEmailToMJMLDemo() {
	// Create a more complex email with social elements
	email := CreateSocialEmail()

	fmt.Println("=== Email Structure ===")
	PrintEmailStructure(email, 0)

	fmt.Println("\n=== Generated MJML ===")
	mjml := ConvertJSONToMJML(email)
	fmt.Println(mjml)

	fmt.Println("\n=== Validation ===")
	if err := ValidateEmailStructure(email); err != nil {
		fmt.Printf("❌ Validation failed: %s\n", err)
	} else {
		fmt.Printf("✅ Email structure is valid\n")
	}
}

// TestConverterFunctions demonstrates individual converter functions
func TestConverterFunctions() {
	fmt.Println("=== Testing Individual Converter Functions ===")

	// Test camelToKebab conversion
	testCases := []string{
		"backgroundColor",
		"fontSize",
		"paddingTop",
		"fullWidthBackgroundColor",
		"innerBorderRadius",
	}

	fmt.Println("CamelCase to kebab-case conversion:")
	for _, test := range testCases {
		kebab := camelToKebab(test)
		fmt.Printf("  %s -> %s\n", test, kebab)
	}

	// Test attribute escaping
	fmt.Println("\nAttribute value escaping:")
	testValues := []string{
		"Hello & Goodbye",
		"<script>alert('test')</script>",
		`He said "Hello"`,
		"It's a test",
	}

	for _, test := range testValues {
		escaped := escapeAttributeValue(test, "title")
		fmt.Printf("  %s -> %s\n", test, escaped)
	}

	// Test content escaping
	fmt.Println("\nContent escaping:")
	testContent := []string{
		"<b>Bold text</b>",
		"A & B > C",
		"<script>alert('xss')</script>",
	}

	for _, test := range testContent {
		escaped := escapeContent(test)
		fmt.Printf("  %s -> %s\n", test, escaped)
	}
}
