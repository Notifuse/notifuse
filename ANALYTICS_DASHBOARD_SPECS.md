# Analytics Dashboard Specifications

## Overview

Create an analytics dashboard in the Notifuse console that displays predefined charts by querying the `analytics.query` endpoint using Cube.js-style JSON payloads. The dashboard shows hardcoded analytics views with predefined measures and dimensions, without allowing users to create custom queries.

## Technical Stack

- **Frontend**: React + TypeScript + Ant Design + TanStack Router
- **Backend**: Go API endpoint
- **Query Format**: Cube.js-style JSON payload for predefined chart configurations
- **Visualization**: Apache ECharts with echarts-for-react

## API Endpoint

### Endpoint

```
POST /api/analytics.query
```

### Request Payload Structure

```typescript
interface AnalyticsQuery {
  schema: string // Predefined schema name (required)
  measures: string[] // Aggregation fields (count, sum, avg, etc.)
  dimensions: string[] // Grouping fields
  timezone?: string // Timezone for date/time operations (e.g., "America/New_York", "UTC")
  timeDimensions?: {
    dimension: string // Date/timestamp field
    granularity: 'hour' | 'day' | 'week' | 'month' | 'year'
    dateRange?: [string, string] // ISO date strings
  }[]
  filters?: {
    member: string // Field name
    operator: 'equals' | 'notEquals' | 'contains' | 'gt' | 'gte' | 'lt' | 'lte' | 'in' | 'notIn'
    values: string[]
  }[]
  limit?: number // Result limit (default: 1000)
  offset?: number // Pagination offset
  order?: {
    [key: string]: 'asc' | 'desc' // Sorting
  }
}
```

### Response Structure

```typescript
interface AnalyticsResponse {
  data: Array<Record<string, any>>
  meta: {
    total: number
    executionTime: number
    query: string // Generated SQL for debugging
    params: any[] // Database parameters for debugging
  }
}
```

### Example Payload

```json
{
  "schema": "contacts",
  "measures": ["count(*)"],
  "dimensions": ["status"],
  "timezone": "America/New_York",
  "timeDimensions": [
    {
      "dimension": "created_at",
      "granularity": "day",
      "dateRange": ["2024-01-01", "2024-12-31"]
    }
  ],
  "filters": [
    {
      "member": "workspace_id",
      "operator": "equals",
      "values": ["workspace-123"]
    }
  ],
  "limit": 100,
  "order": {
    "created_at": "desc"
  }
}
```

## Frontend Components

### 1. Analytics Page (`/analytics`)

- **Location**: `src/pages/AnalyticsPage.tsx`
- **Route**: Add to `src/router.tsx`
- **Layout**: Use existing `WorkspaceLayout`

### 2. Predefined Chart Components

```typescript
// src/components/analytics/PredefinedCharts.tsx
interface PredefinedChartProps {
  chartId: string
  timeRange?: [string, string]
  workspace: Workspace
}

// Predefined chart configurations
interface ChartDefinition {
  id: string
  title: string
  description: string
  query: AnalyticsQuery // Full Cube.js-style query
  chartType: 'line' | 'bar' | 'pie' | 'table'
}
```

**Predefined Charts**:

1. **Email Metrics Multiline Chart**: sent/delivered/bounced/complained/opened/clicked/unsubscribed over time
2. **Contact Growth Line Chart**: New contacts added over time
3. **Last Broadcast Stats**: Statistics from the most recent sent broadcast
4. **New Contacts Table**: Recent contact additions
5. **Failed Messages Table**: Recent message delivery failures

### 3. Chart Visualization Component

```typescript
// src/components/analytics/ChartVisualization.tsx
interface ChartVisualizationProps {
  data: AnalyticsResponse
  chartType: 'line' | 'bar' | 'pie' | 'table'
  query: AnalyticsQuery
}
```

**Chart Types**:

- **Line Chart**: Time series data with ECharts line series
- **Bar Chart**: Categorical comparisons with ECharts bar series
- **Pie Chart**: Distribution analysis with ECharts pie series
- **Data Table**: Raw data display with Ant Design Table component

### 4. Dashboard Layout

```typescript
// src/components/analytics/AnalyticsDashboard.tsx
```

