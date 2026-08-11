package analytics

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Masterminds/squirrel"
)

// SQLBuilder provides methods to convert analytics queries to SQL
type SQLBuilder struct {
	placeholder squirrel.PlaceholderFormat
}

// NewSQLBuilder creates a new SQL builder with PostgreSQL placeholder format
func NewSQLBuilder() *SQLBuilder {
	return &SQLBuilder{
		placeholder: squirrel.Dollar,
	}
}

// BuildSQL converts an analytics Query to SQL using the provided schema definition
func (sb *SQLBuilder) BuildSQL(query Query, schema SchemaDefinition) (string, []interface{}, error) {
	// Start building the SELECT query
	selectBuilder := squirrel.Select().PlaceholderFormat(sb.placeholder)

	// Add measures to SELECT clause
	for _, measure := range query.Measures {
		measureDef, exists := schema.Measures[measure]
		if !exists {
			return "", nil, fmt.Errorf("measure '%s' not found in schema", measure)
		}

		// Build the SQL for the measure
		var measureSQL string
		if measureDef.SQL != "" {
			// Check if the measure type requires wrapping with aggregate function
			measureSQL = sb.buildMeasureSQL(measureDef.Type, measureDef.SQL, measureDef.Filters)
		} else {
			// Use measure name directly if no SQL provided
			measureSQL = measure
		}

		selectBuilder = selectBuilder.Column(fmt.Sprintf("(%s) AS %s", measureSQL, measure))
	}

	// Add dimensions to SELECT clause
	for _, dimension := range query.Dimensions {
		dimensionDef, exists := schema.Dimensions[dimension]
		if !exists {
			return "", nil, fmt.Errorf("dimension '%s' not found in schema", dimension)
		}

		// Use custom SQL if provided, otherwise use the dimension name
		if dimensionDef.SQL != "" {
			selectBuilder = selectBuilder.Column(fmt.Sprintf("%s AS %s", dimensionDef.SQL, dimension))
		} else {
			selectBuilder = selectBuilder.Column(dimension)
		}
	}

	// Add time dimensions to SELECT clause and GROUP BY
	timeDimensionColumns := make([]string, 0, len(query.TimeDimensions))
	for _, timeDim := range query.TimeDimensions {
		dimensionDef, exists := schema.Dimensions[timeDim.Dimension]
		if !exists {
			return "", nil, fmt.Errorf("time dimension '%s' not found in schema", timeDim.Dimension)
		}

		// Generate time dimension SQL based on granularity
		timeDimSQL, err := sb.buildTimeDimensionSQL(timeDim, dimensionDef, query.GetDefaultTimezone())
		if err != nil {
			return "", nil, fmt.Errorf("failed to build time dimension SQL: %w", err)
		}

		columnAlias := fmt.Sprintf("%s_%s", timeDim.Dimension, timeDim.Granularity)
		selectBuilder = selectBuilder.Column(fmt.Sprintf("(%s) AS %s", timeDimSQL, columnAlias))
		timeDimensionColumns = append(timeDimensionColumns, columnAlias)
	}

	// Set FROM clause
	selectBuilder = selectBuilder.From(schema.Name)

	// Add WHERE clauses for filters
	var pruneStart, pruneEnd *time.Time
	for _, filter := range query.Filters {
		// A range filter on a time dimension prunes partitions just as well as
		// a time dimension does. Breakdowns carry their range this way because
		// a granularity would split every row per bucket, and without this
		// they would scan the whole history instead of one partition.
		if filter.Operator == "inDateRange" && len(filter.Values) == 2 {
			if dimensionDef, exists := schema.Dimensions[filter.Member]; exists && dimensionDef.Type == "time" {
				timezone := query.GetDefaultTimezone()
				start, startErr := parseDateRangeBound(filter.Values[0], timezone)
				end, endErr := parseDateRangeBound(filter.Values[1], timezone)
				if startErr == nil && endErr == nil {
					if pruneStart == nil || start.Before(*pruneStart) {
						pruneStart = &start
					}
					if pruneEnd == nil || end.After(*pruneEnd) {
						pruneEnd = &end
					}
				}
			}
		}

		// Check if filter member exists in schema
		var memberSQL string
		if measureDef, exists := schema.Measures[filter.Member]; exists {
			memberSQL = measureDef.SQL
			if memberSQL == "" {
				memberSQL = filter.Member
			}
		} else if dimensionDef, exists := schema.Dimensions[filter.Member]; exists {
			memberSQL = dimensionDef.SQL
			if memberSQL == "" {
				memberSQL = filter.Member
			}
		} else {
			return "", nil, fmt.Errorf("filter member '%s' not found in schema", filter.Member)
		}

		// Build WHERE condition based on operator
		condition, err := sb.buildFilterCondition(memberSQL, filter)
		if err != nil {
			return "", nil, fmt.Errorf("failed to build filter condition: %w", err)
		}

		selectBuilder = selectBuilder.Where(condition)
	}

	// Add time dimension date range filters.
	//
	// The bounds are compared against the bare column so indexes (BRIN) and
	// partition pruning stay usable: for non-UTC timezones the local bounds
	// are converted to UTC instants in Go instead of wrapping the column in
	// AT TIME ZONE (which is kept only inside DATE_TRUNC bucketing). The UTC
	// path binds the raw strings, byte-identical to the historical SQL.
	for _, timeDim := range query.TimeDimensions {
		if timeDim.DateRange == nil {
			continue
		}
		dimensionDef := schema.Dimensions[timeDim.Dimension]
		dimensionSQL := dimensionDef.SQL
		if dimensionSQL == "" {
			dimensionSQL = timeDim.Dimension
		}

		timezone := query.GetDefaultTimezone()
		startLocal, startErr := parseDateRangeBound(timeDim.DateRange[0], timezone)
		endLocal, endErr := parseDateRangeBound(timeDim.DateRange[1], timezone)

		// A bare date names a whole day, so the range's last day belongs to
		// it. Anything else would make a time series silently lose its most
		// recent day — and a single-day range ("today") return nothing at all
		// — since the gap filler only accepts date-only bounds and leaves the
		// caller no way to say "up to the end of that day".
		if endErr == nil && isDateOnlyBound(timeDim.DateRange[1]) {
			endLocal = endLocal.AddDate(0, 0, 1).Add(-time.Millisecond)
		}

		if startErr == nil && endErr == nil {
			if pruneStart == nil || startLocal.Before(*pruneStart) {
				pruneStart = &startLocal
			}
			if pruneEnd == nil || endLocal.After(*pruneEnd) {
				pruneEnd = &endLocal
			}

			// Both bounds are instants now, so the column is compared bare and
			// stays sargable whatever the timezone.
			selectBuilder = selectBuilder.Where(squirrel.GtOrEq{dimensionSQL: startLocal.UTC()})
			selectBuilder = selectBuilder.Where(squirrel.LtOrEq{dimensionSQL: endLocal.UTC()})
			continue
		}

		if timezone != "UTC" {
			// Unparseable bounds: keep the legacy (non-sargable) comparison so
			// exotic formats behave as before.
			sanitizedTimezone := sb.sanitizeTimezone(timezone)
			if sanitizedTimezone != "" {
				dimensionSQL = fmt.Sprintf("%s AT TIME ZONE '%s'", dimensionSQL, sanitizedTimezone)
			}
		}

		selectBuilder = selectBuilder.Where(squirrel.GtOrEq{dimensionSQL: timeDim.DateRange[0]})
		selectBuilder = selectBuilder.Where(squirrel.LtOrEq{dimensionSQL: timeDim.DateRange[1]})
	}

	// Partition pruning: widen the requested range by the schema's slacks and
	// constrain the partition key column so the planner can skip partitions.
	if schema.PartitionHint != nil && pruneStart != nil && pruneEnd != nil {
		hint := schema.PartitionHint
		selectBuilder = selectBuilder.Where(squirrel.GtOrEq{hint.Column: pruneStart.UTC().Add(-hint.SlackBefore).Format("2006-01-02")})
		selectBuilder = selectBuilder.Where(squirrel.LtOrEq{hint.Column: pruneEnd.UTC().Add(hint.SlackAfter).Format("2006-01-02")})
	}

	// Add GROUP BY clause
	groupByColumns := make([]string, 0, len(query.Dimensions)+len(timeDimensionColumns))

	// Add regular dimensions to GROUP BY
	for _, dimension := range query.Dimensions {
		dimensionDef := schema.Dimensions[dimension]
		if dimensionDef.SQL != "" {
			groupByColumns = append(groupByColumns, dimensionDef.SQL)
		} else {
			groupByColumns = append(groupByColumns, dimension)
		}
	}

	// Add time dimensions to GROUP BY
	groupByColumns = append(groupByColumns, timeDimensionColumns...)

	if len(groupByColumns) > 0 {
		selectBuilder = selectBuilder.GroupBy(groupByColumns...)
	}

	// Metric filters (HAVING): conditions on fully-aggregated measures.
	for _, having := range query.Having {
		measureDef, exists := schema.Measures[having.Member]
		if !exists {
			return "", nil, fmt.Errorf("having member '%s' is not a measure in schema", having.Member)
		}
		measureSQL := measureDef.SQL
		if measureSQL == "" {
			measureSQL = having.Member
		}
		aggregateSQL := sb.buildMeasureSQL(measureDef.Type, measureSQL, measureDef.Filters)
		condition, err := sb.buildFilterCondition(aggregateSQL, having)
		if err != nil {
			return "", nil, fmt.Errorf("failed to build having condition: %w", err)
		}
		selectBuilder = selectBuilder.Having(condition)
	}

	// Add ORDER BY clause
	for field, direction := range query.Order {
		orderDirection := strings.ToUpper(direction)
		if orderDirection != "ASC" && orderDirection != "DESC" {
			orderDirection = "ASC"
		}

		// Check if field exists in measures or dimensions
		var fieldSQL string
		if measureDef, exists := schema.Measures[field]; exists {
			fieldSQL = measureDef.SQL
			if fieldSQL == "" {
				fieldSQL = field
			}
		} else if dimensionDef, exists := schema.Dimensions[field]; exists {
			fieldSQL = dimensionDef.SQL
			if fieldSQL == "" {
				fieldSQL = field
			}
		} else {
			return "", nil, fmt.Errorf("order field '%s' not found in schema", field)
		}

		selectBuilder = selectBuilder.OrderBy(fmt.Sprintf("%s %s", fieldSQL, orderDirection))
	}

	// Add LIMIT and OFFSET
	if query.Limit != nil {
		selectBuilder = selectBuilder.Limit(uint64(*query.Limit))
	}
	if query.Offset != nil {
		selectBuilder = selectBuilder.Offset(uint64(*query.Offset))
	}

	// Build the final SQL
	sql, args, err := selectBuilder.ToSql()
	if err != nil {
		return "", nil, fmt.Errorf("failed to build SQL: %w", err)
	}

	return sql, args, nil
}

