package liquid

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlogRendererGo_Basic(t *testing.T) {
	t.Run("renders simple template", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := "<h1>{{ title }}</h1>"
		data := map[string]interface{}{
			"title": "Hello World",
		}

		html, err := renderer.Render(template, data, nil)
		require.NoError(t, err)
		assert.Equal(t, "<h1>Hello World</h1>", html)
	})

	t.Run("renders template with loops", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `<ul>{% for item in items %}<li>{{ item }}</li>{% endfor %}</ul>`
		data := map[string]interface{}{
			"items": []string{"one", "two", "three"},
		}

		html, err := renderer.Render(template, data, nil)
		require.NoError(t, err)
		assert.Equal(t, "<ul><li>one</li><li>two</li><li>three</li></ul>", html)
	})

	t.Run("renders template with conditionals", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `{% if show %}<p>Visible</p>{% endif %}`
		data := map[string]interface{}{
			"show": true,
		}

		html, err := renderer.Render(template, data, nil)
		require.NoError(t, err)
		assert.Equal(t, "<p>Visible</p>", html)
	})

	t.Run("renders workspace data", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `<h1>{{ workspace.name }}</h1><p>ID: {{ workspace.id }}</p>`
		data := map[string]interface{}{
			"workspace": map[string]interface{}{
				"id":   "ws-123",
				"name": "My Workspace",
			},
		}

		html, err := renderer.Render(template, data, nil)
		require.NoError(t, err)
		assert.Contains(t, html, "My Workspace")
		assert.Contains(t, html, "ws-123")
	})

	t.Run("returns error for unclosed tags", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `{% for item in items %}<li>{{ item }}</li>` // Missing endfor
		data := map[string]interface{}{
			"items": []string{"one"},
		}

		// Updated liquidgo properly detects unclosed tags
		_, err := renderer.Render(template, data, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "liquid rendering failed")
	})

	t.Run("returns error for empty template", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := ""
		data := map[string]interface{}{}

		_, err := renderer.Render(template, data, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "template content is empty")
	})

	t.Run("handles missing data gracefully", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `<h1>{{ missing_field }}</h1>`
		data := map[string]interface{}{}

		html, err := renderer.Render(template, data, nil)
		require.NoError(t, err)
		assert.Equal(t, "<h1></h1>", html)
	})
}

func TestBlogRendererGo_WithPartials(t *testing.T) {
	t.Run("renders template with render tag and no parameters", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `<div>{% render 'header' %}</div>`
		partials := map[string]string{
			"header": `<header>Site Header</header>`,
		}
		data := map[string]interface{}{}

		html, err := renderer.Render(template, data, partials)
		require.NoError(t, err)
		assert.Contains(t, html, "Site Header")
	})

	t.Run("renders template with render tag and parameters", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `<div>{% render 'shared', widget: 'newsletter' %}</div>`
		partials := map[string]string{
			"shared": `{%- if widget == 'newsletter' -%}<div class="newsletter">Subscribe now!</div>{%- endif -%}`,
		}
		data := map[string]interface{}{}

		html, err := renderer.Render(template, data, partials)
		require.NoError(t, err)
		assert.Contains(t, html, "Subscribe now!")
	})

	t.Run("renders template with multiple parameters", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `<div>{% render 'product-card', title: "Widget", price: 9.99 %}</div>`
		partials := map[string]string{
			"product-card": "<div>{{ title }} - ${{ price }}</div>",
		}
		data := map[string]interface{}{}

		html, err := renderer.Render(template, data, partials)
		require.NoError(t, err)
		assert.Contains(t, html, "Widget - $9.99")
	})

	t.Run("renders with isolated scope - parent variables not accessible", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `{% assign global_var = 'parent' %}{% render 'partial' %}`
		partials := map[string]string{
			"partial": `{{ global_var }}`, // Should be empty - isolated scope
		}
		data := map[string]interface{}{}

		html, err := renderer.Render(template, data, partials)
		require.NoError(t, err)
		// global_var should NOT be accessible in render (isolated scope)
		assert.NotContains(t, html, "parent")
	})

	t.Run("renders with isolated scope - only parameters accessible", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `{% assign global_var = 'parent' %}{% render 'partial', passed_var: 'child' %}`
		partials := map[string]string{
			"partial": `{{ passed_var }}`, // Should work - passed as parameter
		}
		data := map[string]interface{}{}

		html, err := renderer.Render(template, data, partials)
		require.NoError(t, err)
		assert.Contains(t, html, "child")
	})

	t.Run("renders with multiple partial calls", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `<div>{% render 'header' %}<main>Content</main>{% render 'footer' %}</div>`
		partials := map[string]string{
			"header": "<header>Header</header>",
			"footer": "<footer>Footer</footer>",
		}
		data := map[string]interface{}{}

		html, err := renderer.Render(template, data, partials)
		require.NoError(t, err)
		assert.Contains(t, html, "Header")
		assert.Contains(t, html, "Footer")
		assert.Contains(t, html, "Content")
	})

	t.Run("handles missing partial with error message", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `<div>{% render 'nonexistent' %}</div>`
		partials := map[string]string{
			"header": `<header>Content</header>`,
		}
		data := map[string]interface{}{}

		// liquidgo shows inline error for missing partials (lax mode default)
		html, err := renderer.Render(template, data, partials)
		require.NoError(t, err)
		assert.Contains(t, html, "Liquid error")
	})
}

