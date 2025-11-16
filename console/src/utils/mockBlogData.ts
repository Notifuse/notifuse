export interface MockBlogData {
  // Workspace info (required in backend BlogTemplateDataRequest)
  workspace: {
    id: string
    name: string
  }
  // Public lists for newsletter subscription (required in backend)
  public_lists: Array<{
    id: string
    name: string
    description: string
  }>
  // Blog metadata (for preview only, not sent to backend)
  blog: {
    title: string
    description: string
    logo_url?: string
    icon_url?: string
  }
  seo?: {
    meta_title: string
    meta_description: string
    og_title: string
    og_description: string
    og_image: string
    canonical_url: string
    keywords: string[]
  }
  styling?: any // EditorStyleConfig from workspace settings
  // Posts array (for home/category page listings)
  posts: Array<{
    id: string
    title: string
    slug: string
    excerpt: string
    content: string
    featured_image_url: string
    category_id: string
    category_slug: string
    published_at: string
    reading_time_minutes: number
    authors: Array<{ name: string; avatar_url?: string }>
  }>
  // Categories array (for navigation)
  categories: Array<{
    id: string
    name: string
    slug: string
    description: string
  }>
  // Current post (for post pages only, matches backend BlogTemplateDataRequest)
  post?: any
  // Current category (for category pages only, matches backend BlogTemplateDataRequest)
  category?: any
  // Additional helper fields
  previous_post?: any
  next_post?: any
  current_year: number
  page_title?: string
  page_description?: string
  current_url?: string
}