// isDateOnlyBound reports whether a bound names a day rather than an instant.
func isDateOnlyBound(value string) bool {
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}

// parseDateRangeBound parses a dateRange bound in the query's timezone.
// Accepted formats mirror what the console sends: a bare date (interpreted at
// local midnight, matching the historical text comparison), a date-time, or
// RFC 3339.
func parseDateRangeBound(value string, timezone string) (time.Time, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, err
	}
	for _, layout := range []string{"2006-01-02", "2006-01-02 15:04:05", time.RFC3339} {
		if parsed, err := time.ParseInLocation(layout, value, loc); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported dateRange bound: %q", value)
}

// buildTimeDimensionSQL generates SQL for time dimension grouping based on granularity
func (sb *SQLBuilder) buildTimeDimensionSQL(timeDim TimeDimension, dimensionDef DimensionDefinition, timezone string) (string, error) {
	dimensionSQL := dimensionDef.SQL
	if dimensionSQL == "" {
		dimensionSQL = timeDim.Dimension
	}

	// Bucket in the query's timezone — including UTC.
	//
	// DATE_TRUNC on a timestamptz truncates in the *database session's* zone,
	// so omitting this for UTC does not mean "group by UTC days", it means
	// "group by whatever days the server happens to use". On a server set to
	// anything else the buckets come back shifted by its offset, which then
	// fails to line up with the UTC keys the gap filler generates and zeroes
	// out the entire series.
	if sanitizedTimezone := sb.sanitizeTimezone(timezone); sanitizedTimezone != "" {
		dimensionSQL = fmt.Sprintf("%s AT TIME ZONE '%s'", dimensionSQL, sanitizedTimezone)
	}

	switch timeDim.Granularity {
	case "hour":
		return fmt.Sprintf("DATE_TRUNC('hour', %s)", dimensionSQL), nil
	case "day":
		return fmt.Sprintf("DATE_TRUNC('day', %s)", dimensionSQL), nil
	case "week":
		return fmt.Sprintf("DATE_TRUNC('week', %s)", dimensionSQL), nil
	case "month":
		return fmt.Sprintf("DATE_TRUNC('month', %s)", dimensionSQL), nil
	case "year":
		return fmt.Sprintf("DATE_TRUNC('year', %s)", dimensionSQL), nil
	default:
		return "", fmt.Errorf("unsupported granularity: %s", timeDim.Granularity)
	}
}

