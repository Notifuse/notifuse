# TODO

## custom events

- remove:
  CREATE INDEX IF NOT EXISTS idx_custom_events_integration_id
  ON custom_events(integration_id)
  WHERE integration_id IS NOT NULL;

## Content

- send newsletter to all contacts+previous customers
- post supabase on twitter and facebook
- page vs phplist
- page vs sendy
- page vs mailerlite
- page vs mautic

## Eventual features

- server settings panel for root user
- better design for system email (use MJML for template)
- add contact_list reason string

## Roadmap

- check for updates + newsletter box
- automations with async triggers
