package analytics

import (
	"slices"
	"strconv"
	"strings"
	"time"
)

// ValidateFunc is a function type for validating queries against schema definitions
type ValidateFunc func(query Query, schemas map[string]SchemaDefinition) error

var validGranularities = []string{"hour", "day", "week", "month", "year"}

// Operators by dimension type. A LIKE on a number or a time fails in PostgreSQL, and a date
// operator on text compares strings, so each type takes only the operators that mean
// something on it. A dimension without a type takes them all.
var (
	stringOperators = []string{
		"equals", "notEquals", "contains", "notContains", "startsWith", "notStartsWith", "endsWith", "notEndsWith",
		"gt", "gte", "lt", "lte", "in", "notIn", "set", "notSet",
	}
	numberOperators = []string{
		"equals", "notEquals", "gt", "gte", "lt", "lte", "in", "notIn", "set", "notSet",
	}
	timeOperators = []string{
		"equals", "notEquals", "gt", "gte", "lt", "lte", "in", "notIn", "set", "notSet",
		"inDateRange", "notInDateRange", "beforeDate", "afterDate",
	}
	allOperators = []string{
		"equals", "notEquals", "contains", "notContains", "startsWith", "notStartsWith", "endsWith", "notEndsWith",
		"gt", "gte", "lt", "lte", "in", "notIn", "set", "notSet",
		"inDateRange", "notInDateRange", "beforeDate", "afterDate",
	}
	havingOperators = []string{"equals", "notEquals", "gt", "gte", "lt", "lte"}

	singleValueOperators = []string{
		"contains", "notContains", "startsWith", "notStartsWith", "endsWith", "notEndsWith",
		"gt", "gte", "lt", "lte", "beforeDate", "afterDate",
	}
	rangeOperators = []string{"inDateRange", "notInDateRange"}

	// Measure types whose value is always a number. A max or a min can be a timestamp.
	numericMeasureTypes = []string{"count", "count_distinct", "count_distinct_approx", "sum", "avg", "number"}
)

// operatorsFor returns the filter operators a dimension of this type takes.
func operatorsFor(dimensionType string) []string {
	switch dimensionType {
	case "string":
		return stringOperators
	case "number":
		return numberOperators
	case "time":
		return timeOperators
	default:
		return allOperators
	}
}

// ValidateTimezone checks a query timezone. An empty one means UTC. "Local" is refused: Go
// reads it as the server's own zone, which PostgreSQL does not know and the caller cannot see.
func ValidateTimezone(timezone string) error {
	if timezone == "" {
		return nil
	}
	if strings.EqualFold(timezone, "Local") {
		return invalid(ErrInvalidTimezone, "timezone %q is not an IANA time zone name, such as Europe/Paris", timezone)
	}
	if _, err := time.LoadLocation(timezone); err != nil || SanitizeTimezone(timezone) != timezone {
		return invalid(ErrInvalidTimezone, "timezone %q is not an IANA time zone name, such as Europe/Paris", timezone)
	}
	return nil
}

