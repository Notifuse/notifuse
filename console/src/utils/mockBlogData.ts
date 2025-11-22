// SEO settings structure (matches backend)
export interface SEOSettings {
  meta_title?: string
  meta_description?: string
  og_title?: string
  og_description?: string
  og_image?: string
  canonical_url?: string
  keywords?: string[]
}

// Post in listings (home/category pages) - matches backend
export interface PostListItem {
  id: string
  slug: string
  category_id: string
  category_slug?: string // Added for URL construction in templates
  published_at: string
  title: string
  excerpt: string
  featured_image_url: string
  authors: Array<{ name: string; avatar_url?: string }>
  reading_time_minutes: number
}

// TOC Item structure (matches backend TOCItem)
export interface TOCItem {
  id: string // Anchor ID for linking
  level: number // Heading level (2-6)
  text: string // Heading text content
}

// Full post object (for single post pages) - matches backend
export interface Post extends PostListItem {
  created_at: string
  updated_at: string
  content: string // HTML content to render
  category_slug: string // Added for convenience in templates
  seo?: SEOSettings
  table_of_contents?: TOCItem[] // Table of contents generated from headings
}

// Category structure - matches backend
export interface Category {
  id: string
  slug: string
  name: string
  description: string
  seo?: SEOSettings
}

export interface MockBlogData {
  // Workspace info (required in backend BlogTemplateDataRequest)
  workspace: {
    id: string
    name: string
    blog_title?: string
    logo_url?: string
    icon_url?: string
  }
  // Base URL (required in backend) - workspace website/custom domain
  base_url: string
  // Theme info (required in backend)
  theme: {
    version: number
  }
  // Public lists for newsletter subscription (required in backend)
  public_lists: Array<{
    id: string
    name: string
    description?: string
  }>
  // Posts array (for home/category page listings) - matches backend listing format
  posts: Array<PostListItem>
  // Categories array (for navigation) - matches backend
  categories: Array<Category>
  // Current post (for post pages only, matches backend BlogTemplateDataRequest)
  post?: Post
  // Current category (for category pages only, matches backend BlogTemplateDataRequest)
  category?: Category
  // Pagination data (matches backend BlogTemplateDataRequest)
  pagination?: {
    current_page: number
    total_pages: number
    has_next: boolean
    has_previous: boolean
    total_count: number
    per_page: number
  }
  // Additional fields (not from backend, for preview convenience)
  previous_post?: PostListItem
  next_post?: PostListItem
  current_year: number
  // Legacy fields (frontend only, for backward compatibility)
  blog?: {
    title: string
    description: string
  }
  seo?: SEOSettings
  page_title?: string
  page_description?: string
  current_url?: string
}

