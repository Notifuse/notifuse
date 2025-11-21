import { BlogThemeFiles } from '../../services/api/blog'

export interface ThemePreset {
  id: string
  name: string
  description: string
  placeholderColor: string
  files: BlogThemeFiles
}

// Default Theme - Medium-inspired blog theme
const defaultTheme: ThemePreset = {
  id: 'default',
  name: 'Default',
  description: 'A beautiful Medium-inspired blog theme with full Liquid support',
  placeholderColor: '#7763f1',
  files: {
    'home.liquid': `{%- comment -%} Include Header (shares parent scope for workspace/base_url access) {%- endcomment -%}
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
  {%- assign widget = 'categories' -%}
  {% include 'shared' %}

  {%- comment -%} Featured Post {%- endcomment -%}
  {%- if posts.size > 0 -%}
    {%- assign featured_post = posts.first -%}
    <section class="featured-post-section">
      <div>
        {%- if featured_post.category_slug -%}
          <a href="{{ base_url }}/{{ featured_post.category_slug }}/{{ featured_post.slug }}" class="featured-post">
        {%- else -%}
          <a href="{{ base_url }}/{{ featured_post.slug }}" class="featured-post">
        {%- endif -%}
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
        </a>
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
{% include 'footer' %}`,

    'category.liquid': `{%- comment -%} Include Header (shares parent scope for workspace/base_url access) {%- endcomment -%}
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
          <h1 class="hero-title">{{ category.name }}</h1>
          {%- if category.description -%}
            <p class="hero-subtitle">{{ category.description }}</p>
          {%- else -%}
            <p class="hero-subtitle">
              {%- if pagination.total_count -%}
                {{ pagination.total_count }} {% if pagination.total_count == 1 %}post{% else %}posts{% endif %} in this category
              {%- else -%}
                Explore articles in {{ category.name }}
              {%- endif -%}
            </p>
          {%- endif -%}
        </div>
        {% render 'shared', widget: 'newsletter' %}
      </div>
    </div>
  </section>

  {%- comment -%} Categories Bar with Active State {%- endcomment -%}
  {%- assign widget = 'categories' -%}
  {%- assign active_category = category.slug -%}
  {% include 'shared' %}

  {%- comment -%} Posts Grid {%- endcomment -%}
  <section class="posts-grid-section">
    <div>
      {%- if posts.size > 0 -%}
        <div class="grid grid-cols-1 md:grid-cols-3 gap-12">
          {%- for post in posts -%}
            {% render 'shared', widget: 'post-card', post: post %}
          {%- endfor -%}
        </div>
      {%- else -%}
        <div style="text-align: center; padding: 4rem 2rem;">
          <p style="font-size: 1.25rem; color: var(--color-text-secondary);">No posts found in this category yet.</p>
          <a href="{{ base_url }}/" style="display: inline-block; margin-top: 1.5rem; color: var(--color-link);">← Back to all posts</a>
        </div>
      {%- endif -%}
    </div>
  </section>

  {%- comment -%} Pagination {%- endcomment -%}
  {% render 'shared', widget: 'pagination' %}

{%- comment -%} Include Footer (shares parent scope for workspace/base_url access) {%- endcomment -%}
{% include 'footer' %}`,

    'post.liquid': `{%- comment -%} Include Header (shares parent scope for workspace/base_url access) {%- endcomment -%}
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

  {%- comment -%} Post Article {%- endcomment -%}
  <article class="post-article">
    <header class="post-header">
      {%- if post.category_slug -%}
        <div class="post-category-badge">
          <a href="{{ base_url }}/{{ post.category_slug }}" class="category-pill">
            {%- for cat in categories -%}
              {%- if cat.slug == post.category_slug -%}
                {{ cat.name }}
              {%- endif -%}
            {%- endfor -%}
          </a>
        </div>
      {%- endif -%}

      <h1 class="post-title">{{ post.title }}</h1>

      <div class="post-meta">
        <div class="author-info">
          <div class="author-avatars">
            {%- for author in post.authors -%}
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
      </div>
    </header>

    {%- comment -%} Featured Image {%- endcomment -%}
    {%- if post.featured_image_url -%}
      <div class="post-featured-image">
        <img src="{{ post.featured_image_url }}" alt="{{ post.title }}" />
      </div>
    {%- endif -%}

    {%- comment -%} Post Content - HTML from compiled email template {%- endcomment -%}
    <div class="post-content">
      {{ post.content }}
    </div>
  </article>

  {%- comment -%} Newsletter Section {%- endcomment -%}
  <section class="hero" style="border-bottom: none; border-top: 1px solid var(--color-border);">
    <div>
      <div class="grid grid-cols-1 md:grid-cols-3 gap-12 items-center">
        <div class="md:col-span-2">
          <h2 class="hero-title" style="font-size: 1.5rem;">Get more insights like this</h2>
          <p class="hero-subtitle" style="font-size: 1rem;">
            Subscribe to our newsletter for the latest articles on technology, design, and innovation.
          </p>
        </div>
        {% render 'shared', widget: 'newsletter' %}
      </div>
    </div>
  </section>

  {%- comment -%} Related Posts {%- endcomment -%}
  {%- if posts.size > 0 -%}
    <section class="related-posts-section">
      <div>
        <h2 class="related-posts-title">Last articles</h2>
        <div class="grid grid-cols-1 md:grid-cols-3 gap-12">
          {%- for related_post in posts limit: 3 -%}
            {% render 'shared', widget: 'post-card', post: related_post %}
          {%- endfor -%}
        </div>
      </div>
    </section>
  {%- endif -%}

{%- comment -%} Include Footer (shares parent scope for workspace/base_url access) {%- endcomment -%}
{% include 'footer' %}`,

    'header.liquid': `<!DOCTYPE html>
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
  
  {%- comment -%} SEO Meta Description {%- endcomment -%}
  {%- if post.seo.meta_description -%}
    <meta name="description" content="{{ post.seo.meta_description }}">
  {%- elsif post.excerpt -%}
    <meta name="description" content="{{ post.excerpt }}">
  {%- elsif category.seo.meta_description -%}
    <meta name="description" content="{{ category.seo.meta_description }}">
  {%- elsif category.description -%}
    <meta name="description" content="{{ category.description }}">
  {%- endif -%}
  
  {%- comment -%} SEO Keywords {%- endcomment -%}
  {%- if post.seo.keywords -%}
    <meta name="keywords" content="{{ post.seo.keywords }}">
  {%- elsif category.seo.keywords -%}
    <meta name="keywords" content="{{ category.seo.keywords }}">
  {%- endif -%}
  
  {%- comment -%} Canonical URL {%- endcomment -%}
  {%- if post.seo.canonical_url -%}
    <link rel="canonical" href="{{ post.seo.canonical_url }}">
  {%- elsif category.seo.canonical_url -%}
    <link rel="canonical" href="{{ category.seo.canonical_url }}">
  {%- elsif post -%}
    {%- if post.category_slug -%}
      <link rel="canonical" href="{{ base_url }}/{{ post.category_slug }}/{{ post.slug }}">
    {%- else -%}
      <link rel="canonical" href="{{ base_url }}/{{ post.slug }}">
    {%- endif -%}
  {%- elsif category -%}
    <link rel="canonical" href="{{ base_url }}/{{ category.slug }}">
  {%- else -%}
    <link rel="canonical" href="{{ base_url }}/">
  {%- endif -%}
  
  {%- comment -%} Open Graph Tags {%- endcomment -%}
  {%- if post.seo.og_title or post.title -%}
    <meta property="og:title" content="{% if post.seo.og_title %}{{ post.seo.og_title }}{% else %}{{ post.title }}{% endif %}">
  {%- elsif category.seo.og_title or category.name -%}
    <meta property="og:title" content="{% if category.seo.og_title %}{{ category.seo.og_title }}{% else %}{{ category.name }}{% endif %}">
  {%- else -%}
    <meta property="og:title" content="{{ workspace.name }}">
  {%- endif -%}
  
  {%- if post.seo.og_description or post.excerpt -%}
    <meta property="og:description" content="{% if post.seo.og_description %}{{ post.seo.og_description }}{% else %}{{ post.excerpt }}{% endif %}">
  {%- elsif category.seo.og_description or category.description -%}
    <meta property="og:description" content="{% if category.seo.og_description %}{{ category.seo.og_description }}{% else %}{{ category.description }}{% endif %}">
  {%- endif -%}
  
  {%- if post.seo.og_image or post.featured_image_url -%}
    <meta property="og:image" content="{% if post.seo.og_image %}{{ post.seo.og_image }}{% else %}{{ post.featured_image_url }}{% endif %}">
  {%- elsif category.seo.og_image -%}
    <meta property="og:image" content="{{ category.seo.og_image }}">
  {%- endif -%}
  
  <meta property="og:type" content="{% if post %}article{% else %}website{% endif %}">
  
  {%- if post -%}
    {%- if post.category_slug -%}
      <meta property="og:url" content="{{ base_url }}/{{ post.category_slug }}/{{ post.slug }}">
    {%- else -%}
      <meta property="og:url" content="{{ base_url }}/{{ post.slug }}">
    {%- endif -%}
  {%- elsif category -%}
    <meta property="og:url" content="{{ base_url }}/{{ category.slug }}">
  {%- else -%}
    <meta property="og:url" content="{{ base_url }}/">
  {%- endif -%}
  
  {%- comment -%} Twitter Card Tags {%- endcomment -%}
  <meta name="twitter:card" content="summary_large_image">
  {%- if post.seo.og_title or post.title -%}
    <meta name="twitter:title" content="{% if post.seo.og_title %}{{ post.seo.og_title }}{% else %}{{ post.title }}{% endif %}">
  {%- elsif category.seo.og_title or category.name -%}
    <meta name="twitter:title" content="{% if category.seo.og_title %}{{ category.seo.og_title }}{% else %}{{ category.name }}{% endif %}">
  {%- endif -%}
  
  {%- if post.seo.og_description or post.excerpt -%}
    <meta name="twitter:description" content="{% if post.seo.og_description %}{{ post.seo.og_description }}{% else %}{{ post.excerpt }}{% endif %}">
  {%- elsif category.seo.og_description or category.description -%}
    <meta name="twitter:description" content="{% if category.seo.og_description %}{{ category.seo.og_description }}{% else %}{{ category.description }}{% endif %}">
  {%- endif -%}
  
  {%- if post.seo.og_image or post.featured_image_url -%}
    <meta name="twitter:image" content="{% if post.seo.og_image %}{{ post.seo.og_image }}{% else %}{{ post.featured_image_url }}{% endif %}">
  {%- elsif category.seo.og_image -%}
    <meta name="twitter:image" content="{{ category.seo.og_image }}">
  {%- endif -%}
  
  {%- comment -%} Tailwind CSS CDN {%- endcomment -%}
  <script src="https://cdn.jsdelivr.net/npm/@tailwindcss/browser@4"></script>
  
  {%- comment -%} Theme Styles {%- endcomment -%}
  <style>{% include 'styles' %}</style>
  
  {%- comment -%} Theme Scripts {%- endcomment -%}
  <script>{% include 'scripts' %}</script>
</head>
<body>`,

    'footer.liquid': `  {%- comment -%} Footer {%- endcomment -%}
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
          <p class="text-gray-500 text-xs mt-1">
            Powered by <a href="https://www.notifuse.com" target="_blank" rel="noopener" style="color: var(--color-link); text-decoration: none;">Notifuse</a>
          </p>
        </div>

        <div class="footer-links">
          <a href="{{ base_url }}/terms" class="footer-link">Terms</a>
          <a href="{{ base_url }}/privacy" class="footer-link">Privacy</a>

          <div class="social-links">
            <a href="https://www.notifuse.com" target="_blank" rel="noopener noreferrer" aria-label="Website">
              <svg class="social-icon" fill="currentColor" viewBox="0 0 24 24">
                <path
                  d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z" />
              </svg>
            </a>
            <a href="https://x.com/notifuse" target="_blank" rel="noopener noreferrer" aria-label="X (Twitter)">
              <svg class="social-icon" fill="currentColor" viewBox="0 0 24 24">
                <path
                  d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z" />
              </svg>
            </a>
            <a href="https://github.com/Notifuse/notifuse" target="_blank" rel="noopener noreferrer"
              aria-label="GitHub">
              <svg class="social-icon" fill="currentColor" viewBox="0 0 24 24">
                <path fill-rule="evenodd"
                  d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z"
                  clip-rule="evenodd" />
              </svg>
            </a>
          </div>
        </div>
      </div>
    </div>
  </footer>
</div>
</body>
</html>`,

    'shared.liquid': `{%- comment -%}
  ========================================
  Shared Widgets Library
  ========================================
  
  This file contains reusable widgets for your blog.
  
  Usage Examples:
  
  1. Render newsletter widget:
     {% render 'shared', widget: 'newsletter' %}
  
  2. Render post-card widget with post data:
     {% render 'shared', widget: 'post-card', post: post %}
  
  3. Render categories widget with active category:
     {% render 'shared', widget: 'categories', active_category: category.slug %}
  
  4. Render pagination widget:
     {% render 'shared', widget: 'pagination' %}
  
  Available Widgets:
  - newsletter: Email subscription form
  - pagination: Pagination controls
  - categories: Blog categories navigation
  - post-card: Reusable post card for listings
  
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
      
      {%- assign prev_page = pagination.current_page | minus: 1 -%}
      {%- assign next_page = pagination.current_page | plus: 1 -%}
      {%- assign show_start_ellipsis = false -%}
      {%- assign show_end_ellipsis = false -%}
      
      {%- for i in (1..pagination.total_pages) -%}
        {%- if i == pagination.current_page -%}
          <a href="?page={{ i }}" class="pagination-button active">{{ i }}</a>
        {%- elsif i == 1 -%}
          <a href="?page={{ i }}" class="pagination-button">{{ i }}</a>
        {%- elsif i == pagination.total_pages -%}
          <a href="?page={{ i }}" class="pagination-button">{{ i }}</a>
        {%- elsif i == prev_page -%}
          <a href="?page={{ i }}" class="pagination-button">{{ i }}</a>
        {%- elsif i == next_page -%}
          <a href="?page={{ i }}" class="pagination-button">{{ i }}</a>
        {%- elsif i == 2 -%}
          {%- if pagination.current_page > 3 -%}
            {%- unless show_start_ellipsis -%}
              <span class="pagination-ellipsis">...</span>
              {%- assign show_start_ellipsis = true -%}
            {%- endunless -%}
          {%- else -%}
            <a href="?page={{ i }}" class="pagination-button">{{ i }}</a>
          {%- endif -%}
        {%- else -%}
          {%- assign last_page_minus_1 = pagination.total_pages | minus: 1 -%}
          {%- if i == last_page_minus_1 -%}
            {%- assign pages_from_end = pagination.total_pages | minus: pagination.current_page -%}
            {%- if pages_from_end > 2 -%}
              {%- unless show_end_ellipsis -%}
                <span class="pagination-ellipsis">...</span>
                {%- assign show_end_ellipsis = true -%}
              {%- endunless -%}
            {%- else -%}
              <a href="?page={{ i }}" class="pagination-button">{{ i }}</a>
            {%- endif -%}
          {%- endif -%}
        {%- endif -%}
      {%- endfor -%}
      
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
        {%- if categories -%}
          {%- for cat in categories -%}
            <a href="{{ base_url }}/{{ cat.slug }}" class="category-pill {% if active_category == cat.slug %}active{% endif %}">{{ cat.name }}</a>
          {%- endfor -%}
        {%- endif -%}
      </div>
    </div>
  </div>

{%- elsif widget == 'post-card' -%}
  {%- comment -%} Reusable Post Card {%- endcomment -%}
  {%- if post.category_slug -%}
    <a href="{{ base_url }}/{{ post.category_slug }}/{{ post.slug }}" class="post-card">
  {%- else -%}
    <a href="{{ base_url }}/{{ post.slug }}" class="post-card">
  {%- endif -%}
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
{%- endif -%}`,

    'styles.css': `/* ==================== 
   CSS CUSTOM PROPERTIES
   ==================== */

:root {
  /* ==================== COLORS ==================== */
  --color-text-primary: #1a1a1a;
  /* Primary text color */
  --color-text-secondary: #6b6b6b;
  /* Secondary/muted text */
  --color-text-heading: #1a1a1a;
  /* Heading text color */
  --color-link: #7763f1;
  /* Link color */
  --color-link-hover: #5a47d9;
  /* Link hover */
  --color-cta: #7763f1;
  /* CTA button background (Purple) */
  --color-cta-hover: #5a47d9;
  /* CTA hover state (Darker purple) */
  --color-cta-text: #fff;
  /* CTA text color */
  --color-border: #e5e5e5;
  /* Standard borders */
  --color-border-input: #d4d4d4;
  /* Input borders */
  --color-background: #fafafa;
  /* Body background */
  --color-container: #fafafa;
  /* Container background */
  --color-active-pill: #7763f1;
  /* Active category pill */
  --color-pill: #f5f5f5;
  /* Inactive category pill */
  --color-pill-hover: #e5e5e5;
  /* Category pill hover */

  /* ==================== TYPOGRAPHY ==================== */
  /* Base Settings */
  --font-family-base: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Roboto', 'Helvetica', 'Arial', sans-serif;
  --font-family-heading: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Roboto', 'Helvetica', 'Arial', sans-serif;
  --font-size-base: 1rem;
  /* 16px - Base font size */
  --line-height-base: 1.6;
  /* Body text line height */

  /* Headings */
  --font-size-h1: 3.5rem;
  /* 56px - Large heading */
  --font-size-h2: 2rem;
  /* 32px - Medium heading */
  --font-size-h3: 1.5rem;
  /* 24px - Small heading */
  --line-height-heading: 1.2;
  /* Heading line height */
  --line-height-tight: 1.1;
  /* Tight line height */
  --letter-spacing-tight: -0.02em;
  /* Tight letter spacing */
  --letter-spacing-heading: -0.01em;
  /* Heading letter spacing */

  /* Font Weights */
  --font-weight-normal: 400;
  /* Normal weight */
  --font-weight-medium: 500;
  /* Medium weight */
  --font-weight-semibold: 600;
  /* Semibold weight */
  --font-weight-bold: 700;
  /* Bold weight */

  /* Component Typography */
  --hero-title-size: 2rem;
  /* 32px - Hero title */
  --hero-subtitle-size: 1.25rem;
  /* 20px - Hero subtitle */
  --featured-title-size: 1.75rem;
  /* 28px - Featured post title */
  --featured-excerpt-size: 1.125rem;
  /* 18px - Featured post excerpt */
  --post-card-title-size: 1.5rem;
  /* 24px - Post card title */
  --post-excerpt-size: 0.95rem;
  /* 15.2px - Post excerpt */
  --topbar-logo-size: 1.5rem;
  /* 24px - Logo */
  --topbar-link-size: 0.95rem;
  /* 15.2px - Navigation links */
  --category-pill-size: 0.9rem;
  /* 14.4px - Category pills */
  --author-name-size: 0.875rem;
  /* 14px - Author names */
  --font-size-small: 0.875rem;
  /* 14px - Small text */
  --font-size-tiny: 0.8125rem;
  /* 13px - Tiny text */

  /* ==================== SPACING ==================== */
  /* Standard Scale */
  --spacing-xs: 0.5rem;
  /* 8px */
  --spacing-sm: 0.75rem;
  /* 12px */
  --spacing-md: 1rem;
  /* 16px */
  --spacing-lg: 1.5rem;
  /* 24px */
  --spacing-xl: 2rem;
  /* 32px */
  --spacing-2xl: 3rem;
  /* 48px */
  --spacing-3xl: 4rem;
  /* 64px */
  --spacing-4xl: 5rem;
  /* 80px */

  /* Semantic Spacing */
  --heading-margin-bottom: 1rem;
  /* Bottom margin for headings */
  --paragraph-margin-bottom: 1rem;
  /* Bottom margin for paragraphs */
  --section-padding-y: 4rem;
  /* Vertical section padding */
  --section-padding-x: 2rem;
  /* Horizontal section padding */

  /* ==================== LAYOUT ==================== */
  --topbar-height: 64px;
  /* Topbar height */
  --container-max-width: 1024px;
  /* Max container width */
  --content-gap: 3rem;
  /* Gap between content items */
  --grid-gap: 3rem;
  /* Gap in post grid */

  /* ==================== BORDERS & RADIUS ==================== */
  --border-radius: 0.375rem;
  /* 6px - Default radius */
  --border-radius-lg: 0.5rem;
  /* 8px - Large radius */
  --border-radius-full: 9999px;
  /* Full/pill radius */

  /* ==================== EFFECTS ==================== */
  --shadow-sm: 0 4px 6px -1px rgba(0, 0, 0, 0.05);
  /* Small shadow */
  --shadow-focus: 0 0 0 3px rgba(119, 99, 241, 0.15);
  /* Focus ring shadow */
  --transition-speed: 0.2s;
  /* Default transition duration */
  --transition-timing: ease;
  /* Transition timing function */
  --hover-transform: -4px;
  /* Hover lift effect */
  --active-transform: 1px;
  /* Active press effect */
  --blur-amount: 12px;
  /* Backdrop blur amount */
  --topbar-bg-opacity: 0.8;
  /* Topbar background opacity */

  /* ==================== FORM COMPONENTS ==================== */
  --input-padding: 0.5rem 0.75rem;
  /* Input padding */
  --button-padding: 0.5rem 0.875rem;
  /* Button padding */
  --input-border-width: 1px;
  /* Input border width */
  --author-avatar-size: 2rem;
  /* Author avatar size */
}

/* Custom Medium-inspired styling */
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

body {
  font-family: var(--font-family-base);
  line-height: var(--line-height-base);
  color: var(--color-text-primary);
  background: var(--color-background);
}

/* Main Container */
.main-container {
  max-width: var(--container-max-width);
  margin: 0 auto;
  background: var(--color-container);
  border-left: var(--input-border-width) solid var(--color-border);
  border-right: var(--input-border-width) solid var(--color-border);
  min-height: 100vh;
}

/* Typography */
h1 {
  font-size: var(--font-size-h1);
  font-weight: var(--font-weight-bold);
  line-height: var(--line-height-tight);
  letter-spacing: var(--letter-spacing-tight);
}

h2 {
  font-size: var(--font-size-h2);
  font-weight: var(--font-weight-bold);
  line-height: var(--line-height-heading);
  letter-spacing: var(--letter-spacing-heading);
}

h3 {
  font-size: var(--font-size-h3);
  font-weight: var(--font-weight-semibold);
  line-height: 1.3;
}

/* Topbar */
.topbar {
  border-bottom: var(--input-border-width) solid var(--color-border);
  position: sticky;
  top: 0;
  z-index: 50;
  height: var(--topbar-height);
  display: flex;
  align-items: center;
  background: rgba(250, 250, 250, var(--topbar-bg-opacity));
  backdrop-filter: blur(var(--blur-amount));
  -webkit-backdrop-filter: blur(var(--blur-amount));
}

.topbar > div {
  position: relative;
  width: 100%;
}

.topbar .flex {
  padding-left: var(--section-padding-x);
  padding-right: var(--section-padding-x);
}

.logo {
  font-size: var(--topbar-logo-size);
  font-weight: var(--font-weight-bold);
  color: var(--color-text-heading);
  text-decoration: none;
  letter-spacing: var(--letter-spacing-tight);
}

.nav-link {
  color: var(--color-link);
  text-decoration: none;
  font-size: var(--topbar-link-size);
  transition: color var(--transition-speed) var(--transition-timing);
}

.nav-link:hover {
  color: var(--color-link-hover);
}

/* Navigation - Desktop/Mobile */
.nav-desktop {
  display: flex;
  align-items: center;
  gap: 2rem;
}

.nav-mobile-toggle {
  display: none;
  background: none;
  border: none;
  cursor: pointer;
  padding: 0.5rem;
  color: var(--color-text-primary);
}

.hamburger-icon,
.close-icon {
  width: 1.5rem;
  height: 1.5rem;
}

.nav-mobile-menu {
  display: none;
  flex-direction: column;
  position: absolute;
  top: 50px;
  left: 0;
  right: 0;;
  /* padding: var(--spacing-lg) var(--section-padding-x); */
  background: #fff;
  border-top: var(--input-border-width) solid var(--color-border);
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
  z-index: 40;
}

.nav-mobile-menu.active {
  display: flex;
}

.nav-mobile-link {
  padding: var(--spacing-md) 0 var(--spacing-md) var(--section-padding-x);
  color: var(--color-text-primary);
  text-decoration: none;
  font-size: var(--topbar-link-size);
  transition: color var(--transition-speed) var(--transition-timing);
  border-bottom: var(--input-border-width) solid var(--color-border);
}

.nav-mobile-link:hover {
  color: var(--color-link);
}

/* Hero Section */
.hero {
  border-bottom: var(--input-border-width) solid var(--color-border);
  padding: var(--spacing-4xl) var(--section-padding-x);
}

.hero-title {
  font-size: var(--hero-title-size);
  font-weight: var(--font-weight-bold);
  line-height: var(--line-height-tight);
  letter-spacing: -0.03em;
  margin-bottom: var(--heading-margin-bottom);
}

.hero-subtitle {
  font-size: var(--hero-subtitle-size);
  color: var(--color-text-secondary);
  line-height: 1.5;
}

/* Category Header */
.category-header {
  border-bottom: var(--input-border-width) solid var(--color-border);
  padding: var(--spacing-3xl) var(--section-padding-x);
  text-align: center;
}

.category-title {
  font-size: var(--hero-title-size);
  font-weight: var(--font-weight-bold);
  line-height: var(--line-height-tight);
  letter-spacing: var(--letter-spacing-tight);
  margin-bottom: var(--spacing-sm);
}

.category-description {
  font-size: var(--font-size-base);
  color: var(--color-text-secondary);
  line-height: var(--line-height-base);
  max-width: 600px;
  margin: 0 auto;
}

.category-post-count {
  font-size: var(--font-size-small);
  color: var(--color-text-secondary);
  margin-top: var(--spacing-sm);
}

/* Newsletter Form */
.newsletter-form h3 {
  margin-bottom: var(--spacing-sm);
}

.newsletter-form p {
  margin-bottom: var(--paragraph-margin-bottom);
}

.newsletter-input {
  width: 100%;
  padding: var(--input-padding);
  background: #fff;
  border: var(--input-border-width) solid var(--color-border-input);
  border-radius: var(--border-radius);
  font-size: var(--font-size-tiny);
  margin-bottom: var(--spacing-sm);
  transition: border-color var(--transition-speed) var(--transition-timing),
    box-shadow var(--transition-speed) var(--transition-timing);
}

.newsletter-input:focus {
  outline: none;
  border-color: var(--color-cta);
  box-shadow: var(--shadow-focus);
}

.newsletter-button {
  width: 100%;
  padding: var(--button-padding);
  background: var(--color-cta);
  color: var(--color-cta-text);
  border: none;
  border-radius: var(--border-radius);
  font-size: var(--font-size-tiny);
  font-weight: var(--font-weight-semibold);
  cursor: pointer;
  transition: background var(--transition-speed) var(--transition-timing),
    transform 0.1s var(--transition-timing);
}

.newsletter-button:hover {
  background: var(--color-cta-hover);
}

.newsletter-button:active {
  transform: translateY(var(--active-transform));
}

/* Categories */
.categories-bar {
  border-bottom: var(--input-border-width) solid var(--color-border);
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
  scrollbar-width: none;
  padding: var(--section-padding-x);
}

.categories-bar::-webkit-scrollbar {
  display: none;
}

.category-pill {
  display: inline-block;
  padding: var(--spacing-xs) var(--spacing-lg);
  margin-right: var(--spacing-xs);
  background: var(--color-pill);
  border-radius: var(--border-radius-full);
  font-size: var(--category-pill-size);
  font-weight: var(--font-weight-medium);
  color: var(--color-text-secondary);
  text-decoration: none;
  white-space: nowrap;
  transition: background var(--transition-speed) var(--transition-timing),
    color var(--transition-speed) var(--transition-timing);
}

.category-pill:hover {
  background: var(--color-pill-hover);
  color: var(--color-text-primary);
}

.category-pill.active {
  background: var(--color-active-pill);
  color: var(--color-cta-text);
}

/* Featured Post */
.featured-post-section {
  padding: var(--section-padding-y) var(--section-padding-x);
}

.featured-post {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--content-gap);
  align-items: center;
  text-decoration: none;
  color: inherit;
  transition: transform var(--transition-speed) var(--transition-timing);
}

.featured-post:hover {
  transform: translateY(var(--hover-transform));
}

.featured-image {
  width: 100%;
  aspect-ratio: 16/9;
  object-fit: cover;
  border-radius: var(--border-radius-lg);
}

.featured-content h2 {
  font-size: var(--featured-title-size);
  margin-bottom: var(--heading-margin-bottom);
  line-height: var(--line-height-heading);
}

.featured-content .excerpt {
  font-size: var(--featured-excerpt-size);
  color: var(--color-text-secondary);
  line-height: var(--line-height-base);
  margin-bottom: var(--spacing-lg);
}

/* Post Card */
.post-card {
  display: flex;
  flex-direction: column;
  text-decoration: none;
  color: inherit;
  transition: transform var(--transition-speed) var(--transition-timing);
}

.post-card:hover {
  transform: translateY(var(--hover-transform));
}

.post-card-image {
  width: 100%;
  aspect-ratio: 16/9;
  object-fit: cover;
  border-radius: var(--border-radius-lg);
  margin-bottom: var(--spacing-md);
}

.post-card-title {
  font-size: var(--post-card-title-size);
  font-weight: var(--font-weight-bold);
  margin-bottom: var(--spacing-xs);
  line-height: 1.3;
}

.post-card-excerpt {
  color: var(--color-text-secondary);
  font-size: var(--post-excerpt-size);
  line-height: 1.5;
  margin-bottom: var(--spacing-md);
}

/* Author Info */
.author-info {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-md);
}

.author-avatars {
  display: flex;
  margin-right: var(--spacing-xs);
}

.author-avatar {
  width: var(--author-avatar-size);
  height: var(--author-avatar-size);
  border-radius: 50%;
  border: 2px solid var(--color-container);
  margin-left: calc(var(--spacing-xs) * -1);
}

.author-avatar:first-child {
  margin-left: 0;
}

.author-names {
  font-size: var(--author-name-size);
  color: var(--color-text-primary);
  font-weight: var(--font-weight-medium);
}

.post-date {
  font-size: var(--font-size-small);
  color: var(--color-text-secondary);
}

/* Posts Grid Section */
.posts-grid-section {
  padding: var(--section-padding-x) var(--section-padding-x) var(--spacing-4xl) var(--section-padding-x);
}

/* ==================== POST PAGE STYLES ==================== */

/* Post Article */
.post-article {
  max-width: 720px;
  margin: 0 auto;
  padding: var(--spacing-3xl) var(--section-padding-x);
}

/* Post Header */
.post-header {
  margin-bottom: var(--spacing-3xl);
}

.post-category-badge {
  margin-bottom: var(--spacing-md);
}

.post-title {
  font-size: 2.5rem;
  font-weight: var(--font-weight-bold);
  line-height: var(--line-height-tight);
  letter-spacing: var(--letter-spacing-tight);
  margin-bottom: var(--spacing-lg);
  color: var(--color-text-heading);
}

.post-meta {
  padding-top: var(--spacing-lg);
  border-top: var(--input-border-width) solid var(--color-border);
}

/* Post Featured Image */
.post-featured-image {
  margin-bottom: var(--spacing-3xl);
  border-radius: var(--border-radius-lg);
  overflow: hidden;
}

.post-featured-image img {
  width: 100%;
  height: auto;
  display: block;
}

/* Post Content */
.post-content {
  font-size: 1.125rem;
  line-height: 1.8;
  color: var(--color-text-primary);
}

.post-content p {
  margin-bottom: var(--spacing-xl);
}

.post-content a {
  color: var(--color-link);
  text-decoration: underline;
  transition: color var(--transition-speed) var(--transition-timing);
}

.post-content a:hover {
  color: var(--color-link-hover);
}

.post-lead {
  font-size: 1.25rem;
  line-height: 1.7;
  color: var(--color-text-secondary);
  margin-bottom: var(--spacing-3xl);
  font-weight: var(--font-weight-normal);
}

.post-content h2 {
  font-size: 1.875rem;
  font-weight: var(--font-weight-bold);
  line-height: var(--line-height-heading);
  letter-spacing: var(--letter-spacing-heading);
  margin-top: var(--spacing-3xl);
  margin-bottom: var(--spacing-lg);
  color: var(--color-text-heading);
}

.post-content h3 {
  font-size: 1.5rem;
  font-weight: var(--font-weight-semibold);
  line-height: 1.3;
  margin-top: var(--spacing-xl);
  margin-bottom: var(--spacing-md);
  color: var(--color-text-heading);
}

.post-quote {
  border-left: 4px solid var(--color-link);
  padding-left: var(--spacing-xl);
  margin: var(--spacing-3xl) 0;
  font-size: 1.25rem;
  line-height: 1.7;
  font-style: italic;
  color: var(--color-text-secondary);
}

.post-quote cite {
  display: block;
  margin-top: var(--spacing-md);
  font-size: 1rem;
  font-style: normal;
  color: var(--color-text-secondary);
}

.post-list {
  margin: var(--spacing-xl) 0;
  padding-left: var(--spacing-xl);
  list-style: disc;
}

.post-list li {
  margin-bottom: var(--spacing-md);
  padding-left: var(--spacing-xs);
}

.post-list li strong {
  font-weight: var(--font-weight-semibold);
  color: var(--color-text-heading);
}

.post-conclusion {
  margin-top: var(--spacing-3xl);
  padding: var(--spacing-xl);
  background: var(--color-pill);
  border-radius: var(--border-radius-lg);
  border-left: 4px solid var(--color-link);
}

.post-conclusion p {
  margin-bottom: 0;
  font-size: 1rem;
  line-height: 1.6;
}

/* Code Blocks */
.post-content pre {
  background: #1e1e1e;
  color: #d4d4d4;
  padding: var(--spacing-lg);
  border-radius: var(--border-radius);
  overflow-x: auto;
  margin: var(--spacing-xl) 0;
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
  font-size: 0.875rem;
  line-height: 1.6;
}

.post-content code {
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
  font-size: 0.875rem;
}

.post-content pre code {
  background: none;
  padding: 0;
  color: inherit;
}

.post-content :not(pre) > code {
  background: #f4f4f4;
  color: #e01e5a;
  padding: 0.125rem 0.375rem;
  border-radius: 3px;
  font-size: 0.875em;
}

/* Post Tags */
.post-tags {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--spacing-sm);
  margin-top: var(--spacing-3xl);
  padding-top: var(--spacing-xl);
  border-top: var(--input-border-width) solid var(--color-border);
}

.tag-label {
  font-size: var(--font-size-small);
  font-weight: var(--font-weight-semibold);
  color: var(--color-text-secondary);
}

.post-tag {
  display: inline-block;
  padding: var(--spacing-xs) var(--spacing-md);
  background: var(--color-pill);
  border-radius: var(--border-radius-full);
  font-size: var(--font-size-small);
  font-weight: var(--font-weight-medium);
  color: var(--color-text-secondary);
  text-decoration: none;
  transition: background var(--transition-speed) var(--transition-timing),
    color var(--transition-speed) var(--transition-timing);
}

.post-tag:hover {
  background: var(--color-pill-hover);
  color: var(--color-text-primary);
}

/* Author Bio */
.author-bio {
  display: flex;
  gap: var(--spacing-lg);
  margin-top: var(--spacing-3xl);
  padding: var(--spacing-xl);
  background: var(--color-pill);
  border-radius: var(--border-radius-lg);
}

.author-bio-avatars {
  flex-shrink: 0;
}

.author-bio-avatar {
  width: 4rem;
  height: 4rem;
  border-radius: 50%;
  border: 2px solid var(--color-container);
}

.author-bio-content {
  flex: 1;
}

.author-bio-name {
  font-size: 1.25rem;
  font-weight: var(--font-weight-bold);
  margin-bottom: var(--spacing-xs);
  color: var(--color-text-heading);
}

.author-bio-description {
  font-size: 0.95rem;
  line-height: 1.6;
  color: var(--color-text-secondary);
  margin: 0;
}

/* Related Posts Section */
.related-posts-section {
  padding: var(--spacing-3xl) var(--section-padding-x);
  border-top: var(--input-border-width) solid var(--color-border);
  background: var(--color-pill);
}

.related-posts-title {
  font-size: 1.875rem;
  font-weight: var(--font-weight-bold);
  margin-bottom: var(--spacing-xl);
  text-align: center;
  color: var(--color-text-heading);
}

/* Pagination */
.pagination {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-xl) var(--section-padding-x);
}

.pagination-button {
  padding: var(--spacing-xs) var(--spacing-md);
  border: var(--input-border-width) solid var(--color-border);
  background: var(--color-container);
  color: var(--color-text-primary);
  text-decoration: none;
  border-radius: var(--border-radius);
  font-size: var(--font-size-small);
  font-weight: var(--font-weight-medium);
  transition: background var(--transition-speed) var(--transition-timing),
    border-color var(--transition-speed) var(--transition-timing);
}

.pagination-button:hover {
  background: var(--color-pill);
  border-color: var(--color-border-input);
}

.pagination-button.active {
  background: var(--color-active-pill);
  color: var(--color-cta-text);
  border-color: var(--color-active-pill);
}

.pagination-button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.pagination-ellipsis {
  color: var(--color-text-secondary);
  padding: 0 var(--spacing-xs);
}

/* Footer */
.footer {
  border-top: var(--input-border-width) solid var(--color-border);
  padding: var(--spacing-2xl) var(--section-padding-x);
}

.footer-content {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--spacing-xl);
}

.footer-left {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.footer-links {
  display: flex;
  gap: var(--spacing-xl);
  align-items: center;
}

.footer-link {
  color: var(--color-link);
  text-decoration: none;
  font-size: var(--category-pill-size);
  transition: color var(--transition-speed) var(--transition-timing);
}

.footer-link:hover {
  color: var(--color-link-hover);
}

.social-links {
  display: flex;
  gap: var(--spacing-md);
}

.social-icon {
  width: var(--spacing-lg);
  height: var(--spacing-lg);
  color: var(--color-link);
  transition: color var(--transition-speed) var(--transition-timing);
}

.social-icon:hover {
  color: var(--color-link-hover);
}

/* ==================== RESPONSIVE ==================== */
@media (max-width: 768px) {
  :root {
    /* Override variables for mobile */
    --hero-title-size: 1.25rem;
    /* Smaller hero title on mobile */
    --hero-subtitle-size: 0.875rem;
    /* Smaller hero subtitle on mobile */
    --featured-title-size: 1.25rem;
    /* Smaller featured title on mobile */
    --featured-excerpt-size: 0.875rem;
    /* Smaller featured excerpt on mobile */
    --font-size-h1: 2rem;
    /* Smaller H1 on mobile */
    --section-padding-x: 1rem;
    /* Reduced horizontal padding */
    --content-gap: 2rem;
    /* Reduced content gap */
  }

  /* Mobile Navigation */
  .nav-desktop {
    display: none;
  }

  .nav-mobile-toggle {
    display: block;
  }

  /* Mobile Newsletter Form */
  .newsletter-form h3 {
    font-size: 1rem;
  }

  /* Mobile Category Pills */
  .category-pill {
    padding: 0.375rem 0.875rem;
    /* Reduced padding on mobile: 6px 14px */
    font-size: 0.875rem;
    /* Slightly smaller text */
  }

  /* Remove blur effect on mobile for better performance */
  .topbar {
    backdrop-filter: none;
    -webkit-backdrop-filter: none;
    background: rgba(250, 250, 250, 1);
    /* Full opacity background */
  }

  /* Reduce pagination items on mobile */
  .pagination {
    gap: 0.25rem;
    /* Tighter spacing */
  }

  .pagination-button {
    padding: 0.375rem 0.625rem;
    /* Smaller padding */
    font-size: 0.875rem;
    /* Smaller text */
  }

  /* Hide middle page numbers on mobile, keep first, active, last and nav buttons */
  .pagination-button:not(:first-child):not(:last-child):not(.active):nth-child(n+4):nth-child(-n+6) {
    display: none;
  }

  .hero {
    padding: var(--spacing-2xl) var(--section-padding-x);
  }

  .category-header {
    padding: var(--spacing-2xl) var(--section-padding-x);
  }

  .categories-bar {
    padding: var(--spacing-lg) var(--section-padding-x);
  }

  .featured-post-section {
    padding: var(--spacing-2xl) var(--section-padding-x);
  }

  .featured-post {
    grid-template-columns: 1fr;
    gap: var(--spacing-xl);
  }

  .posts-grid-section {
    padding: var(--spacing-lg) var(--section-padding-x) var(--section-padding-y) var(--section-padding-x);
  }

  .footer {
    padding: var(--spacing-xl) var(--section-padding-x);
  }

  /* Post page mobile styles */
  .post-article {
    padding: var(--spacing-xl) var(--section-padding-x);
  }

  .post-title {
    font-size: 1.875rem;
  }

  .post-content {
    font-size: 1rem;
  }

  .post-lead {
    font-size: 1.125rem;
  }

  .post-content h2 {
    font-size: 1.5rem;
  }

  .post-content h3 {
    font-size: 1.25rem;
  }

  .post-quote {
    font-size: 1.125rem;
    padding-left: var(--spacing-md);
  }

  .author-bio {
    flex-direction: column;
    text-align: center;
  }

  .author-bio-avatar {
    margin: 0 auto;
  }

  .related-posts-section {
    padding: var(--spacing-xl) var(--section-padding-x);
  }

  .related-posts-title {
    font-size: 1.5rem;
  }
}`,

    'scripts.js': `// ==================== CONFIGURATION ====================
// Dynamically configured from workspace settings
const NOTIFUSE_CONFIG = {
  domain: '{{ base_url }}',
  workspaceId: '{{ workspace.id }}',
  listIds: [
    {%- for list in public_lists -%}
      '{{ list.id }}'{% unless forloop.last %},{% endunless %}
    {%- endfor -%}
  ],
  categories: [
    {%- for cat in categories -%}
      {
        id: "{{ cat.id }}",
        slug: "{{ cat.slug }}",
        name: "{{ cat.name | escape }}",
        description: {% if cat.description %}"{{ cat.description | escape }}"{% else %}null{% endif %}
      }{% unless forloop.last %},{% endunless %}
    {%- endfor -%}
  ]
};
// =======================================================

/**
 * Subscribe a contact to newsletter lists via Notifuse API
 * @param {string} email - Contact email address
 * @param {string} firstName - Contact first name (optional)
 * @returns {Promise<{success: boolean, data?: any, error?: string}>}
 */
async function subscribeToNewsletter(email, firstName = null) {
  try {
    const response = await fetch(\`\${NOTIFUSE_CONFIG.domain}/subscribe\`, {
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
}

/**
 * Show a message to the user
 * @param {HTMLElement} form - The form element
 * @param {string} message - Message to display
 * @param {boolean} isError - Whether this is an error message
 */
function showMessage(form, message, isError = false) {
  // Remove any existing message
  const existingMessage = form.querySelector('.newsletter-message');
  if (existingMessage) {
    existingMessage.remove();
  }

  // Create message element
  const messageEl = document.createElement('div');
  messageEl.className = 'newsletter-message';
  messageEl.textContent = message;
  messageEl.style.cssText = \`
    margin-top: 0.75rem;
    padding: 0.75rem;
    border-radius: 0.375rem;
    font-size: 0.875rem;
    \${isError
      ? 'background-color: #fee; color: #c00; border: 1px solid #fcc;'
      : 'background-color: #efe; color: #060; border: 1px solid #cfc;'
    }
  \`;

  form.appendChild(messageEl);

  // Auto-remove success messages after 5 seconds
  if (!isError) {
    setTimeout(() => {
      messageEl.remove();
    }, 5000);
  }
}

/**
 * Set loading state on submit button
 * @param {HTMLButtonElement} button - The submit button
 * @param {boolean} isLoading - Loading state
 */
function setButtonLoading(button, isLoading) {
  if (isLoading) {
    button.dataset.originalText = button.textContent;
    button.textContent = 'Subscribing...';
    button.disabled = true;
    button.style.opacity = '0.7';
    button.style.cursor = 'not-allowed';
  } else {
    button.textContent = button.dataset.originalText || 'Subscribe';
    button.disabled = false;
    button.style.opacity = '1';
    button.style.cursor = 'pointer';
  }
}

/**
 * Validate email format
 * @param {string} email - Email to validate
 * @returns {boolean}
 */
function isValidEmail(email) {
  const emailRegex = /^[^\\s@]+@[^\\s@]+\\.[^\\s@]+$/;
  return emailRegex.test(email);
}

/**
 * Initialize newsletter form handling
 */
function initNewsletterForms() {
  const forms = document.querySelectorAll('.newsletter-form form');

  forms.forEach(form => {
    form.addEventListener('submit', async function (e) {
      e.preventDefault();

      const emailInput = form.querySelector('input[type="email"]');
      const submitButton = form.querySelector('button[type="submit"]');
      const email = emailInput.value.trim();

      // Validate email
      if (!email) {
        showMessage(form, 'Please enter your email address.', true);
        return;
      }

      if (!isValidEmail(email)) {
        showMessage(form, 'Please enter a valid email address.', true);
        return;
      }

      // Set loading state
      setButtonLoading(submitButton, true);

      // Subscribe
      const result = await subscribeToNewsletter(email);

      // Reset loading state
      setButtonLoading(submitButton, false);

      if (result.success) {
        showMessage(form, 'Thank you for subscribing! Please check your email to confirm your subscription.');
        emailInput.value = ''; // Clear the input
      } else {
        showMessage(form, result.error || 'Something went wrong. Please try again.', true);
      }
    });
  });
}

/**
 * Initialize mobile navigation menu toggle
 */
function initMobileNav() {
  const toggleButton = document.querySelector('.nav-mobile-toggle');
  const mobileMenu = document.querySelector('.nav-mobile-menu');
  const hamburgerIcon = toggleButton?.querySelector('.hamburger-icon');
  const closeIcon = toggleButton?.querySelector('.close-icon');

  if (toggleButton && mobileMenu && hamburgerIcon && closeIcon) {
    // Toggle menu and icons
    const toggleMenu = (isOpen) => {
      if (isOpen) {
        mobileMenu.classList.add('active');
        hamburgerIcon.style.display = 'none';
        closeIcon.style.display = 'block';
      } else {
        mobileMenu.classList.remove('active');
        hamburgerIcon.style.display = 'block';
        closeIcon.style.display = 'none';
      }
    };

    toggleButton.addEventListener('click', function () {
      const isOpen = !mobileMenu.classList.contains('active');
      toggleMenu(isOpen);
    });

    // Close mobile menu when clicking a link
    const mobileLinks = mobileMenu.querySelectorAll('.nav-mobile-link');
    mobileLinks.forEach(link => {
      link.addEventListener('click', function () {
        toggleMenu(false);
      });
    });

    // Close mobile menu when clicking outside
    document.addEventListener('click', function (e) {
      if (!toggleButton.contains(e.target) && !mobileMenu.contains(e.target)) {
        toggleMenu(false);
      }
    });
  }
}

// Initialize when DOM is ready
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', function () {
    initNewsletterForms();
    initMobileNav();
  });
} else {
  initNewsletterForms();
  initMobileNav();
}`
  }
}

export const THEME_PRESETS: ThemePreset[] = [defaultTheme]
