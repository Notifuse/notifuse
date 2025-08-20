# Notifuse

[![Go Report Card](https://img.shields.io/badge/go%20report-A+-brightgreen.svg?style=flat)](https://goreportcard.com/report/github.com/Notifuse/notifuse)
[![Go](https://github.com/Notifuse/notifuse/actions/workflows/go.yml/badge.svg)](https://github.com/Notifuse/notifuse/actions/workflows/go.yml)
[![codecov](https://codecov.io/gh/Notifuse/notifuse/graph/badge.svg?token=VZ0HBEM9OZ)](https://codecov.io/gh/Notifuse/notifuse)

**The open-source alternative to Mailchimp, Brevo, Mailjet, Listmonk, Mailerlite, and Klaviyo, Loop.so, etc.**

Notifuse is a modern, self-hosted email marketing platform that allows you to send newsletters and transactional emails at a fraction of the cost. Built with Go and React, it provides enterprise-grade features with the flexibility of open-source software.

## 🚀 Key Features

### 📧 Email Marketing

- **Visual Email Builder**: Drag-and-drop editor with MJML components and real-time preview
- **Campaign Management**: Create, schedule, and send targeted email campaigns
- **A/B Testing**: Optimize campaigns with built-in testing for subject lines, content, and send times
- **List Management**: Advanced subscriber segmentation and list organization
- **Contact Profiles**: Rich contact management with custom fields and detailed profiles

### 🔧 Developer-Friendly

- **Transactional API**: Powerful REST API for automated email delivery
- **Webhook Integration**: Real-time event notifications and integrations
- **Liquid Templating**: Dynamic content with variables like `{{ contact.first_name }}`
- **Multi-Provider Support**: Connect with Amazon SES, SendGrid, Mailgun, Postmark, Mailjet, SparkPost, and SMTP

### 📊 Analytics & Insights

- **Open & Click Tracking**: Detailed engagement metrics and campaign performance
- **Real-time Analytics**: Monitor delivery rates, opens, clicks, and conversions
- **Campaign Reports**: Comprehensive reporting and analytics dashboard

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

## 🚀 Quick Start

### Using Docker Compose (Recommended)

```bash
# Clone the repository
git clone https://github.com/Notifuse/notifuse.git
cd notifuse

# Start with Docker Compose
docker-compose up -d

# Access the console
open http://localhost:8080
```

### Manual Installation

```bash
# Install dependencies
go mod download
cd console && npm install

# Build the application
make build

# Run the server
./bin/notifuse
```

## 🔧 Configuration

Notifuse can be configured through environment variables or configuration files:

```env
# Server configuration
SERVER_PORT=8080
SERVER_HOST=0.0.0.0
ROOT_EMAIL=your-email@example.com
CORS_ALLOW_ORIGIN=*
ENVIRONMENT=production
API_ENDPOINT=notifuse.your_website.com

# Database configuration
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=
DB_PREFIX=notifuse
DB_NAME=${DB_PREFIX}_system
DB_SSLMODE=require

# Other configurations can be added here
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USERNAME=your-username
SMTP_PASSWORD=your-password
SMTP_FROM_EMAIL=noreply@example.com
SMTP_FROM_NAME=Notifuse

# Security
PASETO_PRIVATE_KEY="base64:your-private-key"
PASETO_PUBLIC_KEY="base64:your-public-key"
SECRET_KEY="your_secret_key_for_db_secrets_encryption"

# Tracing
TRACING_ENABLED=true
TRACING_SERVICE_NAME=notifuse
TRACING_SAMPLING_PROBABILITY=
# Trace exporter configuration: jaeger | zipkin | stackdriver | datadog | xray | none
TRACING_TRACE_EXPORTER="none"
# Jaeger settings
TRACING_JAEGER_ENDPOINT="http://localhost:14268/api/traces"
# Zipkin settings
TRACING_ZIPKIN_ENDPOINT="http://localhost:9411/api/v2/spans"
# Stackdriver settings
TRACING_STACKDRIVER_PROJECT_ID=""
# Azure Monitor settings
TRACING_AZURE_INSTRUMENTATION_KEY=""
# Datadog settings
TRACING_DATADOG_AGENT_ADDRESS="localhost:8126"
TRACING_DATADOG_API_KEY=""
# AWS X-Ray settings
TRACING_XRAY_REGION="us-west-2"
# General agent endpoint (for exporters that support a common agent)
TRACING_AGENT_ENDPOINT="localhost:6831"
# Metrics exporter configuration: stackdriver | prometheus | datadog
TRACING_METRICS_EXPORTER="prometheus"
# Prometheus settings
TRACING_PROMETHEUS_PORT=9464
```

## 📚 Documentation

- **[Complete Documentation](https://docs.notifuse.com)** - Comprehensive guides and tutorials

## 🤝 Contributing

We welcome contributions! Please see our [Contributing Guide](https://docs.notifuse.com/development/contributing) for details.

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## 📄 License

Notifuse is released under the [Elastic License 2.0](LICENSE).

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

**Ready to get started?** [Try the live demo](https://demo.notifuse.com) or [deploy your own instance](https://docs.notifuse.com) in minutes.