// Full post data with content (used internally and for post pages)
const FULL_POSTS_DATA: Post[] = [
  {
    id: 'post-1',
    slug: 'style-guide-kitchen-sink',
    category_id: 'cat-1',
    category_slug: 'tutorials',
    published_at: 'March 15, 2024',
    created_at: '2024-03-15T10:00:00Z',
    updated_at: '2024-03-15T10:00:00Z',
    title: 'Complete Style Guide & Kitchen Sink',
    excerpt:
      'A comprehensive showcase of all content blocks, styling options, and typography elements available in our blog theme system.',
    featured_image_url: 'https://images.unsplash.com/photo-1498050108023-c5249f4df085?w=800',
    authors: [
      {
        name: 'Jane Doe',
        avatar_url: 'https://images.unsplash.com/photo-1494790108377-be9c29b29330?w=100'
      }
    ],
    reading_time_minutes: 8,
    table_of_contents: [
      { id: 'section-heading-h2', level: 2, text: 'Section Heading (H2)' },
      { id: 'subsection-heading-h3', level: 3, text: 'Subsection Heading (H3)' },
      { id: 'blockquotes', level: 2, text: 'Blockquotes' },
      { id: 'code-blocks', level: 2, text: 'Code Blocks' },
      { id: 'lists', level: 2, text: 'Lists' },
      { id: 'unordered-list', level: 3, text: 'Unordered List' },
      { id: 'ordered-list', level: 3, text: 'Ordered List' },
      { id: 'images', level: 2, text: 'Images' },
      { id: 'mixed-content', level: 2, text: 'Mixed Content' },
      { id: 'typography-details', level: 3, text: 'Typography Details' },
      { id: 'conclusion', level: 2, text: 'Conclusion' }
    ],
    content: `<p>This post demonstrates every content block type and styling option available in the theme editor. Use this as a reference to see how your theme handles different content types.</p>

<h2 id="section-heading-h2">Section Heading (H2)</h2>
<p>Every blog post needs well-structured sections. This H2 heading marks a major section division. Notice the spacing above and below this heading, as well as the font size and weight.</p>

<h3 id="subsection-heading-h3">Subsection Heading (H3)</h3>
<p>H3 headings are perfect for subsections within your content. They provide hierarchy without overwhelming the reader. The font size should be noticeably smaller than H2 but still prominent.</p>

<p>Here's another paragraph with some <strong>bold text</strong>, <em>italic text</em>, and even <code>inline code</code> to show how inline formatting works. You can also include <a href="https://example.com">hyperlinks</a> that should have their own distinctive styling.</p>

<hr>

<h2 id="blockquotes">Blockquotes</h2>
<p>Blockquotes are used for quotations or to highlight important passages:</p>

<blockquote>
<p>This is a blockquote. It should have distinctive styling to set it apart from regular paragraphs. Blockquotes often use different colors, margins, or even border decorations.</p>
</blockquote>

<p>And here's regular text that follows the blockquote, demonstrating proper spacing between different block types.</p>

<hr>

<h2 id="code-blocks">Code Blocks</h2>
<p>For technical content, code blocks are essential. Here's an example with JavaScript:</p>

<pre><code class="language-javascript">function greet(name) {
  return \`Hello, \${name}!\`;
}

const message = greet('World');
console.log(message); // Output: Hello, World!</code></pre>
<p style="font-size: 14px; color: #6b7280; margin-top: -8px;">Caption: A simple JavaScript greeting function</p>

<p>Code blocks should have distinct background colors and use monospace fonts for readability.</p>

<hr>

<h2 id="lists">Lists</h2>
<p>Both ordered and unordered lists are common in blog posts.</p>

<h3 id="unordered-list">Unordered List</h3>
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

<h3 id="ordered-list">Ordered List</h3>
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

<h2 id="images">Images</h2>
<p>Images are crucial for visual storytelling. Here's an example:</p>

<img src="https://images.unsplash.com/photo-1498050108023-c5249f4df085?w=800" alt="Person typing on laptop" data-caption="A developer working on a laptop" data-show-caption="true" />
<p style="font-size: 14px; color: #6b7280; text-align: center; margin-top: 8px;">Caption: A developer working on a laptop</p>

<p>Notice how images should have proper spacing above and below, and captions should be visually distinct from body text.</p>

<hr>

<h2 id="mixed-content">Mixed Content</h2>
<p>Real-world blog posts combine multiple content types. Here's a paragraph followed by a list of key takeaways:</p>

<ul>
<li>All headings (H1, H2, H3) should have clear hierarchy</li>
<li>Paragraphs need comfortable line height and spacing</li>
<li>Code blocks require monospace fonts</li>
<li>Links should be easily distinguishable</li>
<li>Images need proper captions and spacing</li>
</ul>

<p>And here's more text after the list to show proper spacing. The gap between different elements should feel natural and not too cramped or too spacious.</p>

<h3 id="typography-details">Typography Details</h3>
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

<h2 id="conclusion">Conclusion</h2>
<p>This style guide demonstrates all the essential content blocks you'll use in your blog posts. Each element should have thoughtful styling that contributes to an excellent reading experience. Whether you're writing tutorials, articles, or documentation, these building blocks form the foundation of great content.</p>

<p>Use this page as a reference when customizing your theme. Make sure every element looks polished and works well together to create a cohesive, professional appearance.</p>`
  },
  {
    id: 'post-2',
    slug: 'future-of-ai',
    category_id: 'cat-2',
    category_slug: 'technology',
    published_at: 'March 12, 2024',
    created_at: '2024-03-12T10:00:00Z',
    updated_at: '2024-03-12T10:00:00Z',
    title: 'The Future of Artificial Intelligence',
    excerpt: 'Exploring how AI will transform industries and daily life in the coming years.',
    featured_image_url: 'https://images.unsplash.com/photo-1677442136019-21780ecad995?w=800',
    authors: [
      {
        name: 'John Smith',
        avatar_url: 'https://images.unsplash.com/photo-1507003211169-0a1dd7228f2d?w=100'
      }
    ],
    reading_time_minutes: 8,
    content: `<p>Artificial Intelligence is rapidly evolving, and its impact on society is becoming more profound each day.</p>`
  },
  {
    id: 'post-3',
    slug: 'design-principles-modern-websites',
    category_id: 'cat-3',
    category_slug: 'design',
    published_at: 'March 10, 2024',
    created_at: '2024-03-10T10:00:00Z',
    updated_at: '2024-03-10T10:00:00Z',
    title: 'Design Principles for Modern Websites',
    excerpt: 'Essential design principles that will make your website stand out in 2024.',
    featured_image_url: 'https://images.unsplash.com/photo-1558655146-9f40138edfeb?w=800',
    authors: [
      {
        name: 'Sarah Johnson',
        avatar_url: 'https://images.unsplash.com/photo-1573497019940-1c28c88b4f3e?w=100'
      }
    ],
    reading_time_minutes: 6,
    content: `<p>Good design is not just about aesthetics—it's about creating an intuitive, accessible experience for all users.</p>`
  },
  {
    id: 'post-4',
    slug: 'building-scalable-rest-apis-nodejs',
    category_id: 'cat-1',
    category_slug: 'tutorials',
    published_at: 'March 8, 2024',
    created_at: '2024-03-08T10:00:00Z',
    updated_at: '2024-03-08T10:00:00Z',
    title: 'Building Scalable REST APIs with Node.js',
    excerpt:
      'Learn how to design and implement RESTful APIs that can handle millions of requests with Node.js and Express.',
    featured_image_url: 'https://images.unsplash.com/photo-1555066931-4365d14bab8c?w=800',
    authors: [
      {
        name: 'Alex Chen',
        avatar_url: 'https://images.unsplash.com/photo-1472099645785-5658abf4ff4e?w=100'
      }
    ],
    reading_time_minutes: 10,
    table_of_contents: [
      { id: 'why-nodejs-for-apis', level: 2, text: 'Why Node.js for APIs?' },
      { id: 'key-benefits', level: 3, text: 'Key Benefits' },
      { id: 'setting-up-your-api', level: 2, text: 'Setting Up Your API' },
      { id: 'essential-middleware', level: 3, text: 'Essential Middleware' },
      { id: 'api-design-best-practices', level: 2, text: 'API Design Best Practices' },
      { id: 'performance-optimization', level: 2, text: 'Performance Optimization' },
      { id: 'error-handling', level: 2, text: 'Error Handling' },
      { id: 'conclusion', level: 2, text: 'Conclusion' }
    ],
    content: `<p>Building scalable REST APIs requires careful consideration of architecture, performance, and maintainability. In this comprehensive guide, we'll explore best practices for creating robust APIs with Node.js.</p>

<h2 id="why-nodejs-for-apis">Why Node.js for APIs?</h2>
<p>Node.js has become the go-to choice for building REST APIs due to its non-blocking I/O model and vast ecosystem. Its event-driven architecture makes it perfect for handling concurrent requests efficiently.</p>

<h3 id="key-benefits">Key Benefits</h3>
<ul>
<li>High performance with asynchronous operations</li>
<li>Unified JavaScript across frontend and backend</li>
<li>Rich ecosystem with npm packages</li>
<li>Excellent for real-time applications</li>
</ul>

<h2 id="setting-up-your-api">Setting Up Your API</h2>
<p>Start with a solid foundation using Express.js, the most popular Node.js web framework:</p>

<pre><code class="language-javascript">const express = require('express');
const app = express();

app.use(express.json());
app.use(express.urlencoded({ extended: true }));

const PORT = process.env.PORT || 3000;
app.listen(PORT, () => {
  console.log(\`Server running on port \${PORT}\`);
});</code></pre>

<h3 id="essential-middleware">Essential Middleware</h3>
<p>Middleware functions are crucial for handling cross-cutting concerns like authentication, logging, and error handling:</p>

<ul>
<li><strong>CORS:</strong> Enable cross-origin requests</li>
<li><strong>Helmet:</strong> Secure HTTP headers</li>
<li><strong>Morgan:</strong> HTTP request logging</li>
<li><strong>Rate limiting:</strong> Prevent abuse</li>
</ul>

<h2 id="api-design-best-practices">API Design Best Practices</h2>
<p>Following REST conventions makes your API intuitive and maintainable:</p>

<ol>
<li>Use nouns for resource names (e.g., <code>/users</code>, not <code>/getUsers</code>)</li>
<li>Leverage HTTP methods appropriately (GET, POST, PUT, DELETE)</li>
<li>Implement proper status codes (200, 201, 400, 404, 500)</li>
<li>Version your API (<code>/api/v1/users</code>)</li>
<li>Use pagination for large datasets</li>
</ol>

<blockquote>
<p>A well-designed API is one that developers can understand and use without extensive documentation.</p>
</blockquote>

<h2 id="performance-optimization">Performance Optimization</h2>
<p>To handle high traffic, implement these optimization strategies:</p>

<ul>
<li>Use caching (Redis, in-memory cache)</li>
<li>Implement database connection pooling</li>
<li>Add compression middleware (gzip)</li>
<li>Optimize database queries with indexes</li>
<li>Consider clustering for multi-core systems</li>
</ul>

<h2 id="error-handling">Error Handling</h2>
<p>Proper error handling is critical for production APIs:</p>

<pre><code class="language-javascript">app.use((err, req, res, next) => {
  console.error(err.stack);
  res.status(err.status || 500).json({
    error: {
      message: err.message,
      status: err.status || 500
    }
  });
});</code></pre>

<h2 id="conclusion">Conclusion</h2>
<p>Building scalable REST APIs with Node.js is an iterative process. Start with solid fundamentals, follow best practices, and continuously monitor and optimize your API's performance. With the right architecture and tools, your API can handle millions of requests while remaining maintainable and developer-friendly.</p>`
  },
  {
    id: 'post-5',
    slug: 'understanding-cloud-architecture-patterns',
    category_id: 'cat-2',
    category_slug: 'technology',
    published_at: 'March 5, 2024',
    created_at: '2024-03-05T10:00:00Z',
    updated_at: '2024-03-05T10:00:00Z',
    title: 'Understanding Cloud Architecture Patterns',
    excerpt:
      'Explore essential cloud architecture patterns that enable scalability, reliability, and cost-efficiency in modern applications.',
    featured_image_url: 'https://images.unsplash.com/photo-1544197150-b99a580bb7a8?w=800',
    authors: [
      {
        name: 'Michael Brown',
        avatar_url: 'https://images.unsplash.com/photo-1500648767791-00dcc994a43e?w=100'
      }
    ],
    reading_time_minutes: 7,
    table_of_contents: [
      {
        id: 'why-architecture-patterns-matter',
        level: 2,
        text: 'Why Architecture Patterns Matter'
      },
      { id: 'core-benefits', level: 3, text: 'Core Benefits' },
      { id: 'microservices-architecture', level: 2, text: 'Microservices Architecture' },
      { id: 'key-characteristics', level: 3, text: 'Key Characteristics' },
      { id: 'event-driven-architecture', level: 2, text: 'Event-Driven Architecture' },
      { id: 'cqrs-pattern', level: 2, text: 'CQRS Pattern' },
      { id: 'when-to-use-cqrs', level: 3, text: 'When to Use CQRS' },
      { id: 'serverless-architecture', level: 2, text: 'Serverless Architecture' },
      { id: 'circuit-breaker-pattern', level: 2, text: 'Circuit Breaker Pattern' },
      { id: 'best-practices', level: 2, text: 'Best Practices' },
      { id: 'conclusion', level: 2, text: 'Conclusion' }
    ],
    content: `<p>Cloud architecture patterns are proven solutions to common challenges in distributed systems. Understanding these patterns is crucial for building robust, scalable applications in the cloud.</p>

<h2 id="why-architecture-patterns-matter">Why Architecture Patterns Matter</h2>
<p>Modern cloud applications face unique challenges: unpredictable traffic, global distribution, and the need for high availability. Architecture patterns provide battle-tested solutions to these challenges.</p>

<h3 id="core-benefits">Core Benefits</h3>
<ul>
<li>Improved scalability and performance</li>
<li>Enhanced reliability and fault tolerance</li>
<li>Better cost optimization</li>
<li>Faster development and deployment</li>
</ul>

<h2 id="microservices-architecture">Microservices Architecture</h2>
<p>Break down monolithic applications into smaller, independent services that can be developed, deployed, and scaled independently.</p>

<blockquote>
<p>Microservices enable teams to work autonomously and deploy updates without affecting the entire system.</p>
</blockquote>

<h3 id="key-characteristics">Key Characteristics</h3>
<ol>
<li>Each service has a single responsibility</li>
<li>Services communicate via APIs (REST, gRPC)</li>
<li>Independent data storage per service</li>
<li>Decentralized governance</li>
</ol>

<h2 id="event-driven-architecture">Event-Driven Architecture</h2>
<p>Use events to trigger and communicate between decoupled services, enabling real-time processing and loose coupling.</p>

<ul>
<li><strong>Event producers:</strong> Services that emit events</li>
<li><strong>Event brokers:</strong> Message queues (Kafka, RabbitMQ)</li>
<li><strong>Event consumers:</strong> Services that react to events</li>
</ul>

<h2 id="cqrs-pattern">CQRS Pattern</h2>
<p>Command Query Responsibility Segregation separates read and write operations, allowing for optimized data models for each use case.</p>

<h3 id="when-to-use-cqrs">When to Use CQRS</h3>
<ul>
<li>Complex domain logic</li>
<li>High read-to-write ratio</li>
<li>Need for different data models</li>
<li>Performance optimization requirements</li>
</ul>

<h2 id="serverless-architecture">Serverless Architecture</h2>
<p>Build applications without managing servers, using functions that execute in response to events:</p>

<pre><code class="language-javascript">exports.handler = async (event) => {
  // Process event
  const result = await processData(event);
  
  return {
    statusCode: 200,
    body: JSON.stringify(result)
  };
};</code></pre>

<h2 id="circuit-breaker-pattern">Circuit Breaker Pattern</h2>
<p>Prevent cascading failures by detecting failures and preventing requests to failing services:</p>

<ol>
<li><strong>Closed state:</strong> Normal operation</li>
<li><strong>Open state:</strong> Failures detected, requests blocked</li>
<li><strong>Half-open state:</strong> Testing if service recovered</li>
</ol>

<h2 id="best-practices">Best Practices</h2>
<p>When implementing cloud architecture patterns:</p>

<ul>
<li>Start simple and evolve based on needs</li>
<li>Monitor and measure everything</li>
<li>Design for failure</li>
<li>Use managed services when possible</li>
<li>Implement security at every layer</li>
</ul>

<h2 id="conclusion">Conclusion</h2>
<p>Cloud architecture patterns provide a roadmap for building resilient, scalable applications. By understanding and applying these patterns appropriately, you can create systems that meet modern demands for performance, reliability, and cost-efficiency. Choose patterns based on your specific requirements and don't over-engineer—start with simpler patterns and evolve as needed.</p>`
  },
  {
    id: 'post-6',
    slug: 'mastering-user-experience-practical-guide',
    category_id: 'cat-3',
    category_slug: 'design',
    published_at: 'March 3, 2024',
    created_at: '2024-03-03T10:00:00Z',
    updated_at: '2024-03-03T10:00:00Z',
    title: 'Mastering User Experience: A Practical Guide',
    excerpt:
      'Discover proven UX principles and practical techniques to create intuitive, delightful experiences that users love.',
    featured_image_url: 'https://images.unsplash.com/photo-1561070791-2526d30994b5?w=800',
    authors: [
      {
        name: 'Emily Davis',
        avatar_url: 'https://images.unsplash.com/photo-1438761681033-6461ffad8d80?w=100'
      }
    ],
    reading_time_minutes: 9,
    table_of_contents: [
      { id: 'understanding-ux-fundamentals', level: 2, text: 'Understanding UX Fundamentals' },
      { id: 'the-five-elements-of-ux', level: 3, text: 'The Five Elements of UX' },
      { id: 'user-research-the-foundation', level: 2, text: 'User Research: The Foundation' },
      { id: 'research-methods', level: 3, text: 'Research Methods' },
      { id: 'core-ux-principles', level: 2, text: 'Core UX Principles' },
      { id: '1-clarity', level: 3, text: '1. Clarity' },
      { id: '2-consistency', level: 3, text: '2. Consistency' },
      { id: '3-feedback', level: 3, text: '3. Feedback' },
      { id: '4-simplicity', level: 3, text: '4. Simplicity' },
      { id: '5-accessibility', level: 3, text: '5. Accessibility' },
      { id: 'information-architecture', level: 2, text: 'Information Architecture' },
      { id: 'interaction-design', level: 2, text: 'Interaction Design' },
      { id: 'micro-interactions', level: 3, text: 'Micro-interactions' },
      { id: 'mobile-first-design', level: 2, text: 'Mobile-First Design' },
      { id: 'mobile-best-practices', level: 3, text: 'Mobile Best Practices' },
      { id: 'measuring-ux-success', level: 2, text: 'Measuring UX Success' },
      { id: 'continuous-improvement', level: 2, text: 'Continuous Improvement' },
      { id: 'conclusion', level: 2, text: 'Conclusion' }
    ],
    content: `<p>User Experience (UX) design is the art and science of creating products that provide meaningful and relevant experiences to users. Great UX isn't just about making things look pretty—it's about solving real problems effectively.</p>

<h2 id="understanding-ux-fundamentals">Understanding UX Fundamentals</h2>
<p>UX encompasses every aspect of the user's interaction with a company, its services, and its products. The goal is to create easy, efficient, and enjoyable experiences.</p>

<h3 id="the-five-elements-of-ux">The Five Elements of UX</h3>
<ol>
<li><strong>Strategy:</strong> User needs and business goals</li>
<li><strong>Scope:</strong> Functional requirements and content</li>
<li><strong>Structure:</strong> Information architecture and interaction design</li>
<li><strong>Skeleton:</strong> Interface and navigation design</li>
<li><strong>Surface:</strong> Visual design</li>
</ol>

<blockquote>
<p>Good design is actually harder to notice than poor design, in part because good designs fit our needs so well that the design is invisible.</p>
</blockquote>

<h2 id="user-research-the-foundation">User Research: The Foundation</h2>
<p>Never skip user research. Understanding your users is the foundation of good UX design.</p>

<h3 id="research-methods">Research Methods</h3>
<ul>
<li><strong>User interviews:</strong> Deep insights into user needs</li>
<li><strong>Surveys:</strong> Quantitative data from many users</li>
<li><strong>Usability testing:</strong> Observe real usage</li>
<li><strong>Analytics:</strong> Data-driven insights</li>
<li><strong>Competitive analysis:</strong> Learn from others</li>
</ul>

<h2 id="core-ux-principles">Core UX Principles</h2>
<p>These principles should guide every design decision:</p>

<h3 id="1-clarity">1. Clarity</h3>
<p>Users should immediately understand what they can do and how to do it. Avoid jargon and ambiguity.</p>

<h3 id="2-consistency">2. Consistency</h3>
<p>Maintain consistent patterns throughout your interface. Similar elements should look and behave similarly.</p>

<h3 id="3-feedback">3. Feedback</h3>
<p>Always acknowledge user actions. Loading states, success messages, and error notifications keep users informed.</p>

<h3 id="4-simplicity">4. Simplicity</h3>
<p>Remove unnecessary elements. Every piece of content and functionality should serve a purpose.</p>

<h3 id="5-accessibility">5. Accessibility</h3>
<p>Design for everyone, including users with disabilities. This isn't optional—it's essential.</p>

<h2 id="information-architecture">Information Architecture</h2>
<p>Organize content in a way that makes sense to users:</p>

<ul>
<li>Use clear, descriptive labels</li>
<li>Group related content together</li>
<li>Limit navigation depth (3 clicks rule)</li>
<li>Provide multiple ways to find content</li>
<li>Use breadcrumbs for wayfinding</li>
</ul>

<h2 id="interaction-design">Interaction Design</h2>
<p>Design interactions that feel natural and responsive:</p>

<pre><code class="language-css">/* Smooth transitions enhance perceived performance */
.button {
  transition: all 0.3s ease;
}

.button:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 8px rgba(0,0,0,0.15);
}</code></pre>

<h3 id="micro-interactions">Micro-interactions</h3>
<p>Small details make a big difference:</p>
<ul>
<li>Button states (hover, active, disabled)</li>
<li>Loading animations</li>
<li>Form validation feedback</li>
<li>Success confirmations</li>
</ul>

<h2 id="mobile-first-design">Mobile-First Design</h2>
<p>Start with mobile constraints, then progressively enhance for larger screens. This ensures a solid foundation for all devices.</p>

<h3 id="mobile-best-practices">Mobile Best Practices</h3>
<ol>
<li>Design for thumbs (44x44px minimum touch targets)</li>
<li>Prioritize content ruthlessly</li>
<li>Use native patterns when possible</li>
<li>Optimize for one-handed use</li>
<li>Test on real devices</li>
</ol>

<h2 id="measuring-ux-success">Measuring UX Success</h2>
<p>Track these metrics to validate your design decisions:</p>

<ul>
<li><strong>Task completion rate:</strong> Can users accomplish their goals?</li>
<li><strong>Time on task:</strong> How efficiently?</li>
<li><strong>Error rate:</strong> How often do things go wrong?</li>
<li><strong>User satisfaction:</strong> How do users feel?</li>
<li><strong>Net Promoter Score:</strong> Would they recommend it?</li>
</ul>

<h2 id="continuous-improvement">Continuous Improvement</h2>
<p>UX design is never "done." Continuously gather feedback, analyze data, and iterate:</p>

<ol>
<li>Set up analytics and tracking</li>
<li>Conduct regular usability tests</li>
<li>Monitor support tickets for pain points</li>
<li>Stay current with UX trends and research</li>
<li>Foster a culture of user-centered design</li>
</ol>

<h2 id="conclusion">Conclusion</h2>
<p>Mastering UX is a journey, not a destination. By focusing on user needs, following established principles, and continuously learning and iterating, you can create experiences that truly delight users. Remember: good UX is invisible, but its impact on user satisfaction and business success is undeniable.</p>`
  }
]