// buildFilterCondition builds a WHERE condition based on the filter operator
func (sb *SQLBuilder) buildFilterCondition(memberSQL string, filter Filter) (squirrel.Sqlizer, error) {
	switch filter.Operator {
	case "equals":
		if len(filter.Values) == 1 {
			return squirrel.Eq{memberSQL: filter.Values[0]}, nil
		}
		return squirrel.Eq{memberSQL: filter.Values}, nil
	case "notEquals":
		if len(filter.Values) == 1 {
			return squirrel.NotEq{memberSQL: filter.Values[0]}, nil
		}
		return squirrel.NotEq{memberSQL: filter.Values}, nil
	case "contains":
		if len(filter.Values) != 1 {
			return nil, fmt.Errorf("contains operator requires exactly one value")
		}
		return squirrel.Like{memberSQL: fmt.Sprintf("%%%s%%", filter.Values[0])}, nil
	case "notContains":
		if len(filter.Values) != 1 {
			return nil, fmt.Errorf("notContains operator requires exactly one value")
		}
		return squirrel.NotLike{memberSQL: fmt.Sprintf("%%%s%%", filter.Values[0])}, nil
	case "startsWith":
		if len(filter.Values) != 1 {
			return nil, fmt.Errorf("startsWith operator requires exactly one value")
		}
		return squirrel.Like{memberSQL: fmt.Sprintf("%s%%", filter.Values[0])}, nil
	case "notStartsWith":
		if len(filter.Values) != 1 {
			return nil, fmt.Errorf("notStartsWith operator requires exactly one value")
		}
		return squirrel.NotLike{memberSQL: fmt.Sprintf("%s%%", filter.Values[0])}, nil
	case "endsWith":
		if len(filter.Values) != 1 {
			return nil, fmt.Errorf("endsWith operator requires exactly one value")
		}
		return squirrel.Like{memberSQL: fmt.Sprintf("%%%s", filter.Values[0])}, nil
	case "notEndsWith":
		if len(filter.Values) != 1 {
			return nil, fmt.Errorf("notEndsWith operator requires exactly one value")
		}
		return squirrel.NotLike{memberSQL: fmt.Sprintf("%%%s", filter.Values[0])}, nil
	case "gt":
		if len(filter.Values) != 1 {
			return nil, fmt.Errorf("gt operator requires exactly one value")
		}
		return squirrel.Gt{memberSQL: filter.Values[0]}, nil
	case "gte":
		if len(filter.Values) != 1 {
			return nil, fmt.Errorf("gte operator requires exactly one value")
		}
		return squirrel.GtOrEq{memberSQL: filter.Values[0]}, nil
	case "lt":
		if len(filter.Values) != 1 {
			return nil, fmt.Errorf("lt operator requires exactly one value")
		}
		return squirrel.Lt{memberSQL: filter.Values[0]}, nil
	case "lte":
		if len(filter.Values) != 1 {
			return nil, fmt.Errorf("lte operator requires exactly one value")
		}
		return squirrel.LtOrEq{memberSQL: filter.Values[0]}, nil
	case "in":
		return squirrel.Eq{memberSQL: filter.Values}, nil
	case "notIn":
		return squirrel.NotEq{memberSQL: filter.Values}, nil
	case "set":
		// Check if field is not null
		return squirrel.NotEq{memberSQL: nil}, nil
	case "notSet":
		// Check if field is null
		return squirrel.Eq{memberSQL: nil}, nil
	case "inDateRange":
		if len(filter.Values) != 2 {
			return nil, fmt.Errorf("inDateRange operator requires exactly two values")
		}
		return squirrel.And{
			squirrel.GtOrEq{memberSQL: filter.Values[0]},
			squirrel.LtOrEq{memberSQL: filter.Values[1]},
		}, nil
	case "notInDateRange":
		if len(filter.Values) != 2 {
			return nil, fmt.Errorf("notInDateRange operator requires exactly two values")
		}
		return squirrel.Or{
			squirrel.Lt{memberSQL: filter.Values[0]},
			squirrel.Gt{memberSQL: filter.Values[1]},
		}, nil
	case "beforeDate":
		if len(filter.Values) != 1 {
			return nil, fmt.Errorf("beforeDate operator requires exactly one value")
		}
		return squirrel.Lt{memberSQL: filter.Values[0]}, nil
	case "afterDate":
		if len(filter.Values) != 1 {
			return nil, fmt.Errorf("afterDate operator requires exactly one value")
		}
		return squirrel.Gt{memberSQL: filter.Values[0]}, nil
	default:
		return nil, fmt.Errorf("unsupported operator: %s", filter.Operator)
	}
}

