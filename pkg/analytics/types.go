package analytics

import (
	"errors"
	"time"
)

var (
	ErrInvalidSchema          = errors.New("invalid schema")
	ErrUnsupportedMeasure     = errors.New("unsupported measure")
	ErrUnsupportedDimension   = errors.New("unsupported dimension")
	ErrUnsupportedGranularity = errors.New("unsupported granularity")
	ErrUnsupportedOperator    = errors.New("unsupported operator")
	ErrInvalidTimezone        = errors.New("invalid timezone")
)

// Query represents a Cube.js-style analytics query
type Query struct {
	Schema         string            `json:"schema" valid:"required"`
	Measures       []string          `json:"measures" valid:"required"`
	Dimensions     []string          `json:"dimensions"`
	Timezone       *string           `json:"timezone,omitempty"`
	TimeDimensions []TimeDimension   `json:"timeDimensions,omitempty"`
	Filters        []Filter          `json:"filters,omitempty"`
	Limit          *int              `json:"limit,omitempty"`
	Offset         *int              `json:"offset,omitempty"`
	Order          map[string]string `json:"order,omitempty"`
}

// TimeDimension represents time-based grouping
type TimeDimension struct {
	Dimension   string     `json:"dimension" valid:"required"`
	Granularity string     `json:"granularity" valid:"required,in(hour|day|week|month|year)"`
	DateRange   *[2]string `json:"dateRange,omitempty"`
}

// Filter represents a query filter
type Filter struct {
	Member   string   `json:"member" valid:"required"`
	Operator string   `json:"operator" valid:"required,in(equals|notEquals|contains|gt|gte|lt|lte|in|notIn)"`
	Values   []string `json:"values" valid:"required"`
}

// Response represents the response from an analytics query
type Response struct {
	Data []map[string]interface{} `json:"data"`
	Meta Meta                     `json:"meta"`
}

// Meta contains metadata about the query execution
type Meta struct {
	Total         int           `json:"total"`
	ExecutionTime time.Duration `json:"executionTime"`
	Query         string        `json:"query"`
	Params        []interface{} `json:"params"`
}

// SchemaDefinition defines the structure of an analytics schema
type SchemaDefinition struct {
	Name       string                         `json:"name"`
	Measures   map[string]MeasureDefinition   `json:"measures"`
	Dimensions map[string]DimensionDefinition `json:"dimensions"`
}

// MeasureFilter defines a filter condition for measures (Cube.js compatible)
type MeasureFilter struct {
	SQL string `json:"sql"`
}

// MeasureDefinition defines an analytics measure
type MeasureDefinition struct {
	Type        string          `json:"type" valid:"in(count|sum|avg|min|max)"`
	SQL         string          `json:"sql,omitempty"`
	Description string          `json:"description"`
	Filters     []MeasureFilter `json:"filters,omitempty"`
}

// DimensionDefinition defines an analytics dimension
type DimensionDefinition struct {
	Type        string `json:"type" valid:"in(string|number|time)"`
	SQL         string `json:"sql,omitempty"`
	Description string `json:"description"`
}

// GetDefaultTimezone returns the default timezone for queries
func (q *Query) GetDefaultTimezone() string {
	if q.Timezone != nil {
		return *q.Timezone
	}
	return "UTC"
}

// HasTimeDimensions returns true if the query has time dimensions
func (q *Query) HasTimeDimensions() bool {
	return len(q.TimeDimensions) > 0
}

// GetLimit returns the query limit or default
func (q *Query) GetLimit() int {
	if q.Limit != nil {
		return *q.Limit
	}
	return 1000 // Default limit
}

// GetOffset returns the query offset or default
func (q *Query) GetOffset() int {
	if q.Offset != nil {
		return *q.Offset
	}
	return 0
}