// Convert full posts to PostListItem format (for listings)
const POST_LIST_ITEMS: PostListItem[] = FULL_POSTS_DATA.map((post) => ({
  id: post.id,
  slug: post.slug,
  category_id: post.category_id,
  category_slug: post.category_slug, // Include for URL construction
  published_at: post.published_at,
  title: post.title,
  excerpt: post.excerpt,
  featured_image_url: post.featured_image_url,
  authors: post.authors,
  reading_time_minutes: post.reading_time_minutes
}))

export const MOCK_BLOG_DATA: MockBlogData = {
  // Workspace (matches backend BlogTemplateDataRequest)
  workspace: {
    id: 'workspace-123',
    name: 'My Workspace',
    blog_title: 'Thoughts, Stories & Ideas'
  },
  // Base URL (matches backend)
  base_url: 'https://example.com',
  // Theme info (matches backend)
  theme: {
    version: 1
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
  // Posts array (listing format - no content or category_slug)
  posts: POST_LIST_ITEMS,
  // Categories
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
  current_year: new Date().getFullYear(),
  // Legacy fields (for backward compatibility)
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
  }
}

// Mock data for specific views
export function getMockDataForView(view: 'home' | 'category' | 'post'): MockBlogData {
  // Deep copy to avoid mutating the original MOCK_BLOG_DATA
  const baseData: MockBlogData = {
    ...MOCK_BLOG_DATA,
    workspace: { ...MOCK_BLOG_DATA.workspace },
    public_lists: [...MOCK_BLOG_DATA.public_lists],
    posts: [...MOCK_BLOG_DATA.posts],
    categories: [...MOCK_BLOG_DATA.categories],
    blog: MOCK_BLOG_DATA.blog ? { ...MOCK_BLOG_DATA.blog } : undefined,
    theme: { ...MOCK_BLOG_DATA.theme }
  }

  // Get the blog title from workspace (fallback to workspace name)
  const blogTitle = baseData.workspace.blog_title || baseData.workspace.name

  if (view === 'category') {
    // Match backend BlogTemplateDataRequest.Category field
    const category = baseData.categories[0]
    baseData.category = category
    baseData.posts = baseData.posts.filter((p) => p.category_id === 'cat-1') // Tutorials
    baseData.page_title = `${category.name} - ${blogTitle}`
    baseData.page_description = category.description
    baseData.current_url = `${baseData.base_url}/${category.slug}`

    // Add pagination data for category view
    baseData.pagination = {
      current_page: 1,
      total_pages: Math.ceil(baseData.posts.length / 10),
      has_next: false,
      has_previous: false,
      total_count: baseData.posts.length,
      per_page: 10
    }
  }

  if (view === 'post') {
    // Match backend BlogTemplateDataRequest.Post field - use full post data
    const fullPost = FULL_POSTS_DATA[0]
    baseData.post = fullPost
    // Match backend BlogTemplateDataRequest.Category field - set category for post page
    const category = baseData.categories.find((cat) => cat.id === fullPost.category_id)
    if (category) {
      baseData.category = category
    }
    baseData.previous_post = POST_LIST_ITEMS[2]
    baseData.next_post = POST_LIST_ITEMS[1]
    baseData.page_title = `${fullPost.title} - ${blogTitle}`
    baseData.page_description = fullPost.excerpt
    baseData.current_url = `${baseData.base_url}/${fullPost.category_slug}/${fullPost.slug}`
  }

  if (view === 'home') {
    baseData.current_url = baseData.base_url

    // Add pagination data for home view
    baseData.pagination = {
      current_page: 1,
      total_pages: Math.ceil(baseData.posts.length / 10),
      has_next: false,
      has_previous: false,
      total_count: baseData.posts.length,
      per_page: 10
    }
  }

  return baseData
}

// Helper to get mock data with empty public lists (for testing empty state)
export function getMockDataWithEmptyLists(view: 'home' | 'category' | 'post'): MockBlogData {
  const data = getMockDataForView(view)
  data.public_lists = []
  return data
}
