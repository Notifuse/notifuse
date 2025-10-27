# Changelog

All notable changes to this project will be documented in this file.

## [13.7] - 2025-10-25

- New feature: transactional email API now supports `from_name` parameter to override the default sender name
- Fix: SMTP now supports unauthenticated/anonymous connections (e.g., local mail relays on port 25)
- Magic code emails, workspace invitations, and circuit breaker alerts now work without SMTP credentials
- SMTP authentication is only configured when both username and password are provided

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