**Layout Structure**:

```
┌─────────────────────────────────────────┐
│ Analytics Dashboard Header              │
│ [Time Range Picker] [Export] [Refresh]  │
├─────────────────────────────────────────┤
│ Email Metrics Chart                     │
│ [All] [Broadcasts] [Transactional] 📊   │
│ 📈 Multiline: sent/delivered/bounced... │
├─────────────────────────────────────────┤
│ ┌─────────────┐ ┌─────────────────────┐ │
│ │ Contact     │ │ Last Broadcast      │ │
│ │ Growth      │ │ Stats               │ │
│ │ 📈 Line     │ │ 📊 Metrics          │ │
│ └─────────────┘ └─────────────────────┘ │
├─────────────────────────────────────────┤
│ ┌─────────────┐ ┌─────────────────────┐ │
│ │ New         │ │ Failed Messages     │ │
│ │ Contacts    │ │ History             │ │
│ │ 📋 Table    │ │ 📋 Table            │ │
│ └─────────────┘ └─────────────────────┘ │
└─────────────────────────────────────────┘
```

## Detailed Chart Specifications

### 1. Email Metrics Multiline Chart

**Purpose**: Track email engagement metrics over time with filtering capability

**Features**:

- **Chart Type**: ECharts multiline chart
- **Metrics**: 7 lines showing sent, delivered, bounced, complained, opened, clicked, unsubscribed
- **Filter**: Ant Design Segmented component with options: "All", "Broadcasts", "Transactional"
- **Time Range**: Configurable via dashboard header time picker

**Analytics Query Structure**:

```typescript
{
  schema: "messages",
  measures: [
    "count_sent", "count_delivered", "count_bounced",
    "count_complained", "count_opened", "count_clicked", "count_unsubscribed"
  ],
  dimensions: [],
  timezone: "America/New_York",
  timeDimensions: [{
    dimension: "created_at",
    granularity: "day",
    dateRange: ["2024-01-01", "2024-12-31"]
  }],
  filters: [
    {
      member: "workspace_id",
      operator: "equals",
      values: ["workspace-123"]
    },
    // Dynamic filter based on segmented selection:
    // For "broadcasts": { member: "message_type", operator: "equals", values: ["broadcast"] }
    // For "transactional": { member: "message_type", operator: "equals", values: ["transactional"] }
    // For "all": no additional filter
  ]
}
```

### 2. Contact Growth Line Chart

**Purpose**: Show new contact registrations over time

**Features**:

- **Chart Type**: ECharts line chart
- **Metric**: New contacts added per day/week/month
- **Time Range**: Configurable via dashboard header

**Analytics Query Structure**:

```typescript
{
  schema: "contacts",
  measures: ["count"],
  dimensions: [],
  timezone: "America/New_York",
  timeDimensions: [{
    dimension: "created_at",
    granularity: "day",
    dateRange: ["2024-01-01", "2024-12-31"]
  }],
  filters: [{
    member: "workspace_id",
    operator: "equals",
    values: ["workspace-123"]
  }]
}
```

### 3. Last Broadcast Statistics

**Purpose**: Display key metrics from the most recently sent broadcast

**Features**:

- **Display Type**: Stat cards/metrics display
- **Metrics**: Total sent, delivered, opened, clicked, bounce rate, open rate, click rate
- **Data Source**: Most recent broadcast with status "sent"

**Analytics Query Structure**:

```typescript
{
  schema: "broadcasts",
  measures: [
    "total_sent", "total_delivered", "total_opened",
    "total_clicked", "total_bounced", "open_rate", "click_rate"
  ],
  dimensions: ["broadcast_id", "subject", "sent_at"],
  timezone: "America/New_York",
  filters: [
    {
      member: "workspace_id",
      operator: "equals",
      values: ["workspace-123"]
    },
    {
      member: "status",
      operator: "equals",
      values: ["sent"]
    }
  ],
  order: {
    "sent_at": "desc"
  },
  limit: 1
}
```

### 4. New Contacts Table

**Purpose**: Show recently added contacts

**Features**:

- **Display Type**: Ant Design Table
- **Columns**: Email, Name, Created Date, Source, Status
- **Pagination**: Show last 50 contacts
- **Sorting**: By creation date (newest first)

