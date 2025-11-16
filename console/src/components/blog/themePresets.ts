import { BlogThemeFiles } from '../../services/api/blog'
import { minimalBlogPreset } from '../blog_editor/presets/minimalBlog'
import { modernMagazinePreset } from '../blog_editor/presets/modernMagazine'
import { timesJournalPreset } from '../blog_editor/presets/timesJournal'

export interface ThemePreset {
  id: string
  name: string
  description: string
  placeholderColor: string
  files: BlogThemeFiles
  styling: any
}

// Blank Theme - Basic structure with header/footer imports
const blankTheme: ThemePreset = {
  id: 'blank',
  name: 'Blank',
  description: 'Start from scratch with a clean slate and basic structure',
  placeholderColor: '#f5f5f5',
  files: {
    home: `<div class="blog-home">
  <div class="hero">
    <h1>{{ blog.title }}</h1>
    <p class="subtitle">{{ blog.description }}</p>
  </div>

  <div class="posts-container">
    {% for post in posts %}
      <article class="post-item">
        <h2><a href="/{{ post.category_slug }}/{{ post.slug }}">{{ post.title }}</a></h2>
        <p>{{ post.excerpt }}</p>
        <div class="post-meta">
          <span>{{ post.published_at }}</span>
        </div>
      </article>
    {% endfor %}
  </div>
</div>`,

    category: `<div class="category-page">
  <header class="category-header">
    <h1>{{ category.name }}</h1>
    {% if category.description %}
      <p>{{ category.description }}</p>
    {% endif %}
  </header>

  <div class="posts-container">
    {% for post in posts %}
      <article class="post-item">
        <h2><a href="/{{ category.slug }}/{{ post.slug }}">{{ post.title }}</a></h2>
        <p>{{ post.excerpt }}</p>
        <div class="post-meta">
          <span>{{ post.published_at }}</span>
        </div>
      </article>
    {% endfor %}
  </div>
</div>`,

    post: `<article class="blog-post">
  <header class="post-header">
    <h1>{{ post.title }}</h1>
    <div class="post-meta">
      {% for author in post.authors %}
        <span>{{ author.name }}</span>
      {% endfor %}
      <span>{{ post.published_at }}</span>
    </div>
  </header>

  <div class="post-content">
    {{ post.content }}
  </div>

  <footer class="post-footer">
    {% if previous_post %}
      <a href="/{{ post.category_slug }}/{{ previous_post.slug }}">← Previous</a>
    {% endif %}
    {% if next_post %}
      <a href="/{{ post.category_slug }}/{{ next_post.slug }}">Next →</a>
    {% endif %}
  </footer>
</article>`,

    header: `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  
  <title>{% if page_title %}{{ page_title }}{% elsif seo.meta_title %}{{ seo.meta_title }}{% else %}{{ blog.title }}{% endif %}</title>
  
  {% if page_description %}
    <meta name="description" content="{{ page_description }}">
  {% elsif seo.meta_description %}
    <meta name="description" content="{{ seo.meta_description }}">
  {% endif %}
  
  {% if seo.keywords and seo.keywords.size > 0 %}
    <meta name="keywords" content="{{ seo.keywords | join: ', ' }}">
  {% endif %}

  <style>
    * {
      margin: 0;
      padding: 0;
      box-sizing: border-box;
    }

    body {
      font-family: system-ui, -apple-system, sans-serif;
      font-size: 16px;
      line-height: 1.6;
      color: #1a1a1a;
      background: #ffffff;
    }

    .container {
      max-width: 1200px;
      margin: 0 auto;
      padding: 0 20px;
    }

    .site-header {
      border-bottom: 1px solid #e5e7eb;
      padding: 20px 0;
    }

    .site-title a {
      font-size: 24px;
      font-weight: 700;
      text-decoration: none;
      color: #000;
    }

    a {
      color: #2563eb;
      text-decoration: none;
    }

    a:hover {
      text-decoration: underline;
    }
  </style>
</head>
<body>
  <header class="site-header">
    <div class="container">
      <div class="site-title">
        <a href="/">{{ blog.title }}</a>
      </div>
    </div>
  </header>
  <main class="site-main">`,

    footer: `  </main>
  <footer class="site-footer">
    <div class="container">
      <p>&copy; {{ current_year }} {{ blog.title }}. All rights reserved.</p>
    </div>
  </footer>
</body>
</html>`,

    shared: `<!-- Shared components and utilities -->
{% comment %}
  This file can contain reusable Liquid snippets and components
  that can be included across different templates.
{% endcomment %}`
  },
  styling: {
    default: {
      fontFamily: 'system-ui, -apple-system, sans-serif',
      fontSize: { value: 16, unit: 'px' },
      color: '#1a1a1a',
      backgroundColor: '#ffffff',
      lineHeight: 1.6
    },
    paragraph: {
      marginTop: { value: 0, unit: 'px' },
      marginBottom: { value: 16, unit: 'px' },
      lineHeight: 1.6
    },
    headings: {
      fontFamily: 'inherit'
    },
    h1: {
      fontSize: { value: 2.5, unit: 'rem' },
      color: '#000000',
      marginTop: { value: 48, unit: 'px' },
      marginBottom: { value: 24, unit: 'px' }
    },
    h2: {
      fontSize: { value: 2, unit: 'rem' },
      color: '#1a1a1a',
      marginTop: { value: 40, unit: 'px' },
      marginBottom: { value: 20, unit: 'px' }
    },
    h3: {
      fontSize: { value: 1.5, unit: 'rem' },
      color: '#1a1a1a',
      marginTop: { value: 32, unit: 'px' },
      marginBottom: { value: 16, unit: 'px' }
    },
    caption: {
      fontSize: { value: 14, unit: 'px' },
      color: '#6b7280'
    },
    separator: {
      color: '#e5e7eb',
      marginTop: { value: 32, unit: 'px' },
      marginBottom: { value: 32, unit: 'px' }
    },
    codeBlock: {
      marginTop: { value: 16, unit: 'px' },
      marginBottom: { value: 16, unit: 'px' }
    },
    blockquote: {
      fontSize: { value: 18, unit: 'px' },
      color: '#4b5563',
      marginTop: { value: 24, unit: 'px' },
      marginBottom: { value: 24, unit: 'px' },
      lineHeight: 1.6
    },
    inlineCode: {
      fontFamily: 'monospace',
      fontSize: { value: 14, unit: 'px' },
      color: '#e11d48',
      backgroundColor: '#f3f4f6'
    },
    list: {
      marginTop: { value: 16, unit: 'px' },
      marginBottom: { value: 16, unit: 'px' },
      paddingLeft: { value: 24, unit: 'px' }
    },
    link: {
      color: '#2563eb',
      hoverColor: '#1d4ed8'
    }
  }
}