// DefaultValidate checks a query against its schema. Every error it returns is a *QueryError
// whose message names what to change.
func DefaultValidate(query Query, schemas map[string]SchemaDefinition) error {
	schema, exists := schemas[query.Schema]
	if !exists {
		return invalid(ErrInvalidSchema, "unknown schema %q", query.Schema)
	}

	if len(query.Measures) == 0 && len(query.Dimensions) == 0 && len(query.TimeDimensions) == 0 {
		return invalid(ErrInvalidQuery, "a query needs at least one measure or dimension")
	}

	for _, measure := range query.Measures {
		if _, exists := schema.Measures[measure]; !exists {
			return invalid(ErrUnsupportedMeasure, "measure %q is not in schema %q", measure, query.Schema)
		}
	}

	for _, dimension := range query.Dimensions {
		if _, exists := schema.Dimensions[dimension]; !exists {
			return invalid(ErrUnsupportedDimension, "dimension %q is not in schema %q", dimension, query.Schema)
		}
	}

	// The timezone first: the date ranges are read in it.
	if query.Timezone != nil {
		if err := ValidateTimezone(*query.Timezone); err != nil {
			return err
		}
	}
	timezone := query.GetDefaultTimezone()

	// One time dimension at most: the gap filling and the bucket order work on one time axis,
	// and filling a second one would pair every bucket of the first with buckets of another
	// day.
	if len(query.TimeDimensions) > 1 {
		return invalid(ErrInvalidQuery, "a query takes at most one time dimension, got %d", len(query.TimeDimensions))
	}
	for _, timeDim := range query.TimeDimensions {
		dimensionDef, exists := schema.Dimensions[timeDim.Dimension]
		if !exists {
			return invalid(ErrUnsupportedDimension, "time dimension %q is not in schema %q", timeDim.Dimension, query.Schema)
		}
		if dimensionDef.Type != "time" {
			return invalid(ErrUnsupportedDimension, "time dimension %q is a %s dimension, not a time one: group by it in dimensions", timeDim.Dimension, dimensionDef.Type)
		}
		if !slices.Contains(validGranularities, timeDim.Granularity) {
			return invalid(ErrUnsupportedGranularity, "granularity %q of time dimension %q is not one of %s", timeDim.Granularity, timeDim.Dimension, strings.Join(validGranularities, ", "))
		}
		if timeDim.DateRange != nil {
			if err := validateDateRange(timeDim, timezone); err != nil {
				return err
			}
		}
	}

	for _, filter := range query.Filters {
		dimensionDef, isDimension := schema.Dimensions[filter.Member]
		if !isDimension {
			if _, isMeasure := schema.Measures[filter.Member]; isMeasure {
				return invalid(ErrUnsupportedDimension, "filters take dimensions, and %q is a measure: put conditions on measures in having", filter.Member)
			}
			return invalid(ErrUnsupportedDimension, "filter member %q is not a dimension of schema %q", filter.Member, query.Schema)
		}
		if !slices.Contains(allOperators, filter.Operator) {
			return invalid(ErrUnsupportedOperator, "filter operator %q is not supported", filter.Operator)
		}
		if !slices.Contains(operatorsFor(dimensionDef.Type), filter.Operator) {
			return invalid(ErrUnsupportedOperator, "operator %q does not apply to %s dimension %q: use one of %s",
				filter.Operator, dimensionDef.Type, filter.Member, strings.Join(operatorsFor(dimensionDef.Type), ", "))
		}
		if err := validateValues("filter on "+strconv.Quote(filter.Member), filter); err != nil {
			return err
		}
		if dimensionDef.Type == "number" {
			if err := validateNumbers("filter on "+strconv.Quote(filter.Member), filter.Values); err != nil {
				return err
			}
		}
	}

	for _, having := range query.Having {
		measureDef, exists := schema.Measures[having.Member]
		if !exists {
			return invalid(ErrUnsupportedMeasure, "having member %q is not a measure of schema %q", having.Member, query.Schema)
		}
		if !slices.Contains(havingOperators, having.Operator) {
			return invalid(ErrUnsupportedOperator, "having operator %q is not one of %s", having.Operator, strings.Join(havingOperators, ", "))
		}
		if len(having.Values) == 0 {
			return invalid(ErrInvalidQuery, "having on %q needs a value", having.Member)
		}
		if err := validateValues("having on "+strconv.Quote(having.Member), having); err != nil {
			return err
		}
		if slices.Contains(numericMeasureTypes, measureDef.Type) {
			if err := validateNumbers("having on "+strconv.Quote(having.Member), having.Values); err != nil {
				return err
			}
		}
	}

	if query.Limit != nil && (*query.Limit < 0 || *query.Limit > MaxRows) {
		return invalid(ErrInvalidQuery, "limit must be between 0 and %d", MaxRows)
	}
	if query.Offset != nil && *query.Offset < 0 {
		return invalid(ErrInvalidQuery, "offset cannot be negative")
	}

	return validateOrder(query, schema)
}

