# Changelog

All notable changes to this project will be documented in this file.

## [26.12] - 2026-01-24

- **Transactional Notifications**: Added card-based UI with delivery stats (sent, delivered, failed, bounced) and period selector (7D/30D/60D)

## [26.11] - 2026-01-24

- **Broadcasts**: Fixed crash on Broadcasts page when `variations` is null instead of empty array (#233)
- **SMTP Email**: Fixed URL corruption in emails where dots were stripped after quoted-printable line breaks by adding proper SMTP dot-stuffing

## [26.10] - 2026-01-23

- **Automations**: Email node template selector now shows all template categories instead of only marketing templates
- **File Manager**: Added warning modal when selecting images larger than 200KB from email editor to prevent slow email loading times
- **SES Email**: Fixed incorrect quoted-printable encoding in raw emails causing Gmail to break rendering for broadcasts with List-Unsubscribe headers or attachments (#230)
- **Email Builder**: Fixed Raw HTML block content not appearing in preview or sent emails (#229)

## [26.9] - 2026-01-21

- **Contacts**: Fixed CSV import not trimming non-breaking spaces (NBSP) from email addresses and string fields (#223)
- **Email Builder**: Fixed MJML export stripping Liquid template syntax like `{{contact.external_id}}` from links (#225)
- **Email Builder**: Preview now shows `[undefined: varName]` debug message when Liquid variables are missing from template data (#226)

## [26.8] - 2026-01-19

- **File Manager**: Added explicit "Select" button in actions column when opened from email editor, replacing row-click selection behavior
- **File Manager**: Fixed files with spaces in filename breaking image delivery by URL-encoding path segments (#209)
- **Email Builder**: Fixed MJML template import failing when content contains HTML tags like `<br>` or entities like `&nbsp;` (#218)
- **Email Builder**: Fixed link editing in text blocks requiring exact text selection to update URL

## [26.7] - 2026-01-18

- **MJML Engine**: Switched from `mjml-go` to `gomjml` v0.10.0 for MJML-to-HTML compilation
- **File Manager**: Fixed drop zone overlay staying active when dragging files out without dropping
- **Contacts**: Added CSV export with support for current page filters and all contact fields

## [26.6] - 2026-01-18

### Improvements

- **Broadcast Progress UX**: Added progress bar with ETA, clearer status badges, and tooltips to show sending progress (#185)
- **File Manager**: Added single file download, bulk delete, and multi-file ZIP download for selected files (#201)
- **File Manager**: Folder navigation now syncs with browser URL, enabling back/forward navigation and deep linking to folders

## [26.5] - 2026-01-18

### New Features

- Added **SMTP OAuth2 authentication** method with support for Microsoft 365 and Gmail SMTP servers (#184)
- Added **Twilio SendGrid email provider** integration (#178)

### Bug Fixes

- Fix resubcribe to private lists in notification center
- Fix "custom CSS" edit when imported MJML templates have missing default attributes
- Fix email editor switch settings (fullWidth, fluidOnMobile) not persisting after page refresh (#206)

## [26.4] - 2026-01-15

### Bug Fixes

- **Supabase Integration**: Fixed "Before User Created" webhook URL mismatch causing HTTP 405 errors (#198)
- **Email Builder Image Width**: Fixed invalid MJML generated when adding images via the editor. The default `width="100%"` is not valid for mj-image (only px values allowed). Images now default to filling container width when no explicit width is set (#196)

## [26.3] - 2026-01-13

### Bug Fixes

- **Email Builder Column Width**: Fixed columns displaying with zero width in WYSIWYG editor, causing text to wrap character-by-character

## [26.2] - 2026-01-13

### Bug Fixes

- **Email Reply-To Header**: Fixed Reply-To address from email templates not being included in automation and broadcast queue emails (#193)

## [26.1] - 2026-01-13

### Bug Fixes

- **Supabase Integration**: Fixed incorrect JSON field mapping for new email address in email_change webhooks. Changed `email_new` to `new_email` to match Supabase Auth API specification.

## [26.0] - 2026-01-12

### Bug Fixes

- **Automation Email Templating**: Fixed `{{ contact.first_name }}` rendering as `&{John false}` instead of `John` in automation emails. Contact data is now properly converted before passing to the Liquid template engine.
- **List Status Values**: Fixed `add_to_list` automation nodes using invalid `subscribed` status instead of `active`
- **Automation Stats**: Fixed automation stats being reset to 0 when updating automation

### Migration

- **v26**: Automatically fixes `subscribed` → `active` in contact_lists, automation nodes, and timeline entries. Recomputes automation stats from actual data.

## [25.1] - 2026-01-11

### Bug Fixes

- **Automation Terminal Delay Nodes**: Fixed bug where contacts at terminal delay nodes (delay node with no next node) were incorrectly marked as "completed" instead of staying "active" while waiting for the delay to expire
  - Contacts now correctly wait at terminal delay nodes until scheduled time, then complete

### Testing Improvements

- Fixed invalid delay node configurations in automation e2e tests (invalid unit "seconds" and duration 0)
- Added `waitForStatsCompleted()` helper for polling-based stats verification to fix test flakiness
- Improved test stability for automation scheduler timing edge cases

## [24.0] - 2026-01-10

### Bug Fixes

- **Automation Enrollment Failure**: Fixed "cannot set path in scalar" error when enrolling contacts in automations where the `stats` field contained a JSONB scalar value instead of an object
  - Migration v24 fixes existing automations by setting `stats = '{}'` where `jsonb_typeof(stats) != 'object'`

## [23.0] - 2026-01-09

### Bug Fixes

- **Automation Triggers Not Firing**: Fixed critical bug where automations with "List / Subscribed" trigger never fired for workspaces created after v18 (#190)
  - Root cause: `init.go` had outdated trigger functions producing generic event kinds (`insert_contact_list`) instead of semantic event kinds (`list.subscribed`)
  - New workspaces only got `InitializeWorkspaceDatabase()` without running migrations, so they never received the v18 trigger updates
  - V23 migration reinstalls correct trigger functions on ALL existing workspaces
  - Updated `init.go` to use semantic event kinds for new workspaces
  - Fixed e2e test that was masking the bug by manually inserting timeline events

### Database Schema Changes

- Migration v23.0 reinstalls three PostgreSQL trigger functions with semantic event naming:
  - `track_contact_list_changes()`: `insert_contact_list` → `list.subscribed`, `list.confirmed`, etc.
  - `track_contact_segment_changes()`: `join_segment` → `segment.joined`, `leave_segment` → `segment.left`
  - `track_contact_changes()`: `insert_contact` → `contact.created`, `update_contact` → `contact.updated`

## [22.6] - 2026-01-06

### Bug Fixes

- **SMTP Multi-line Banner**: Fixed SMTP connections failing with "EHLO rejected with code: 220" error when server sends multi-line 220 greeting banner (RFC 5321 compliant fix for #183)

## [22.5] - 2026-01-06

### Bug Fixes

- **Segment Date Filters**: Fixed date picker sending dates in wrong format (`YYYY-MM-DD HH:mm:ss` instead of ISO8601). Frontend now sends proper RFC3339 format for all segment date filters including contact fields, timeline timeframes, and custom events goal timeframes (fixes #182)

## [22.4] - 2026-01-06

### Bug Fixes

- **Broadcast Emails**: Fixed regression from v21.0 email queue system where system template variables (`{{ unsubscribe_url }}`, `{{ notification_center_url }}`, `{{ broadcast.name }}`, etc.) were not rendering in broadcast emails (fixes #180)
- **Message History**: Fixed regression where template data was empty in message history for queue-based sends

## [22.3] - 2026-01-06

### Bug Fixes

- **Automation Flow Editor**: Fixed stale closure bug causing nodes to disappear when adding children to ListStatusBranch handles (fixes #179)

## [22.2] - 2025-12-31

### Features

- **Programmatic Root Authentication**: New `/api/user.rootSignin` endpoint for CI/CD and automation
  - HMAC-SHA256 signature authentication using existing `SECRET_KEY`
  - 60-second timestamp window to prevent replay attacks
  - Rate limited (5 attempts per 5 minutes)

## [22.1] - 2025-12-29

### Features

- **Email AI Assistant**: AI-powered design assistant for email templates
  - Streaming chat with Anthropic Claude models
  - Tool use for modifying email structure, blocks, and content
  - Server-side web scraping and search for content inspiration
  - Auto-expand tree to selected block for better navigation

### Improvements

- **AI Assistant UX**: Both Email and Blog AI assistants now show a helpful setup prompt when the Anthropic integration is not configured, guiding users to the integration settings
- **Unified AI Assistant codebase**: Refactored Email and Blog AI assistants to share common code, reducing duplication by ~60% and ensuring consistent behavior
- **Consistent AI Assistant styling**: Both assistants now use the same color scheme for a unified experience

## [22.0] - 2025-12-28

### Features

- **Blog AI Assistant**: AI-powered writing assistant for blog posts

  - Streaming chat with Anthropic Claude models (Opus, Sonnet, Haiku)
  - Tool use for updating blog content and metadata directly in editor
  - Server-side web scraping and search via Firecrawl integration
  - Session cost tracking with input/output token breakdown

- **LLM Integration**: Anthropic API support with encrypted API key storage
- **Firecrawl Integration**: Web scraping (`scrape_url`) and search (`search_web`) tools

## [21.0] - 2025-12-23

### Database Schema Changes

- Migration v21.0 introduces the email queue system:
  - `email_queue` table for unified broadcast and automation email delivery
  - Added `enqueued_count` column to `broadcasts` table
  - Migrated broadcast statuses: `sending` → `processing`, `sent` → `processed`

### Features

- **Email Queue System**: Centralized queue for all outbound marketing emails

  - Unified delivery for broadcasts and automations
  - Priority-based processing with retry logic
  - Per-integration rate limiting
  - Background worker with graceful shutdown

- **Automation Performance**: Single-tick execution optimization

  - Process multiple nodes per scheduler tick until delay or completion
  - 10-node safety limit per tick prevents runaway loops
  - State persisted after each node for crash recovery

- **Automation Timeline Events**: Track contact journey lifecycle
  - `automation.start` event on enrollment
  - `automation.end` event on completion/exit/failure

### Breaking Changes

- Broadcast statuses renamed: `sending` → `processing`, `sent` → `processed`

## [20.0] - 2025-12-21

### Database Schema Changes

- Migration v20.0 introduces the automations system with 4 new workspace tables:
  - `automations` - Workflow definitions with trigger config, nodes, and statistics
  - `contact_automations` - Tracks each contact's journey through automations
  - `automation_node_executions` - Audit log of node executions for debugging
  - `automation_trigger_log` - Trigger event logging

### Features

- **Marketing Automations**: Visual workflow builder for automated contact journeys

  - Event-driven triggers from contact timeline (contact, list, segment, email, custom events)
  - Trigger frequency control: `once` (first occurrence) or `every_time`
  - Conditional triggers using segment filter conditions
  - Field-specific triggers for contact updates (e.g., trigger only when `custom_string_1` changes)

- **Automation Node Types**:

  - **Trigger**: Entry point based on timeline events with configurable conditions
  - **Delay**: Pause workflow for minutes, hours, or days
  - **Email**: Send templated emails using workspace email provider with tracking
  - **Branch**: Conditional branching with multiple paths based on segment conditions
  - **Filter**: Pass/fail routing based on contact attributes
  - **Add to List**: Subscribe contacts to additional lists
  - **Remove from List**: Unsubscribe contacts from lists
  - **A/B Test**: Deterministic variant selection using FNV-32a hashing for consistent splits
  - **Webhook**: POST contact data to external URLs with authorization headers

- **Visual Flow Editor**: Drag-and-drop canvas for designing automation workflows

  - Node positioning with visual connections
  - Type-specific configuration panels
  - Real-time validation

- **Automation Lifecycle Management**:

  - Draft mode for building and testing
  - Activate to go live (creates PostgreSQL triggers)
  - Pause to stop new enrollments while preserving in-progress journeys
  - Soft-delete with recovery capability

- **Contact Journey Tracking**:

  - Full audit trail of node executions with timestamps and duration
  - Contact status tracking (active, completed, exited, failed)
  - Exit reasons for debugging
  - Node execution output logging

- **Execution Engine**:

  - Background scheduler polling every 10 seconds
  - Batch processing (50 contacts per batch)
  - Round-robin workload distribution across workspaces
  - Retry logic with exponential backoff (max 5 retries)
  - Graceful shutdown handling

- **Statistics Dashboard**:

  - Enrolled contacts count
  - Completed journeys
  - Exited contacts (with reasons)
  - Failed executions

- **API Endpoints** (`/api/automations.*`):
  - `create`, `get`, `list`, `update`, `delete`
  - `activate`, `pause` for lifecycle management
  - `nodeExecutions` for contact journey audit trail

## [19.6] - 2025-12-19

- Fix: SMTP integration now works with strict SMTP servers (#172)
  - Replaced go-mail SMTP client with raw SMTP command implementation
  - MAIL FROM command no longer includes BODY=8BITMIME or SMTPUTF8 extensions that caused "501 5.5.4 Syntax error in parameters" errors
  - Message composition (MIME, headers, attachments) still handled by go-mail

## [19.5] - 2025-12-16

- Fix: Task completion now saves final state to prevent stale progress display in UI (#157)
- Fix: Prevent concurrent task execution race condition that could cause duplicate broadcast emails
- Fix: Unsubscribes via notification center link are now tracked in broadcast statistics (#165)
- Fix: Email builder now respects column width attributes in section blocks
- Fix: Contact bulk import now handles duplicate emails in a single batch (#167)
- Fix: Image alt text now supports Liquid template variables (#168)

## [19.4] - 2025-12-12

- Fix: Broadcast recipient count mismatch - `CountContactsForBroadcast` now filters soft-deleted lists consistently with `GetContactsForBroadcast`
- Fix: Contact list pagination now uses nanosecond precision timestamps to prevent skipping contacts created within the same second (#159)
- Fix: Broadcast delivery now uses deterministic ordering (`created_at ASC, email ASC`) to prevent skipping contacts with identical timestamps during bulk imports (#157)
- Enhancement: SES configuration set is now optional for transactional emails
- Fix: `mailto:` links are no longer tracked (prevents broken email client links)

## [19.3] - 2025-12-09

- Fix SES non-ASCII characters in email local part (e.g., `Añejandramendo@gmail.com`) now encoded with RFC 2047

## [19.2] - 2025-12-04

- fix update contact form
- fix non-ASCII characters in SES
- fix team table overflow when email is too long (#149)
- add TLS switch to setup wizard SMTP settings

## [19.1] - 2025-12-02

- Enhancement: Added `full_name` field to the Add Contact drawer

## [19.0] - 2025-12-01

### Features

- **Outgoing Webhooks**: Subscribe to workspace events and receive HTTP notifications
  - CRUD API for webhook subscriptions (`/api/webhookSubscriptions.*`)
  - Event types: `contact.*`, `email.*`, `list.*`, `segment.*`, `custom_event.*`
  - HMAC-SHA256 signature verification (Standard Webhooks spec)
  - Automatic retries with exponential backoff (up to 10 attempts over 24h)
  - Custom event filters for fine-grained subscription control
  - Test webhook endpoint for integration debugging
  - Delivery logs with 7-day retention
- **Contact `full_name` field**: Native field for systems without separate first/last names

### Bug Fixes

- Fixed timeline timestamps showing incorrect times (now uses `CURRENT_TIMESTAMP` in triggers)
- Fixed JSON field filters with number/time values failing validation (#140)
- Fixed contact scan error on migrated databases due to column ordering mismatch (replaced `SELECT *` with explicit column list)

### Breaking Changes

- Renamed `webhook_events` table to `inbound_webhook_events` to distinguish from outgoing webhooks

## [18.3] - 2025-11-30

- Fix: SEO settings not being persisted when creating a new blog post
- Enhancement: Featured image thumbnail now displays in the Title column with a popover for full-size preview in the blog posts list

## [18.2] - 2025-11-29

### Changes

- **API Endpoints**: Renamed custom events endpoints from singular to plural for consistency
  - `POST /api/customEvent.upsert` → `POST /api/customEvents.upsert`
  - `POST /api/customEvent.import` → `POST /api/customEvents.import`
  - `GET /api/customEvent.get` → `GET /api/customEvents.get`
  - `GET /api/customEvent.list` → `GET /api/customEvents.list`

## [18.1] - 2025-11-29

### Enhancements

- **File Manager**: Added support for modern image formats (WebP, AVIF, JPEG XL)
  - `.webp` files now display with proper image previews
  - `.avif` files now display with proper image previews
  - `.jxl` (JPEG XL) files now display with proper image previews

## [18.0] - 2025-11-29

### Database Schema Changes

- Migration v18.0 introduces custom events tracking system
- Removed deprecated contact fields: `lifetime_value`, `orders_count`, `last_order_at`
- Renamed contact timeline event kinds to semantic dotted format:
  - Contact: `insert_contact` → `contact.created`, `update_contact` → `contact.updated`
  - Lists: `insert_contact_list` → `list.subscribed`/`list.pending`, status changes → `list.confirmed`/`list.resubscribed`/`list.unsubscribed`/`list.bounced`/`list.complained`
  - Segments: `join_segment` → `segment.joined`, `leave_segment` → `segment.left`
- Added `custom_events` table for tracking user behavior and goals
- Added computed fields in segmentation engine for custom events goal aggregations

### Features

- **Custom Events API**: Track user behavior and conversion goals

  - `POST /api/customEvents.upsert` - Create or update a single event
  - `POST /api/customEvents.import` - Batch import up to 50 events
  - `GET /api/customEvents.get` - Retrieve event by workspace, event name, and external ID
  - `GET /api/customEvents.list` - List events by email or event name
  - Goal tracking with types: `purchase`, `subscription`, `lead`, `signup`, `booking`, `trial`, `other`
  - Soft-delete support via `deleted_at` field

- **Segmentation with Custom Events Goals**: Build segments based on goal aggregations
  ```json
  {
    "source": "custom_events_goals",
    "custom_events_goal": {
      "goal_type": "purchase",
      "aggregate_operator": "sum",
      "operator": "gte",
      "value": 500,
      "timeframe_operator": "in_the_last_days",
      "timeframe_values": ["30"]
    }
  }
  ```
  - Aggregate operators: `sum`, `count`, `avg`, `min`, `max`
  - Timeframe operators: `anytime`, `in_the_last_days`, `in_date_range`, `before_date`, `after_date`

### Breaking Changes

- Removed `lifetime_value`, `orders_count`, `last_order_at` from Contact model
- Segments using deprecated contact fields will be deleted during migration
- Contact timeline event kinds renamed (existing data migrated automatically)
- Segment tree `TreeNodeLeaf.table` field renamed to `source` (existing segments migrated automatically)

## [17.4] - 2025-11-28

- Enhancement: The File Manager now supports S3-compatible storage providers with path-style bucket endpoint.

## [17.3] - 2025-11-27

- New feature: Blog posts can now be scheduled for publication at past or future dates, allowing you to plan posts in advance or import posts with their original publication dates.

## [17.2] - 2025-11-27

- Fix: retrieve broadcasts list when `pause_reason` or `winning_template` is null to avoid SQL errors
- Fix: blog cache is cleared when updating mailing lists to ensure subscription forms work correctly
- Enahncement: blog theme shows a helpful error message when no public lists are configured for newsletter subscription forms

## [17.1] - 2025-11-27

- Fix: Invitation links now correctly point to `/console/accept-invitation` instead of `/accept-invitation` to match the new console path

## [17.0] - 2025-11-26

### Database Schema Changes

- Migration v17.0 introduces blog feature support

### Features

- **Blog Feature**: Full-featured blogging system with advanced templating capabilities

  - Notion-like blog post editor for intuitive content creation
  - Full control over templating using Liquid syntax for dynamic content
  - The blog is served at the root path `/` of the custom domain configured in the workspace settings when it's enabled

- **Segmentation Engine JSON Support**: Enhanced contact segmentation with JSON attribute matching
  - Support for matching contact `custom_json_x` attributes in segmentation rules
  - Advanced filtering capabilities for complex contact data structures
  - Improved targeting precision for email campaigns and contact management

### Enhancements

- **Template Permission Updates**: Custom email blocks now use template write permissions instead of workspace write permissions for better security granularity
- **Auto-unsubscribe Enhancement**: Notification center now automatically processes unsubscribe actions when the unsubscribe link loads, improving user experience

### Breaking Changes

- The console UI is now serverd at `/console` instead of `/` to avoid conflicts with the new blog feature. When the blog is disabled the `/` path will redirect to `/console`.
- Permission system updated: saving custom email blocks requires `templates:write` permission instead of `workspace:write`

## [16.3] - 2025-11-15

### Fixes

- Fix: Replace hardcoded SMTP PLAIN auth with auto-discover to support Azure Communication Email and other providers requiring LOGIN authentication

## [16.2] - 2025-11-14

### Fixes

- Fix: Setup wizard redirection loop when API_ENDPOINT differs from actual console host name
- Add `/healthz` endpoint for container health checks that pings the database

## [16.1] - 2025-11-06

### Features

- **SMTP Relay for Transactional Emails**: Built-in SMTP relay server for sending transactional emails via standard SMTP clients
  - Send transactional emails using SMTP protocol instead of HTTP API calls
  - Useful for integrating legacy systems and standard email libraries
  - TLS encryption support on port 587 with STARTTLS
  - Authentication using workspace API credentials
  - Email body contains JSON payload matching Transactional API format
  - Configuration via environment variables: `SMTP_RELAY_ENABLED`, `SMTP_RELAY_PORT`, `SMTP_RELAY_DOMAIN`, `SMTP_RELAY_TLS_CERT_BASE64`, `SMTP_RELAY_TLS_KEY_BASE64`

## [16.0] - 2025-11-03

### Database Schema Changes

- Added `integration_id` column to `transactional_notifications` and `templates` tables for integration-managed resources

### Security

- **Message Data Encryption**: Template data variables are now encrypted at rest in the database
  - Message history `message_data.data` field is encrypted using AES-256-GCM with workspace secret key
  - Only new messages are encrypted (no migration of existing data)
  - Automatic encryption on message creation and decryption on retrieval
  - Backward compatible: unencrypted messages from before this version can still be read
  - Metadata remains unencrypted for query performance
  - Protects sensitive data like tokens, API keys, and personal information in template variables

### Features

- **Supabase Integration**: Connect Supabase Auth with Notifuse
  - Auth Email Hook: Send branded authentication emails (signup, magic link, password recovery, email change, reauthentication)
  - Before User Created Hook: Automatically sync new Supabase users to Notifuse contacts
  - Auto-generated email templates for all Supabase auth events with proper Liquid variables
  - HMAC-SHA256 webhook signature verification for security
  - Optional automatic list subscription and disposable email rejection
  - Field mapping: Supabase `user_metadata` to configurable Notifuse custom JSON field
- Integration-managed templates and transactional notifications cannot be deleted (but can be edited)

## [15.0] - 2025-11-01

### 🔒 SECURITY UPGRADE: PASETO → JWT + Enhanced Authentication Security

This is a **major security release** that migrates the authentication system from PASETO to JWT (HS256) and implements comprehensive security improvements.

### ⚠️ BREAKING CHANGES

**Migration Requirements:**

- **REQUIRED**: Set `SECRET_KEY` environment variable before upgrading
  - **CRITICAL FOR EXISTING DEPLOYMENTS**:
    - If you already have `SECRET_KEY` set: **Keep it unchanged** (do not generate a new one)
    - If migrating from PASETO: Use your existing PASETO key: `export SECRET_KEY="$PASETO_PRIVATE_KEY"`
  - **For new installations only**: Generate new key: `export SECRET_KEY=$(openssl rand -base64 32)`
- Server will automatically restart after migration to reload JWT configuration

**🚨 CRITICAL WARNING**:

- **DO NOT change your existing SECRET_KEY** - it encrypts all workspace integration secrets (email provider API keys, SMTP passwords, etc.)
- Changing SECRET_KEY will:
  - ❌ Make all encrypted integration secrets unreadable (permanent data loss)
  - ❌ Break all email sending (SparkPost, Mailjet, Mailgun, SMTP credentials lost)
  - ❌ Invalidate all JWT tokens and user sessions
- **Only generate a new SECRET_KEY for fresh installations with no existing data**

**What Gets Invalidated (During PASETO → JWT Migration):**

- ✗ All user sessions (users must log in again - PASETO tokens → JWT tokens)
- ✗ All API keys (must be regenerated - PASETO format → JWT format)
- ✗ All pending workspace invitations (invitation tokens were PASETO-signed)
- ✗ All active magic codes (migrating from plain-text → HMAC-SHA256 hashes)

**Why:** PASETO tokens are incompatible with JWT verification. Clean migration ensures no security gaps.

**Important Notes:**

- If you're already using JWT (not migrating from PASETO), and you keep your existing `SECRET_KEY`, your existing sessions remain valid.
- The `SECRET_KEY` is also used to encrypt workspace integration secrets (API keys, SMTP credentials). **Never change it on existing deployments** or you'll lose access to all encrypted credentials permanently.

### Security Improvements

#### 1. **JWT Authentication (HS256)**

- Migrated from PASETO to industry-standard JWT with HMAC-SHA256 signing
- **Simplified setup**: Uses symmetric key (`SECRET_KEY`) instead of PASETO's asymmetric key pair
  - No need to generate and manage separate public/private keys
  - Single `SECRET_KEY` environment variable for all cryptographic operations
  - Easier deployment and configuration management
- Algorithm confusion attack prevention (strict HMAC validation)
- Comprehensive token validation (signature, expiration, claims)
- Compatible with standard JWT libraries and tools

#### 2. **HMAC-Protected Magic Codes**

- Magic codes now stored as HMAC-SHA256 hashes (no plain text in database)
- Database compromise cannot reveal authentication codes
- Constant-time comparison prevents timing attacks
- Migration clears all existing plain-text codes

#### 3. **Server-Side Logout**

- New `/api/user.logout` endpoint (POST, requires authentication)
- Deletes ALL sessions for the authenticated user from database
- Tokens become immediately invalid after logout
- Protected endpoints now verify session exists in database
- Returns 401 Unauthorized if session has been deleted
- Frontend integration with graceful error handling

#### 4. **Rate Limiting for Authentication Endpoints**

- Protection against brute force attacks and email bombing
- In-memory rate limiter with sliding window algorithm
- Sign-in endpoint: 5 attempts per 5 minutes per email address
- Verify code endpoint: 5 attempts per 5 minutes per email address
- Rate limiter automatically resets on successful authentication
- Thread-safe concurrent access with automatic cleanup
- Independent rate limits per user and per endpoint
- Prevents magic code brute force attacks (blocks 99%+ of attempts)

### Features

- Enhanced session verification in `GetCurrentUser` endpoint
- Added `DeleteAllSessionsByUserID` method to user repository
- Added `Logout` method to user service interface
- Frontend `AuthContext` now calls backend logout before clearing local storage
- Migration automatically cleans up incompatible authentication artifacts

### Testing

- New integration tests for logout functionality (`tests/integration/user_logout_test.go`)
- New integration tests for rate limiter (`tests/integration/rate_limiter_test.go`)
- Unit tests for rate limiter with race detection
- Fixed race condition in concurrent rate limiter test (atomic operations)
- All tests pass with `-race` flag enabled

### Documentation

- Updated security audit document (`SECURITY_AUDIT_JWT_SESSIONS.md`)
- Changed "No Server-Side Logout" from 🔴 CRITICAL to ✅ IMPLEMENTED
- Changed "Brute Force Risk" from MEDIUM to LOW
- Added detailed implementation notes and testing coverage
- Comprehensive migration guide in v15 migration file

### Post-Migration Actions Required

1. **Users**: Log in again with email/password (or magic code)
2. **API Key Holders**: Regenerate API keys in Settings → API Keys
3. **Integrations**: Update all API integrations with new keys
4. **Workspace Admins**: Resend pending invitations via Settings → Members → Invitations

### Migration Notes

- Migration v15 is idempotent and safe to run multiple times
- Estimated migration time: < 1 second
- Server automatically restarts after migration
- Migration validates `SECRET_KEY` environment variable before proceeding
- Comprehensive migration summary displayed in console

## [14.1] - 2025-11-01

### Features

- **Bulk Contact Import**: New `/api/contacts.import` endpoint for efficiently importing large numbers of contacts
  - Creates or updates multiple contacts in a single batch operation using PostgreSQL bulk upsert
  - Returns individual operation results (created/updated/error) for each contact
  - Optional bulk subscription to lists via `subscribe_to_lists` parameter
  - Significantly faster than individual upsert operations for large imports
  - Batch size of 100 contacts processed at a time in the UI
  - Supports partial success - some contacts can succeed while others fail validation

## [14.0] - 2025-10-31

### Database Schema Changes

- Added `channel_options` JSONB column to `message_history` table to store email/SMS/push delivery options (CC, BCC, FromName, ReplyTo...)

### Features

- **Internal Task Scheduler**: Tasks now execute automatically every 30 seconds
  - No external cron job required
  - Configurable via `TASK_SCHEDULER_ENABLED`, `TASK_SCHEDULER_INTERVAL`, `TASK_SCHEDULER_MAX_TASKS`
  - Starts automatically with the app, stops gracefully on shutdown
  - Faster task processing (30s vs 60s minimum with external cron)
- **Privacy Settings**: New optional configuration for telemetry and update checks
  - `TELEMETRY` environment variable (optional) - Send anonymous usage statistics
  - `CHECK_FOR_UPDATES` environment variable (optional) - Check for new versions
  - Both can be configured via setup wizard if not set as environment variables
  - For existing installations: migration v14 sets both to `true` by default (respects env vars if set)
  - Environment variables always take precedence over database settings
- Message history now stores email delivery options:
  - CC (carbon copy recipients)
  - BCC (blind carbon copy recipients)
  - FromName (sender display name override)
  - ReplyTo (reply-to address override)
- Message preview drawer displays email delivery options when present
- Only stores email options in this version (SMS/push to be added later)
- Modernized Docker Compose to use current standards: renamed `docker-compose.yml` to `compose.yaml`, removed deprecated `version` field, updated commands to use `docker compose` plugin syntax, and improved `.env` file integration

### UI Changes

- Removed cron setup instructions from setup wizard
- Removed cron status warning banner from workspace layout
- Simpler onboarding experience - no manual cron configuration needed
- Added preview mode to notification center
- **Setup Wizard Improvements**:
  - Added newsletter subscription option
  - PASETO keys configuration moved to collapsible "Advanced Settings" section
  - Added "Privacy Settings" section for telemetry and update check configuration
  - Improved restart handling: displays setup completion screen immediately while server restarts
  - User can review generated keys before manually redirecting to signin

### Deprecated (kept for backward compatibility)

- `/api/cron` HTTP endpoint (internal scheduler is now primary)
- `/api/cron.status` HTTP endpoint (still functional but not advertised)

### Fixes

- Fix: SMTP now supports unauthenticated/anonymous connections (e.g., local mail relays on port 25)
- Fix: Docker images now built with CGO disabled to prevent SIGILL crashes on older CPUs
- Fix: Decode HTML entities in URL attributes to ensure links with query parameters work correctly in MJML-compiled emails
- Fix: Normalize browser timezone names to canonical IANA format to prevent timezone mismatch errors
- Fix: Broadcast pause also pauses the associated task

### Migration Notes

- Added `ShouldRestartServer()` method to migration interface
- Migrations can now trigger automatic server restart when config reload is needed
- Existing messages will have `channel_options = NULL` (no backfill)
- Migration v14 adds default telemetry and update check settings for existing installations (both default to `true`)
- Migration is idempotent and safe to run multiple times
- Estimated migration time: < 1 second per workspace
- Server will automatically restart after migration to reload all configuration settings

## [13.7] - 2025-10-25

- New feature: transactional email API now supports `from_name` parameter to override the default sender name

## [13.6] - 2025-10-24

- Upgrade github.com/wneessen/go-mail from v0.7.1 to v0.7.2

## [13.5] - 2025-10-23

- Fix: SMTP transport now supports multiple CC and BCC recipients

## [13.4] - 2025-10-22

- Fix: segment filters now support multiple values for contains/not_contains operators
- Multiple values are combined with OR logic as indicated in the UI

## [13.3] - 2025-10-11

- Fix: custom field labels now display consistently in contacts table column headers and JSON viewer popups
- Contacts table columns now use custom field labels from workspace settings instead of generic defaults
- JSON custom fields now show custom labels in their popover titles

## [13.2] - 2025-10-10

- Add new filters to message history: filter by message ID, external ID, and list ID
- List ID filter supports searching messages sent to a specific list
- New feature: customize display names for contact custom fields in workspace settings

## [13.1] - 2025-10-09

- Fix SMTP form default `use_tls` not being included in form submissions

## [13.0] - 2025-10-09

- New feature: segmentation engine now supports relative dates (e.g., "in the last 30 days")
- Segments containing relative dates are automatically refreshed every day at 5am in the segment timezone
- Fix critical regression introduced in v11 that blocked broadcast sending

## [12.0] - 2025-10-08

- Move rate limit configuration from broadcast audience settings to email integration settings
- Rate limit is now a required field on email integrations (default: 25 emails/minute)
- Simplifies broadcast configuration and centralizes rate limiting at the integration level
- Migration v12 automatically sets default rate limit on all existing email integrations

## [11.0] - 2025-10-08

- New feature: setup wizard for initial configuration
- Many environment variables are now optional and can be configured through the setup wizard: `ROOT_EMAIL`, `API_ENDPOINT`, `PASETO_PRIVATE_KEY`, `PASETO_PUBLIC_KEY`, `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM_EMAIL`, `SMTP_FROM_NAME`
- Database environment variables remain required: `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`
- `SECRET_KEY` remains required (or `PASETO_PRIVATE_KEY` as backward compatibility fallback) to encrypt sensitive data in the database
- Configuration can be provided through setup wizard on first install and stored securely in database
- PASETO keys can be generated automatically and shown at the end of the setup wizard
- Environment variables always override database settings when present

## [10.0] - 2025-10-07

- New feature: automatic contact list status updates on bounce and complaint events
- Add `list_ids` TEXT[] column to `message_history` table to track which lists a message was sent to
- Add database trigger to automatically update `contact_lists` status to 'bounced' or 'complained' when hard bounces or complaints occur
- Distinguish between hard bounces (permanent failures) and soft bounces (temporary failures) - only hard bounces affect contact status
- Status hierarchy: complained > bounced > other statuses
- Backfill historical broadcast messages with list associations
- Render broadcast name in message logs
- escape special characters in MJML import/export

## [9.0] - 2025-10-06

- New feature: email attachments support for transactional emails
- Add `message_attachments` table for deduplication and storage of attachment content
- Add `attachments` JSONB column to `message_history` table
- Support for attachments across all ESP integrations (SMTP, SES, SparkPost, Postmark, Mailgun, Mailjet)
- Maximum 20 attachments per email, 3MB per file, 10MB total

## [8.0] - 2025-10-04

- New feature: real-time contact segmentation engine
- Add `db_created_at` and `db_updated_at` fields to contacts table for accurate database tracking
- Add `kind` field to contact timeline for granular event types (e.g., open_email, click_email)
- Make `created_at` and `updated_at` optional with database defaults to support historical data imports
- Ensure all timestamps stored in UTC timezone

## [7.1] - 2025-10-04

- Fix panic when broadcast rate limit is set to less than 60 emails per minute
- Improve rate limiting calculation to properly handle low rate limits

## [7.0] - 2025-10-02

- New feature: contact events timeline (messages, webhook events, profile mutations etc...). It's the backbone of the upcoming automations feature.

## [6.11] - 2025-09-30

- Implement per-broadcast rate limiting functionality
- Add support for broadcast-specific rate limits that override system defaults
- Make rate limit field required in broadcast form with default value of 25 emails/minute
- Add comprehensive test coverage for per-broadcast rate limiting

## [6.10] - 2025-09-29

- Upgrade github.com/wneessen/go-mail from v0.6.2 to v0.7.1

## [6.9] - 2025-09-28

- Add cron status monitoring endpoint `/api/cron.status`
- Add SettingRepository for managing application settings
- Add automatic cron health checking in frontend console
- Add visual banner with setup instructions when cron is not running
- Update TaskService to track last cron execution timestamp

## [6.8] - 2025-09-24

- Fix scheduled broadcast time handling to use string format instead of time.Time
- Remove broadcast service dependency from task service tests
- Update ParseScheduledDateTime tests to match implementation behavior

## [6.7] - 2025-09-19

- Add new workspace dashboard

## [6.6] - 2025-09-12

- Bulk update contacts functionality to console

## [6.5] - 2025-09-10

- Add delete contact functionality to console
- Redact email addresses in message history and webhook events when deleting a contact

## [6.4] - 2025-09-10

- Add test email functionality to broadcast variations
- Fix permissions for test emails to require read template and write contact permissions

## [6.3] - 2025-09-08

- Fix set permissions on root user
- Force all permissions to owners

## [6.2] - 2025-09-08

- Fix circuit breaker error message in broadcast pause reason
- Simplify broadcast circuit breaker notification email

## [6.1] - 2025-09-07

- hide menu items in console when user doesn't have access to the resource
- disabled create/update buttons in console when user doesn't have write permissions

## [6.0] - 2025-09-07

- Add permissions with roles per workspace

## [5.0] - 2025-09-06

- Add pause_reason column to the broadcasts table to store the reason for broadcast pause
- Pause broadcasts when circuit breaker is triggered
- Add system notification service to email circuit breaker events

## [4.0] - 2025-09-06

- Add migrations to the system and workspace databases
- Add permissions column to the user_workspaces table for future permission management
- Add UI previsions about broadcast rate limit per hour/day

## [3.14] - 2025-09-05

- Fix VARCHAR(255) constraint for status_info in message_history table

## [3.13] - 2025-09-03

- Fix z-index for file manager in template editor
- Improve broadcast UI with remaining test time, refresh button, and variations stats
- Improve transactional email API command modal with more examples and better documentation

## [3.12] - 2025-09-02

### Security

- Only root user can create new workspaces
- Added server-side validation to restrict workspace creation to the user specified in `ROOT_EMAIL` environment variable
- Create workspace UI elements are now hidden for non-root users in the console interface

## [v3.11] - 2025-09-01

### Fixed

- Hide deleted list in notification center when user has subscribed

## [v3.10] - 2025-09-01

### Added

- View a resend member invitations
- Access template test data in "Send test template" transactional email

## [v3.9] - 2025-08-31

### Added

- Mailgun integration now supports broadcast campaigns and newsletters, in addition to transactional emails

## [v3.8] - 2025-08-30

### Fixed

- Fixed issue: accept invitation

## [v3.7] - 2025-08-28

### Added

- New feature: custom endpoint URL in workspace settings to customize the tracking links and notification center URLs

## [v3.6] - 2025-08-28

### Fixed

- MJML raw-block is now editable

## [v3.5] - 2025-08-28

### Changed

- Dates format is only English

## [v3.4] - 2025-08-27

### Changed

- Anonymous users can't signin anymore, they need to be invited to a workspace

## [v3.3] - 2025-08-27

### Deprecated

- The SECRET_KEY env var is now deprecated, and uses the PASETO_PRIVATE_KEY value to simplify deployments

## [v3.2] - 2025-08-27

### Added

- Install Notifuse quickly for non-production workload using a Docker compose that embeds Postgres

## [v3.1] - 2025-08-25

### Added

- Launch of the new Notifuse V3
