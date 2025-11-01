# Changelog

All notable changes to this project will be documented in this file.

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
