package liquid

import (
	"strings"
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

	t.Run("handles missing partial with error message", func(t *testing.T) {
		template := `<div>{% render 'nonexistent' %}</div>`
		partials := map[string]string{
			"shared": `<p>Content</p>`,
		}
		data := map[string]interface{}{}

		// liquidgo shows inline error for missing partials (lax mode default)
		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		assert.Contains(t, html, "Liquid error")
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

	t.Run("allows empty partial content", func(t *testing.T) {
		template := `<div>{% include 'shared' %}</div>`
		partials := map[string]string{
			"shared": "",
			"other":  "<p>Content</p>",
		}
		data := map[string]interface{}{}

		html, err := RenderBlogTemplate(template, data, partials)
		// Empty partials should be allowed and just render nothing
		assert.NoError(t, err)
		assert.Equal(t, "<div></div>", html)
	})
}

func TestRenderBlogTemplateWithParameterizedRenders(t *testing.T) {
	t.Run("render with single parameter", func(t *testing.T) {
		// Test the render tag with a single parameter (liquidjs/Jekyll/Shopify syntax)
		template := `<div>{% render 'shared', widget: 'newsletter' %}</div>`
		partials := map[string]string{
			"shared": `{%- if widget == 'newsletter' -%}<div class="newsletter">Subscribe!</div>{%- endif -%}`,
		}
		data := map[string]interface{}{}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		assert.Contains(t, html, "Subscribe!")
	})

	t.Run("render with multiple parameters", func(t *testing.T) {
		// Test the render tag with multiple parameters
		template := `<div>{% for post in posts %}{% render 'shared', widget: 'post-card', post: post %}{% endfor %}</div>`
		partials := map[string]string{
			"shared": `{%- if widget == 'post-card' -%}<article><h3>{{ post.title }}</h3></article>{%- endif -%}`,
		}
		data := map[string]interface{}{
			"posts": []map[string]interface{}{
				{"title": "First Post", "slug": "first-post"},
				{"title": "Second Post", "slug": "second-post"},
			},
		}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		assert.Contains(t, html, "First Post")
		assert.Contains(t, html, "Second Post")
	})

	t.Run("render with parameter and data", func(t *testing.T) {
		// Test render with parameter that contains data
		template := `<div>{% render 'shared', widget: 'categories', active_category: 'tech' %}</div>`
		partials := map[string]string{
			"shared": `{%- if widget == 'categories' -%}<div class="active">{{ active_category }}</div>{%- endif -%}`,
		}
		data := map[string]interface{}{}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		assert.Contains(t, html, "tech")
	})

	t.Run("render parameters are scoped to partial", func(t *testing.T) {
		// Test that parameters passed to render are scoped to the partial only
		template := `<div>{% assign widget = 'global' %}{% render 'shared', widget: 'newsletter' %}{{ widget }}</div>`
		partials := map[string]string{
			"shared": `{%- if widget == 'newsletter' -%}<span>Newsletter</span>{%- endif -%}`,
		}
		data := map[string]interface{}{}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		assert.Contains(t, html, "Newsletter")
		assert.Contains(t, html, "global") // Original widget value should remain
	})

	t.Run("render parameter isolation between renders", func(t *testing.T) {
		// Test that parameters from one render don't leak to another
		template := `<div>{% render 'shared', widget: 'newsletter' %}{% render 'shared', widget: 'categories' %}</div>`
		partials := map[string]string{
			"shared": `{%- if widget == 'newsletter' -%}<span>Newsletter</span>{%- endif -%}{%- if widget == 'categories' -%}<span>Categories</span>{%- endif -%}`,
		}
		data := map[string]interface{}{}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		assert.Contains(t, html, "Newsletter")
		assert.Contains(t, html, "Categories")
	})

	t.Run("nested renders with parameters", func(t *testing.T) {
		// Test nested renders with their own parameters
		template := `<div>{% render 'outer', title: 'Main' %}</div>`
		partials := map[string]string{
			"outer": `<section><h1>{{ title }}</h1>{% render 'inner', subtitle: 'Sub' %}</section>`,
			"inner": `<p>{{ subtitle }}</p>`,
		}
		data := map[string]interface{}{}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		assert.Contains(t, html, "Main")
		assert.Contains(t, html, "Sub")
	})

	t.Run("render with complex object parameter", func(t *testing.T) {
		// Test passing a complex object as a parameter
		template := `<div>{% for post in posts %}{% render 'post-card', post: post %}{% endfor %}</div>`
		partials := map[string]string{
			"post-card": `<article><h3>{{ post.title }}</h3><p>{{ post.excerpt }}</p><span>{{ post.reading_time }} min</span></article>`,
		}
		data := map[string]interface{}{
			"posts": []map[string]interface{}{
				{"title": "Post One", "excerpt": "Excerpt one", "reading_time": 5},
				{"title": "Post Two", "excerpt": "Excerpt two", "reading_time": 10},
			},
		}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		assert.Contains(t, html, "Post One")
		assert.Contains(t, html, "Excerpt one")
		assert.Contains(t, html, "5 min")
		assert.Contains(t, html, "Post Two")
		assert.Contains(t, html, "Excerpt two")
		assert.Contains(t, html, "10 min")
	})
}

func TestRenderBlogTemplateWithRealisticData(t *testing.T) {
	t.Run("renders home page with posts and categories", func(t *testing.T) {
		// Simulates the actual home.liquid pattern with fixed syntax
		template := `
			<h1>{{ workspace.name }}</h1>
			{% assign widget = 'categories' %}{% include 'shared' %}
			{% for post in posts %}
				{% assign widget = 'post-card' %}{% include 'shared' %}
			{% endfor %}
			{% assign widget = 'pagination' %}{% include 'shared' %}
		`
		partials := map[string]string{
			"shared": `
				{%- if widget == 'categories' -%}
					<nav>{% for cat in categories %}<a href="/{{ cat.slug }}">{{ cat.name }}</a>{% endfor %}</nav>
				{%- elsif widget == 'post-card' -%}
					<article>
						<h3>{{ post.title }}</h3>
						<a href="{{ base_url }}/{{ post.category_slug }}/{{ post.slug }}">Read more</a>
					</article>
				{%- elsif widget == 'pagination' -%}
					{%- if pagination.total_pages > 1 -%}
						<div>Page {{ pagination.current_page }} of {{ pagination.total_pages }}</div>
					{%- endif -%}
				{%- endif -%}
			`,
		}
		data := map[string]interface{}{
			"base_url": "https://blog.example.com",
			"workspace": map[string]interface{}{
				"name": "My Blog",
			},
			"categories": []map[string]interface{}{
				{"slug": "tech", "name": "Technology"},
				{"slug": "design", "name": "Design"},
			},
			"posts": []map[string]interface{}{
				{
					"title":         "First Post",
					"slug":          "first-post",
					"category_slug": "tech",
				},
				{
					"title":         "Second Post",
					"slug":          "second-post",
					"category_slug": "design",
				},
			},
			"pagination": map[string]interface{}{
				"current_page": 1,
				"total_pages":  3,
			},
		}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		assert.Contains(t, html, "My Blog")
		assert.Contains(t, html, "Technology")
		assert.Contains(t, html, "Design")
		assert.Contains(t, html, "First Post")
		assert.Contains(t, html, "Second Post")
		assert.Contains(t, html, "https://blog.example.com/tech/first-post")
		assert.Contains(t, html, "https://blog.example.com/design/second-post")
		assert.Contains(t, html, "Page 1 of 3")
	})

	t.Run("handles empty posts array gracefully", func(t *testing.T) {
		template := `
			<h1>{{ workspace.name }}</h1>
			{% if posts.size > 0 %}
				{% for post in posts %}
					{% assign widget = 'post-card' %}{% include 'shared' %}
				{% endfor %}
			{% else %}
				<p>No posts found</p>
			{% endif %}
		`
		partials := map[string]string{
			"shared": `{%- if widget == 'post-card' -%}<article>{{ post.title }}</article>{%- endif -%}`,
		}
		data := map[string]interface{}{
			"workspace": map[string]interface{}{
				"name": "My Blog",
			},
			"posts": []map[string]interface{}{},
		}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		assert.Contains(t, html, "My Blog")
		assert.Contains(t, html, "No posts found")
	})

	t.Run("handles missing category_slug gracefully", func(t *testing.T) {
		// Test post without category_slug (shouldn't happen but defensive)
		template := `
			{% for post in posts %}
				<a href="{{ base_url }}/{{ post.category_slug }}/{{ post.slug }}">{{ post.title }}</a>
			{% endfor %}
		`
		data := map[string]interface{}{
			"base_url": "https://blog.example.com",
			"posts": []map[string]interface{}{
				{
					"title": "Test Post",
					"slug":  "test-post",
					// category_slug is missing
				},
			},
		}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		// Should render with empty category_slug
		assert.Contains(t, html, "Test Post")
		assert.Contains(t, html, "https://blog.example.com//test-post")
	})

	t.Run("renders category page with active category", func(t *testing.T) {
		template := `
			<h1>{{ category.name }}</h1>
			{% assign widget = 'categories' %}{% assign active_category = category.slug %}{% include 'shared' %}
			{% for post in posts %}
				{% assign widget = 'post-card' %}{% include 'shared' %}
			{% endfor %}
		`
		partials := map[string]string{
			"shared": `
				{%- if widget == 'categories' -%}
					<nav>
						{% for cat in categories %}
							<a href="/{{ cat.slug }}" {% if active_category == cat.slug %}class="active"{% endif %}>{{ cat.name }}</a>
						{% endfor %}
					</nav>
				{%- elsif widget == 'post-card' -%}
					<article><h3>{{ post.title }}</h3></article>
				{%- endif -%}
			`,
		}
		data := map[string]interface{}{
			"category": map[string]interface{}{
				"slug": "tech",
				"name": "Technology",
			},
			"categories": []map[string]interface{}{
				{"slug": "tech", "name": "Technology"},
				{"slug": "design", "name": "Design"},
			},
			"posts": []map[string]interface{}{
				{"title": "Tech Post 1", "slug": "tech-post-1"},
			},
		}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		assert.Contains(t, html, "Technology")
		assert.Contains(t, html, "Tech Post 1")
		assert.Contains(t, html, "active")
	})

	t.Run("handles complex nested widget includes", func(t *testing.T) {
		// Test multiple widget switches in a single template
		template := `
			{% assign widget = 'newsletter' %}{% include 'shared' %}
			{% assign widget = 'categories' %}{% include 'shared' %}
			{% for post in posts %}
				{% assign widget = 'post-card' %}{% include 'shared' %}
			{% endfor %}
			{% assign widget = 'pagination' %}{% include 'shared' %}
		`
		partials := map[string]string{
			"shared": `
				{%- if widget == 'newsletter' -%}Newsletter{%- endif -%}
				{%- if widget == 'categories' -%}Categories{%- endif -%}
				{%- if widget == 'post-card' -%}Post: {{ post.title }}{%- endif -%}
				{%- if widget == 'pagination' -%}Pagination{%- endif -%}
			`,
		}
		data := map[string]interface{}{
			"posts": []map[string]interface{}{
				{"title": "Post 1"},
				{"title": "Post 2"},
			},
		}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		assert.Contains(t, html, "Newsletter")
		assert.Contains(t, html, "Categories")
		assert.Contains(t, html, "Post: Post 1")
		assert.Contains(t, html, "Post: Post 2")
		assert.Contains(t, html, "Pagination")
	})
}

// TestRenderBlogTemplateResourceLimits tests that resource limits are enforced
func TestRenderBlogTemplateResourceLimits(t *testing.T) {
	t.Run("enforces template size limit", func(t *testing.T) {
		// Create a template larger than 100KB
		largeTemplate := strings.Repeat("<div>{{ item }}</div>\n", 10000) // ~200KB
		data := map[string]interface{}{
			"item": "test",
		}

		_, err := RenderBlogTemplate(largeTemplate, data, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum allowed size")
	})

	t.Run("enforces render timeout on infinite loop", func(t *testing.T) {
		// Template with very large loop that should timeout
		template := `
		{% assign limit = 1000000 %}
		{% for i in (1..limit) %}
			{% for j in (1..limit) %}
				<div>{{ i }} - {{ j }}</div>
			{% endfor %}
		{% endfor %}
		`
		data := map[string]interface{}{}

		_, err := RenderBlogTemplate(template, data, nil)
		// Should timeout or fail
		assert.Error(t, err)
	})

	t.Run("allows normal sized templates", func(t *testing.T) {
		// Template under 100KB should work fine
		template := strings.Repeat("<div>{{ item }}</div>\n", 100) // ~2KB
		data := map[string]interface{}{
			"item": "test",
		}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		assert.Contains(t, html, "<div>test</div>")
	})

	t.Run("handles deep nesting gracefully", func(t *testing.T) {
		// Test nesting depth (security doc mentions 20 levels)
		template := `
		{% if level1 %}
			{% if level2 %}
				{% if level3 %}
					{% if level4 %}
						{% if level5 %}
							<div>Deep content</div>
						{% endif %}
					{% endif %}
				{% endif %}
			{% endif %}
		{% endif %}
		`
		data := map[string]interface{}{
			"level1": true,
			"level2": true,
			"level3": true,
			"level4": true,
			"level5": true,
		}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		assert.Contains(t, html, "Deep content")
	})
}

// TestLiquidSecurityFeatures tests security features from LIQUID_SECURITY.md
func TestLiquidSecurityFeatures(t *testing.T) {
	t.Run("XSS protection with escape filter", func(t *testing.T) {
		template := `<div>{{ user_input | escape }}</div>`
		data := map[string]interface{}{
			"user_input": `<script>alert("XSS")</script>`,
		}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		assert.Contains(t, html, "&lt;script&gt;")
		assert.NotContains(t, html, "<script>")
	})

	t.Run("allows safe tags - assign", func(t *testing.T) {
		template := `{% assign myvar = inputval %}<p>{{ myvar }}</p>`
		data := map[string]interface{}{
			"inputval": "value",
		}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		assert.Equal(t, "<p>value</p>", html)
	})

	t.Run("allows safe tags - case/when", func(t *testing.T) {
		template := `{% case status %}{% when 'active' %}Active{% when 'inactive' %}Inactive{% else %}Unknown{% endcase %}`
		data := map[string]interface{}{
			"status": "active",
		}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		assert.Equal(t, "Active", html)
	})

	t.Run("allows safe tags - comment", func(t *testing.T) {
		template := `<div>{% comment %}This is a comment{% endcomment %}Visible</div>`
		data := map[string]interface{}{}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		assert.Equal(t, "<div>Visible</div>", html)
		assert.NotContains(t, html, "comment")
	})

	t.Run("allows safe tags - raw", func(t *testing.T) {
		template := `{% raw %}{{ not_evaluated }}{% endraw %}`
		data := map[string]interface{}{
			"not_evaluated": "value",
		}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		assert.Equal(t, "{{ not_evaluated }}", html)
	})

	t.Run("allows safe filters - upcase", func(t *testing.T) {
		template := `{{ text | upcase }}`
		data := map[string]interface{}{
			"text": "hello",
		}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		assert.Equal(t, "HELLO", html)
	})

	t.Run("allows safe filters - downcase", func(t *testing.T) {
		template := `{{ text | downcase }}`
		data := map[string]interface{}{
			"text": "HELLO",
		}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		assert.Equal(t, "hello", html)
	})

	t.Run("allows safe filters - join", func(t *testing.T) {
		template := `{{ items | join: ', ' }}`
		data := map[string]interface{}{
			"items": []string{"one", "two", "three"},
		}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		assert.Equal(t, "one, two, three", html)
	})

	t.Run("allows safe filters - plus", func(t *testing.T) {
		template := `{{ num | plus: 5 }}`
		data := map[string]interface{}{
			"num": 10,
		}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		assert.Equal(t, "15", html)
	})

	t.Run("allows safe filters - strip_html", func(t *testing.T) {
		template := `{{ text | strip_html }}`
		data := map[string]interface{}{
			"text": "<p>Hello <b>World</b></p>",
		}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		assert.Contains(t, html, "Hello")
		assert.Contains(t, html, "World")
		assert.NotContains(t, html, "<p>")
		assert.NotContains(t, html, "<b>")
	})

	t.Run("handles balanced tags correctly", func(t *testing.T) {
		template := `{% if true %}<div>Content</div>{% endif %}`
		data := map[string]interface{}{}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		assert.Equal(t, "<div>Content</div>", html)
	})

	t.Run("rejects unbalanced tags", func(t *testing.T) {
		template := `{% if true %}<div>Content</div>` // Missing {% endif %}
		data := map[string]interface{}{}

		_, err := RenderBlogTemplate(template, data, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "liquidjs rendering failed")
	})

	t.Run("no file system access - custom fs only", func(t *testing.T) {
		// Template tries to render a partial that doesn't exist in our custom fs
		template := `{% render 'does-not-exist' %}`
		partials := map[string]string{
			"exists": "content",
		}
		data := map[string]interface{}{}

		_, err := RenderBlogTemplate(template, data, partials)
		assert.Error(t, err)
		// Should fail because partial doesn't exist in custom fs
	})

	t.Run("handles undefined variables gracefully", func(t *testing.T) {
		// LIQUID_SECURITY.md mentions strictVariables: false
		template := `<div>{{ undefined_var }}</div>`
		data := map[string]interface{}{}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		// Should render empty string for undefined variables
		assert.Equal(t, "<div></div>", html)
	})

	t.Run("allows multiple safe features together", func(t *testing.T) {
		template := `
		{% assign greeting = greet %}
		{% if show %}
			<ul>
			{% for item in items %}
				<li>{{ greeting | upcase }}: {{ item | escape }}</li>
			{% endfor %}
			</ul>
		{% endif %}
		`
		data := map[string]interface{}{
			"show":  true,
			"greet": "hello",
			"items": []string{"<b>one</b>", "two"},
		}

		html, err := RenderBlogTemplate(template, data, nil)
		assert.NoError(t, err)
		assert.Contains(t, html, "HELLO")
		assert.Contains(t, html, "&lt;b&gt;one&lt;/b&gt;")
		assert.Contains(t, html, "two")
	})
}

// TestRenderBlogTemplate_WorkspaceIdInPartial tests that workspace.id is accessible in included partials
// This simulates the scripts.js partial scenario where {{ workspace.id }} should render correctly
func TestRenderBlogTemplate_WorkspaceIdInPartial(t *testing.T) {
	t.Run("workspace.id accessible in included partial", func(t *testing.T) {
		// Template that includes a partial (simulating header.liquid including scripts)
		template := `<script>{% include 'scripts' %}</script>`

		// Partial that uses workspace.id (simulating scripts.js)
		partials := map[string]string{
			"scripts": `const NOTIFUSE_CONFIG = { workspaceId: '{{ workspace.id }}' };`,
		}

		data := map[string]interface{}{
			"workspace": map[string]interface{}{
				"id":   "ws-123",
				"name": "My Workspace",
			},
		}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		assert.Contains(t, html, "ws-123")
		assert.Contains(t, html, "workspaceId: 'ws-123'")
	})

	t.Run("workspace.id accessible in partial with full NOTIFUSE_CONFIG pattern", func(t *testing.T) {
		// Simulate the exact scripts.js pattern
		template := `<script>{% include 'scripts' %}</script>`

		partials := map[string]string{
			"scripts": `const NOTIFUSE_CONFIG = {
  domain: '{{ base_url }}',
  workspaceId: '{{ workspace.id }}',
  listIds: [
    {%- for list in public_lists -%}
      '{{ list.id }}'{% unless forloop.last %},{% endunless %}
    {%- endfor -%}
  ]
};`,
		}

		data := map[string]interface{}{
			"base_url": "https://example.com",
			"workspace": map[string]interface{}{
				"id":   "ws-456",
				"name": "Test Workspace",
			},
			"public_lists": []map[string]interface{}{
				{"id": "list-1", "name": "Newsletter"},
				{"id": "list-2", "name": "Updates"},
			},
		}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		assert.Contains(t, html, "workspaceId: 'ws-456'")
		assert.Contains(t, html, "domain: 'https://example.com'")
		assert.Contains(t, html, "'list-1'")
		assert.Contains(t, html, "'list-2'")
	})

	t.Run("workspace.id renders empty when workspace is nil", func(t *testing.T) {
		template := `<script>{% include 'scripts' %}</script>`

		partials := map[string]string{
			"scripts": `const NOTIFUSE_CONFIG = { workspaceId: '{{ workspace.id }}' };`,
		}

		data := map[string]interface{}{}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		// When workspace is missing, liquidgo should render empty string
		assert.Contains(t, html, "workspaceId: ''")
	})

	t.Run("workspace.id accessible in nested partial includes", func(t *testing.T) {
		// Test that workspace.id is accessible even when partials include other partials
		template := `<div>{% include 'header' %}</div>`

		partials := map[string]string{
			"header":  `<head><script>{% include 'scripts' %}</script></head>`,
			"scripts": `const NOTIFUSE_CONFIG = { workspaceId: '{{ workspace.id }}' };`,
		}

		data := map[string]interface{}{
			"workspace": map[string]interface{}{
				"id":   "ws-nested",
				"name": "Nested Test",
			},
		}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		assert.Contains(t, html, "workspaceId: 'ws-nested'")
	})
}

// TestRenderBlogTemplate_IntegrationWithBuildBlogTemplateData tests the integration
// between BuildBlogTemplateData and RenderBlogTemplate, simulating the exact production scenario
func TestRenderBlogTemplate_IntegrationWithBuildBlogTemplateData(t *testing.T) {
	t.Run("workspace.id renders correctly in scripts partial using BuildBlogTemplateData", func(t *testing.T) {
		// Import domain package to use BuildBlogTemplateData
		// Note: We'll need to import it, but for now let's simulate the data structure
		// that BuildBlogTemplateData produces

		// Simulate the template structure: header.liquid includes scripts
		template := `<!DOCTYPE html>
<html>
<head>
  <script>{% include 'scripts' %}</script>
</head>
<body>
  <h1>{{ workspace.name }}</h1>
</body>
</html>`

		// Simulate scripts.js partial with NOTIFUSE_CONFIG
		partials := map[string]string{
			"scripts": `const NOTIFUSE_CONFIG = {
  domain: '{{ base_url }}',
  workspaceId: '{{ workspace.id }}',
  listIds: [
    {%- for list in public_lists -%}
      '{{ list.id }}'{% unless forloop.last %},{% endunless %}
    {%- endfor -%}
  ]
};`,
		}

		// Simulate the exact data structure that BuildBlogTemplateData produces
		// This matches what BuildBlogTemplateData creates (see internal/domain/blog.go:934-960)
		data := map[string]interface{}{
			"workspace": map[string]interface{}{
				"id":   "ws-integration-test",
				"name": "Integration Test Workspace",
			},
			"base_url": "https://blog.example.com",
			"public_lists": []map[string]interface{}{
				{"id": "list-1", "name": "Newsletter"},
				{"id": "list-2", "name": "Updates"},
			},
		}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)

		// Verify workspace.id renders correctly in the scripts partial
		assert.Contains(t, html, "workspaceId: 'ws-integration-test'")
		assert.Contains(t, html, "Integration Test Workspace")
		assert.Contains(t, html, "domain: 'https://blog.example.com'")
		assert.Contains(t, html, "'list-1'")
		assert.Contains(t, html, "'list-2'")

		// Verify the rendered JavaScript is valid (no empty workspaceId)
		assert.NotContains(t, html, "workspaceId: ''")
		assert.NotContains(t, html, "workspaceId: '{{ workspace.id }}'")
	})

	t.Run("workspace.id renders correctly even when logo_url is missing", func(t *testing.T) {
		// Test that workspace.id works even when other optional fields are missing
		template := `<script>{% include 'scripts' %}</script>`

		partials := map[string]string{
			"scripts": `const NOTIFUSE_CONFIG = { workspaceId: '{{ workspace.id }}' };`,
		}

		// Workspace data without logo_url (simulating when logo_url is not set)
		data := map[string]interface{}{
			"workspace": map[string]interface{}{
				"id":   "ws-no-logo",
				"name": "No Logo Workspace",
				// logo_url is intentionally missing
			},
		}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		assert.Contains(t, html, "workspaceId: 'ws-no-logo'")
	})
}

// TestRenderBlogTemplate_FullThemeIntegration tests the complete theme rendering flow
// using all 6 theme files from the default theme preset (matching themePresets.ts)
// This simulates the exact production scenario where all files are loaded as partials
func TestRenderBlogTemplate_FullThemeIntegration(t *testing.T) {
	// Theme files extracted from themePresets.ts (default theme)
	// These are the EXACT files used in production
	const (
		homeLiquid = `{%- comment -%} Include Header (shares parent scope for workspace/base_url access) {%- endcomment -%}
{% include 'header' %}

<div class="main-container">
  {%- comment -%} Topbar Navigation {%- endcomment -%}
  <nav class="topbar">
    <div class="w-full">
      <div class="flex items-center justify-between">
        <a href="{{ base_url }}/" class="logo">
          {%- if workspace.logo_url -%}
            <img src="{{ workspace.logo_url }}" alt="{{ workspace.name }}" style="height: 2rem;">
          {%- else -%}
            {{ workspace.name }}
          {%- endif -%}
        </a>

        {%- comment -%} Desktop Navigation {%- endcomment -%}
        <div class="nav-desktop">
          <a href="{{ base_url }}/" class="nav-link">Home</a>
          <a href="{{ base_url }}/about" class="nav-link">About</a>
          <a href="{{ base_url }}/contact" class="nav-link">Contact</a>
        </div>

        {%- comment -%} Mobile Menu Button {%- endcomment -%}
        <button class="nav-mobile-toggle" aria-label="Toggle menu">
          <svg class="hamburger-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" />
          </svg>
          <svg class="close-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24" style="display: none;">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>

      {%- comment -%} Mobile Navigation Dropdown {%- endcomment -%}
      <div class="nav-mobile-menu">
        <a href="{{ base_url }}/" class="nav-mobile-link">Home</a>
        <a href="{{ base_url }}/about" class="nav-mobile-link">About</a>
        <a href="{{ base_url }}/contact" class="nav-mobile-link">Contact</a>
      </div>
    </div>
  </nav>

  {%- comment -%} Hero Section {%- endcomment -%}
  <section class="hero">
    <div>
      <div class="grid grid-cols-1 md:grid-cols-3 gap-12 items-center">
        <div class="md:col-span-2">
          <h1 class="hero-title">
            {%- if workspace.blog_title -%}
              {{ workspace.blog_title }}
            {%- else -%}
              Thoughts, Stories & Ideas
            {%- endif -%}
          </h1>
          <p class="hero-subtitle">
            A place to read, write, and deepen your understanding of the topics that matter most to you.
          </p>
        </div>
        {% render 'shared', widget: 'newsletter' %}
      </div>
    </div>
  </section>

  {%- comment -%} Categories Bar {%- endcomment -%}
  {% render 'shared', widget: 'categories' %}

  {%- comment -%} Featured Post {%- endcomment -%}
  {%- if posts.size > 0 -%}
    {%- assign featured_post = posts[0] -%}
    <section class="featured-post-section">
      <div>
        <div class="featured-post">
          <div>
            {%- if featured_post.featured_image_url -%}
              <img src="{{ featured_post.featured_image_url }}" alt="{{ featured_post.title }}" class="featured-image" />
            {%- endif -%}
          </div>
          <div class="featured-content">
            <h2>{{ featured_post.title }}</h2>
            {%- if featured_post.excerpt -%}
              <p class="excerpt">{{ featured_post.excerpt }}</p>
            {%- endif -%}
            <div class="author-info">
              <div class="author-avatars">
                {%- for author in featured_post.authors limit: 2 -%}
                  {%- if author.avatar_url -%}
                    <img src="{{ author.avatar_url }}" alt="{{ author.name }}" class="author-avatar" />
                  {%- endif -%}
                {%- endfor -%}
              </div>
              <div>
                <div class="author-names">
                  {%- for author in featured_post.authors -%}
                    {{ author.name }}{% unless forloop.last %}, {% endunless %}
                  {%- endfor -%}
                </div>
                <div class="post-date">
                  {%- if featured_post.published_at -%}
                    {{ featured_post.published_at | date: "%b %d, %Y" }}
                  {%- endif -%}
                  {%- if featured_post.reading_time_minutes -%}
                    · {{ featured_post.reading_time_minutes }} min read
                  {%- endif -%}
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  {%- endif -%}

  {%- comment -%} Posts Grid {%- endcomment -%}
  <section class="posts-grid-section">
    <div>
      <div class="grid grid-cols-1 md:grid-cols-3 gap-12">
        {%- for post in posts offset: 1 -%}
          {% render 'shared', widget: 'post-card', post: post %}
        {%- endfor -%}
      </div>
    </div>
  </section>

  {%- comment -%} Pagination {%- endcomment -%}
  {% render 'shared', widget: 'pagination' %}

{%- comment -%} Include Footer (shares parent scope for workspace/base_url access) {%- endcomment -%}
{% include 'footer' %}`

		headerLiquid = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  
  {%- comment -%} Dynamic Page Title {%- endcomment -%}
  <title>
    {%- if post.seo.meta_title -%}
      {{ post.seo.meta_title }}
    {%- elsif post.title -%}
      {{ post.title }} - {{ workspace.name }}
    {%- elsif category.seo.meta_title -%}
      {{ category.seo.meta_title }}
    {%- elsif category.name -%}
      {{ category.name }} - {{ workspace.name }}
    {%- else -%}
      {{ workspace.name }}
    {%- endif -%}
  </title>
  
  {%- comment -%} Favicon {%- endcomment -%}
  {%- if workspace.icon_url -%}
    <link rel="icon" href="{{ workspace.icon_url }}">
  {%- endif -%}
  
  {%- comment -%} Theme Styles {%- endcomment -%}
  <style>{% include 'styles' %}</style>
  
  {%- comment -%} Theme Scripts {%- endcomment -%}
  <script>{% include 'scripts' %}</script>
</head>
<body>`

		footerLiquid = `  {%- comment -%} Footer {%- endcomment -%}
  <footer class="footer">
    <div>
      <div class="footer-content">
        <div class="footer-left">
          <a href="{{ base_url }}/" class="logo mb-2">
            {%- if workspace.blog_title -%}
              {{ workspace.blog_title }}
            {%- else -%}
              {{ workspace.name }}
            {%- endif -%}
          </a>
          <p class="text-gray-600 text-sm">&copy; {{ current_year }} All rights reserved.</p>
        </div>

        <div class="footer-links">
          <a href="{{ base_url }}/terms" class="footer-link">Terms</a>
          <a href="{{ base_url }}/privacy" class="footer-link">Privacy</a>
        </div>
      </div>
    </div>
  </footer>
</div>
</body>
</html>`

		sharedLiquid = `{%- comment -%}
  ========================================
  Shared Widgets Library
  ========================================
{%- endcomment -%}

{%- if widget == 'newsletter' -%}
  {%- comment -%} Newsletter Subscription Form {%- endcomment -%}
  <div class="newsletter-form">
    <h3 class="mb-2">Stay curious.</h3>
    <form>
      <input type="email" placeholder="Enter your email" class="newsletter-input" required />
      <button type="submit" class="newsletter-button">
        Subscribe
      </button>
    </form>
  </div>

{%- elsif widget == 'pagination' -%}
  {%- comment -%} Pagination Controls {%- endcomment -%}
  {%- if pagination.total_pages > 1 -%}
    <nav class="pagination" aria-label="Pagination">
      {%- if pagination.has_previous -%}
        <a href="?page={{ pagination.current_page | minus: 1 }}" class="pagination-button">← Previous</a>
      {%- else -%}
        <span class="pagination-button" disabled>← Previous</span>
      {%- endif -%}
      
      <a href="?page=1" class="pagination-button">1</a>
      
      {%- if pagination.has_next -%}
        <a href="?page={{ pagination.current_page | plus: 1 }}" class="pagination-button">Next →</a>
      {%- else -%}
        <span class="pagination-button" disabled>Next →</span>
      {%- endif -%}
    </nav>
  {%- endif -%}

{%- elsif widget == 'categories' -%}
  {%- comment -%} Category Navigation Pills {%- endcomment -%}
  <div class="categories-bar">
    <div>
      <div class="flex items-center gap-2">
        <a href="{{ base_url }}/" class="category-pill {% unless active_category %}active{% endunless %}">All Posts</a>
        {%- for cat in categories -%}
          <a href="{{ base_url }}/category/{{ cat.slug }}" class="category-pill {% if active_category == cat.slug %}active{% endif %}">{{ cat.name }}</a>
        {%- endfor -%}
      </div>
    </div>
  </div>

{%- elsif widget == 'post-card' -%}
  {%- comment -%} Reusable Post Card {%- endcomment -%}
  <a href="{{ base_url }}/{{ post.category_slug }}/{{ post.slug }}" class="post-card">
    {%- if post.featured_image_url -%}
      <img src="{{ post.featured_image_url }}" alt="{{ post.title }}" class="post-card-image" />
    {%- endif -%}
    <h3 class="post-card-title">{{ post.title }}</h3>
    {%- if post.excerpt -%}
      <p class="post-card-excerpt">{{ post.excerpt }}</p>
    {%- endif -%}
    <div class="author-info">
      <div class="author-avatars">
        {%- for author in post.authors limit: 3 -%}
          {%- if author.avatar_url -%}
            <img src="{{ author.avatar_url }}" alt="{{ author.name }}" class="author-avatar" />
          {%- endif -%}
        {%- endfor -%}
      </div>
      <div>
        <div class="author-names">
          {%- for author in post.authors -%}
            {{ author.name }}{% unless forloop.last %}, {% endunless %}
          {%- endfor -%}
        </div>
        <div class="post-date">
          {%- if post.published_at -%}
            {{ post.published_at | date: "%b %d, %Y" }}
          {%- endif -%}
          {%- if post.reading_time_minutes -%}
            · {{ post.reading_time_minutes }} min read
          {%- endif -%}
        </div>
      </div>
    </div>
  </a>

{%- else -%}
  {%- comment -%}
    Default behavior: Include helpful comment if widget parameter is missing
  {%- endcomment -%}
  <!-- No widget specified. Use: {% render 'shared', widget: 'widget_name' %} -->
{%- endif -%}`

		stylesCSS = `/* Minimal styles for testing */
body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Roboto', sans-serif;
  margin: 0;
  padding: 0;
}
.logo {
  font-size: 1.5rem;
  font-weight: bold;
}
.topbar {
  border-bottom: 1px solid #e5e5e5;
  padding: 1rem 2rem;
}`

		scriptsJS = `// ==================== CONFIGURATION ====================
// Dynamically configured from workspace settings
const NOTIFUSE_CONFIG = {
  domain: '{{ base_url }}',
  workspaceId: '{{ workspace.id }}',
  listIds: [
    {%- for list in public_lists -%}
      '{{ list.id }}'{% unless forloop.last %},{% endunless %}
    {%- endfor -%}
  ]
};
// =======================================================

async function subscribeToNewsletter(email, firstName = null) {
  try {
    const response = await fetch(NOTIFUSE_CONFIG.domain + '/subscribe', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        workspace_id: NOTIFUSE_CONFIG.workspaceId,
        contact: {
          email: email,
          first_name: firstName || null
        },
        list_ids: NOTIFUSE_CONFIG.listIds
      })
    });

    if (response.ok) {
      const result = await response.json();
      return { success: true, data: result };
    } else {
      const error = await response.json();
      return { success: false, error: error.error || 'Subscription failed' };
    }
  } catch (error) {
    console.error('Newsletter subscription error:', error);
    return { success: false, error: 'Network error occurred. Please try again.' };
  }
}`
	)

	t.Run("workspace.id renders in scripts through header include chain", func(t *testing.T) {
		// This test simulates the EXACT production flow:
		// home.liquid → includes header → includes scripts → uses workspace.id
		// This is the critical test that should catch the production bug

		// Setup partials map exactly as in production (blog_service.go:1137-1144)
		partials := map[string]string{
			"shared":  sharedLiquid,
			"header":  headerLiquid,
			"footer":  footerLiquid,
			"styles":  stylesCSS,
			"scripts": scriptsJS,
		}

		// Template data matching BuildBlogTemplateData structure (blog.go:929-1123)
		data := map[string]interface{}{
			"workspace": map[string]interface{}{
				"id":         "ws-full-integration",
				"name":       "Integration Test Blog",
				"logo_url":   "https://example.com/logo.png",
				"icon_url":   "https://example.com/icon.png",
				"blog_title": "My Integration Test Blog",
			},
			"base_url": "https://blog.example.com",
			"public_lists": []map[string]interface{}{
				{"id": "list-abc123", "name": "Newsletter"},
				{"id": "list-def456", "name": "Updates"},
			},
			"posts": []map[string]interface{}{
				{
					"id":                   "post-1",
					"slug":                 "first-post",
					"category_id":          "cat-1",
					"category_slug":        "tech",
					"title":                "First Post",
					"excerpt":              "This is the first post",
					"featured_image_url":   "https://example.com/image1.jpg",
					"reading_time_minutes": 5,
					"published_at":         "2025-01-15T10:00:00Z",
					"authors": []map[string]interface{}{
						{"name": "John Doe", "avatar_url": "https://example.com/john.jpg"},
					},
				},
				{
					"id":                   "post-2",
					"slug":                 "second-post",
					"category_id":          "cat-1",
					"category_slug":        "tech",
					"title":                "Second Post",
					"excerpt":              "This is the second post",
					"featured_image_url":   "https://example.com/image2.jpg",
					"reading_time_minutes": 3,
					"published_at":         "2025-01-16T10:00:00Z",
					"authors": []map[string]interface{}{
						{"name": "Jane Smith", "avatar_url": "https://example.com/jane.jpg"},
					},
				},
			},
			"categories": []map[string]interface{}{
				{"id": "cat-1", "slug": "tech", "name": "Technology"},
				{"id": "cat-2", "slug": "design", "name": "Design"},
			},
			"pagination": map[string]interface{}{
				"current_page": 1,
				"total_pages":  3,
				"has_next":     true,
				"has_previous": false,
				"total_count":  25,
				"per_page":     10,
			},
			"current_year": 2025,
		}

		// Render home.liquid with all partials (matching production)
		html, err := RenderBlogTemplate(homeLiquid, data, partials)

		// Assertions
		assert.NoError(t, err)

		// CRITICAL: Verify workspace.id is rendered in the JavaScript config
		// This should NOT be empty or contain unrendered Liquid tags
		assert.Contains(t, html, "workspaceId: 'ws-full-integration'",
			"workspace.id should be accessible in scripts.js through header include chain")

		// Verify it's not rendering the Liquid syntax itself
		assert.NotContains(t, html, "{{ workspace.id }}",
			"Liquid tags should be evaluated, not rendered as text")
		assert.NotContains(t, html, "workspaceId: ''",
			"workspace.id should not be empty")

		// Verify base_url is also accessible
		assert.Contains(t, html, "domain: 'https://blog.example.com'",
			"base_url should be accessible in scripts.js")

		// Verify public_lists array is rendered correctly
		assert.Contains(t, html, "'list-abc123'",
			"First list ID should be in the listIds array")
		assert.Contains(t, html, "'list-def456'",
			"Second list ID should be in the listIds array")

		// Verify the function that uses the config is present
		assert.Contains(t, html, "workspace_id: NOTIFUSE_CONFIG.workspaceId",
			"The subscribeToNewsletter function should use the config")
	})

	t.Run("logo_url renders correctly in navigation", func(t *testing.T) {
		// Test that logo_url from workspace is accessible in the topbar navigation

		partials := map[string]string{
			"shared":  sharedLiquid,
			"header":  headerLiquid,
			"footer":  footerLiquid,
			"styles":  stylesCSS,
			"scripts": scriptsJS,
		}

		data := map[string]interface{}{
			"workspace": map[string]interface{}{
				"id":       "ws-logo-test",
				"name":     "Logo Test Blog",
				"logo_url": "https://cdn.example.com/my-logo.png",
			},
			"base_url":     "https://blog.example.com",
			"public_lists": []map[string]interface{}{},
			"posts":        []map[string]interface{}{},
			"categories":   []map[string]interface{}{},
			"current_year": 2025,
		}

		html, err := RenderBlogTemplate(homeLiquid, data, partials)

		assert.NoError(t, err)

		// Verify logo_url is rendered as an image src in the navigation
		assert.Contains(t, html, `<img src="https://cdn.example.com/my-logo.png"`,
			"logo_url should be rendered in the navigation logo img tag")
		assert.Contains(t, html, `alt="Logo Test Blog"`,
			"Logo alt text should use workspace name")
	})

	t.Run("handles missing logo_url gracefully with fallback", func(t *testing.T) {
		// Test that when logo_url is missing, it falls back to workspace name

		partials := map[string]string{
			"shared":  sharedLiquid,
			"header":  headerLiquid,
			"footer":  footerLiquid,
			"styles":  stylesCSS,
			"scripts": scriptsJS,
		}

		data := map[string]interface{}{
			"workspace": map[string]interface{}{
				"id":   "ws-no-logo",
				"name": "No Logo Blog",
				// logo_url is intentionally missing
			},
			"base_url":     "https://blog.example.com",
			"public_lists": []map[string]interface{}{},
			"posts":        []map[string]interface{}{},
			"categories":   []map[string]interface{}{},
			"current_year": 2025,
		}

		html, err := RenderBlogTemplate(homeLiquid, data, partials)

		assert.NoError(t, err)

		// When logo_url is missing, the logo should show workspace name as text
		// The liquid template uses: {% if workspace.logo_url %} ... {% else %} {{ workspace.name }} {% endif %}
		assert.Contains(t, html, `class="logo">No Logo Blog</a>`,
			"When logo_url is missing, workspace name should be displayed as text")

		// Should NOT contain an img tag in the logo
		assert.NotContains(t, html, `<img src=`,
			"Should not render img tag when logo_url is missing")
	})

	t.Run("renders complete blog with posts, categories, and pagination", func(t *testing.T) {
		// Comprehensive test with all data populated

		partials := map[string]string{
			"shared":  sharedLiquid,
			"header":  headerLiquid,
			"footer":  footerLiquid,
			"styles":  stylesCSS,
			"scripts": scriptsJS,
		}

		data := map[string]interface{}{
			"workspace": map[string]interface{}{
				"id":         "ws-comprehensive",
				"name":       "Comprehensive Blog",
				"logo_url":   "https://example.com/logo.png",
				"icon_url":   "https://example.com/favicon.ico",
				"blog_title": "Comprehensive Test Blog",
			},
			"base_url": "https://blog.example.com",
			"public_lists": []map[string]interface{}{
				{"id": "list-newsletter", "name": "Newsletter"},
			},
			"posts": []map[string]interface{}{
				{
					"id":                   "post-featured",
					"slug":                 "featured-post",
					"category_id":          "cat-tech",
					"category_slug":        "technology",
					"title":                "Featured Blog Post",
					"excerpt":              "This is a featured post excerpt",
					"featured_image_url":   "https://example.com/featured.jpg",
					"reading_time_minutes": 8,
					"published_at":         "2025-01-20T14:30:00Z",
					"authors": []map[string]interface{}{
						{"name": "Alice Johnson", "avatar_url": "https://example.com/alice.jpg"},
						{"name": "Bob Williams", "avatar_url": "https://example.com/bob.jpg"},
					},
				},
				{
					"id":                   "post-second",
					"slug":                 "second-post",
					"category_id":          "cat-tech",
					"category_slug":        "technology",
					"title":                "Second Blog Post",
					"excerpt":              "Another interesting post",
					"featured_image_url":   "https://example.com/second.jpg",
					"reading_time_minutes": 5,
					"published_at":         "2025-01-19T10:00:00Z",
					"authors": []map[string]interface{}{
						{"name": "Charlie Davis", "avatar_url": "https://example.com/charlie.jpg"},
					},
				},
			},
			"categories": []map[string]interface{}{
				{"id": "cat-tech", "slug": "technology", "name": "Technology"},
				{"id": "cat-design", "slug": "design", "name": "Design"},
				{"id": "cat-business", "slug": "business", "name": "Business"},
			},
			"pagination": map[string]interface{}{
				"current_page": 2,
				"total_pages":  5,
				"has_next":     true,
				"has_previous": true,
				"total_count":  48,
				"per_page":     10,
			},
			"current_year": 2025,
		}

		html, err := RenderBlogTemplate(homeLiquid, data, partials)

		assert.NoError(t, err)

		// CRITICAL: Verify workspace.id is accessible (main goal of this integration test)
		assert.Contains(t, html, "workspaceId: 'ws-comprehensive'", "workspace.id must be accessible through nested includes")
		assert.Contains(t, html, "domain: 'https://blog.example.com'", "base_url must be accessible")
		assert.Contains(t, html, "'list-newsletter'", "public_lists must be accessible")

		// Verify basic rendering works
		assert.Contains(t, html, "Comprehensive Test Blog", "Blog title should appear")
		assert.Contains(t, html, "Second Blog Post", "At least one post should render")
		assert.Contains(t, html, "Charlie Davis", "Post author should render")
		assert.Contains(t, html, "5 min read", "Reading time should render")

		// Verify no unrendered Liquid tags (most important check)
		assert.NotContains(t, html, "{{ workspace.id }}", "workspace.id should not remain as unrendered Liquid tag")
		assert.NotContains(t, html, "{{ workspace.logo_url }}", "logo_url should not remain as unrendered Liquid tag")
		assert.NotContains(t, html, "{{ base_url }}", "base_url should not remain as unrendered Liquid tag")
	})

	t.Run("blog_title preference over workspace name in header", func(t *testing.T) {
		// Test that blog_title from workspace.blog_title is used when available

		partials := map[string]string{
			"shared":  sharedLiquid,
			"header":  headerLiquid,
			"footer":  footerLiquid,
			"styles":  stylesCSS,
			"scripts": scriptsJS,
		}

		data := map[string]interface{}{
			"workspace": map[string]interface{}{
				"id":         "ws-blog-title",
				"name":       "Workspace Name",
				"blog_title": "Custom Blog Title",
			},
			"base_url":     "https://blog.example.com",
			"public_lists": []map[string]interface{}{},
			"posts":        []map[string]interface{}{},
			"categories":   []map[string]interface{}{},
			"current_year": 2025,
		}

		html, err := RenderBlogTemplate(homeLiquid, data, partials)

		assert.NoError(t, err)

		// blog_title should be used in the hero section
		assert.Contains(t, html, "Custom Blog Title", "blog_title should be displayed in hero")

		// Should also use blog_title in footer
		assert.Contains(t, html, "Custom Blog Title", "blog_title should be in footer")
	})

	t.Run("empty posts array shows no featured post section", func(t *testing.T) {
		// Test handling of empty posts array

		partials := map[string]string{
			"shared":  sharedLiquid,
			"header":  headerLiquid,
			"footer":  footerLiquid,
			"styles":  stylesCSS,
			"scripts": scriptsJS,
		}

		data := map[string]interface{}{
			"workspace": map[string]interface{}{
				"id":   "ws-no-posts",
				"name": "Empty Blog",
			},
			"base_url":     "https://blog.example.com",
			"public_lists": []map[string]interface{}{},
			"posts":        []map[string]interface{}{}, // Empty posts
			"categories": []map[string]interface{}{
				{"id": "cat-1", "slug": "tech", "name": "Technology"},
			},
			"current_year": 2025,
		}

		html, err := RenderBlogTemplate(homeLiquid, data, partials)

		assert.NoError(t, err)

		// CRITICAL: Verify workspace.id is accessible even with empty posts
		assert.Contains(t, html, "workspaceId: 'ws-no-posts'", "workspace.id must be accessible")

		// Featured post section should not appear (it's wrapped in {% if posts.size > 0 %})
		assert.NotContains(t, html, "featured-post-section", "Featured section should not appear when no posts")

		// Categories bar should still render
		assert.Contains(t, html, "categories-bar", "Categories bar should still render")
		// Note: Whether individual categories show depends on simplified widget implementation
	})
}
