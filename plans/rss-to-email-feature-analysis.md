# RSS to Email Feature Analysis

**Issue**: [#228 - Feature: RSS to Email](https://github.com/Notifuse/notifuse/issues/228)
**Date**: 2026-01-24
**Status**: Research Complete

---

## Executive Summary

RSS to Email is a widely requested automation feature that converts RSS feed content (blog posts, podcasts, news articles) into email newsletters and sends them to subscribers on a defined schedule. This feature eliminates manual effort for content creators while ensuring subscribers receive timely updates.

This analysis covers 6 major competitors and identifies common patterns, unique differentiators, and technical implementation requirements for Notifuse.

---

## Competitor Analysis

### 1. Buttondown

**Source**: [Buttondown RSS-to-Email Documentation](https://docs.buttondown.com/rss-to-email)

#### Features
- **Feed Detection**: Checks RSS feed every hour for new content
- **Sending Modes**:
  - Immediate: Send email per new post
  - Aggregated: Daily, weekly, or monthly digest
- **Draft Management**: New items append to draft marked "Automated" before sending
- **Pause Control**: Can pause automation, skip individual items
- **Multi-feed Support**: Multiple RSS feeds per account, sending to different audience segments
- **Technical**: Supports `xml:base` attribute for relative URLs

#### Unique Aspects
- Granular skip control for individual RSS items
- Hourly feed checking with aggregation to scheduled cadence
- Sophisticated audience segmentation per feed

#### Limitations
- Does not support base-64 encoded images
- Can be blocked by Cloudflare WAF (requires IP whitelist: 54.221.205.107)

---

### 2. Kit (formerly ConvertKit)

**Source**: [Kit RSS Integration Guide](https://help.kit.com/en/articles/2502636-how-to-connect-your-rss-feed-to-kit)

#### Features
- **Delivery Options**:
  - Single emails: One email per new blog post
  - Digest format: Compile multiple posts into one email
- **Auto-send Toggle**: Draft broadcasts can auto-send 30 minutes after creation
- **Template System**: Liquid templating with tags like `{{post.title}}`, `{{post.date}}`, `{{post.summary}}`, `{{post.content}}`
- **Conditional Filtering**: `{% categorymatch %}` tag to display posts matching subscriber tags
- **Subject Line Dynamic**: `{{title}}` tag for dynamic subject lines

#### Unique Aspects
- Deep Liquid template integration (aligns well with Notifuse's existing Liquid support)
- Category-based filtering using subscriber tags
- 30-minute delay before auto-sending (review window)

#### Limitations
- Generates drafts first (extra step before sending)
- Limited to single list per RSS connection

---

### 3. Mailchimp

**Source**: [Mailchimp RSS Automation](https://mailchimp.com/help/share-your-blog-posts-with-mailchimp/)

#### Features
- **Scheduling Options**:
  - Daily sends
  - Weekly sends (specific day/time)
  - Monthly sends (specific day/time)
- **Content Rules**:
  - Only sends when new content exists
  - First send: includes posts from last 24h/7d/30d based on frequency
  - Subsequent: all posts since last send
  - Posts must publish 3+ hours before scheduled send
- **Template System**: RSS merge tags for content extraction
- **Image Handling**: Optional image resizing

#### Unique Aspects
- Smart content filtering (only sends when there's new content)
- 3-hour buffer requirement for new posts
- One send per day maximum limit

#### Limitations
- Cannot trigger multiple sends per day
- Limited to one RSS automation per day regardless of config changes

---

### 4. MailerLite

**Source**: [MailerLite RSS to Email](https://www.mailerlite.com/features/rss-to-email)

#### Features
- **Scheduling**: Daily, weekly, or monthly
- **Post Control**: Configure number of posts per email
- **Editor**: Drag-and-drop editor with RSS-specific templates
- **Layouts**: Single post highlight or horizontal multi-post layout
- **WordPress Plugin**: Automatically adds featured images to RSS feeds

#### Unique Aspects
- Dedicated WordPress plugin for enhanced featured images
- Pre-built RSS-optimized templates
- Flexible layout options (single vs. multi-post)

#### Limitations
- Premium feature (Growing Business and Advanced plans only)

---

### 5. Brevo (formerly Sendinblue)

**Source**: [Brevo RSS Campaign Integration](https://help.brevo.com/hc/en-us/articles/360013130059-RSS-Campaign-integration-Automatically-share-your-blog-posts-with-your-subscribers)

#### Features
- **Sending Modes**:
  - Automatic: Sends immediately when RSS campaign is created
  - Manual: Creates drafts for review
- **Scheduling Options**:
  - Weekly/monthly recurring
  - Immediate on new content
  - Select specific days for checking
  - Configure check time
- **Template System**:
  - Default RSS template with pre-configured dynamic elements
  - Repeatable content blocks for multiple articles
  - RSS-specific tags for dynamic population
- **Data Feeds**: Alternative approach using Brevo's data feed system with Automations

#### Unique Aspects
- Repeatable content blocks (iterate over articles in single template)
- Flexible check schedule (select specific days and times)
- Integration with broader automation workflows

#### Limitations
- One contact list per RSS Campaign integration
- Requires multiple integrations for multiple lists

---

### 6. ActiveCampaign

**Source**: [WP RSS Aggregator Comparison](https://www.wprssaggregator.com/best-rss-to-email-services-compared/)

#### Features
- **RSS Content Blocks**: Drag-and-drop into any template
- **Visual Automation Builder**: Complex workflow creation
- **Triggered Campaigns**: Beyond scheduled, event-based triggers
- **Multi-feed Support**: Multiple feeds in single campaign

#### Unique Aspects
- Most sophisticated automation builder
- Multi-feed aggregation in single email
- Enterprise-level workflow capabilities

#### Limitations
- Higher price point (starts $15/month)
- More complexity than needed for simple use cases

---

## Feature Comparison Matrix

| Feature | Buttondown | Kit | Mailchimp | MailerLite | Brevo | ActiveCampaign |
|---------|------------|-----|-----------|------------|-------|----------------|
| **Scheduling** |
| Immediate (per post) | ✅ | ✅ | ❌ | ❌ | ✅ | ✅ |
| Daily digest | ✅ | ❌ | ✅ | ✅ | ❌ | ✅ |
| Weekly digest | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Monthly digest | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ |
| Custom days/times | ❌ | ❌ | ✅ | ❌ | ✅ | ✅ |
| **Content Control** |
| Skip individual items | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Posts per email limit | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ |
| Draft before send | ✅ | ✅ | ❌ | ❌ | ✅ | ❌ |
| Only send with new content | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Templates** |
| Dynamic tags | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Liquid/templating | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ |
| Repeatable blocks | ❌ | ❌ | ❌ | ❌ | ✅ | ✅ |
| Pre-built templates | ❌ | ❌ | ✅ | ✅ | ✅ | ✅ |
| **Targeting** |
| Multiple feeds | ✅ | ❌ | ❌ | ❌ | ❌ | ✅ |
| Audience segmentation | ✅ | ✅ | ✅ | ❌ | ✅ | ✅ |
| Category filtering | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ |
| **Integrations** |
| WordPress plugin | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ |
| Automation workflows | ❌ | ❌ | ❌ | ❌ | ✅ | ✅ |

---

## Common RSS Feed Variables/Tags

Based on competitor analysis, these are the standard RSS item fields that should be exposed in templates:

### Core Fields
| Variable | Description | Example |
|----------|-------------|---------|
| `{{post.title}}` | Article title | "How to Build a Newsletter" |
| `{{post.url}}` / `{{post.link}}` | Full URL to article | "https://blog.example.com/article" |
| `{{post.content}}` | Full article content (HTML) | Full HTML body |
| `{{post.summary}}` / `{{post.excerpt}}` | Article excerpt/description | First 200 chars |
| `{{post.date}}` / `{{post.published}}` | Publication date | "2026-01-24" |
| `{{post.author}}` | Author name | "John Doe" |
| `{{post.image}}` | Featured image URL | "https://..." |

### Feed-Level Fields
| Variable | Description |
|----------|-------------|
| `{{feed.title}}` | RSS feed title |
| `{{feed.description}}` | Feed description |
| `{{feed.url}}` | Feed URL |
| `{{feed.image}}` | Feed logo/image |

### Iteration
```liquid
{% for post in posts %}
  <h2>{{ post.title }}</h2>
  <p>{{ post.summary }}</p>
  <a href="{{ post.url }}">Read more</a>
{% endfor %}
```

---

## Technical Implementation Considerations

### Recommended Go Libraries

#### RSS Parsing: gofeed
**Repository**: [github.com/mmcdole/gofeed](https://github.com/mmcdole/gofeed)

- **Formats**: RSS 0.90-2.0, Atom 0.3/1.0, JSON Feed 1.0/1.1
- **Resilience**: Best-effort handling of malformed feeds
- **Extensions**: Dublin Core, iTunes extensions built-in
- **Features**: Custom User-Agent, Basic Auth, Context timeout support

```go
import "github.com/mmcdole/gofeed"

fp := gofeed.NewParser()
fp.UserAgent = "Notifuse/1.0"
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
feed, err := fp.ParseURLWithContext(feedURL, ctx)
```

#### Alternative Libraries
- **SlyMarbo/rss**: Simpler API, automatic update interval management
- **gorilla/feeds**: For generating RSS feeds (not needed for this feature)

### Open-Source Reference Implementations

1. **[skx/rss2email](https://github.com/skx/rss2email)**: Go-based, cron-driven, multi-part emails
2. **[slurdge/goeland](https://github.com/slurdge/goeland)**: Go-based with filtering/transformation
3. **[firefart/rss_fetcher](https://github.com/FireFart/rss_fetcher)**: Hourly fetching with state tracking

### Database Schema Considerations

```sql
-- RSS Feed Sources
CREATE TABLE rss_feeds (
    id VARCHAR(32) PRIMARY KEY,
    workspace_id VARCHAR(32) NOT NULL,
    name VARCHAR(255) NOT NULL,
    feed_url TEXT NOT NULL,
    last_fetched_at TIMESTAMP WITH TIME ZONE,
    last_item_guid TEXT,
    last_item_date TIMESTAMP WITH TIME ZONE,
    fetch_interval_minutes INT DEFAULT 60,
    status VARCHAR(32) DEFAULT 'active', -- active, paused, error
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- RSS Campaign Configuration
CREATE TABLE rss_campaigns (
    id VARCHAR(32) PRIMARY KEY,
    workspace_id VARCHAR(32) NOT NULL,
    feed_id VARCHAR(32) REFERENCES rss_feeds(id),
    name VARCHAR(255) NOT NULL,
    template_id VARCHAR(32), -- Reference to email template
    list_id VARCHAR(32), -- Target subscriber list/segment

    -- Scheduling
    schedule_type VARCHAR(32) NOT NULL, -- immediate, daily, weekly, monthly
    schedule_days INT[], -- For weekly: [1,3,5] = Mon,Wed,Fri
    schedule_time TIME, -- Send time
    timezone VARCHAR(64) DEFAULT 'UTC',

    -- Content settings
    max_posts_per_email INT DEFAULT 5,
    include_full_content BOOLEAN DEFAULT false,
    draft_mode BOOLEAN DEFAULT false, -- Create draft vs auto-send

    -- State
    last_sent_at TIMESTAMP WITH TIME ZONE,
    next_scheduled_at TIMESTAMP WITH TIME ZONE,
    status VARCHAR(32) DEFAULT 'active',

    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Tracked RSS Items (to avoid duplicates)
CREATE TABLE rss_items (
    id VARCHAR(32) PRIMARY KEY,
    feed_id VARCHAR(32) REFERENCES rss_feeds(id),
    guid TEXT NOT NULL, -- RSS item GUID
    title TEXT,
    url TEXT,
    published_at TIMESTAMP WITH TIME ZONE,
    fetched_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    sent BOOLEAN DEFAULT false,
    skipped BOOLEAN DEFAULT false,
    UNIQUE(feed_id, guid)
);
```

### Integration with Existing Notifuse Architecture

#### Leveraging Existing Features

1. **Liquid Templating**: Notifuse already uses `osteele/liquid` - RSS variables can be injected into existing template system
2. **Email Sending**: Reuse existing email provider integrations (SES, SMTP, Mailgun, etc.)
3. **Lists/Segments**: Target existing subscriber lists and segments
4. **Automation System**: If Notifuse has automations, RSS can be a trigger type

#### New Components Needed

1. **RSS Fetcher Service**: Background worker to poll feeds on schedule
2. **RSS Feed Manager**: CRUD for feed sources
3. **RSS Campaign Manager**: CRUD for campaign configurations
4. **Scheduler**: Cron-like scheduling for digest compilation and sending

### Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                        RSS to Email Flow                         │
└─────────────────────────────────────────────────────────────────┘

┌──────────────┐     ┌──────────────┐     ┌──────────────────────┐
│  RSS Feeds   │────▶│ RSS Fetcher  │────▶│  Feed Items Cache    │
│  (External)  │     │  (Worker)    │     │  (PostgreSQL)        │
└──────────────┘     └──────────────┘     └──────────────────────┘
                            │                        │
                            ▼                        ▼
                     ┌──────────────┐     ┌──────────────────────┐
                     │  Scheduler   │────▶│  Content Compiler    │
                     │  (Cron)      │     │  (Liquid Templates)  │
                     └──────────────┘     └──────────────────────┘
                                                     │
                                                     ▼
                                          ┌──────────────────────┐
                                          │  Broadcast/Email     │
                                          │  (Existing System)   │
                                          └──────────────────────┘
```

---

## Recommended MVP Features for Notifuse

### Phase 1: Core Functionality

1. **Feed Management**
   - Add/edit/delete RSS feed sources
   - Validate feed URL on creation
   - Display feed preview (last 5 items)
   - Show fetch status and errors

2. **Campaign Configuration**
   - Select feed source
   - Select target list/segment
   - Choose schedule: immediate, daily, weekly, monthly
   - Set send time and timezone
   - Configure max posts per email

3. **Template Integration**
   - RSS-specific template variables
   - Post iteration block
   - Pre-built RSS email template

4. **Sending**
   - Draft mode (create broadcast as draft)
   - Auto-send mode
   - Duplicate detection (don't resend items)

### Phase 2: Advanced Features

1. **Multi-feed Support**: Aggregate multiple feeds into one digest
2. **Item Filtering**: Skip individual items, category filters
3. **Content Transformation**: Extract images, truncate content
4. **Analytics**: Track RSS email performance

### Phase 3: Power Features

1. **Conditional Content**: Different content based on subscriber attributes
2. **A/B Testing**: Test different templates
3. **Automation Integration**: RSS as trigger in automation workflows

---

## UI/UX Recommendations

### Feed Setup Flow

1. **Enter RSS URL** → Validate and preview
2. **Configure Schedule** → Visual day/time picker
3. **Select Template** → Choose from templates or create new
4. **Select Audience** → Choose list/segment
5. **Review & Activate**

### Dashboard Elements

- Feed health status indicators
- Next scheduled send time
- Recent sends with performance metrics
- Quick pause/resume controls

### Template Editor Additions

- RSS variable picker/inserter
- Preview with sample feed data
- Post iteration block component

---

## Pricing Considerations

Based on competitor analysis:
- **MailerLite**: Premium feature only
- **Mailchimp**: Available on all plans
- **Kit**: Available on all plans
- **Brevo**: Available on free plan (with limits)

**Recommendation**: Include in Notifuse core features to differentiate from competitors, with potential limits on:
- Number of RSS feeds (e.g., 5 on free tier)
- Check frequency (e.g., hourly on free, 15-min on paid)

---

## Implementation Effort Estimate

### Components

| Component | Complexity | Dependencies |
|-----------|------------|--------------|
| RSS Fetcher Worker | Medium | gofeed library |
| Feed CRUD API | Low | Existing patterns |
| Campaign CRUD API | Medium | Existing patterns |
| Scheduler Integration | Medium | Existing automation system or new |
| Template Variables | Low | Existing Liquid integration |
| Frontend: Feed Manager | Medium | Existing component patterns |
| Frontend: Campaign Editor | High | New workflow |
| Database Migrations | Low | Standard migration |

### Suggested Order

1. Database schema + migrations
2. RSS fetcher service + feed management API
3. Basic frontend for feed management
4. Campaign configuration backend
5. Template variable integration
6. Campaign frontend
7. Scheduler and sending logic
8. Testing and refinement

---

## Sources

- [Buttondown RSS-to-Email Documentation](https://docs.buttondown.com/rss-to-email)
- [Buttondown Features](https://buttondown.com/features/rss)
- [Kit RSS Integration Guide](https://help.kit.com/en/articles/2502636-how-to-connect-your-rss-feed-to-kit)
- [Mailchimp RSS Automation](https://mailchimp.com/help/share-your-blog-posts-with-mailchimp/)
- [MailerLite RSS to Email](https://www.mailerlite.com/features/rss-to-email)
- [Brevo RSS Campaign Integration](https://help.brevo.com/hc/en-us/articles/360013130059-RSS-Campaign-integration-Automatically-share-your-blog-posts-with-your-subscribers)
- [Brevo Blog: RSS to Email Campaigns](https://www.brevo.com/blog/rss-to-email-campaigns/)
- [WP RSS Aggregator: Best RSS-to-Email Services Compared](https://www.wprssaggregator.com/best-rss-to-email-services-compared/)
- [gofeed Go Library](https://github.com/mmcdole/gofeed)
- [skx/rss2email](https://github.com/skx/rss2email)
- [slurdge/goeland](https://github.com/slurdge/goeland)
