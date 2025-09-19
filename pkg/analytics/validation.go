package analytics

import (
	"errors"
	"time"
)

// ValidateFunc is a function type for validating queries against schema definitions
type ValidateFunc func(query Query, schemas map[string]SchemaDefinition) error

// DefaultValidate provides the default validation logic for analytics queries
func DefaultValidate(query Query, schemas map[string]SchemaDefinition) error {
	// Check if schema exists
	schema, exists := schemas[query.Schema]
	if !exists {
		return ErrInvalidSchema
	}

	// Validate measures
	for _, measure := range query.Measures {
		if _, exists := schema.Measures[measure]; !exists {
			return ErrUnsupportedMeasure
		}
	}

	// Validate dimensions
	for _, dimension := range query.Dimensions {
		if _, exists := schema.Dimensions[dimension]; !exists {
			return ErrUnsupportedDimension
		}
	}

	// Validate time dimensions
	for _, timeDim := range query.TimeDimensions {
		// Check if dimension exists
		if _, exists := schema.Dimensions[timeDim.Dimension]; !exists {
			return ErrUnsupportedDimension
		}

		// Check granularity
		validGranularities := map[string]bool{
			"hour": true, "day": true, "week": true, "month": true, "year": true,
		}
		if !validGranularities[timeDim.Granularity] {
			return ErrUnsupportedGranularity
		}
	}

	// Validate filters
	for _, filter := range query.Filters {
		// Check if dimension exists
		if _, exists := schema.Dimensions[filter.Member]; !exists {
			// Also check measures for filters
			if _, exists := schema.Measures[filter.Member]; !exists {
				return ErrUnsupportedDimension
			}
		}

		// Check operator
		validOperators := map[string]bool{
			"equals": true, "notEquals": true, "contains": true, "notContains": true,
			"startsWith": true, "notStartsWith": true, "endsWith": true, "notEndsWith": true,
			"gt": true, "gte": true, "lt": true, "lte": true,
			"in": true, "notIn": true, "set": true, "notSet": true,
			"inDateRange": true, "notInDateRange": true, "beforeDate": true, "afterDate": true,
		}
		if !validOperators[filter.Operator] {
			return ErrUnsupportedOperator
		}

		// Check values (allow empty values for set/notSet operators)
		if len(filter.Values) == 0 && filter.Operator != "set" && filter.Operator != "notSet" {
			return errors.New("filter values cannot be empty")
		}
	}

	// Validate timezone if provided
	if query.Timezone != nil {
		_, err := time.LoadLocation(*query.Timezone)
		if err != nil {
			return ErrInvalidTimezone
		}
	}

	// Validate limits
	if query.Limit != nil && *query.Limit < 0 {
		return errors.New("limit cannot be negative")
	}
	if query.Offset != nil && *query.Offset < 0 {
		return errors.New("offset cannot be negative")
	}

	// Validate order fields
	for field := range query.Order {
		// Check if field exists in dimensions or measures
		if _, exists := schema.Dimensions[field]; !exists {
			if _, exists := schema.Measures[field]; !exists {
				return errors.New("order field does not exist in schema")
			}
		}
	}

	return nil
}

// Validate validates the analytics query using the default validation logic
func (q *Query) Validate(schemas map[string]SchemaDefinition) error {
	return DefaultValidate(*q, schemas)
}
