import { BlogThemeFiles } from '../services/api/blog'

export const DEFAULT_BLOG_TEMPLATES: BlogThemeFiles = {
  home: `<div class="blog-home">
  <header class="hero">
    <h1>{{ blog.title }}</h1>
    <p class="subtitle">{{ blog.description }}</p>
  </header>

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
  
  <!-- Title -->
  <title>{% if page_title %}{{ page_title }}{% elsif seo.meta_title %}{{ seo.meta_title }}{% else %}{{ blog.title }}{% endif %}</title>
  
  <!-- Meta Description -->
  {% if page_description %}
    <meta name="description" content="{{ page_description }}">
  {% elsif seo.meta_description %}
    <meta name="description" content="{{ seo.meta_description }}">
  {% endif %}
  
  <!-- Keywords -->
  {% if seo.keywords and seo.keywords.size > 0 %}
    <meta name="keywords" content="{{ seo.keywords | join: ', ' }}">
  {% endif %}
  
  <!-- Canonical URL -->
  {% if seo.canonical_url %}
    <link rel="canonical" href="{{ seo.canonical_url }}">
  {% endif %}
  
  <!-- Favicon -->
  {% if blog.icon_url %}
    <link rel="icon" href="{{ blog.icon_url }}">
  {% endif %}
  
  <!-- Open Graph / Facebook -->
  <meta property="og:type" content="{% if post %}article{% else %}website{% endif %}">
  <meta property="og:title" content="{% if seo.og_title %}{{ seo.og_title }}{% elsif page_title %}{{ page_title }}{% elsif seo.meta_title %}{{ seo.meta_title }}{% else %}{{ blog.title }}{% endif %}">
  {% if seo.og_description %}
    <meta property="og:description" content="{{ seo.og_description }}">
  {% elsif page_description %}
    <meta property="og:description" content="{{ page_description }}">
  {% elsif seo.meta_description %}
    <meta property="og:description" content="{{ seo.meta_description }}">
  {% endif %}
  {% if seo.og_image %}
    <meta property="og:image" content="{{ seo.og_image }}">
  {% elsif post.featured_image_url %}
    <meta property="og:image" content="{{ post.featured_image_url }}">
  {% endif %}
  {% if current_url %}
    <meta property="og:url" content="{{ current_url }}">
  {% endif %}
  <meta property="og:site_name" content="{{ blog.title }}">
  
  <!-- Twitter Card -->
  <meta name="twitter:card" content="summary_large_image">
  <meta name="twitter:title" content="{% if seo.og_title %}{{ seo.og_title }}{% elsif page_title %}{{ page_title }}{% elsif seo.meta_title %}{{ seo.meta_title }}{% else %}{{ blog.title }}{% endif %}">
  {% if seo.og_description %}
    <meta name="twitter:description" content="{{ seo.og_description }}">
  {% elsif page_description %}
    <meta name="twitter:description" content="{{ page_description }}">
  {% elsif seo.meta_description %}
    <meta name="twitter:description" content="{{ seo.meta_description }}">
  {% endif %}
  {% if seo.og_image %}
    <meta name="twitter:image" content="{{ seo.og_image }}">
  {% elsif post.featured_image_url %}
    <meta name="twitter:image" content="{{ post.featured_image_url }}">
  {% endif %}
  
  <!-- Article specific meta tags -->
  {% if post %}
    <meta property="article:published_time" content="{{ post.published_at }}">
    {% if post.updated_at %}
      <meta property="article:modified_time" content="{{ post.updated_at }}">
    {% endif %}
    {% for author in post.authors %}
      <meta property="article:author" content="{{ author.name }}">
    {% endfor %}
    {% if post.category %}
      <meta property="article:section" content="{{ post.category }}">
    {% endif %}
  {% endif %}

  <style>
    * {
      margin: 0;
      padding: 0;
      box-sizing: border-box;
    }

    body {
      font-family: {{ styling.default.fontFamily | default: 'system-ui, -apple-system, sans-serif' }};
      font-size: {{ styling.default.fontSize.value }}{{ styling.default.fontSize.unit | default: '16px' }};
      line-height: {{ styling.default.lineHeight | default: 1.6 }};
      color: {{ styling.default.color | default: '#1a1a1a' }};
      background: {{ styling.default.backgroundColor | default: '#ffffff' }};
    }

    .container {
      max-width: 1200px;
      margin: 0 auto;
      padding: 0 20px;
    }

    /* Header */
    .site-header {
      border-bottom: 1px solid #e5e7eb;
      padding: 20px 0;
    }

    .site-header .container {
      display: flex;
      justify-content: space-between;
      align-items: center;
    }

    .site-title a {
      font-size: 24px;
      font-weight: 700;
      text-decoration: none;
      color: #000;
    }

    .site-nav {
      display: flex;
      gap: 24px;
    }

    .site-nav a {
      text-decoration: none;
      color: #4b5563;
      transition: color 0.2s;
    }

    .site-nav a:hover {
      color: #000;
    }

    /* Home Page */
    .blog-home {
      padding: 60px 20px;
      max-width: 1200px;
      margin: 0 auto;
    }

    .hero {
      text-align: center;
      margin-bottom: 60px;
    }

    .hero h1 {
      font-size: 48px;
      margin-bottom: 16px;
    }

    .hero .subtitle {
      font-size: 20px;
      color: #6b7280;
    }

    .posts-grid {
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
      gap: 32px;
    }

    .post-card {
      border: 1px solid #e5e7eb;
      border-radius: 8px;
      overflow: hidden;
      transition: box-shadow 0.2s;
    }

    .post-card:hover {
      box-shadow: 0 4px 12px rgba(0,0,0,0.1);
    }

    .post-card .featured-image {
      width: 100%;
      height: 200px;
      object-fit: cover;
    }

    .post-card .post-content {
      padding: 20px;
    }

    .post-card h2 {
      font-size: 24px;
      margin-bottom: 12px;
    }

    .post-card h2 a {
      text-decoration: none;
      color: #000;
    }

    .post-card h2 a:hover {
      color: #2563eb;
    }

    .post-card .excerpt {
      color: #6b7280;
      margin-bottom: 12px;
    }

    .post-card .meta {
      display: flex;
      gap: 16px;
      font-size: 14px;
      color: #9ca3af;
    }

    /* Headings */
    h1, h2, h3, h4, h5, h6 {
      font-family: {{ styling.headings.fontFamily | default: 'inherit' }};
    }

    h1 {
      font-size: {{ styling.h1.fontSize.value }}{{ styling.h1.fontSize.unit | default: '2.5rem' }};
      color: {{ styling.h1.color | default: '#000000' }};
      margin-top: {{ styling.h1.marginTop.value }}{{ styling.h1.marginTop.unit | default: '48px' }};
      margin-bottom: {{ styling.h1.marginBottom.value }}{{ styling.h1.marginBottom.unit | default: '24px' }};
    }

    h2 {
      font-size: {{ styling.h2.fontSize.value }}{{ styling.h2.fontSize.unit | default: '2rem' }};
      color: {{ styling.h2.color | default: '#1a1a1a' }};
      margin-top: {{ styling.h2.marginTop.value }}{{ styling.h2.marginTop.unit | default: '40px' }};
      margin-bottom: {{ styling.h2.marginBottom.value }}{{ styling.h2.marginBottom.unit | default: '20px' }};
    }

    h3 {
      font-size: {{ styling.h3.fontSize.value }}{{ styling.h3.fontSize.unit | default: '1.5rem' }};
      color: {{ styling.h3.color | default: '#1a1a1a' }};
      margin-top: {{ styling.h3.marginTop.value }}{{ styling.h3.marginTop.unit | default: '32px' }};
      margin-bottom: {{ styling.h3.marginBottom.value }}{{ styling.h3.marginBottom.unit | default: '16px' }};
    }

    /* Paragraphs */
    p {
      margin-top: {{ styling.paragraph.marginTop.value }}{{ styling.paragraph.marginTop.unit | default: '0' }};
      margin-bottom: {{ styling.paragraph.marginBottom.value }}{{ styling.paragraph.marginBottom.unit | default: '16px' }};
      line-height: {{ styling.paragraph.lineHeight | default: 1.6 }};
    }

    /* Links */
    a {
      color: {{ styling.link.color | default: '#2563eb' }};
      text-decoration: none;
    }

    a:hover {
      color: {{ styling.link.hoverColor | default: '#1d4ed8' }};
    }

    /* Lists */
    ul, ol {
      margin-top: {{ styling.list.marginTop.value }}{{ styling.list.marginTop.unit | default: '16px' }};
      margin-bottom: {{ styling.list.marginBottom.value }}{{ styling.list.marginBottom.unit | default: '16px' }};
      padding-left: {{ styling.list.paddingLeft.value }}{{ styling.list.paddingLeft.unit | default: '24px' }};
    }

    /* Blockquote */
    blockquote {
      font-size: {{ styling.blockquote.fontSize.value }}{{ styling.blockquote.fontSize.unit | default: '18px' }};
      color: {{ styling.blockquote.color | default: '#4b5563' }};
      margin-top: {{ styling.blockquote.marginTop.value }}{{ styling.blockquote.marginTop.unit | default: '24px' }};
      margin-bottom: {{ styling.blockquote.marginBottom.value }}{{ styling.blockquote.marginBottom.unit | default: '24px' }};
      line-height: {{ styling.blockquote.lineHeight | default: 1.6 }};
      padding-left: 20px;
      border-left: 4px solid #e5e7eb;
    }

    /* Code */
    code {
      font-family: {{ styling.inlineCode.fontFamily | default: 'monospace' }};
      font-size: {{ styling.inlineCode.fontSize.value }}{{ styling.inlineCode.fontSize.unit | default: '14px' }};
      color: {{ styling.inlineCode.color | default: '#e11d48' }};
      background: {{ styling.inlineCode.backgroundColor | default: '#f3f4f6' }};
      padding: 2px 6px;
      border-radius: 3px;
    }

    pre {
      margin-top: {{ styling.codeBlock.marginTop.value }}{{ styling.codeBlock.marginTop.unit | default: '16px' }};
      margin-bottom: {{ styling.codeBlock.marginBottom.value }}{{ styling.codeBlock.marginBottom.unit | default: '16px' }};
      padding: 16px;
      background: #f3f4f6;
      border-radius: 6px;
      overflow-x: auto;
    }

    pre code {
      background: none;
      padding: 0;
    }

    /* Horizontal Rule */
    hr {
      border: none;
      border-top: 1px solid {{ styling.separator.color | default: '#e5e7eb' }};
      margin-top: {{ styling.separator.marginTop.value }}{{ styling.separator.marginTop.unit | default: '32px' }};
      margin-bottom: {{ styling.separator.marginBottom.value }}{{ styling.separator.marginBottom.unit | default: '32px' }};
    }

    /* Caption */
    figcaption {
      font-size: {{ styling.caption.fontSize.value }}{{ styling.caption.fontSize.unit | default: '14px' }};
      color: {{ styling.caption.color | default: '#6b7280' }};
      margin-top: 8px;
      text-align: center;
    }

    /* Post Page */
    .blog-post {
      max-width: 800px;
      margin: 60px auto;
      padding: 0 20px;
    }

    .post-header {
      margin-bottom: 40px;
    }

    .post-meta {
      display: flex;
      gap: 16px;
      font-size: 14px;
      color: #6b7280;
    }

    .blog-post .featured-image {
      width: 100%;
      border-radius: 8px;
      margin-bottom: 40px;
    }

    .post-content {
      font-size: 18px;
      line-height: 1.7;
    }

    .post-navigation {
      display: flex;
      justify-content: space-between;
      margin-top: 60px;
      padding-top: 40px;
      border-top: 1px solid #e5e7eb;
    }

    .post-navigation a {
      text-decoration: none;
      color: #2563eb;
    }

    /* Footer */
    .site-footer {
      border-top: 1px solid #e5e7eb;
      padding: 40px 0;
      margin-top: 80px;
    }

    .site-footer .container {
      display: flex;
      justify-content: space-between;
      align-items: center;
    }

    .footer-links {
      display: flex;
      gap: 24px;
    }

    .footer-links a {
      text-decoration: none;
      color: #6b7280;
    }

    /* Category Page */
    .category-page {
      max-width: 800px;
      margin: 60px auto;
      padding: 0 20px;
    }

    .category-header {
      margin-bottom: 40px;
    }

    .category-header h1 {
      font-size: 40px;
      margin-bottom: 16px;
    }

    .category-header .description {
      font-size: 18px;
      color: #6b7280;
    }

    .posts-list {
      display: flex;
      flex-direction: column;
      gap: 32px;
    }

    .post-item {
      padding-bottom: 32px;
      border-bottom: 1px solid #e5e7eb;
    }

    .post-item h2 {
      font-size: 28px;
      margin-bottom: 12px;
    }

    .post-item h2 a {
      text-decoration: none;
      color: #000;
    }

    .post-item h2 a:hover {
      color: #2563eb;
    }

    .post-item .excerpt {
      color: #6b7280;
      margin-bottom: 12px;
    }

    .post-item .meta {
      display: flex;
      gap: 16px;
      font-size: 14px;
      color: #9ca3af;
    }
  </style>
</head>
<body>
  <header class="site-header">
    <div class="container">
      <div class="site-title">
        <a href="/">
          {% if blog.logo_url %}
            <img src="{{ blog.logo_url }}" alt="{{ blog.title }}" class="site-logo" style="max-height: 40px; vertical-align: middle;">
          {% else %}
            {{ blog.title }}
          {% endif %}
        </a>
      </div>
      <nav class="site-nav">
        <a href="/">Home</a>
        {% for category in categories %}
          <a href="/{{ category.slug }}">{{ category.name }}</a>
        {% endfor %}
      </nav>
    </div>
  </header>`,

  footer: `  <footer class="site-footer">
    <div class="container">
      <p>&copy; {{ current_year }} {{ blog.title }}. All rights reserved.</p>
      <div class="footer-links">
        <a href="/privacy">Privacy</a>
        <a href="/terms">Terms</a>
      </div>
    </div>
  </footer>
</body>
</html>`,

  shared: `{%- comment -%}
  This file is for Liquid macros, helper functions, and reusable snippets.
  Define your custom macros here to keep your templates DRY.
  
  Example:
  {% macro format_date(date) %}
    {{ date | date: "%B %d, %Y" }}
  {% endmacro %}
{%- endcomment -%}`
}
