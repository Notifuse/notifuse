# Notifuse Tech Stack Documentation

## Overview

Notifuse is a modern, self-hosted email marketing platform built with a clean architecture approach. The application follows a microservices-inspired design with clear separation between frontend and backend components.

## 🏗️ Architecture

The application follows **Clean Architecture** principles with distinct layers:

- **Domain Layer**: Core business logic and entities
- **Service Layer**: Business logic implementation
- **Repository Layer**: Data access and storage
- **HTTP Layer**: API handlers and middleware
- **Frontend Layer**: Multiple React-based user interfaces

## 🔧 Backend Tech Stack

### Core Framework & Language

- **Language**: Go 1.23.x
- **HTTP Framework**: Standard library `http.ServeMux` (no external web framework)
- **Architecture**: Clean Architecture with dependency injection

### Database & Storage

- **Primary Database**: PostgreSQL 17
- **Query Builder**: Squirrel for type-safe SQL queries
- **Migrations**: Custom migration system
- **Connection Pooling**: Built-in database/sql with OpenCensus integration

### Authentication & Security

- **Token System**: PASETO (Platform-Agnostic Security Tokens)
- **Password Hashing**: bcrypt via golang.org/x/crypto
- **API Security**: Custom middleware for authentication and CORS

### Email & Communication

- **Email Engine**: Multiple provider support:
  - Amazon SES (AWS SDK v1.55.7)
  - SMTP (go-mail v0.6.2)
  - Mailgun, Mailjet, Postmark, SparkPost integrations
- **Template Engine**: Liquid templating (osteele/liquid v1.7.0)
- **MJML Support**: MJML-Go v0.15.0 for email rendering
- **HTML Parsing**: PuerkitoBio/goquery v1.10.2

### Observability & Monitoring

- **Logging**: Zerolog v1.33.0 (structured logging)
- **Tracing**: OpenCensus with multiple exporters:
  - Jaeger, Zipkin, Stackdriver, DataDog, AWS X-Ray
  - Prometheus metrics integration
- **Health Checks**: Built-in health check endpoints

### Configuration & Utilities

- **Configuration**: Viper v1.19.0 for environment/file-based config
- **UUID Generation**: Google UUID v1.6.0
- **JSON Processing**: tidwall/gjson v1.18.0
- **Validation**: asaskevich/govalidator
- **Concurrency**: golang.org/x/sync for advanced synchronization

### Testing & Development

- **Testing Framework**: Standard library testing + Testify v1.9.0
- **Mocking**: GoMock v1.6.0 for interface mocking
- **SQL Mocking**: go-sqlmock v1.5.2 for database testing

## 🎨 Frontend Tech Stack

### Console Application (Admin Interface)

#### Core Framework

- **Framework**: React 18.2.0 with TypeScript 5.2.2
- **Build Tool**: Vite 7.1.3
- **Routing**: TanStack Router v1.15.7 with devtools

#### UI Framework & Styling

- **UI Library**: Ant Design v5.14.0
- **Icons**:
  - Ant Design Icons v5.3.0
  - FontAwesome v6.7.2 (solid, regular, brands)
  - Lucide React v0.487.0
- **Styling**: Tailwind CSS v4.1.10
- **Scrollbars**: OverlayScrollbars React v0.5.6

#### State Management & Data Fetching

- **Data Fetching**: TanStack Query v5.18.1
- **Form Handling**: Built-in React state management
- **Utilities**: Lodash v4.17.21

#### Rich Text & Email Editor

- **Rich Text Editor**: Tiptap v2.14.0 with extensions:
  - Highlight, Subscript, Superscript, Typography, Underline
  - Starter Kit for basic functionality
- **Email Builder**: MJML Browser v4.15.3
- **Code Editor**: Monaco Editor React v4.7.0
- **Syntax Highlighting**: Prism React Renderer v2.4.1

#### File Management & Media