// buildMeasureSQL wraps the SQL expression with the appropriate aggregate function based on measure type
// and applies any measure filters using PostgreSQL FILTER clause
func (sb *SQLBuilder) buildMeasureSQL(measureType, sql string, filters []MeasureFilter) string {
	// If the SQL already contains an aggregate function (has parentheses and common functions),
	// return it as-is to support complex expressions, but still apply filters if present
	upperSQL := strings.ToUpper(sql)
	if strings.Contains(upperSQL, "COUNT(") ||
		strings.Contains(upperSQL, "SUM(") ||
		strings.Contains(upperSQL, "AVG(") ||
		strings.Contains(upperSQL, "MIN(") ||
		strings.Contains(upperSQL, "MAX(") ||
		strings.Contains(upperSQL, "FILTER") {
		return sb.applyMeasureFilters(sql, filters)
	}

	// Apply Cube.js-style automatic wrapping based on measure type
	var baseSQL string
	switch measureType {
	case "count":
		// For count, if it's just a column name, wrap with COUNT()
		if !strings.Contains(upperSQL, "COUNT") {
			baseSQL = fmt.Sprintf("COUNT(%s)", sql)
		} else {
			baseSQL = sql
		}
	case "sum":
		baseSQL = fmt.Sprintf("SUM(%s)", sql)
	case "avg":
		baseSQL = fmt.Sprintf("AVG(%s)", sql)
	case "min":
		baseSQL = fmt.Sprintf("MIN(%s)", sql)
	case "max":
		baseSQL = fmt.Sprintf("MAX(%s)", sql)
	case "count_distinct":
		baseSQL = fmt.Sprintf("COUNT(DISTINCT %s)", sql)
	case "count_distinct_approx":
		// Use HyperLogLog approximation if available, fallback to COUNT DISTINCT
		baseSQL = fmt.Sprintf("COUNT(DISTINCT %s)", sql)
	default:
		// For unknown types or custom expressions, return as-is
		baseSQL = sql
	}

	// Apply filters to the base SQL
	return sb.applyMeasureFilters(baseSQL, filters)
}