// Minimal Blog Theme - Clean, Medium-inspired
const minimalBlogTheme: ThemePreset = {
  id: 'minimal',
  name: 'Minimal Blog',
  description: 'Clean, distraction-free design inspired by Medium',
  placeholderColor: '#e0f2fe',
  files: {
    home: `<div class="blog-home">
  <div class="hero">
    <h1>{{ blog.title }}</h1>
    {% if blog.description %}
      <p class="subtitle">{{ blog.description }}</p>
    {% endif %}
  </div>

  <div class="posts-grid">
    {% for post in posts %}
      <article class="post-card">
        {% if post.featured_image_url %}
          <img src="{{ post.featured_image_url }}" alt="{{ post.title }}" class="featured-image">
        {% endif %}
        <div class="post-content">
          <h2><a href="/{{ post.category_slug }}/{{ post.slug }}">{{ post.title }}</a></h2>
          <p class="excerpt">{{ post.excerpt }}</p>
          <div class="meta">
            <span class="date">{{ post.published_at }}</span>
            <span class="reading-time">{{ post.reading_time_minutes }} min read</span>
          </div>
        </div>
      </article>
    {% endfor %}
  </div>
</div>`,

    category: `<div class="category-page">
  <header class="category-header">
    <h1>{{ category.name }}</h1>
    {% if category.description %}
      <p class="description">{{ category.description }}</p>
    {% endif %}
  </header>

  <div class="posts-list">
    {% for post in posts %}
      <article class="post-item">
        <h2><a href="/{{ category.slug }}/{{ post.slug }}">{{ post.title }}</a></h2>
        <p class="excerpt">{{ post.excerpt }}</p>
        <div class="meta">
          <span class="date">{{ post.published_at }}</span>
          <span class="reading-time">{{ post.reading_time_minutes }} min read</span>
        </div>
      </article>
    {% endfor %}
  </div>
</div>`,

    post: `<article class="blog-post">
  <header class="post-header">
    <h1>{{ post.title }}</h1>
    <div class="post-meta">
      <div class="authors">
        {% for author in post.authors %}
          <span class="author">{{ author.name }}</span>
        {% endfor %}
      </div>
      <span class="date">{{ post.published_at }}</span>
      <span class="reading-time">{{ post.reading_time_minutes }} min read</span>
    </div>
  </header>

  {% if post.featured_image_url %}
    <img src="{{ post.featured_image_url }}" alt="{{ post.title }}" class="featured-image">
  {% endif %}

  <div class="post-content">
    {{ post.content }}
  </div>

  <footer class="post-footer">
    <div class="post-navigation">
      {% if previous_post %}
        <a href="/{{ post.category_slug }}/{{ previous_post.slug }}" class="prev-post">
          ← {{ previous_post.title }}
        </a>
      {% endif %}
      {% if next_post %}
        <a href="/{{ post.category_slug }}/{{ next_post.slug }}" class="next-post">
          {{ next_post.title }} →
        </a>
      {% endif %}
    </div>
  </footer>
</article>`,

    header: `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  
  <title>{% if page_title %}{{ page_title }}{% elsif seo.meta_title %}{{ seo.meta_title }}{% else %}{{ blog.title }}{% endif %}</title>
  
  {% if page_description %}
    <meta name="description" content="{{ page_description }}">
  {% elsif seo.meta_description %}
    <meta name="description" content="{{ seo.meta_description }}">
  {% endif %}

  <style>
    * {
      margin: 0;
      padding: 0;
      box-sizing: border-box;
    }

    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      font-size: 18px;
      line-height: 1.6;
      color: #1f2937;
      background: #ffffff;
    }

    .site-header {
      border-bottom: 1px solid #e5e7eb;
      padding: 24px 0;
    }

    .container {
      max-width: 680px;
      margin: 0 auto;
      padding: 0 24px;
    }

    .site-title a {
      font-size: 20px;
      font-weight: 600;
      text-decoration: none;
      color: #111827;
    }

    .blog-home, .category-page, .blog-post {
      padding: 48px 24px;
      max-width: 680px;
      margin: 0 auto;
    }

    h1 {
      font-size: 2.25rem;
      color: #111827;
      margin-bottom: 0.5rem;
    }

    h2 {
      font-size: 1.75rem;
      color: #1f2937;
      margin-top: 2.5rem;
      margin-bottom: 0.75rem;
    }

    h3 {
      font-size: 1.375rem;
      color: #374151;
      margin-top: 2rem;
      margin-bottom: 0.5rem;
    }

    p {
      margin-top: 1.75rem;
      line-height: 1.6;
    }

    a {
      color: #111827;
      text-decoration: none;
    }

    a:hover {
      color: #6b7280;
    }

    .post-card {
      margin-bottom: 48px;
      border-bottom: 1px solid #e5e7eb;
      padding-bottom: 48px;
    }

    .post-card .featured-image {
      width: 100%;
      height: auto;
      margin-bottom: 24px;
      border-radius: 4px;
    }

    .post-card h2 a {
      color: #111827;
    }

    .post-card .excerpt {
      color: #6b7280;
      margin: 12px 0;
    }

    .meta {
      font-size: 14px;
      color: #9ca3af;
    }
  </style>
</head>
<body>
  <header class="site-header">
    <div class="container">
      <div class="site-title">
        <a href="/">{{ blog.title }}</a>
      </div>
    </div>
  </header>
  <main>`,

    footer: `  </main>
  <footer class="site-footer" style="border-top: 1px solid #e5e7eb; padding: 24px 0; margin-top: 48px;">
    <div class="container" style="text-align: center; color: #9ca3af; font-size: 14px;">
      <p>&copy; {{ current_year }} {{ blog.title }}</p>
    </div>
  </footer>
</body>
</html>`,

    shared: ``
  },
  styling: minimalBlogPreset
}

