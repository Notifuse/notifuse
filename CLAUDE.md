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

- **Language**: Go 1.25.x
- **HTTP Framework**: Standard library `http.ServeMux` (no external web framework)
- **Architecture**: Clean Architecture with dependency injection

### Database & Storage

- **Primary Database**: PostgreSQL 17
- **Query Builder**: Squirrel for type-safe SQL queries
- **Migrations**: Custom migration system with version-based schema management
- **Connection Pooling**: Built-in database/sql with OpenCensus integration

### Database Migration System

Notifuse uses a custom migration system that manages database schema changes across both the system database and individual workspace databases. The migration system is designed to handle schema evolution safely and consistently.

#### Version Format

- **Tag Format**: `vMAJOR.minor` (e.g., `v6.5`, `v4.0`)
- **Major Version**: Incremented when database schemas change
- **Minor Version**: Incremented for non-schema changes (features, bug fixes)
- **Code Version**: Stored in `config/config.go` as `VERSION = "6.5"`

#### Migration Types

The system supports two types of migrations:

1. **System Migrations**: Changes to the main system database schema

   - User management tables
   - Workspace management tables
   - System configuration tables
   - Global settings and permissions

2. **Workspace Migrations**: Changes to individual workspace database schemas
   - Contact management tables
   - Template and broadcast tables
   - Message history tables
   - Workspace-specific settings

#### Migration Structure

Each migration implements the `MajorMigrationInterface`:

```go
type MajorMigrationInterface interface {
    GetMajorVersion() float64                    // Returns major version (e.g., 6.0)
    HasSystemUpdate() bool                       // Indicates system database changes
    HasWorkspaceUpdate() bool                    // Indicates workspace database changes
    UpdateSystem(ctx, config, db) error          // Executes system migrations
    UpdateWorkspace(ctx, config, workspace, db) error // Executes workspace migrations
}
```

#### Migration Execution

1. **Version Comparison**: System compares current database version with code version
2. **Migration Selection**: Identifies migrations that need to be executed
3. **System Updates**: Executes system migrations in a transaction
4. **Workspace Updates**: For each workspace, connects to its database and executes workspace migrations
5. **Version Update**: Updates database version after successful completion

#### Migration Safety

- **Idempotent**: Migrations can be run multiple times safely using `IF NOT EXISTS` clauses
- **Transactional**: Each migration runs in a database transaction
- **Rollback**: Failed migrations are automatically rolled back
- **Backward Compatibility**: Existing data is preserved with default values

#### Example Migration

```go
// V6Migration adds permissions system
type V6Migration struct{}

func (m *V6Migration) GetMajorVersion() float64 { return 6.0 }
func (m *V6Migration) HasSystemUpdate() bool { return true }
func (m *V6Migration) HasWorkspaceUpdate() bool { return false }

func (m *V6Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
    // Add permissions column to user_workspaces table
    _, err := db.ExecContext(ctx, `
        ALTER TABLE user_workspaces
        ADD COLUMN IF NOT EXISTS permissions JSONB DEFAULT '{}'::jsonb
    `)
    return err
}
```

#### Changelog Integration

- **CHANGELOG.md**: Updated with each version release
- **Version History**: Tracks all schema changes and their impact
- **Breaking Changes**: Clearly documented for upgrade planning

### Authentication & Security

- **Token System**: JWT (Platform-Agnostic Security Tokens)
- **Password Hashing**: bcrypt via golang.org/x/crypto
- **API Security**: Custom middleware for authentication and CORS

### Email & Communication

- **Email Engine**: Multiple provider support:
  - Amazon SES (AWS SDK v1.55.7)
  - SMTP (go-mail v0.7.2)
  - Mailgun, Mailjet, Postmark, SparkPost integrations
- **Template Engine**: Liquid templating (Notifuse/liquidgo)
- **MJML Support**: gomjml v0.10.0 for email rendering
- **HTML Parsing**: PuerkitoBio/goquery v1.10.3

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
- **Build Tool**: Vite 7.1.x
- **Routing**: TanStack Router v1.15.7 with devtools

#### UI Framework & Styling

- **UI Library**: Ant Design v5.27.x
- **Icons**:
  - Ant Design Icons v5.3.0
  - FontAwesome v6.7.2 (solid, regular, brands)
  - Lucide React v0.487.0
- **Styling**: Tailwind CSS v4.1.10

#### State Management & Data Fetching