// validateDateRange checks that both bounds of a time dimension's range parse, in order, and
// that the range has no more periods than gap filling may produce. The count is computed from
// the bounds, before anything is allocated.
func validateDateRange(timeDim TimeDimension, timezone string) error {
	for _, bound := range timeDim.DateRange {
		if _, err := parseDateRangeBound(bound, timezone); err != nil {
			return invalid(ErrInvalidQuery, "dateRange bound %q of %q is not a date (2006-01-02), a date and time (2006-01-02 15:04:05) or an RFC 3339 time", bound, timeDim.Dimension)
		}
	}
	start, end, _, err := gapFillRange(timeDim, timezone)
	if err != nil {
		return invalid(ErrInvalidQuery, "dateRange of %q: %v", timeDim.Dimension, err)
	}
	if end.Before(start) {
		return invalid(ErrInvalidQuery, "dateRange of %q ends before it starts", timeDim.Dimension)
	}
	if periods := countPeriods(start, end, timeDim.Granularity); periods > maxGapFilledRows {
		return invalid(ErrInvalidQuery, "dateRange of %q holds %d periods of a %s, more than the %d a series may have: shorten the range or use a coarser granularity",
			timeDim.Dimension, periods, timeDim.Granularity, maxGapFilledRows)
	}
	return nil
}

// validateValues checks the number of values the operator takes.
func validateValues(what string, filter Filter) error {
	switch {
	case filter.Operator == "set" || filter.Operator == "notSet":
		return nil
	case len(filter.Values) == 0:
		return invalid(ErrInvalidQuery, "%s with operator %q needs a value", what, filter.Operator)
	case slices.Contains(singleValueOperators, filter.Operator) && len(filter.Values) != 1:
		return invalid(ErrInvalidQuery, "%s with operator %q takes exactly one value, got %d", what, filter.Operator, len(filter.Values))
	case slices.Contains(rangeOperators, filter.Operator) && len(filter.Values) != 2:
		return invalid(ErrInvalidQuery, "%s with operator %q takes exactly two values, got %d", what, filter.Operator, len(filter.Values))
	}
	return nil
}

// validateNumbers checks that every value is a number.
func validateNumbers(what string, values []string) error {
	for _, value := range values {
		if _, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err != nil {
			return invalid(ErrInvalidQuery, "%s takes numbers, and %q is not one", what, value)
		}
	}
	return nil
}

// validateOrder checks every order entry against what the query selects. A dimension orders
// only when the query groups by it, as PostgreSQL refuses to sort on a column outside GROUP BY.
// The time dimension orders by its bucket, under its own name or its column name
// (created_at or created_at_day). A measure orders by its aggregate, so any measure of the
// schema can order a plain query; a gap-filled query is sorted after the filling, on the
// columns it returns, so its order needs a selected measure.
func validateOrder(query Query, schema SchemaDefinition) error {
	// A direction other than desc, empty or unknown, sorts ascending, as it always has.
	for _, field := range query.OrderKeys() {
		if _, isMeasure := schema.Measures[field]; isMeasure {
			if query.isGapFilled() && !slices.Contains(query.Measures, field) {
				return invalid(ErrInvalidQuery, "order by measure %q needs it in measures on a time series with a dateRange", field)
			}
			continue
		}
		if slices.Contains(query.Dimensions, field) {
			continue
		}
		if _, ok := timeDimensionColumn(query, field); ok {
			continue
		}
		if _, isDimension := schema.Dimensions[field]; isDimension {
			return invalid(ErrInvalidQuery, "order by dimension %q needs it in dimensions or as the time dimension", field)
		}
		return invalid(ErrInvalidQuery, "order field %q is not a measure or a dimension of schema %q", field, query.Schema)
	}
	return nil
}

// timeDimensionColumn returns the bucket column of the query's time dimension when field
// names it, by its dimension name or by the column name itself.
func timeDimensionColumn(query Query, field string) (string, bool) {
	for _, timeDim := range query.TimeDimensions {
		column := timeDim.Dimension + "_" + timeDim.Granularity
		if field == timeDim.Dimension || field == column {
			return column, true
		}
	}
	return "", false
}

// Validate validates the analytics query using the default validation logic
func (q *Query) Validate(schemas map[string]SchemaDefinition) error {
	return DefaultValidate(*q, schemas)
}
