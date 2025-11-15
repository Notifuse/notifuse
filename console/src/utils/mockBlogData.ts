export interface MockBlogData {
  blog: {
    title: string
    description: string
  }
  seo: {
    meta_title: string
    meta_description: string
    og_title: string
    og_description: string
    og_image: string
    canonical_url: string
    keywords: string[]
  }
  styling?: any // EditorStyleConfig from workspace settings
  posts: Array<{
    title: string
    slug: string
    excerpt: string
    content: string
    featured_image_url: string
    category_slug: string
    published_at: string
    reading_time_minutes: number
    authors: Array<{ name: string; avatar_url?: string }>
  }>
  categories: Array<{
    name: string
    slug: string
    description: string
  }>
  currentPost?: any
  currentCategory?: any
  previous_post?: any
  next_post?: any
  current_year: number
  page_title?: string
  page_description?: string
  current_url?: string
}

export const MOCK_BLOG_DATA: MockBlogData = {
  blog: {
    title: 'My Awesome Blog',
    description: 'Thoughts, ideas, and stories from our team'
  },
  seo: {
    meta_title: 'My Awesome Blog - Insights & Stories',
    meta_description: 'Explore our latest thoughts, ideas, and stories on web development, technology, and design.',
    og_title: 'My Awesome Blog',
    og_description: 'Join us as we share insights about web development, AI, and modern design practices.',
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
      title: 'Getting Started with Web Development',
      slug: 'getting-started-web-development',
      excerpt:
        'Learn the basics of HTML, CSS, and JavaScript to kickstart your web development journey.',
      content: `<p>Web development is an exciting field that combines creativity with technical skills. In this post, we'll explore the fundamentals.</p>
      
<h2>The Building Blocks</h2>
<p>Every website is built using three core technologies: HTML for structure, CSS for styling, and JavaScript for interactivity.</p>

<h3>HTML - The Structure</h3>
<p>HTML provides the skeleton of your webpage. It defines headings, paragraphs, links, and more.</p>

<h3>CSS - The Style</h3>
<p>CSS makes your website beautiful. It controls colors, layouts, fonts, and animations.</p>

<h3>JavaScript - The Behavior</h3>
<p>JavaScript brings your site to life with dynamic interactions and real-time updates.</p>

<h2>Next Steps</h2>
<p>Start with HTML, then move to CSS, and finally learn JavaScript. Practice by building real projects!</p>`,
      featured_image_url: 'https://images.unsplash.com/photo-1498050108023-c5249f4df085?w=800',
      category_slug: 'tutorials',
      published_at: 'March 15, 2024',
      reading_time_minutes: 5,
      authors: [{ name: 'Jane Doe' }]
    },
    {
      title: 'The Future of Artificial Intelligence',
      slug: 'future-of-ai',
      excerpt:
        'Exploring how AI will transform industries and daily life in the coming years.',
      content: `<p>Artificial Intelligence is rapidly evolving, and its impact on society is becoming more profound each day.</p>`,
      featured_image_url: 'https://images.unsplash.com/photo-1677442136019-21780ecad995?w=800',
      category_slug: 'technology',
      published_at: 'March 12, 2024',
      reading_time_minutes: 8,
      authors: [{ name: 'John Smith' }]
    },
    {
      title: 'Design Principles for Modern Websites',
      slug: 'design-principles-modern-websites',
      excerpt:
        'Essential design principles that will make your website stand out in 2024.',
      content: `<p>Good design is not just about aesthetics—it's about creating an intuitive, accessible experience for all users.</p>`,
      featured_image_url: 'https://images.unsplash.com/photo-1558655146-9f40138edfeb?w=800',
      category_slug: 'design',
      published_at: 'March 10, 2024',
      reading_time_minutes: 6,
      authors: [{ name: 'Sarah Johnson' }]
    }
  ],
  categories: [
    { name: 'Tutorials', slug: 'tutorials', description: 'Step-by-step guides and how-tos' },
    { name: 'Technology', slug: 'technology', description: 'Latest tech news and trends' },
    { name: 'Design', slug: 'design', description: 'Design inspiration and best practices' }
  ],
  current_year: new Date().getFullYear()
}

// Mock data for specific views
export function getMockDataForView(view: 'home' | 'category' | 'post'): MockBlogData {
  const baseData = { ...MOCK_BLOG_DATA }

  if (view === 'category') {
    baseData.currentCategory = baseData.categories[0]
    baseData.posts = baseData.posts.filter((p) => p.category_slug === 'tutorials')
    baseData.page_title = `${baseData.categories[0].name} - ${baseData.blog.title}`
    baseData.page_description = baseData.categories[0].description
    baseData.current_url = `https://example.com/${baseData.categories[0].slug}`
  }

  if (view === 'post') {
    const post = baseData.posts[0]
    baseData.currentPost = post
    baseData.post = post
    baseData.previous_post = baseData.posts[2]
    baseData.next_post = baseData.posts[1]
    baseData.page_title = `${post.title} - ${baseData.blog.title}`
    baseData.page_description = post.excerpt
    baseData.current_url = `https://example.com/${post.category_slug}/${post.slug}`
  }

  if (view === 'home') {
    baseData.current_url = 'https://example.com'
  }

  return baseData
}

