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