- **Data Fetching**: TanStack Query v5.18.1
- **Form Handling**: Built-in React state management
- **Utilities**: Lodash v4.17.21

#### Rich Text & Email Editor

- **Rich Text Editor**: Tiptap v3.10.x with extensions:
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

- **Templating**: LiquidJS v10.24.x for template preview
- **Date Handling**: Day.js v1.11.13
- **Color Picker**: React Color v2.19.3
- **Emoji Support**: Emoji Mart v5.6.0
- **UUID**: Short UUID v5.2.0

#### Testing & Quality

- **Testing**: Vitest v3.0.x with React Testing Library
- **Linting**: ESLint v9.19.x with TypeScript support
- **Type Checking**: TypeScript v5.2.2

### Notification Center Widget

#### Core Framework

- **Framework**: React 19.1.0 with TypeScript 5.8.x
- **Build Tool**: Vite 7.1.x

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
  - Go 1.25 Alpine for backend builds
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
├── internal/              # Private application code
│   ├── domain/           # Business entities and interfaces
│   ├── service/          # Business logic implementation
│   ├── repository/       # Data access layer
│   ├── http/             # HTTP handlers and middleware
│   ├── database/         # Database configuration and schema
│   └── migrations/       # Database migration system
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
- **Migrations**: Version-based schema management with automatic execution

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

#### Internationalization (i18n)

The console uses **LinguiJS** for internationalization with natural language keys.

**Setup:**

- Runtime: `@lingui/core`, `@lingui/react`
- Build: `@lingui/cli`, `@lingui/babel-plugin-lingui-macro`, `@lingui/vite-plugin`
- Config: `console/lingui.config.ts`
- Translations: `console/src/i18n/locales/{locale}.po`

**Usage Pattern:**

```tsx
import { useLingui } from '@lingui/react/macro'

function MyComponent() {
  const { t } = useLingui()

  return (
    <div>
      <h1>{t`Create Broadcast`}</h1>
      <p>{t`You have ${count} messages`}</p>
    </div>
  )
}
```

**Key Rules:**

- Always use `useLingui()` hook from `@lingui/react/macro`
- Use template literals: `` t`text` `` not `t("text")`
- Variables use `${var}` syntax: `` t`Hello ${name}` ``
- For JSX content, use `<Trans>` component from `@lingui/react/macro`
- All user-facing strings must be wrapped with `t` or `<Trans>`
- Run `npm run lingui:extract` after adding new strings
- Run `npm run lingui:compile` before building

**Commands:**

- `npm run lingui:extract` - Extract strings to PO files
- `npm run lingui:compile` - Compile PO files for production

**File Locations:**

- Config: `console/lingui.config.ts`
- Setup: `console/src/i18n/index.ts`
- Locales: `console/src/i18n/locales/`

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

#### Creating Database Migrations

When database schema changes are needed, follow this process:

1. **Update Version**: Increment the major version in `config/config.go`
2. **Create Migration File**: Create a new file in `internal/migrations/` (e.g., `v7.go`)
3. **Implement Interface**: Create a struct that implements `MajorMigrationInterface`
4. **Write Migration Logic**: Implement system and/or workspace update methods
5. **Update Changelog**: Document changes in `CHANGELOG.md`
6. **Test Migration**: Run tests to ensure migration works correctly

```go
// Example: internal/migrations/v7.go
package migrations

import (
    "context"
    "fmt"
    "github.com/Notifuse/notifuse/config"
    "github.com/Notifuse/notifuse/internal/domain"
)

type V7Migration struct{}

func (m *V7Migration) GetMajorVersion() float64 { return 7.0 }
func (m *V7Migration) HasSystemUpdate() bool { return true }
func (m *V7Migration) HasWorkspaceUpdate() bool { return false }

func (m *V7Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
    // Add new system table or column
    _, err := db.ExecContext(ctx, `
        CREATE TABLE IF NOT EXISTS new_feature (
            id VARCHAR(32) PRIMARY KEY,
            name VARCHAR(255) NOT NULL,
            created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
        )
    `)
    return err
}

func (m *V7Migration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
    // No workspace changes for this migration
    return nil
}

func init() {
    Register(&V7Migration{})
}
```

**Migration Best Practices**:

- Use `IF NOT EXISTS` for table/column creation to make migrations idempotent
- Provide default values for new columns to maintain backward compatibility
- Test migrations on a copy of production data
- Document breaking changes in the changelog
- Keep migrations focused on a single schema change

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