**Analytics Query Structure**:

```typescript
{
  schema: "contacts",
  measures: [],
  dimensions: ["email", "first_name", "last_name", "created_at", "source", "status"],
  timezone: "America/New_York",
  filters: [{
    member: "workspace_id",
    operator: "equals",
    values: ["workspace-123"]
  }],
  order: {
    "created_at": "desc"
  },
  limit: 50
}
```

### 5. Failed Messages History Table

**Purpose**: Show recent message delivery failures for troubleshooting

**Features**:

- **Display Type**: Ant Design Table
- **Columns**: Recipient, Subject, Failed At, Error Message, Message Type
- **Pagination**: Show last 100 failed messages
- **Sorting**: By failure date (newest first)

**Analytics Query Structure**:

```typescript
{
  schema: "message_history",
  measures: [],
  dimensions: ["recipient_email", "subject", "failed_at", "error_message", "message_type"],
  timezone: "America/New_York",
  filters: [
    {
      member: "workspace_id",
      operator: "equals",
      values: ["workspace-123"]
    },
    {
      member: "status",
      operator: "equals",
      values: ["failed"]
    }
  ],
  order: {
    "failed_at": "desc"
  },
  limit: 100
}
```

## Available Schemas & Definitions

### Predefined Analytics Schemas

```typescript
interface SchemaDefinition {
  name: string
  measures: {
    [key: string]: {
      type: 'count' | 'sum' | 'avg' | 'min' | 'max'
      sql?: string
      description: string
    }
  }
  dimensions: {
    [key: string]: {
      type: 'string' | 'number' | 'time'
      sql?: string
      description: string
    }
  }
}
```

### Expected Schemas

1. **messages**: Email delivery and engagement metrics (sent, delivered, bounced, opened, etc.)
2. **contacts**: Contact management and growth analytics
3. **broadcasts**: Campaign performance and statistics
4. **message_history**: Message delivery history and failure tracking

## Backend Implementation

### 1. Analytics Handler

```go
// internal/handlers/analytics.go
func (h *Handler) AnalyticsQuery(c *gin.Context) {
    var query AnalyticsQuery
    // Parse JSON payload
    // Validate single schema restriction
    // Convert to SQL with security checks
    // Execute query
    // Return formatted response
}
```

### 2. Query Builder Service

```go
// internal/services/analytics.go
type AnalyticsService struct {
    db *gorm.DB
}

func (s *AnalyticsService) BuildQuery(query AnalyticsQuery) (string, []interface{}, error) {
    // Convert JSON to SQL
    // Apply security filters (workspace isolation)
    // Validate schema access permissions
    // Return SQL + parameters
}
```

### 3. Security Considerations

- **Schema Whitelist**: Only allow predefined schemas
- **Field Validation**: Validate all measures/dimensions exist
- **Query Limits**: Enforce max result limits
- **SQL Injection**: Use parameterized queries only

## UI/UX Requirements

### Design System

- Use existing Ant Design components
- Follow current app's color scheme and typography
- Responsive design for mobile/tablet

### User Experience

- **Loading States**: Show spinners during query execution
- **Error Handling**: Display friendly error messages
- **Shareable URLs**: Encode query in URL parameters

### Performance

- **Caching**: Cache results for 5 minutes
- **Pagination**: Handle large result sets
- **Query Timeout**: 30-second timeout with progress indicator

## Implementation Phases

### Phase 1: Core Infrastructure

- [ ] Create analytics API endpoint
- [ ] Implement query builder service
- [ ] Add basic schema definitions
- [ ] Create analytics page route

### Phase 2: Query Builder UI

- [ ] Build query builder components
- [ ] Implement form validation
- [ ] Add query preview functionality
- [ ] Connect to backend API

### Phase 3: Visualizations

- [ ] Integrate charting library
- [ ] Implement chart type switching
- [ ] Add data table component
- [ ] Create export functionality

### Phase 4: Polish & Features

- [ ] Add query history/bookmarks
- [ ] Implement dashboard sharing
- [ ] Add advanced filtering options
- [ ] Performance optimizations

## Testing Strategy

### Backend Tests

- SQL generation unit tests
- Security validation tests
- Performance tests with large datasets
