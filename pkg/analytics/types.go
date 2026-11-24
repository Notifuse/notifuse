package analytics

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

var (
	ErrInvalidSchema          = errors.New("invalid schema")
	ErrUnsupportedMeasure     = errors.New("unsupported measure")
	ErrUnsupportedDimension   = errors.New("unsupported dimension")
	ErrUnsupportedGranularity = errors.New("unsupported granularity")
	ErrUnsupportedOperator    = errors.New("unsupported operator")
	ErrInvalidTimezone        = errors.New("invalid timezone")
	ErrInvalidQuery           = errors.New("invalid query")
)

// Bounds of one analytics query, whoever sends it.
const (
	// MaxRows is the most rows a query returns. A query without a limit gets every row up to
	// it and is refused past it; a limit above it is refused.
	MaxRows = 50000
	// MaxQueryDuration bounds the statement of one query in the database.
	MaxQueryDuration = 60 * time.Second
)

// QueryError is a query the engine refuses as the caller wrote it: a member the schema does
// not have, an operator its type does not take, a range too long to fill, a result past
// MaxRows. The caller has to change the query; retrying it cannot help. errors.Is matches the
// sentinel it carries (ErrUnsupportedMeasure, ErrInvalidTimezone, ErrInvalidQuery...).
type QueryError struct {
	Message string
	kind    error
}

func (e *QueryError) Error() string { return e.Message }

func (e *QueryError) Unwrap() error { return e.kind }

// invalid builds a QueryError of the given kind.
func invalid(kind error, format string, args ...interface{}) error {
	return &QueryError{Message: fmt.Sprintf(format, args...), kind: kind}
}

// Query represents a Cube.js-style analytics query
type Query struct {
	Schema         string            `json:"schema" valid:"required"`
	Measures       []string          `json:"measures" valid:"required"`
	Dimensions     []string          `json:"dimensions"`
	Timezone       *string           `json:"timezone,omitempty"`
	TimeDimensions []TimeDimension   `json:"timeDimensions,omitempty"`
	Filters        []Filter          `json:"filters,omitempty"`
	Having         []Filter          `json:"having,omitempty"` // metric filters on aggregated measures (SQL HAVING)
	Limit          *int              `json:"limit,omitempty"`
	Offset         *int              `json:"offset,omitempty"`
	Order          map[string]string `json:"order,omitempty"`

	// orderKeys holds the keys of Order in the order the JSON object gave them: the first
	// sorts first. A Go map keeps no order, and ranging over it would sort a two-key query
	// differently from one request to the next.
	orderKeys []string

	// topGroupCells, set by executionQuery, makes BuildSQL return the full series of the
	// groups that hold the first topGroupCells rows of the query's order, and no other.
	topGroupCells *int
}

// UnmarshalJSON decodes a query and records the order of the keys of its order object.
func (q *Query) UnmarshalJSON(data []byte) error {
	type plain Query
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*q = Query(decoded)
	q.orderKeys = nil

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	var raw json.RawMessage
	for name, value := range fields {
		// encoding/json matches field names without regard to case.
		if strings.EqualFold(name, "order") {
			raw = value
		}
	}
	if len(raw) == 0 || raw[0] != '{' {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if _, err := decoder.Token(); err != nil { // {
		return err
	}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		key, _ := token.(string)
		if !slices.Contains(q.orderKeys, key) {
			q.orderKeys = append(q.orderKeys, key)
		}
		var skip json.RawMessage
		if err := decoder.Decode(&skip); err != nil {
			return err
		}
	}
	// One key has one order: recorded only when there is a priority to keep, so that a query
	// decoded from JSON stays equal to the same query built in Go.
	if len(q.orderKeys) < 2 {
		q.orderKeys = nil
	}
	return nil
}