- JWT tokens for stateless authentication
- Middleware-based authentication checking
- Role-based permissions with granular access control

### AI-Assisted Development & Planning

#### Plans Directory

Notifuse uses the `plans/` directory to store AI-generated implementation plans created with Cursor's AI planning tools. This provides a centralized location for tracking feature implementations, bug fixes, and architectural changes.

#### Planning Workflow

When planning a new feature or significant change with Cursor:

1. **Create Plan**: Use Cursor's plan mode to generate a detailed implementation plan
2. **Save to Plans Directory**: All `.md` plan files should be saved to `/Users/pierre/Sites/notifuse3/code/notifuse/plans/`
3. **Follow the Plan**: Use the plan as a guide during implementation
4. **Update as Needed**: Modify the plan if requirements change during development

#### Plan Naming Convention

Use descriptive, kebab-case names for plan files:

- **Features**: `feature-name-plan.md` (e.g., `email-scheduling-plan.md`)
- **Bug Fixes**: `fix-description-plan.md` (e.g., `fix-contact-import-validation-plan.md`)
- **Refactoring**: `refactor-component-plan.md` (e.g., `refactor-workspace-service-plan.md`)
- **Migrations**: `migration-v7-plan.md` (e.g., `migration-v7-permissions-plan.md`)

#### Plan Lifecycle

- **Active Plans**: Plans currently being implemented should remain in the `plans/` directory
- **Completed Plans**: Keep completed plans for historical reference and documentation
- **Archived Plans**: If needed, create a `plans/archive/` subdirectory for old or cancelled plans

#### Testing Requirements in Plans

All implementation plans must include comprehensive testing strategy:

**Backend Testing (Go)**:

- **Unit Tests**: Every touched backend file must have corresponding unit tests
  - Domain layer: Test entity validation and business logic
  - Service layer: Test business operations with mocked dependencies
  - Repository layer: Test data access with sqlmock
  - HTTP layer: Test handlers and middleware
- **Integration Tests**: Add integration tests for new features that exercise the full stack
  - Database interactions with test database
  - API endpoint testing end-to-end
  - Cross-service integration scenarios

**Frontend Testing (React/TypeScript)**:

- Component tests for new or modified UI components
- Integration tests for complex user flows
- API interaction tests with mocked responses

**Test Execution in Plans**:

Every plan must include a dedicated step to run the updated tests after implementation:

1. **Write/Update Tests**: Create or modify test files for all touched code
2. **Run Unit Tests**: Execute layer-specific unit tests to verify changes
3. **Run Integration Tests**: Execute integration tests for end-to-end validation
4. **Verify Coverage**: Ensure adequate test coverage for new code

Plans should include specific test commands to run based on the changes made.

**Test Commands**:

All backend test commands are defined in the `Makefile`. Use the appropriate command for the layer(s) you modified:

```bash
# Backend (see Makefile for full details)
make test-unit              # Run all unit tests
make test-domain            # Domain layer tests
make test-service           # Service layer tests
make test-repo              # Repository tests
make test-http              # HTTP handler tests
make test-migrations        # Migrations tests
make test-database          # Database layer tests
make test-pkg               # Package layer tests (logger, mailer, tracing, etc.)
make test-integration       # Integration tests
make coverage               # Comprehensive coverage report with HTML output

# Frontend
cd console && npm test      # Run frontend tests
```

Refer to the `Makefile` for the most up-to-date test commands and their configurations.

#### Best Practices

- Write plans to the notifuse /plans folder
- Include specific file paths and code snippets in plans
- Break down complex features into clear, actionable steps
- Reference existing architectural patterns documented in this file
- Update plans if implementation differs significantly from original design
- Use plans to coordinate work across frontend and backend changes
- **Always include test files in the plan**: For each implementation file, specify the corresponding test file and test cases to add

This structured approach to AI-assisted planning ensures all team members and AI tools have clear context about implementation strategies and design decisions.

---

These coding standards ensure consistency, maintainability, and reliability across the entire Notifuse codebase.

## 🤖 Claude Agent Rules

- **Never self-advertise**: Do not add "Generated with Claude", "Co-Authored-By: Claude", or any AI attribution
- **Clean commits**: No AI signatures, footers, or mentions in git commit messages
- **Clean releases**: No AI credits in changelogs, release notes, or PR descriptions
- **No AI markers**: No comments like "// Generated by Claude" in code