// applyMeasureFilters applies measure filters using PostgreSQL FILTER clause
func (sb *SQLBuilder) applyMeasureFilters(baseSQL string, filters []MeasureFilter) string {
	if len(filters) == 0 {
		return baseSQL
	}

	// Collect all filter conditions
	var conditions []string
	for _, filter := range filters {
		if filter.SQL != "" {
			conditions = append(conditions, filter.SQL)
		}
	}

	if len(conditions) == 0 {
		return baseSQL
	}

	// Join conditions with AND and apply FILTER clause
	filterClause := strings.Join(conditions, " AND ")
	return fmt.Sprintf("%s FILTER (WHERE %s)", baseSQL, filterClause)
}

// sanitizeTimezone validates and sanitizes timezone strings to prevent SQL injection
func (sb *SQLBuilder) sanitizeTimezone(timezone string) string {
	return SanitizeTimezone(timezone)
}

// SanitizeTimezone returns a timezone name safe to inline into SQL, or an
// empty string when the input cannot be one. Schemas that bake a timezone into
// a dimension expression need the same guarantee the builder does.
func SanitizeTimezone(timezone string) string {
	// Remove any quotes or dangerous characters
	timezone = strings.ReplaceAll(timezone, "'", "")
	timezone = strings.ReplaceAll(timezone, "\"", "")
	timezone = strings.ReplaceAll(timezone, ";", "")
	timezone = strings.ReplaceAll(timezone, "--", "")
	timezone = strings.ReplaceAll(timezone, "/*", "")
	timezone = strings.ReplaceAll(timezone, "*/", "")

	// Trim whitespace
	timezone = strings.TrimSpace(timezone)

	// Check if it's empty after sanitization
	if timezone == "" {
		return ""
	}

	// Validate against common timezone patterns
	// Allow only alphanumeric, underscore, slash, plus, minus, and colon
	for _, char := range timezone {
		if (char < 'A' || char > 'Z') &&
			(char < 'a' || char > 'z') &&
			(char < '0' || char > '9') &&
			char != '_' && char != '/' && char != '+' && char != '-' && char != ':' {
			return "" // Invalid character found
		}
	}

	// Additional length check
	if len(timezone) > 50 {
		return "" // Too long, likely malicious
	}

	return timezone
}