// OrderKeys returns the keys of Order by priority: as the JSON object listed them, or sorted
// by name for a query built in Go, whose map carries no order.
func (q Query) OrderKeys() []string {
	if len(q.orderKeys) == len(q.Order) {
		complete := true
		for _, key := range q.orderKeys {
			if _, ok := q.Order[key]; !ok {
				complete = false
				break
			}
		}
		if complete {
			return slices.Clone(q.orderKeys)
		}
	}
	keys := make([]string, 0, len(q.Order))
	for key := range q.Order {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
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
	Operator string   `json:"operator" valid:"required,in(equals|notEquals|contains|notContains|startsWith|notStartsWith|endsWith|notEndsWith|gt|gte|lt|lte|in|notIn|set|notSet|inDateRange|notInDateRange|beforeDate|afterDate)"`
	Values   []string `json:"values"`
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

	// PartitionHint, when set, tells the builder that the schema's table is
	// range-partitioned on Column and that any time-dimension dateRange can be
	// widened by the slacks and applied to Column as well — turning a filter
	// on the (row-level) time column into plan-time partition pruning.
	// Internal query-planning metadata: not exposed through analytics.schemas.
	PartitionHint *PartitionHint `json:"-"`
}

// PartitionHint describes the partition-pruning predicate of a schema.
type PartitionHint struct {
	Column      string        // the DATE partition key column
	SlackBefore time.Duration // widening before the range start
	SlackAfter  time.Duration // widening after the range end
}

// MeasureFilter defines a filter condition for measures (Cube.js compatible)
type MeasureFilter struct {
	SQL string `json:"sql"`
}

// MeasureDefinition defines an analytics measure
type MeasureDefinition struct {
	Type        string          `json:"type" valid:"in(count|sum|avg|min|max)"`
	Title       string          `json:"title"`
	SQL         string          `json:"sql,omitempty"`
	Description string          `json:"description"`
	Filters     []MeasureFilter `json:"filters,omitempty"`

	// Cast is the SQL type the measure's aggregate is cast to, after its filters:
	// (SUM(x) FILTER (WHERE ...))::float8. A cast written at the end of SQL would come before
	// the FILTER clause, which PostgreSQL refuses.
	// Internal query-planning metadata: not exposed through analytics.schemas.
	Cast string `json:"-"`
}

// DimensionDefinition defines an analytics dimension
type DimensionDefinition struct {
	Type        string `json:"type" valid:"in(string|number|time)"`
	Title       string `json:"title"`
	SQL         string `json:"sql,omitempty"`
	Description string `json:"description"`

	// NullableColumn names the bare column of a dimension whose SQL reads its NULL as the
	// empty string (COALESCE(column, '')). The filters that can compare the column itself do
	// (nullableColumnCondition): set and notSet then mean a non-empty and an empty value, as
	// they would on the dimension's values, and an equality can use an index on the column.
	// Internal query-planning metadata: not exposed through analytics.schemas.
	NullableColumn string `json:"-"`
}

// GetDefaultTimezone returns the timezone of the query, UTC when it names none. An empty
// string names none: it is what a client that always sends the field sends when unset.
func (q *Query) GetDefaultTimezone() string {
	if q.Timezone != nil && *q.Timezone != "" {
		return *q.Timezone
	}
	return "UTC"
}

// HasTimeDimensions returns true if the query has time dimensions
func (q *Query) HasTimeDimensions() bool {
	return len(q.TimeDimensions) > 0
}

// GetLimit returns the query limit, MaxRows when it sets none.
func (q *Query) GetLimit() int {
	if q.Limit != nil {
		return *q.Limit
	}
	return MaxRows
}

// isGapFilled reports whether the rows of the query go through gap filling: its time
// dimension has a date range.
func (q Query) isGapFilled() bool {
	return len(q.TimeDimensions) > 0 && q.TimeDimensions[0].DateRange != nil
}

// executionQuery is the query as the database runs it. A gap-filled query reads every row of
// its range: its order, offset and limit apply after the filling (ProcessRows), since a row
// cut in SQL would come back as a filled zero. A gap-filled query grouped by dimensions and
// limited reads only the series of the groups that hold its first offset+limit rows
// (topGroupCells), so a long range over many groups stays under MaxRows. Every query reads at
// most one row past MaxRows, so that a result past it is refused rather than held in memory.
func (q Query) executionQuery() Query {
	bound := MaxRows + 1
	if q.isGapFilled() {
		if len(q.Dimensions) > 0 && q.Limit != nil {
			cells := *q.Limit + q.GetOffset()
			q.topGroupCells = &cells
		}
		q.Limit = &bound
		q.Offset = nil
	} else if q.Limit == nil {
		q.Limit = &bound
	}
	return q
}

// GetOffset returns the query offset or default
func (q *Query) GetOffset() int {
	if q.Offset != nil {
		return *q.Offset
	}
	return 0
}

// Querier is the querying capability Query needs — satisfied by *sql.DB and
// *sql.Tx, so callers can wrap execution in a transaction (e.g. to SET LOCAL
// work_mem for heavy aggregate sorts).
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
}

// MeasureTypes maps each measure of the schema to its type.
func (s SchemaDefinition) MeasureTypes() map[string]string {
	types := make(map[string]string, len(s.Measures))
	for name, measure := range s.Measures {
		types[name] = measure.Type
	}
	return types
}

// DimensionTypes maps each dimension of the schema to its type.
func (s SchemaDefinition) DimensionTypes() map[string]string {
	types := make(map[string]string, len(s.Dimensions))
	for name, dimension := range s.Dimensions {
		types[name] = dimension.Type
	}
	return types
}

// Query executes the analytics query against the database and returns an analytics response
func (q *Query) Query(ctx context.Context, db Querier, schema SchemaDefinition) (*Response, error) {
	// Validate that the query schema name matches the provided schema
	if q.Schema != schema.Name {
		return nil, ErrInvalidSchema
	}

	// Validate the query against the schema
	schemas := map[string]SchemaDefinition{q.Schema: schema}
	if err := DefaultValidate(*q, schemas); err != nil {
		return nil, err
	}

	// Build SQL using the SQL builder
	sqlBuilder := NewSQLBuilder()
	sqlQuery, args, err := sqlBuilder.BuildSQL(q.executionQuery(), schema)
	if err != nil {
		return nil, err
	}

	// Execute the query
	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	// Process rows with time series gap filling, then the order, offset and limit of a
	// gap-filled query.
	data, err := ProcessRows(rows, *q, schema.MeasureTypes(), schema.DimensionTypes())
	if err != nil {
		return nil, err
	}

	// Create response with debug information
	response := &Response{
		Data: data,
		Meta: Meta{
			Query:  sqlQuery,
			Params: args,
			Total:  len(data),
		},
	}

	return response, nil
}