export const MOCK_BLOG_DATA: MockBlogData = {
  // Workspace (matches backend BlogTemplateDataRequest)
  workspace: {
    id: 'workspace-123',
    name: 'My Workspace'
  },
  // Public lists (matches backend BlogTemplateDataRequest)
  public_lists: [
    {
      id: 'list-1',
      name: 'Weekly Newsletter',
      description: 'Get our latest posts every week'
    },
    {
      id: 'list-2',
      name: 'Product Updates',
      description: 'New features and improvements'
    }
  ],
  // Blog metadata
  blog: {
    title: 'My Awesome Blog',
    description: 'Thoughts, ideas, and stories from our team'
  },
  seo: {
    meta_title: 'My Awesome Blog - Insights & Stories',
    meta_description:
      'Explore our latest thoughts, ideas, and stories on web development, technology, and design.',
    og_title: 'My Awesome Blog',
    og_description:
      'Join us as we share insights about web development, AI, and modern design practices.',
    og_image: 'https://images.unsplash.com/photo-1499750310107-5fef28a66643?w=1200',
    canonical_url: 'https://example.com',
    keywords: ['web development', 'technology', 'design', 'tutorials', 'blog']
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
  },
  posts: [
    {
      id: 'post-1',
      title: 'Complete Style Guide & Kitchen Sink',
      slug: 'style-guide-kitchen-sink',
      category_id: 'cat-1',
      excerpt:
        'A comprehensive showcase of all content blocks, styling options, and typography elements available in our blog theme system.',
      content: `<p>This post demonstrates every content block type and styling option available in the theme editor. Use this as a reference to see how your theme handles different content types.</p>

<h2>Section Heading (H2)</h2>
<p>Every blog post needs well-structured sections. This H2 heading marks a major section division. Notice the spacing above and below this heading, as well as the font size and weight.</p>

<h3>Subsection Heading (H3)</h3>
<p>H3 headings are perfect for subsections within your content. They provide hierarchy without overwhelming the reader. The font size should be noticeably smaller than H2 but still prominent.</p>

<p>Here's another paragraph with some <strong>bold text</strong>, <em>italic text</em>, and even <code>inline code</code> to show how inline formatting works. You can also include <a href="https://example.com">hyperlinks</a> that should have their own distinctive styling.</p>

<hr>

<h2>Blockquotes</h2>
<p>Blockquotes are used for quotations or to highlight important passages:</p>

<blockquote>
<p>This is a blockquote. It should have distinctive styling to set it apart from regular paragraphs. Blockquotes often use different colors, margins, or even border decorations.</p>
</blockquote>

<p>And here's regular text that follows the blockquote, demonstrating proper spacing between different block types.</p>

<hr>

<h2>Code Blocks</h2>
<p>For technical content, code blocks are essential. Here's an example with JavaScript:</p>

<pre><code class="language-javascript">function greet(name) {
  return \`Hello, \${name}!\`;
}

const message = greet('World');
console.log(message); // Output: Hello, World!</code></pre>
<p style="font-size: 14px; color: #6b7280; margin-top: -8px;">Caption: A simple JavaScript greeting function</p>

<p>Code blocks should have distinct background colors and use monospace fonts for readability.</p>

<hr>

<h2>Lists</h2>
<p>Both ordered and unordered lists are common in blog posts.</p>

<h3>Unordered List</h3>
<ul>
<li>First item in an unordered list</li>
<li>Second item with more text to show how wrapping works</li>
<li>Third item</li>
<li>Fourth item with a nested list:
<ul>
<li>Nested item one</li>
<li>Nested item two</li>
<li>Nested item three</li>
</ul>
</li>
<li>Fifth item back at the original level</li>
</ul>

<h3>Ordered List</h3>
<ol>
<li>First step in a process</li>
<li>Second step with detailed instructions</li>
<li>Third step</li>
<li>Fourth step with substeps:
<ol>
<li>Substep A</li>
<li>Substep B</li>
</ol>
</li>
<li>Final step</li>
</ol>

<hr>

<h2>Images</h2>
<p>Images are crucial for visual storytelling. Here's an example:</p>

<img src="https://images.unsplash.com/photo-1498050108023-c5249f4df085?w=800" alt="Person typing on laptop" data-caption="A developer working on a laptop" data-show-caption="true" />
<p style="font-size: 14px; color: #6b7280; text-align: center; margin-top: 8px;">Caption: A developer working on a laptop</p>

<p>Notice how images should have proper spacing above and below, and captions should be visually distinct from body text.</p>

<hr>

<h2>Mixed Content</h2>
<p>Real-world blog posts combine multiple content types. Here's a paragraph followed by a list of key takeaways:</p>

<ul>
<li>All headings (H1, H2, H3) should have clear hierarchy</li>
<li>Paragraphs need comfortable line height and spacing</li>
<li>Code blocks require monospace fonts</li>
<li>Links should be easily distinguishable</li>
<li>Images need proper captions and spacing</li>
</ul>

<p>And here's more text after the list to show proper spacing. The gap between different elements should feel natural and not too cramped or too spacious.</p>

<h3>Typography Details</h3>
<p>Pay attention to these subtle but important details:</p>

<ol>
<li><strong>Line height:</strong> Should be comfortable for reading (typically 1.5-1.8)</li>
<li><strong>Paragraph spacing:</strong> Creates breathing room between thoughts</li>
<li><strong>Font sizes:</strong> Should scale proportionally across heading levels</li>
<li><strong>Color contrast:</strong> Text must be readable against backgrounds</li>
</ol>

<blockquote>
<p>Good typography is invisible. Bad typography is everywhere.</p>
</blockquote>

<h2>Conclusion</h2>
<p>This style guide demonstrates all the essential content blocks you'll use in your blog posts. Each element should have thoughtful styling that contributes to an excellent reading experience. Whether you're writing tutorials, articles, or documentation, these building blocks form the foundation of great content.</p>

<p>Use this page as a reference when customizing your theme. Make sure every element looks polished and works well together to create a cohesive, professional appearance.</p>`,
      featured_image_url: 'https://images.unsplash.com/photo-1498050108023-c5249f4df085?w=800',
      category_slug: 'tutorials',
      published_at: 'March 15, 2024',
      reading_time_minutes: 8,
      authors: [{ name: 'Jane Doe' }]
    },
    {
      id: 'post-2',
      title: 'The Future of Artificial Intelligence',
      slug: 'future-of-ai',
      category_id: 'cat-2',
      excerpt: 'Exploring how AI will transform industries and daily life in the coming years.',
      content: `<p>Artificial Intelligence is rapidly evolving, and its impact on society is becoming more profound each day.</p>`,
      featured_image_url: 'https://images.unsplash.com/photo-1677442136019-21780ecad995?w=800',
      category_slug: 'technology',
      published_at: 'March 12, 2024',
      reading_time_minutes: 8,
      authors: [{ name: 'John Smith' }]
    },
    {
      id: 'post-3',
      title: 'Design Principles for Modern Websites',
      slug: 'design-principles-modern-websites',
      category_id: 'cat-3',
      excerpt: 'Essential design principles that will make your website stand out in 2024.',
      content: `<p>Good design is not just about aesthetics—it's about creating an intuitive, accessible experience for all users.</p>`,
      featured_image_url: 'https://images.unsplash.com/photo-1558655146-9f40138edfeb?w=800',
      category_slug: 'design',
      published_at: 'March 10, 2024',
      reading_time_minutes: 6,
      authors: [{ name: 'Sarah Johnson' }]
    }
  ],
  categories: [
    {
      id: 'cat-1',
      name: 'Tutorials',
      slug: 'tutorials',
      description: 'Step-by-step guides and how-tos'
    },
    {
      id: 'cat-2',
      name: 'Technology',
      slug: 'technology',
      description: 'Latest tech news and trends'
    },
    {
      id: 'cat-3',
      name: 'Design',
      slug: 'design',
      description: 'Design inspiration and best practices'
    }
  ],
  current_year: new Date().getFullYear()
}

// Mock data for specific views
export function getMockDataForView(view: 'home' | 'category' | 'post'): MockBlogData {
  const baseData = { ...MOCK_BLOG_DATA }

  if (view === 'category') {
    // Match backend BlogTemplateDataRequest.Category field
    baseData.category = baseData.categories[0]
    baseData.posts = baseData.posts.filter((p) => p.category_slug === 'tutorials')
    baseData.page_title = `${baseData.categories[0].name} - ${baseData.blog.title}`
    baseData.page_description = baseData.categories[0].description
    baseData.current_url = `https://example.com/${baseData.categories[0].slug}`
  }

  if (view === 'post') {
    // Match backend BlogTemplateDataRequest.Post field
    const postData = baseData.posts[0]
    baseData.post = postData
    baseData.previous_post = baseData.posts[2]
    baseData.next_post = baseData.posts[1]
    baseData.page_title = `${postData.title} - ${baseData.blog.title}`
    baseData.page_description = postData.excerpt
    baseData.current_url = `https://example.com/${postData.category_slug}/${postData.slug}`
  }

  if (view === 'home') {
    baseData.current_url = 'https://example.com'
  }

  return baseData
}

// Helper to get mock data with empty public lists (for testing empty state)
export function getMockDataWithEmptyLists(view: 'home' | 'category' | 'post'): MockBlogData {
  const data = getMockDataForView(view)
  data.public_lists = []
  return data
}