// ScanRows scans SQL rows and converts them to a slice of maps for JSON serialization
func ScanRows(rows *sql.Rows) ([]map[string]interface{}, error) {
	// Get column information
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// Parse results.
	//
	// Non-nil from the start: a nil slice marshals to JSON null, and every
	// consumer's type declares `data` as an array. A breakdown that matched
	// nothing would hand the client null where it promised a list, and the
	// client would call .map on it — an empty workspace, or any filter
	// combination with no matches, is enough to crash the page.
	data := make([]map[string]interface{}, 0)
	for rows.Next() {
		// Create a slice of interface{} to hold the values
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		// Scan the row
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Convert to map
		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			// Convert []byte to string for better JSON serialization
			if b, ok := val.([]byte); ok {
				val = string(b)
			}
			// Convert time.Time to string for date dimensions.
			//
			// Normalized to UTC first. A bucket built without AT TIME ZONE
			// stays a timestamptz, which the driver hands back in the database
			// session's zone — so on a server set to anything but UTC the same
			// instant would serialize with an offset while the gap filler
			// generates its keys in UTC. Every real bucket would then miss its
			// key and be replaced by a zero-filled one, flattening the chart.
			if t, ok := val.(time.Time); ok {
				val = t.UTC().Format(time.RFC3339)
			}
			row[col] = val
		}
		data = append(data, row)
	}

	// Check for iteration errors
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during result iteration: %w", err)
	}

	return data, nil
}

