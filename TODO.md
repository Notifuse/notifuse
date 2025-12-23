# TODO

- use analytics endpoint for email queue stats

- docs: blog setup / theme / post
- docs: custom events + segments for goals

- create notifuse blog
- pages on homepage for:
  - rich contact profiles with custom events + segments
  - newsletter campaigns
  - transactional API
  - blog posts

## Content

- send newsletter to all contacts+previous customers
- post supabase on twitter and facebook
- page vs mailerlite
- page vs mautic

## Eventual features

- server settings panel for root user
- better design for system email (use MJML for template)
- add contact_list reason string

## Roadmap

- check for updates + newsletter box
- automations with async triggers

---

Issue 8: Rate Limiter Not Workspace-Isolated

Status: VALID ✓

Confirmed at worker.go:296. Rate limiter key is entry.IntegrationID
only, not workspace-scoped.

---

Issue 9: Message History in Hot Path

Status: VALID ✓

Confirmed at worker.go:436-485. Individual upsert per email with
JSON marshaling and encryption.
