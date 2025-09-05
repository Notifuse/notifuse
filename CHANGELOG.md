# Changelog

All notable changes to this project will be documented in this file.

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