// Modern Magazine Theme - Contemporary, bold design
const modernMagazineTheme: ThemePreset = {
  id: 'magazine',
  name: 'Modern Magazine',
  description: 'Bold, contemporary design for lifestyle and tech publications',
  placeholderColor: '#f5f5f4',
  files: {
    home: `<div class="blog-home">
  <div class="hero">
    <h1>{{ blog.title }}</h1>
    {% if blog.description %}
      <p class="subtitle">{{ blog.description }}</p>
    {% endif %}
  </div>

  <div class="featured-posts">
    {% for post in posts limit:1 %}
      <article class="featured-post">
        {% if post.featured_image_url %}
          <img src="{{ post.featured_image_url }}" alt="{{ post.title }}" class="featured-image">
        {% endif %}
        <div class="featured-content">
          <span class="category-badge">{{ post.category_slug }}</span>
          <h2><a href="/{{ post.category_slug }}/{{ post.slug }}">{{ post.title }}</a></h2>
          <p class="excerpt">{{ post.excerpt }}</p>
          <div class="meta">
            <span class="date">{{ post.published_at }}</span>
            <span class="reading-time">{{ post.reading_time_minutes }} min read</span>
          </div>
        </div>
      </article>
    {% endfor %}
  </div>

  <div class="posts-grid">
    {% for post in posts offset:1 %}
      <article class="post-card">
        {% if post.featured_image_url %}
          <img src="{{ post.featured_image_url }}" alt="{{ post.title }}" class="card-image">
        {% endif %}
        <div class="card-content">
          <span class="category-badge">{{ post.category_slug }}</span>
          <h3><a href="/{{ post.category_slug }}/{{ post.slug }}">{{ post.title }}</a></h3>
          <p class="excerpt">{{ post.excerpt }}</p>
          <div class="meta">
            <span>{{ post.published_at }}</span>
          </div>
        </div>
      </article>
    {% endfor %}
  </div>
</div>`,

    category: `<div class="category-page">
  <header class="category-header">
    <span class="category-badge">{{ category.slug }}</span>
    <h1>{{ category.name }}</h1>
    {% if category.description %}
      <p class="description">{{ category.description }}</p>
    {% endif %}
  </header>

  <div class="posts-grid">
    {% for post in posts %}
      <article class="post-card">
        {% if post.featured_image_url %}
          <img src="{{ post.featured_image_url }}" alt="{{ post.title }}" class="card-image">
        {% endif %}
        <div class="card-content">
          <h3><a href="/{{ category.slug }}/{{ post.slug }}">{{ post.title }}</a></h3>
          <p class="excerpt">{{ post.excerpt }}</p>
          <div class="meta">
            <span>{{ post.published_at }}</span>
            <span>{{ post.reading_time_minutes }} min</span>
          </div>
        </div>
      </article>
    {% endfor %}
  </div>
</div>`,

    post: `<article class="blog-post">
  <header class="post-header">
    <span class="category-badge">{{ post.category_slug }}</span>
    <h1>{{ post.title }}</h1>
    <div class="post-meta">
      <div class="authors">
        {% for author in post.authors %}
          <span class="author">{{ author.name }}</span>
        {% endfor %}
      </div>
      <span class="date">{{ post.published_at }}</span>
      <span class="reading-time">{{ post.reading_time_minutes }} min read</span>
    </div>
  </header>

  {% if post.featured_image_url %}
    <div class="featured-image-wrapper">
      <img src="{{ post.featured_image_url }}" alt="{{ post.title }}" class="featured-image">
    </div>
  {% endif %}

  <div class="post-content">
    {{ post.content }}
  </div>

  <footer class="post-footer">
    <div class="post-navigation">
      {% if previous_post %}
        <a href="/{{ post.category_slug }}/{{ previous_post.slug }}" class="nav-link prev">
          <span class="label">Previous</span>
          <span class="title">{{ previous_post.title }}</span>
        </a>
      {% endif %}
      {% if next_post %}
        <a href="/{{ post.category_slug }}/{{ next_post.slug }}" class="nav-link next">
          <span class="label">Next</span>
          <span class="title">{{ next_post.title }}</span>
        </a>
      {% endif %}
    </div>
  </footer>
</article>`,

    header: `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  
  <title>{% if page_title %}{{ page_title }}{% elsif seo.meta_title %}{{ seo.meta_title }}{% else %}{{ blog.title }}{% endif %}</title>
  
  {% if page_description %}
    <meta name="description" content="{{ page_description }}">
  {% elsif seo.meta_description %}
    <meta name="description" content="{{ seo.meta_description }}">
  {% endif %}

  <style>
    * {
      margin: 0;
      padding: 0;
      box-sizing: border-box;
    }

    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      font-size: 17px;
      line-height: 1.75;
      color: #111827;
      background: #ffffff;
    }

    .site-header {
      background: #000;
      color: #fff;
      padding: 32px 0;
    }

    .container {
      max-width: 1200px;
      margin: 0 auto;
      padding: 0 32px;
    }

    .site-title a {
      font-size: 32px;
      font-weight: 800;
      text-decoration: none;
      color: #fff;
      letter-spacing: -0.5px;
    }

    .blog-home, .category-page {
      padding: 64px 32px;
      max-width: 1200px;
      margin: 0 auto;
    }

    .blog-post {
      padding: 64px 32px;
      max-width: 800px;
      margin: 0 auto;
    }

    h1 {
      font-size: 3rem;
      color: #000;
      margin-bottom: 1rem;
      font-weight: 800;
      letter-spacing: -1px;
    }

    h2 {
      font-size: 2rem;
      color: #111827;
      margin-top: 3rem;
      margin-bottom: 0.75rem;
      font-weight: 700;
    }

    h3 {
      font-size: 1.5rem;
      color: #374151;
      margin-top: 2.5rem;
      margin-bottom: 0.5rem;
      font-weight: 600;
    }

    p {
      margin-top: 1.5rem;
      line-height: 1.75;
    }

    a {
      color: #2563eb;
      text-decoration: none;
    }

    a:hover {
      color: #1d4ed8;
    }

    .category-badge {
      display: inline-block;
      background: #2563eb;
      color: #fff;
      padding: 4px 12px;
      border-radius: 4px;
      font-size: 12px;
      font-weight: 600;
      text-transform: uppercase;
      margin-bottom: 16px;
    }

    .posts-grid {
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(350px, 1fr));
      gap: 32px;
      margin-top: 48px;
    }

    .post-card {
      border: 1px solid #e5e7eb;
      border-radius: 8px;
      overflow: hidden;
      transition: transform 0.2s, box-shadow 0.2s;
    }

    .post-card:hover {
      transform: translateY(-4px);
      box-shadow: 0 12px 24px rgba(0,0,0,0.1);
    }

    .card-image {
      width: 100%;
      height: 220px;
      object-fit: cover;
    }

    .card-content {
      padding: 24px;
    }

    .meta {
      font-size: 14px;
      color: #6b7280;
      margin-top: 12px;
    }
  </style>
</head>
<body>
  <header class="site-header">
    <div class="container">
      <div class="site-title">
        <a href="/">{{ blog.title }}</a>
      </div>
    </div>
  </header>
  <main>`,

    footer: `  </main>
  <footer class="site-footer" style="background: #111827; color: #9ca3af; padding: 48px 0; margin-top: 64px;">
    <div class="container" style="text-align: center;">
      <p style="margin-bottom: 8px; font-weight: 600; color: #fff;">{{ blog.title }}</p>
      <p>&copy; {{ current_year }} All rights reserved.</p>
    </div>
  </footer>
</body>
</html>`,

    shared: ``
  },
  styling: modernMagazinePreset
}

