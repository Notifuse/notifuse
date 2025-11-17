package liquid

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderBlogTemplate(t *testing.T) {
	t.Run("renders simple template", func(t *testing.T) {
		template := "<h1>{{ title }}</h1>"
		data := map[string]interface{}{
			"title": "Hello World",
		}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		assert.Equal(t, "<h1>Hello World</h1>", html)
	})

	t.Run("renders template with loops", func(t *testing.T) {
		template := `<ul>{% for item in items %}<li>{{ item }}</li>{% endfor %}</ul>`
		data := map[string]interface{}{
			"items": []string{"one", "two", "three"},
		}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		assert.Equal(t, "<ul><li>one</li><li>two</li><li>three</li></ul>", html)
	})

	t.Run("renders template with conditionals", func(t *testing.T) {
		template := `{% if show %}<p>Visible</p>{% endif %}`
		data := map[string]interface{}{
			"show": true,
		}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		assert.Equal(t, "<p>Visible</p>", html)
	})

	t.Run("renders workspace data", func(t *testing.T) {
		template := `<h1>{{ workspace.name }}</h1><p>ID: {{ workspace.id }}</p>`
		data := map[string]interface{}{
			"workspace": map[string]interface{}{
				"id":   "ws-123",
				"name": "My Workspace",
			},
		}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		assert.Contains(t, html, "My Workspace")
		assert.Contains(t, html, "ws-123")
	})

	t.Run("renders public lists", func(t *testing.T) {
		template := `{% for list in public_lists %}<div>{{ list.name }}</div>{% endfor %}`
		data := map[string]interface{}{
			"public_lists": []map[string]interface{}{
				{"id": "list-1", "name": "Newsletter"},
				{"id": "list-2", "name": "Updates"},
			},
		}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		assert.Contains(t, html, "Newsletter")
		assert.Contains(t, html, "Updates")
	})

	t.Run("handles empty public lists", func(t *testing.T) {
		template := `{% if public_lists.size > 0 %}Has lists{% else %}No lists{% endif %}`
		data := map[string]interface{}{
			"public_lists": []map[string]interface{}{},
		}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		assert.Contains(t, html, "No lists")
	})

	t.Run("returns error for invalid template", func(t *testing.T) {
		template := `{% for item in items %}<li>{{ item }}</li>` // Missing endfor
		data := map[string]interface{}{
			"items": []string{"one"},
		}

		_, err := RenderBlogTemplate(template, data, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "liquid rendering failed")
	})

	t.Run("returns error for empty template", func(t *testing.T) {
		template := ""
		data := map[string]interface{}{}

		_, err := RenderBlogTemplate(template, data, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "template content is empty")
	})

	t.Run("renders complex nested data", func(t *testing.T) {
		template := `<h1>{{ post.title }}</h1>
{% if post.seo %}
<meta name="description" content="{{ post.seo.meta_description }}">
{% endif %}`
		data := map[string]interface{}{
			"post": map[string]interface{}{
				"title": "My Post",
				"seo": map[string]interface{}{
					"meta_description": "Post description",
				},
			},
		}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		assert.Contains(t, html, "My Post")
		assert.Contains(t, html, "Post description")
	})

	t.Run("renders post with authors array", func(t *testing.T) {
		template := `{% for author in post.authors %}<span>{{ author.name }}</span>{% endfor %}`
		data := map[string]interface{}{
			"post": map[string]interface{}{
				"authors": []map[string]interface{}{
					{"name": "John Doe"},
					{"name": "Jane Smith"},
				},
			},
		}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		assert.Contains(t, html, "John Doe")
		assert.Contains(t, html, "Jane Smith")
	})

	t.Run("handles missing data gracefully", func(t *testing.T) {
		template := `<h1>{{ missing_field }}</h1>`
		data := map[string]interface{}{}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		assert.Equal(t, "<h1></h1>", html)
	})
}