func TestBlogRendererGo_WithParameterizedRenders(t *testing.T) {
	t.Run("render with single parameter", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `<div>{% render 'shared', widget: 'newsletter' %}</div>`
		partials := map[string]string{
			"shared": `{%- if widget == 'newsletter' -%}<div class="newsletter">Subscribe!</div>{%- endif -%}`,
		}
		data := map[string]interface{}{}

		html, err := renderer.Render(template, data, partials)
		require.NoError(t, err)
		assert.Contains(t, html, "Subscribe!")
	})

	t.Run("render with multiple parameters", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `<div>{% for post in posts %}{% render 'shared', widget: 'post-card', post: post %}{% endfor %}</div>`
		partials := map[string]string{
			"shared": `{%- if widget == 'post-card' -%}<article><h3>{{ post.title }}</h3></article>{%- endif -%}`,
		}
		data := map[string]interface{}{
			"posts": []interface{}{
				map[string]interface{}{"title": "First Post", "slug": "first-post"},
				map[string]interface{}{"title": "Second Post", "slug": "second-post"},
			},
		}

		html, err := renderer.Render(template, data, partials)
		require.NoError(t, err)
		assert.Contains(t, html, "First Post")
		assert.Contains(t, html, "Second Post")
	})

	t.Run("render parameters are scoped to partial", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `<div>{% assign widget = 'global' %}{% render 'shared', widget: 'newsletter' %}{{ widget }}</div>`
		partials := map[string]string{
			"shared": `{%- if widget == 'newsletter' -%}<span>Newsletter</span>{%- endif -%}`,
		}
		data := map[string]interface{}{}

		html, err := renderer.Render(template, data, partials)
		require.NoError(t, err)
		assert.Contains(t, html, "Newsletter")
		assert.Contains(t, html, "global") // Original widget value should remain
	})

	t.Run("render parameter isolation between renders", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `<div>{% render 'shared', widget: 'newsletter' %}{% render 'shared', widget: 'categories' %}</div>`
		partials := map[string]string{
			"shared": `{%- if widget == 'newsletter' -%}<span>Newsletter</span>{%- endif -%}{%- if widget == 'categories' -%}<span>Categories</span>{%- endif -%}`,
		}
		data := map[string]interface{}{}

		html, err := renderer.Render(template, data, partials)
		require.NoError(t, err)
		assert.Contains(t, html, "Newsletter")
		assert.Contains(t, html, "Categories")
	})

	t.Run("nested renders with parameters", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `<div>{% render 'outer', title: 'Main' %}</div>`
		partials := map[string]string{
			"outer": `<section><h1>{{ title }}</h1>{% render 'inner', subtitle: 'Sub' %}</section>`,
			"inner": `<p>{{ subtitle }}</p>`,
		}
		data := map[string]interface{}{}

		html, err := renderer.Render(template, data, partials)
		require.NoError(t, err)
		assert.Contains(t, html, "Main")
		assert.Contains(t, html, "Sub")
	})

	t.Run("render with complex object parameter", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `<div>{% for post in posts %}{% render 'post-card', post: post %}{% endfor %}</div>`
		partials := map[string]string{
			"post-card": `<article><h3>{{ post.title }}</h3><p>{{ post.excerpt }}</p><span>{{ post.reading_time }} min</span></article>`,
		}
		data := map[string]interface{}{
			"posts": []interface{}{
				map[string]interface{}{"title": "Post One", "excerpt": "Excerpt one", "reading_time": 5},
				map[string]interface{}{"title": "Post Two", "excerpt": "Excerpt two", "reading_time": 10},
			},
		}

		html, err := renderer.Render(template, data, partials)
		require.NoError(t, err)
		assert.Contains(t, html, "Post One")
		assert.Contains(t, html, "Excerpt one")
		assert.Contains(t, html, "5 min")
		assert.Contains(t, html, "Post Two")
		assert.Contains(t, html, "Excerpt two")
		assert.Contains(t, html, "10 min")
	})
}

