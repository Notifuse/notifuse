import { BlogThemeFiles } from '../../services/api/blog'

export interface ThemePreset {
  id: string
  name: string
  description: string
  placeholderColor: string
  files: BlogThemeFiles
}

// Default Theme - Basic structure with header/footer imports
const defaultTheme: ThemePreset = {
  id: 'default',
  name: 'Default',
  description: 'A default theme with a clean slate and basic structure',
  placeholderColor: '#f5f5f5',
  files: {
    'home.liquid': `todo`,

    'category.liquid': `todo`,

    'post.liquid': `todo`,

    'header.liquid': `todo`,

    'footer.liquid': `todo`,

    'shared.liquid': `{%- comment -%}
  ========================================
  Shared Widgets Library
  ========================================
  
  This file contains reusable widgets for your blog.
  
  Usage Examples:
  
  1. Render specific widget:
     {% assign widget = 'newsletter' %}
     {% include 'shared' %}
  
  2. Render all widgets (default):
     {% include 'shared' %}
  
  3. Add your own widgets:
     - Copy an existing widget block
     - Change the widget name
     - Customize the HTML/CSS
  
  Available Widgets:
  - newsletter: Email subscription form
  - categories: Blog categories list
  - authors: Display post authors with avatars
  
{%- endcomment -%}

{%- if widget == 'newsletter' -%}
  todo

{%- elsif widget == 'categories' -%}
  todo

{%- elsif widget == 'authors' -%}
  todo

{%- else -%}
  {%- comment -%}
    Default behavior: render all widgets when no specific widget requested
  {%- endcomment -%}
  {% assign widget = 'newsletter' %}
  {% include 'shared' %}

{%- endif -%}`,

    'styles.css': `todo`
  }
}

export const THEME_PRESETS: ThemePreset[] = [defaultTheme]
