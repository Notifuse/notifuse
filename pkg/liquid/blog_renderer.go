package liquid

// RenderBlogTemplate renders a Liquid template with the provided data using pure Go Liquid.
// This uses the pierre-b/liquid fork (v1.7.1) which includes full Shopify-compatible render tag support.
//
// The partials parameter is optional - pass nil if no partials are needed.
// Partials can be rendered in templates using: {% render 'partial_name' %}
// or with parameters: {% render 'partial_name', param: value %}
func RenderBlogTemplate(template string, data map[string]interface{}, partials map[string]string) (string, error) {
	return RenderBlogTemplateGo(template, data, partials)
}