// Times Journal Theme - Traditional newspaper
const timesJournalTheme: ThemePreset = {
  id: 'journal',
  name: 'Times Journal',
  description: 'Classic newspaper typography for serious publications',
  placeholderColor: '#fef3c7',
  files: {
    home: `<div class="blog-home">
  <div class="masthead">
    <h1 class="site-name">{{ blog.title }}</h1>
    <p class="tagline">{{ blog.description }}</p>
    <div class="divider"></div>
  </div>

  <div class="articles">
    {% for post in posts %}
      <article class="article-item">
        <h2 class="headline"><a href="/{{ post.category_slug }}/{{ post.slug }}">{{ post.title }}</a></h2>
        <div class="byline">
          {% for author in post.authors %}
            <span class="author">By {{ author.name }}</span>
          {% endfor %}
          <span class="date">{{ post.published_at }}</span>
        </div>
        <p class="lede">{{ post.excerpt }}</p>
        {% if post.featured_image_url %}
          <img src="{{ post.featured_image_url }}" alt="{{ post.title }}" class="article-image">
        {% endif %}
      </article>
    {% endfor %}
  </div>
</div>`,

    category: `<div class="category-page">
  <header class="section-header">
    <h1 class="section-title">{{ category.name }}</h1>
    {% if category.description %}
      <p class="section-description">{{ category.description }}</p>
    {% endif %}
    <div class="divider"></div>
  </header>

  <div class="articles">
    {% for post in posts %}
      <article class="article-item">
        <h2 class="headline"><a href="/{{ category.slug }}/{{ post.slug }}">{{ post.title }}</a></h2>
        <div class="byline">
          {% for author in post.authors %}
            <span class="author">By {{ author.name }}</span>
          {% endfor %}
          <span class="date">{{ post.published_at }}</span>
        </div>
        <p class="lede">{{ post.excerpt }}</p>
      </article>
    {% endfor %}
  </div>
</div>`,

    post: `<article class="blog-post">
  <header class="article-header">
    <h1 class="headline">{{ post.title }}</h1>
    <div class="byline">
      {% for author in post.authors %}
        <span class="author">By {{ author.name }}</span>
      {% endfor %}
      <span class="date">{{ post.published_at }}</span>
    </div>
  </header>

  {% if post.featured_image_url %}
    <figure class="lead-image">
      <img src="{{ post.featured_image_url }}" alt="{{ post.title }}">
    </figure>
  {% endif %}

  <div class="article-body">
    {{ post.content }}
  </div>

  <footer class="article-footer">
    <div class="article-navigation">
      {% if previous_post %}
        <a href="/{{ post.category_slug }}/{{ previous_post.slug }}" class="nav-prev">
          ← {{ previous_post.title }}
        </a>
      {% endif %}
      {% if next_post %}
        <a href="/{{ post.category_slug }}/{{ next_post.slug }}" class="nav-next">
          {{ next_post.title }} →
        </a>
      {% endif %}
    </div>
  </footer>
</article>`,

    header: `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  
  <title>{% if page_title %}{{ page_title }}{% elsif seo.meta_title %}{{ seo.meta_title }}{% else %}{{ blog.title }}{% endif %}</title>
  
  {% if page_description %}
    <meta name="description" content="{{ page_description }}">
  {% elsif seo.meta_description %}
    <meta name="description" content="{{ seo.meta_description }}">
  {% endif %}

  <style>
    * {
      margin: 0;
      padding: 0;
      box-sizing: border-box;
    }

    body {
      font-family: Georgia, "Times New Roman", Times, serif;
      font-size: 18px;
      line-height: 1.7;
      color: #1a1a1a;
      background: #fafaf9;
    }

    .site-header {
      background: #fff;
      border-bottom: 3px double #2a2a2a;
      padding: 32px 0 24px;
    }

    .container {
      max-width: 900px;
      margin: 0 auto;
      padding: 0 32px;
    }

    .site-title {
      text-align: center;
      font-size: 48px;
      font-weight: 700;
      font-family: "Times New Roman", Times, serif;
      letter-spacing: 2px;
      text-transform: uppercase;
    }

    .site-title a {
      text-decoration: none;
      color: #000;
    }

    .blog-home, .category-page, .blog-post {
      background: #fff;
      padding: 48px 32px;
      max-width: 900px;
      margin: 32px auto;
      box-shadow: 0 1px 3px rgba(0,0,0,0.1);
    }

    .masthead {
      text-align: center;
      margin-bottom: 48px;
      padding-bottom: 24px;
      border-bottom: 1px solid #2a2a2a;
    }

    .site-name {
      font-size: 2.5rem;
      margin-bottom: 8px;
    }

    .tagline {
      font-size: 16px;
      color: #4a4a4a;
      font-style: italic;
    }

    .divider {
      width: 60px;
      height: 2px;
      background: #2a2a2a;
      margin: 16px auto;
    }

    .headline {
      font-size: 1.875rem;
      font-weight: 700;
      margin-bottom: 8px;
    }

    .headline a {
      color: #000;
      text-decoration: none;
    }

    .headline a:hover {
      color: #0055aa;
    }

    .byline {
      font-size: 14px;
      color: #4a4a4a;
      margin-bottom: 16px;
    }

    .author {
      font-style: italic;
      margin-right: 12px;
    }

    .lede {
      font-size: 1.125rem;
      line-height: 1.6;
      margin-bottom: 24px;
    }

    .article-item {
      margin-bottom: 48px;
      padding-bottom: 32px;
      border-bottom: 1px solid #e5e7eb;
    }

    .article-item:last-child {
      border-bottom: none;
    }

    h2 {
      font-size: 1.875rem;
      margin-top: 2rem;
      margin-bottom: 0.5rem;
    }

    h3 {
      font-size: 1.5rem;
      margin-top: 1.75rem;
      margin-bottom: 0.5rem;
    }

    p {
      margin-top: 1.25rem;
    }

    a {
      color: #0055aa;
      text-decoration: none;
    }

    a:hover {
      color: #003377;
      text-decoration: underline;
    }
  </style>
</head>
<body>
  <header class="site-header">
    <div class="container">
      <div class="site-title">
        <a href="/">{{ blog.title }}</a>
      </div>
    </div>
  </header>
  <main>`,

    footer: `  </main>
  <footer class="site-footer" style="background: #fff; border-top: 3px double #2a2a2a; padding: 32px 0; margin-top: 32px;">
    <div class="container" style="text-align: center; font-size: 14px; color: #4a4a4a;">
      <p>&copy; {{ current_year }} {{ blog.title }}. All rights reserved.</p>
    </div>
  </footer>
</body>
</html>`,

    shared: ``
  },
  styling: timesJournalPreset
}

export const THEME_PRESETS: ThemePreset[] = [
  blankTheme,
  minimalBlogTheme,
  modernMagazineTheme,
  timesJournalTheme
]

export const getPresetById = (id: string): ThemePreset | undefined => {
  return THEME_PRESETS.find((preset) => preset.id === id)
}