// ProcessRows scans SQL rows and fills time series gaps if the query has time dimensions
func ProcessRows(rows *sql.Rows, query Query) ([]map[string]interface{}, error) {
	// First scan the rows normally
	data, err := ScanRows(rows)
	if err != nil {
		return nil, err
	}

	// Check if this is a time series query that needs gap filling
	if !query.HasTimeDimensions() {
		// A totals query with nothing to total still has an answer — zero — and
		// a KPI tile would otherwise have to render a blank. A *breakdown* with
		// nothing to break down does not: inventing a row there puts a phantom
		// entry with an empty dimension at the top of every table, which reads
		// as real data and hides the widget's own empty state.
		if len(data) == 0 && len(query.Dimensions) == 0 {
			return generateZeroValueRow(query), nil
		}
		return data, nil
	}

	// For time series queries, fill gaps with zero values
	return fillTimeSeriesGaps(data, query)
}

// generateZeroValueRow creates a single row with zero values for all measures
func generateZeroValueRow(query Query) []map[string]interface{} {
	row := make(map[string]interface{})

	// Set all measures to zero
	for _, measure := range query.Measures {
		row[measure] = 0
	}

	// Add dimensions with empty/zero values
	for _, dimension := range query.Dimensions {
		row[dimension] = ""
	}

	return []map[string]interface{}{row}
}

// fillTimeSeriesGaps fills missing time periods with zero values
func fillTimeSeriesGaps(data []map[string]interface{}, query Query) ([]map[string]interface{}, error) {
	if len(query.TimeDimensions) == 0 {
		return data, nil
	}

	// For now, handle single time dimension (most common case)
	timeDim := query.TimeDimensions[0]

	// If no date range specified, return data as-is
	if timeDim.DateRange == nil {
		return data, nil
	}

	// Create time dimension column name
	timeDimColumn := fmt.Sprintf("%s_%s", timeDim.Dimension, timeDim.Granularity)

	// Determine time range strategy based on granularity and data
	var timeRange []string
	startTime, err := time.Parse("2006-01-02", timeDim.DateRange[0])
	if err != nil {
		return data, fmt.Errorf("failed to parse start date: %w", err)
	}

	endTime, err := time.Parse("2006-01-02", timeDim.DateRange[1])
	if err != nil {
		return data, fmt.Errorf("failed to parse end date: %w", err)
	}

	// For hour granularity with same-day range and existing data, use data-driven approach
	if timeDim.Granularity == "hour" && startTime.Equal(endTime) && len(data) > 0 {
		timeRange = generateTimeRangeFromData(data, timeDimColumn, timeDim)
	} else {
		// For other cases, use the full date range
		timeRange = generateTimeRange(startTime, endTime, timeDim.Granularity)
	}

	if len(timeRange) == 0 {
		return data, nil
	}

	// Create a map of existing data by time dimension
	existingData := make(map[string]map[string]interface{})
	for _, row := range data {
		if timeVal, exists := row[timeDimColumn]; exists {
			if timeStr, ok := timeVal.(string); ok {
				existingData[timeStr] = row
			}
		}
	}

	// Fill gaps
	result := make([]map[string]interface{}, 0, len(timeRange))
	for _, timeStr := range timeRange {
		if existingRow, exists := existingData[timeStr]; exists {
			// Use existing data
			result = append(result, existingRow)
		} else {
			// Create zero-value row
			zeroRow := make(map[string]interface{})

			// Set time dimension
			zeroRow[timeDimColumn] = timeStr

			// Set all measures to zero
			for _, measure := range query.Measures {
				zeroRow[measure] = 0
			}

			// Set dimensions to empty strings or copy from first existing row
			for _, dimension := range query.Dimensions {
				zeroRow[dimension] = ""
			}

			result = append(result, zeroRow)
		}
	}

	return result, nil
}