- **File Uploads**: AWS SDK S3 Client v3.779.0
- **Image Processing**: HTML2Canvas v1.4.1
- **File Size Utils**: Filesize v10.1.6
- **CSV Processing**: PapaParse v5.5.2

#### Developer Experience

- **Templating**: LiquidJS v10.21.0 for template preview
- **Date Handling**: Day.js v1.11.13
- **Color Picker**: React Color v2.19.3
- **Emoji Support**: Emoji Mart v5.6.0
- **UUID**: Short UUID v5.2.0

#### Testing & Quality

- **Testing**: Vitest v3.0.8 with React Testing Library
- **Linting**: ESLint v8.55.0 with TypeScript support
- **Type Checking**: TypeScript v5.2.2

### Notification Center Widget

#### Core Framework

- **Framework**: React 19.1.0 with TypeScript 5.8.3
- **Build Tool**: Vite 6.3.5

#### UI & Styling

- **UI Components**: Radix UI React Slot v1.2.3
- **Design System**: Shadcn/ui v0.0.4
- **Styling**: Tailwind CSS v4.1.6 with merge utilities
- **Icons**: Lucide React v0.511.0
- **Theming**: Next Themes v0.4.6 for dark/light mode
- **Notifications**: Sonner v2.0.3 for toast notifications
- **Animations**: tw-animate-css v1.3.0

#### Utilities

- **Class Management**:
  - clsx v2.1.1 for conditional classes
  - class-variance-authority v0.7.1 for component variants
  - tailwind-merge v3.3.0 for Tailwind class optimization

## 🐳 DevOps & Deployment

### Containerization

- **Base Images**:
  - Node 20 Alpine for frontend builds
  - Go 1.23 Alpine for backend builds
  - Alpine Linux for final runtime
- **Multi-stage Build**: Optimized Docker builds with separate stages
- **Container Orchestration**: Docker Compose for development

### Database

- **Production**: External PostgreSQL (managed service recommended)
- **Development**: PostgreSQL 17 Alpine container
- **SSL**: Configurable SSL modes for secure connections

### File Storage

- **Local**: File system storage for development
- **Cloud**: S3-compatible storage for production
- **CDN**: Integrated file manager with CDN delivery

## 📁 Project Structure

```
notifuse/
├── cmd/                    # Application entry points
│   ├── api/               # Main API server
│   └── keygen/            # PASETO key generation utility
├── internal/              # Private application code
│   ├── domain/           # Business entities and interfaces
│   ├── service/          # Business logic implementation
│   ├── repository/       # Data access layer
│   ├── http/             # HTTP handlers and middleware
│   ├── database/         # Database configuration
│   └── migrations/       # Database migrations
├── console/              # React admin interface
│   ├── src/
│   │   ├── components/   # Reusable UI components
│   │   ├── pages/        # Application pages
│   │   └── utils/        # Utility functions
│   └── dist/             # Built assets
├── notification_center/   # Embeddable widget
│   ├── src/
│   │   └── components/   # Widget components
│   └── dist/             # Built widget assets
├── pkg/                  # Public packages
│   ├── logger/           # Logging utilities
│   ├── mailer/           # Email sending abstraction
│   └── tracing/          # Observability tools
└── config/               # Configuration management
```

## 🚀 Development Workflow

### Backend Development

- **Hot Reload**: Built-in with Go's fast compilation
- **Testing**: Comprehensive test suite with mocks
- **Database**: Automatic migrations on startup
- **Debugging**: Structured logging with multiple levels

### Frontend Development

- **Hot Reload**: Vite's fast HMR for instant updates
- **Type Safety**: Full TypeScript coverage
- **Component Development**: Isolated component development
- **Testing**: Unit and integration tests with Vitest

### Integration

- **API-First**: OpenAPI specification for API documentation
- **Real-time**: WebSocket support for live updates
- **File Uploads**: Integrated S3-compatible file management
- **Email Preview**: Real-time MJML rendering and preview

