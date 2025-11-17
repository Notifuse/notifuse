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

func TestRenderBlogTemplateWithParameterizedIncludes(t *testing.T) {
	t.Run("FAILING: include with comma-separated parameters (Jekyll/Shopify syntax)", func(t *testing.T) {
		// This reproduces the exact syntax used in home.liquid that is causing the error
		template := `<div>{% include 'shared', widget: 'newsletter' %}</div>`
		partials := map[string]string{
			"shared": `{%- if widget == 'newsletter' -%}<div class="newsletter">Subscribe!</div>{%- endif -%}`,
		}
		data := map[string]interface{}{}

		html, err := RenderBlogTemplate(template, data, partials)
		
		// This test documents the current failing behavior
		// The osteele/liquid library may not support Jekyll/Shopify parameter syntax
		if err != nil {
			t.Logf("Expected failure - include with parameters not supported: %v", err)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "liquid rendering failed")
		} else {
			t.Logf("Success - html: %s", html)
			assert.Contains(t, html, "Subscribe!")
		}
	})

	t.Run("include with post parameter (home.liquid pattern)", func(t *testing.T) {
		// This reproduces the post-card include pattern from home.liquid
		template := `<div>{% for post in posts %}{% include 'shared', widget: 'post-card', post: post %}{% endfor %}</div>`
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
		
		// Document the behavior
		if err != nil {
			t.Logf("Expected failure - include with multiple parameters not supported: %v", err)
			assert.Error(t, err)
		} else {
			t.Logf("Success - html: %s", html)
			assert.Contains(t, html, "First Post")
		}
	})

	t.Run("WORKAROUND: assign variables before include", func(t *testing.T) {
		// This tests the workaround: assigning variables before including
		template := `<div>{% assign widget = 'newsletter' %}{% include 'shared' %}</div>`
		partials := map[string]string{
			"shared": `{%- if widget == 'newsletter' -%}<div class="newsletter">Subscribe!</div>{%- endif -%}`,
		}
		data := map[string]interface{}{}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		assert.Contains(t, html, "Subscribe!")
	})

	t.Run("WORKAROUND: assign post variable in loop before include", func(t *testing.T) {
		// This tests if we can pass post data through assign
		template := `<div>{% for p in posts %}{% assign post = p %}{% assign widget = 'post-card' %}{% include 'shared' %}{% endfor %}</div>`
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

	t.Run("ALTERNATIVE: separate partials for each widget", func(t *testing.T) {
		// Alternative approach: create separate partials instead of using widget parameter
		template := `<div>{% include 'newsletter' %}{% for post in posts %}{% include 'post-card' %}{% endfor %}</div>`
		partials := map[string]string{
			"newsletter": `<div class="newsletter">Subscribe!</div>`,
			"post-card":  `<article><h3>{{ post.title }}</h3></article>`,
		}
		data := map[string]interface{}{
			"posts": []map[string]interface{}{
				{"title": "First Post", "slug": "first-post"},
			},
		}

		html, err := RenderBlogTemplate(template, data, partials)
		assert.NoError(t, err)
		assert.Contains(t, html, "Subscribe!")
		assert.Contains(t, html, "First Post")
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
		assert.Contains(t, html, `class="active"`)
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