// generateTimeRange generates a slice of time strings based on start, end, and granularity
func generateTimeRange(start, end time.Time, granularity string) []string {
	var times []string
	current := start

	// Ensure we're working with UTC and truncated times
	current = current.UTC()
	end = end.UTC()

	switch granularity {
	case "hour":
		current = time.Date(current.Year(), current.Month(), current.Day(), current.Hour(), 0, 0, 0, time.UTC)
		end = time.Date(end.Year(), end.Month(), end.Day(), end.Hour(), 0, 0, 0, time.UTC)
		for current.Before(end) || current.Equal(end) {
			times = append(times, current.Format(time.RFC3339))
			current = current.Add(time.Hour)
		}
	case "day":
		current = time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, time.UTC)
		end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
		for current.Before(end) || current.Equal(end) {
			times = append(times, current.Format(time.RFC3339))
			current = current.AddDate(0, 0, 1)
		}
	case "week":
		// Start from beginning of week (Monday)
		weekday := current.Weekday()
		if weekday == time.Sunday {
			weekday = 7
		}
		current = current.AddDate(0, 0, -int(weekday-time.Monday))
		current = time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, time.UTC)

		endWeekday := end.Weekday()
		if endWeekday == time.Sunday {
			endWeekday = 7
		}
		end = end.AddDate(0, 0, -int(endWeekday-time.Monday))
		end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)

		for current.Before(end) || current.Equal(end) {
			times = append(times, current.Format(time.RFC3339))
			current = current.AddDate(0, 0, 7)
		}
	case "month":
		current = time.Date(current.Year(), current.Month(), 1, 0, 0, 0, 0, time.UTC)
		end = time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.UTC)
		for current.Before(end) || current.Equal(end) {
			times = append(times, current.Format(time.RFC3339))
			current = current.AddDate(0, 1, 0)
		}
	case "year":
		current = time.Date(current.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		end = time.Date(end.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		for current.Before(end) || current.Equal(end) {
			times = append(times, current.Format(time.RFC3339))
			current = current.AddDate(1, 0, 0)
		}
	}

	return times
}

// generateTimeRangeFromData generates a time range based on existing data points
func generateTimeRangeFromData(data []map[string]interface{}, timeDimColumn string, timeDim TimeDimension) []string {
	if len(data) == 0 {
		return nil
	}

	// Extract all time values from existing data
	var existingTimes []time.Time
	for _, row := range data {
		if timeVal, exists := row[timeDimColumn]; exists {
			if timeStr, ok := timeVal.(string); ok {
				if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
					existingTimes = append(existingTimes, t)
				}
			}
		}
	}

	if len(existingTimes) == 0 {
		return nil
	}

	// Find min and max times
	minTime := existingTimes[0]
	maxTime := existingTimes[0]
	for _, t := range existingTimes {
		if t.Before(minTime) {
			minTime = t
		}
		if t.After(maxTime) {
			maxTime = t
		}
	}

	// Generate time range from min to max
	return generateTimeRange(minTime, maxTime, timeDim.Granularity)
}

// ToSQL is a convenience method on Query to build SQL using the default builder
func (q *Query) ToSQL(schema SchemaDefinition) (string, []interface{}, error) {
	builder := NewSQLBuilder()
	return builder.BuildSQL(*q, schema)
}
