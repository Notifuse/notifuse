# Notifuse

[![Go Report Card](https://img.shields.io/badge/go%20report-A+-brightgreen.svg?style=flat)](https://goreportcard.com/report/github.com/Notifuse/notifuse)
[![Go](https://github.com/Notifuse/notifuse/actions/workflows/go.yml/badge.svg)](https://github.com/Notifuse/notifuse/actions/workflows/go.yml)
[![codecov](https://codecov.io/gh/Notifuse/notifuse/graph/badge.svg?token=VZ0HBEM9OZ)](https://codecov.io/gh/Notifuse/notifuse)

**[☁️ Notifuse Cloud — from $16/month](https://www.notifuse.com/)** · **[🎯 Try the Live Demo](https://demo.notifuse.com/console/signin?email=demo@notifuse.com)**

Skip the setup and get started instantly with **[Notifuse Cloud](https://www.notifuse.com/)** — fully managed hosting starting at just **$16/month**.

**The open-source alternative to Mailchimp, Brevo, Mailjet, Listmonk, Mailerlite, and Klaviyo, Loop.so, etc.**

Notifuse is a modern, self-hosted emailing platform that allows you to send newsletters and transactional emails at a fraction of the cost. Built with Go and React, it provides enterprise-grade features with the flexibility of open-source software.

<img alt="Email Editor" src="https://github.com/user-attachments/assets/f650ac1b-58fd-44fb-884d-e9811255f1e4" />

## 🚀 Key Features

### 📧 Email Marketing

- **Visual Email Builder**: Drag-and-drop editor with MJML components and real-time preview
- **Campaign Management**: Create, schedule, and send targeted email campaigns
- **A/B Testing**: Optimize campaigns with built-in testing for subject lines, content, and send times
- **List Management**: Advanced subscriber segmentation and list organization
- **Contact Profiles**: Rich contact management with custom fields and detailed profiles

### 🔀 Automations

- **Visual Flow Builder**: Build multi-step journeys on a canvas — each automation is a graph of nodes with a single trigger as its root
- **Node Types**: `delay`, `email`, `branch`, `filter`, `add_to_list`, `remove_from_list`, `ab_test`, `webhook`, and `list_status_branch`
- **Timed Delays**: Wait minutes, hours or days between steps
- **Branching & Filters**: Split a journey on contact conditions, or on a contact's status in a given list
- **A/B Testing In-Flow**: Weighted variants inside an automation, not just on broadcasts
- **Webhook Steps**: Call an external URL mid-journey, with retry budget
- **Enrollment Control**: Enroll a contact `once` or `every_time` the trigger fires, with optional `exit_on_reply`
- **Per-Contact State**: Every enrollment tracks its own status (`active`, `completed`, `exited`, `failed`) and current node, with a per-node history (`entered`, `processing`, `completed`, `failed`, `skipped`)
- **Draft / Live / Paused**: Edit safely, then publish — paused automations hold their enrollments

### 🔧 Developer-Friendly

- **Easy Setup**: Interactive setup wizard for quick deployment and configuration
- **Transactional API**: Powerful REST API for automated email delivery
- **Webhook Integration**: Real-time event notifications and integrations
- **Liquid Templating**: Dynamic content with variables like `{{ contact.first_name }}`
- **Multi-Provider Support**: Connect with Amazon SES, Mailgun, Postmark, Mailjet, SparkPost, SendGrid, and SMTP

### 📊 Analytics & Insights

- **Open & Click Tracking**: Detailed engagement metrics and campaign performance
- **Real-time Analytics**: Monitor delivery rates, opens, clicks, and conversions
- **Campaign Reports**: Comprehensive reporting and analytics dashboard

### 🌐 Web Analytics (Staminads)

Privacy-first, cookieless web analytics built in — the [Staminads](https://github.com/staminads) feature set merged into Notifuse, running entirely on PostgreSQL (no ClickHouse, no extra services):

- **Engaged-time sessions**: sessions measure real visible/focused time (TimeScore), not wall-clock guesses; bounce rate is engagement-based
- **Channel attribution**: 40 default rules classify traffic (paid click-ids like gclid/fbclid, organic search, social, email) — fully editable per workspace, with one-click backfill of historical data when rules change
- **Goals & custom dimensions**: conversion tracking with values and properties, plus 10 custom dimension slots (`custom_1..custom_10`)
- **Cookieless & GDPR-friendly**: no cookies, no visitor fingerprinting, IPs used only for optional geo lookup and never stored
- **Lightweight SDK**: ~21 KB gzipped script served by your own Notifuse instance (`/na.js`); the user agent is parsed in the browser, so the raw string is never sent to the server
- **Setup**: enable Web Analytics on a workspace, paste the snippet from its settings page, done. Country, region and city come from the MaxMind GeoLite2 City database shipped in the image at `/app/geoip/GeoLite2-City.mmdb` — to keep it fresher, drop your own copy into the mounted `data/` directory as `GeoLite2-City.mmdb` (it takes precedence) or set `GEOIP_DB_PATH` (see [geoipupdate](https://github.com/maxmind/geoipupdate)). This product includes GeoLite2 data created by MaxMind, available from [maxmind.com](https://www.maxmind.com)
- **Data retention**: every session is kept for as long as you keep it. Monthly partitions (`web_sessions_y2026m08` and its `web_pages_*` / `web_goals_*` siblings) are created automatically, and expiring old data is a deliberate `DROP TABLE` on the partitions you no longer want — instant, and the disk comes back immediately
- **Scale path**: data lives in monthly-partitioned tables per workspace; heavy-traffic installs can run [AlloyDB Omni](https://cloud.google.com/alloydb/omni) via `compose.alloydb.yaml` and get columnar-engine acceleration for dashboard queries with zero schema changes
- **Requirements**: PostgreSQL 17 or newer (the shipped compose file stays on 17 — upgrading a major version needs `pg_upgrade`, and PostgreSQL 18 also changed its data directory layout)

### 🎨 Advanced Features

- **S3 File Manager**: Integrated file management with CDN delivery
- **Notification Center**: Centralized notification system for your applications
- **Responsive Templates**: Mobile-optimized email templates
- **Custom Fields**: Flexible contact data management
- **Workspace Management**: Multi-tenant support for teams and agencies

## 🏗️ Architecture

Notifuse follows clean architecture principles with clear separation of concerns:

### Backend (Go)

- **Domain Layer**: Core business logic and entities (`internal/domain/`)
- **Service Layer**: Business logic implementation (`internal/service/`)
- **Repository Layer**: Data access and storage (`internal/repository/`)
- **HTTP Layer**: API handlers and middleware (`internal/http/`)

### Frontend (React)

- **Console**: Admin interface built with React, Ant Design, and TypeScript (`console/`)
- **Notification Center**: Embeddable widget for customer notifications (`notification_center/`)

### Database

- **PostgreSQL**: Primary data storage with Squirrel query builder

## 📁 Project Structure

```
├── cmd/                    # Application entry points
├── internal/               # Private application code
│   ├── domain/            # Business entities and logic
│   ├── service/           # Business logic implementation
│   ├── repository/        # Data access layer
│   ├── http/              # HTTP handlers and middleware
│   └── database/          # Database configuration
├── console/               # React-based admin interface
├── notification_center/   # Embeddable notification widget
├── pkg/                   # Public packages
└── config/                # Configuration files
```

## 📚 Documentation

- **[Complete Documentation](https://docs.notifuse.com)** - Comprehensive guides and tutorials

## 🤝 Contributing

**We don't accept pull requests.** Notifuse is developed in-house, and unsolicited pull requests will be closed without review — please don't spend your time on one.

Issues, on the other hand, are very welcome and are the best way to help:

- **[Report a bug](https://github.com/Notifuse/notifuse/issues)** — steps to reproduce, your version, and what you expected instead
- **[Request a feature](https://github.com/Notifuse/notifuse/issues)** — describe the problem you're trying to solve rather than a specific implementation

The license grants you every right to fork Notifuse and modify it for your own use. If we ever invite a code contribution directly, we'll agree on the terms with you at that point.

## 📄 License

Notifuse is released under the [GNU Affero General Public License v3.0](LICENSE).

## 🆘 Support

- **Documentation**: [docs.notifuse.com](https://docs.notifuse.com)
- **Email Support**: [hello@notifuse.com](mailto:hello@notifuse.com)
- **GitHub Issues**: [Report bugs or request features](https://github.com/Notifuse/notifuse/issues)

## 🌟 Why Choose Notifuse?

- **💰 Cost-Effective**: Self-hosted solution with no per-email pricing
- **🔒 Privacy-First**: Your data stays on your infrastructure
- **🛠️ Customizable**: Open-source with extensive customization options
- **📈 Scalable**: Built to handle millions of emails
- **🚀 Modern**: Built with modern technologies and best practices
- **🔧 Developer-Friendly**: Comprehensive API and webhook support

---

**Ready to get started?** [Try the live demo](https://demo.notifuse.com/console/signin?email=demo@notifuse.com) or [deploy your own instance](https://docs.notifuse.com) in minutes.