func TestBlogRendererGo_RealisticBlogData(t *testing.T) {
	t.Run("renders home page with posts and categories", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `
			<h1>{{ workspace.name }}</h1>
			{% render 'shared', widget: 'categories', categories: categories %}
			{% for post in posts %}
				{% render 'shared', widget: 'post-card', post: post, base_url: base_url %}
			{% endfor %}
			{% render 'shared', widget: 'pagination', pagination: pagination %}
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
			"categories": []interface{}{
				map[string]interface{}{"slug": "tech", "name": "Technology"},
				map[string]interface{}{"slug": "design", "name": "Design"},
			},
			"posts": []interface{}{
				map[string]interface{}{
					"title":         "First Post",
					"slug":          "first-post",
					"category_slug": "tech",
				},
				map[string]interface{}{
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

		html, err := renderer.Render(template, data, partials)
		require.NoError(t, err)
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
		renderer := NewBlogTemplateRenderer()
		template := `
			<h1>{{ workspace.name }}</h1>
			{% if posts.size > 0 %}
				{% for post in posts %}
					{% render 'shared', widget: 'post-card', post: post %}
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
			"posts": []interface{}{},
		}

		html, err := renderer.Render(template, data, partials)
		require.NoError(t, err)
		assert.Contains(t, html, "My Blog")
		assert.Contains(t, html, "No posts found")
	})
}

func TestBlogRendererGo_SecurityLimits(t *testing.T) {
	t.Run("enforces template size limit", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		// Create a template larger than 100KB
		largeTemplate := strings.Repeat("<div>{{ item }}</div>\n", 10000) // ~200KB
		data := map[string]interface{}{
			"item": "test",
		}

		_, err := renderer.Render(largeTemplate, data, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum allowed size")
	})

	t.Run("allows normal sized templates", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		// Template under 100KB should work fine
		template := strings.Repeat("<div>{{ item }}</div>\n", 100) // ~2KB
		data := map[string]interface{}{
			"item": "test",
		}

		html, err := renderer.Render(template, data, nil)
		require.NoError(t, err)
		assert.Contains(t, html, "<div>test</div>")
	})

	t.Run("enforces partial size limit", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `<div>{% render 'huge' %}</div>`
		partials := map[string]string{
			"huge": strings.Repeat("<div>content</div>\n", 10000), // ~200KB
		}
		data := map[string]interface{}{}

		_, err := renderer.Render(template, data, partials)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum allowed size")
	})
}