## 🔧 Key Design Decisions

### Backend Choices

- **Standard Library HTTP**: Chose simplicity over framework complexity
- **Clean Architecture**: Enables easy testing and maintainability
- **PostgreSQL**: Robust relational database for complex queries
- **PASETO**: More secure alternative to JWT tokens
- **OpenCensus**: Vendor-neutral observability

### Frontend Choices

- **React 18+**: Latest React features with concurrent rendering
- **Ant Design**: Comprehensive component library for admin interfaces
- **TanStack Router**: Type-safe routing with excellent DX
- **Vite**: Fast build tool with excellent HMR
- **MJML**: Industry-standard email template rendering

### Architecture Benefits

- **Scalability**: Clean separation allows independent scaling
- **Testing**: Dependency injection enables comprehensive testing
- **Maintainability**: Clear boundaries between layers
- **Flexibility**: Pluggable components for different providers
- **Performance**: Optimized builds and efficient database queries

This tech stack provides a robust foundation for a modern email marketing platform with enterprise-grade features while maintaining the flexibility of open-source software.

## 📝 Coding Styles & Conventions

### Backend (Go) Coding Standards

#### Code Organization

- **Package Structure**: Follow Go's standard package layout with clear separation of concerns
- **Naming Conventions**:
  - Use PascalCase for exported functions, types, and constants
  - Use camelCase for unexported functions and variables
  - Interface names should end with appropriate suffixes (e.g., `Repository`, `Service`)
  - Use descriptive names that clearly indicate purpose

#### Go-Specific Patterns

```go
// Struct definitions with clear field organization
type WorkspaceService struct {
    repo               domain.WorkspaceRepository
    userRepo           domain.UserRepository
    logger             logger.Logger
    // ... grouped by functionality
}

// Constructor pattern with dependency injection
func NewWorkspaceService(
    repo domain.WorkspaceRepository,
    userRepo domain.UserRepository,
    logger logger.Logger,
    // ... dependencies
) *WorkspaceService {
    return &WorkspaceService{
        repo:     repo,
        userRepo: userRepo,
        logger:   logger,
    }
}
```

#### Error Handling

- Use explicit error handling with descriptive error messages
- Wrap errors with context using `fmt.Errorf("context: %w", err)`
- Log errors at appropriate levels with structured logging

#### Constants and Enums

```go
// Use typed constants for better type safety
type PermissionResource string

const (
    PermissionResourceContacts       PermissionResource = "contacts"
    PermissionResourceLists          PermissionResource = "lists"
    PermissionResourceTemplates      PermissionResource = "templates"
)
```

#### Interface Design

- Keep interfaces small and focused (Interface Segregation Principle)
- Use `//go:generate mockgen` for generating mocks
- Define interfaces in the consuming package, not the implementing package

### Frontend (React/TypeScript) Coding Standards

#### File Organization

- **Components**: Organized by feature in dedicated folders
- **Services**: API calls grouped by domain (e.g., `contacts.ts`, `workspace.ts`)
- **Types**: Shared types in dedicated files
- **Utils**: Utility functions separated by purpose

#### Component Structure

```tsx
// Import order: React, third-party, internal
import React from 'react'
import { Drawer, Space, Typography } from 'antd'
import { Contact } from '../../services/api/contacts'

// Interface definitions before component
interface ContactDetailsDrawerProps {
  workspace: Workspace
  contactEmail: string
  visible?: boolean
  onClose?: () => void
}

// Component with proper TypeScript typing
export const ContactDetailsDrawer: React.FC<ContactDetailsDrawerProps> = ({
  workspace,
  contactEmail,
  visible = false,
  onClose
}) => {
  // Component logic
}
```

#### TypeScript Conventions

- Use strict TypeScript configuration
- Prefer interfaces over types for object definitions
- Use proper generic typing for API responses
- Avoid `any` type - use proper typing or `unknown`

