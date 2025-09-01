# Changelog

All notable changes to this project will be documented in this file.

## [v3.11] - 2024-12-22

### Fixed

- Hide deleted list in notification center when user has subscribed

## [v3.10] - 2024-12-21

### Added

- View a resend member invitations
- Access template test data in "Send test template" transactional email

## [v3.9] - 2024-12-20

### Added

- Mailgun integration now supports broadcast campaigns and newsletters, in addition to transactional emails

## [v3.8] - 2024-12-19

### Fixed

- Fixed issue: accept invitation

## [v3.7] - 2024-12-19

### Added

- New feature: custom endpoint URL in workspace settings to customize the tracking links and notification center URLs

## [v3.6] - 2024-08-28

### Fixed

- MJML raw-block is now editable

## [v3.5] - 2024-08-28

### Changed

- Dates format is only English

## [v3.4] - 2024-08-27

### Changed

- Anonymous users can't signin anymore, they need to be invited to a workspace

## [v3.3] - 2024-08-27

### Deprecated

- The SECRET_KEY env var is now deprecated, and uses the PASETO_PRIVATE_KEY value to simplify deployments

## [v3.2] - 2024-08-27

### Added

- Install Notifuse quickly for non-production workload using a Docker compose that embeds Postgres

## [v3.1] - 2024-08-25

### Added

- Launch of the new Notifuse V3