func TestRenderBlogTemplateWithPartials(t *testing.T) {
	t.Run("renders template with simple partial", func(t *testing.T) {
		template := `<div>{% include 'shared' %}</div>`
		partials := map[string]string{
			"shared": `<p>This is a shared partial</p>`,
		}
		data := map[string]interface{}{}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		assert.Contains(t, html, "This is a shared partial")
	})

	t.Run("renders template with partial using widget parameter", func(t *testing.T) {
		template := `<div>{% assign widget = 'newsletter' %}{% include 'shared' %}</div>`
		partials := map[string]string{
			"shared": `{%- if widget == 'newsletter' -%}<div class="newsletter">Subscribe now!</div>{%- endif -%}`,
		}
		data := map[string]interface{}{}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		assert.Contains(t, html, "Subscribe now!")
	})

	t.Run("renders template with categories widget", func(t *testing.T) {
		template := `<div>{% assign widget = 'categories' %}{% include 'shared' %}</div>`
		partials := map[string]string{
			"shared": `{%- if widget == 'categories' -%}<ul>{% for cat in categories %}<li>{{ cat.name }}</li>{% endfor %}</ul>{%- endif -%}`,
		}
		data := map[string]interface{}{
			"categories": []map[string]interface{}{
				{"name": "Tech", "slug": "tech"},
				{"name": "Design", "slug": "design"},
			},
		}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		assert.Contains(t, html, "Tech")
		assert.Contains(t, html, "Design")
	})

	t.Run("renders template with authors widget", func(t *testing.T) {
		template := `<div>{% assign authors = post.authors %}{% assign widget = 'authors' %}{% include 'shared' %}</div>`
		partials := map[string]string{
			"shared": `{%- if widget == 'authors' -%}<div class="authors">{% for author in authors %}<span>{{ author.name }}</span>{% endfor %}</div>{%- endif -%}`,
		}
		data := map[string]interface{}{
			"post": map[string]interface{}{
				"authors": []map[string]interface{}{
					{"name": "John Doe"},
					{"name": "Jane Smith"},
				},
			},
		}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		assert.Contains(t, html, "John Doe")
		assert.Contains(t, html, "Jane Smith")
	})

	t.Run("renders template with multiple partial calls", func(t *testing.T) {
		template := `<div>{% assign widget = 'newsletter' %}{% include 'shared' %}{% assign widget = 'categories' %}{% include 'shared' %}</div>`
		partials := map[string]string{
			"shared": `{%- if widget == 'newsletter' -%}<div>Newsletter</div>{%- elsif widget == 'categories' -%}<div>Categories</div>{%- endif -%}`,
		}
		data := map[string]interface{}{}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		assert.Contains(t, html, "Newsletter")
		assert.Contains(t, html, "Categories")
	})

	t.Run("handles missing partial gracefully", func(t *testing.T) {
		template := `<div>{% include 'nonexistent' %}</div>`
		partials := map[string]string{
			"shared": `<p>Content</p>`,
		}
		data := map[string]interface{}{}

		_, err := RenderBlogTemplate(template, data, partials)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "liquid rendering failed")
	})

	t.Run("renders with nil partials", func(t *testing.T) {
		template := `<h1>{{ title }}</h1>`
		data := map[string]interface{}{
			"title": "Test",
		}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		assert.Equal(t, "<h1>Test</h1>", html)
	})

	t.Run("renders with empty partials map", func(t *testing.T) {
		template := `<h1>{{ title }}</h1>`
		partials := map[string]string{}
		data := map[string]interface{}{
			"title": "Test",
		}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		assert.Equal(t, "<h1>Test</h1>", html)
	})

	t.Run("ignores empty partial content", func(t *testing.T) {
		template := `<div>{% include 'shared' %}</div>`
		partials := map[string]string{
			"shared": "",
			"other":  "<p>Content</p>",
		}
		data := map[string]interface{}{}

		_, err := RenderBlogTemplate(template, data, partials)
		// Should error because 'shared' partial is not registered (empty content is skipped)
		assert.Error(t, err)
	})
}