#### State Management

- Use React Query (TanStack Query) for server state
- Local component state with `useState` and `useReducer`
- Context API for shared application state (authentication)

#### Styling Approach

- **Primary**: Tailwind CSS for utility-first styling
- **Components**: Ant Design for complex UI components
- **Custom**: CSS modules or styled-components for specific needs

### Testing Standards

#### Backend Testing (Go)

The project uses a comprehensive testing strategy with multiple test commands available via Makefile:

##### Test Commands

```bash
# Run all unit tests
make test-unit

# Run tests by layer
make test-domain      # Domain layer tests
make test-service     # Service layer tests
make test-repo        # Repository layer tests
make test-http        # HTTP handler tests

# Integration tests
make test-integration # Full integration test suite

# Coverage reporting
make coverage         # Generate HTML coverage report
```

##### Test Structure

```go
func TestWorkspace_Validate(t *testing.T) {
    testCases := []struct {
        name      string
        workspace Workspace
        expectErr bool
    }{
        {
            name: "valid workspace",
            workspace: Workspace{
                ID:   "test123",
                Name: "Test Workspace",
                // ... test data
            },
            expectErr: false,
        },
        // ... more test cases
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            err := tc.workspace.Validate()
            if tc.expectErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

##### Testing Tools

- **Framework**: Standard library `testing` package
- **Assertions**: Testify (`assert`, `require`, `mock`)
- **Mocking**: GoMock with generated mocks
- **Database**: go-sqlmock for database testing
- **Coverage**: Built-in Go coverage tools

#### Frontend Testing (React/TypeScript)

##### Test Commands

```bash
# Run frontend tests
cd console && npm test

# Run with coverage
cd console && npm run test:coverage

# Watch mode for development
cd console && npm run test -- --watch
```

##### Testing Tools

- **Framework**: Vitest (fast Vite-native testing)
- **React Testing**: React Testing Library
- **Assertions**: Built-in Vitest assertions
- **User Interactions**: Testing Library User Event
- **DOM**: jsdom for browser environment simulation

### Code Quality Tools

#### Backend (Go)

- **Linting**: Built-in `go vet` and `gofmt`
- **Imports**: `goimports` for import organization
- **Static Analysis**: Go's built-in race detector
- **Documentation**: Go doc comments following standard conventions

#### Frontend (React/TypeScript)

- **Linting**: ESLint with TypeScript support
- **Type Checking**: TypeScript compiler with strict mode
- **Code Formatting**: Built-in Prettier integration via Vite
- **Import Organization**: ESLint import sorting rules

#### ESLint Configuration

```javascript
// eslint.config.js
export default tseslint.config(
  { ignores: ['dist'] },
  {
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    files: ['**/*.{ts,tsx}'],
    plugins: {
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      'react-refresh/only-export-components': ['warn', { allowConstantExport: true }]
    }
  }
)
```

### Development Workflow

#### Backend Development

```bash
# Development with hot reload
make dev              # Uses Air for hot reloading

# Build and run
make build            # Build binary
make run              # Run from source

# Clean build artifacts
make clean            # Remove build files and coverage reports
```

#### Frontend Development

```bash
# Development server
cd console && npm run dev

# Production build
cd console && npm run build

# Linting
cd console && npm run lint
```

### API Design Patterns

#### RPC-Style Endpoints

The backend uses RPC-style API endpoints with dot notation:

```
POST /api/workspace.create
POST /api/workspace.update
POST /api/contact.create
GET  /api/contact.list
```

#### Request/Response Structure

- Consistent JSON request/response format
- Proper HTTP status codes
- Structured error responses
- Request validation with detailed error messages

#### Authentication

- PASETO tokens for stateless authentication
- Middleware-based authentication checking
- Role-based permissions with granular access control

These coding standards ensure consistency, maintainability, and reliability across the entire Notifuse codebase.