func TestBlogRendererGo_ForLoopFeatures(t *testing.T) {
	t.Run("for loop with offset", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `{% for post in posts offset: 1 %}<div>{{ post.title }}</div>{% endfor %}`
		data := map[string]interface{}{
			"posts": []interface{}{
				map[string]interface{}{"title": "First"},
				map[string]interface{}{"title": "Second"},
				map[string]interface{}{"title": "Third"},
			},
		}

		html, err := renderer.Render(template, data, nil)
		require.NoError(t, err)
		assert.NotContains(t, html, "First") // Skipped by offset
		assert.Contains(t, html, "Second")
		assert.Contains(t, html, "Third")
	})

	t.Run("for loop with limit", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `{% for author in authors limit: 2 %}<span>{{ author.name }}</span>{% endfor %}`
		data := map[string]interface{}{
			"authors": []interface{}{
				map[string]interface{}{"name": "John"},
				map[string]interface{}{"name": "Jane"},
				map[string]interface{}{"name": "Bob"},
			},
		}

		html, err := renderer.Render(template, data, nil)
		require.NoError(t, err)
		assert.Contains(t, html, "John")
		assert.Contains(t, html, "Jane")
		assert.NotContains(t, html, "Bob") // Limited to 2
	})

	t.Run("forloop.last variable", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `{% for author in authors %}{{ author.name }}{% unless forloop.last %}, {% endunless %}{% endfor %}`
		data := map[string]interface{}{
			"authors": []interface{}{
				map[string]interface{}{"name": "John"},
				map[string]interface{}{"name": "Jane"},
			},
		}

		html, err := renderer.Render(template, data, nil)
		require.NoError(t, err)
		assert.Equal(t, "John, Jane", html)
	})

	t.Run("forloop.index variable", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `{% for item in items %}<div>{{ forloop.index }}: {{ item }}</div>{% endfor %}`
		data := map[string]interface{}{
			"items": []string{"A", "B", "C"},
		}

		html, err := renderer.Render(template, data, nil)
		require.NoError(t, err)
		assert.Contains(t, html, "1: A")
		assert.Contains(t, html, "2: B")
		assert.Contains(t, html, "3: C")
	})
}

func TestBlogRendererGo_DateFilter(t *testing.T) {
	t.Run("date filter with *time.Time pointer", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `{{ published_at | date: "%b %d, %Y" }}`
		now := time.Date(2025, 11, 18, 10, 30, 0, 0, time.UTC)
		data := map[string]interface{}{
			"published_at": &now, // Use *time.Time instead of string
		}

		html, err := renderer.Render(template, data, nil)
		require.NoError(t, err)
		assert.Contains(t, html, "Nov")
		assert.Contains(t, html, "18")
		assert.Contains(t, html, "2025")
	})

	t.Run("date filter with nil *time.Time", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `{{ published_at | date: "%b %d, %Y" }}`
		data := map[string]interface{}{
			"published_at": (*time.Time)(nil),
		}

		html, err := renderer.Render(template, data, nil)
		require.NoError(t, err)
		// When ToDate returns nil, Date filter returns original input (nil pointer)
		// which gets stringified. This is expected behavior - the filter handles nil gracefully
		// by returning the original value rather than crashing
		assert.Contains(t, html, "<nil>")
	})
}

func TestBlogRendererGo_WhitespaceControl(t *testing.T) {
	t.Run("whitespace control with dash", func(t *testing.T) {
		renderer := NewBlogTemplateRenderer()
		template := `{%- if true -%}
		Content
		{%- endif -%}`
		data := map[string]interface{}{}

		html, err := renderer.Render(template, data, nil)
		require.NoError(t, err)
		// Should strip surrounding whitespace
		assert.Contains(t, html, "Content")
		assert.NotContains(t, html, "\n\t\tContent\n\t\t")
	})
}

// TestRenderBlogTemplateGo tests the convenience function
func TestRenderBlogTemplateGo(t *testing.T) {
	t.Run("convenience function works", func(t *testing.T) {
		template := "<h1>{{ title }}</h1>"
		data := map[string]interface{}{
			"title": "Test",
		}

		html, err := RenderBlogTemplateGo(template, data, nil)
		require.NoError(t, err)
		assert.Equal(t, "<h1>Test</h1>", html)
	})

	t.Run("convenience function with partials", func(t *testing.T) {
		template := `<div>{% render 'header' %}</div>`
		partials := map[string]string{
			"header": "<header>Site</header>",
		}
		data := map[string]interface{}{}

		html, err := RenderBlogTemplateGo(template, data, partials)
		require.NoError(t, err)
		assert.Contains(t, html, "Site")
	})
}
